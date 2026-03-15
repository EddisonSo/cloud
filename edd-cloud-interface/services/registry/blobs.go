package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// blobPath returns the GFS path for a finalized blob.
// Format: blobs/{digest_hex}
func blobGFSPath(digest string) string {
	// digest is "sha256:<hex>"; strip the "sha256:" prefix for storage
	hex := strings.TrimPrefix(digest, "sha256:")
	return "blobs/" + hex
}

// uploadGFSPath returns the GFS path for an in-progress upload.
func uploadGFSPath(uploadUUID string) string {
	return "uploads/" + uploadUUID
}

// parseBlobPath parses /v2/{name}/blobs/{digest} and returns (repoName, digest).
// repo names may contain slashes.
func parseBlobPath(r *http.Request) (repoName, digest string, ok bool) {
	// path looks like: /v2/foo/bar/blobs/sha256:abc123
	path := r.URL.Path
	// Strip leading /v2/
	path = strings.TrimPrefix(path, "/v2/")
	// Find "/blobs/" segment
	idx := strings.LastIndex(path, "/blobs/")
	if idx < 0 {
		return "", "", false
	}
	repoName = path[:idx]
	digest = path[idx+len("/blobs/"):]
	if repoName == "" || digest == "" {
		return "", "", false
	}
	return repoName, digest, true
}

// parseUploadRepoName parses /v2/{name}/blobs/uploads/ and returns the repo name.
func parseUploadRepoName(r *http.Request) (string, bool) {
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/v2/")
	// Accept both /blobs/uploads/ and /blobs/uploads
	path = strings.TrimSuffix(path, "/")
	idx := strings.LastIndex(path, "/blobs/uploads")
	if idx < 0 {
		return "", false
	}
	repoName := path[:idx]
	if repoName == "" {
		return "", false
	}
	return repoName, true
}

// parseUploadPath parses /v2/{name}/blobs/uploads/{uuid} and returns (repoName, uuid).
func parseUploadPath(r *http.Request) (repoName, uploadUUID string, ok bool) {
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/v2/")
	idx := strings.LastIndex(path, "/blobs/uploads/")
	if idx < 0 {
		return "", "", false
	}
	repoName = path[:idx]
	uploadUUID = path[idx+len("/blobs/uploads/"):]
	if repoName == "" || uploadUUID == "" {
		return "", "", false
	}
	return repoName, uploadUUID, true
}

// ociError writes an OCI-spec JSON error response.
func ociError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"errors": []map[string]string{
			{"code": code, "message": message},
		},
	})
}

// handleBlobHead handles HEAD /v2/{name}/blobs/{digest}
func (s *server) handleBlobHead(w http.ResponseWriter, r *http.Request) {
	repoName, digest, ok := parseBlobPath(r)
	if !ok {
		http.NotFound(w, r)
		return
	}

	auth := s.authenticate(r)
	if !hasAccess(auth, repoName, "pull") {
		s.requireAuth(w, repoName, "pull")
		return
	}

	repoID, _, _, err := getRepoByName(r.Context(), s.db, repoName)
	if err == sql.ErrNoRows {
		ociError(w, http.StatusNotFound, "NAME_UNKNOWN", "repository not found")
		return
	}
	if err != nil {
		slog.Error("handleBlobHead: getRepoByName", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	exists, size, err := blobExistsInRepo(r.Context(), s.db, repoID, digest)
	if err != nil {
		slog.Error("handleBlobHead: blobExistsInRepo", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if !exists {
		ociError(w, http.StatusNotFound, "BLOB_UNKNOWN", "blob unknown to registry")
		return
	}

	w.Header().Set("Docker-Content-Digest", digest)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
}

// handleBlobGet handles GET /v2/{name}/blobs/{digest}
func (s *server) handleBlobGet(w http.ResponseWriter, r *http.Request) {
	repoName, digest, ok := parseBlobPath(r)
	if !ok {
		http.NotFound(w, r)
		return
	}

	auth := s.authenticate(r)
	if !hasAccess(auth, repoName, "pull") {
		s.requireAuth(w, repoName, "pull")
		return
	}

	repoID, _, _, err := getRepoByName(r.Context(), s.db, repoName)
	if err == sql.ErrNoRows {
		ociError(w, http.StatusNotFound, "NAME_UNKNOWN", "repository not found")
		return
	}
	if err != nil {
		slog.Error("handleBlobGet: getRepoByName", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	exists, size, err := blobExistsInRepo(r.Context(), s.db, repoID, digest)
	if err != nil {
		slog.Error("handleBlobGet: blobExistsInRepo", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if !exists {
		ociError(w, http.StatusNotFound, "BLOB_UNKNOWN", "blob unknown to registry")
		return
	}

	w.Header().Set("Docker-Content-Digest", digest)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	w.Header().Set("Content-Type", "application/octet-stream")

	if _, err := s.gfs.ReadToWithNamespace(r.Context(), blobGFSPath(digest), gfsNamespace, w); err != nil {
		slog.Error("handleBlobGet: ReadToWithNamespace", "digest", digest, "err", err)
		// Headers already sent; can't change status code
	}
}

// handleBlobDelete handles DELETE /v2/{name}/blobs/{digest}
func (s *server) handleBlobDelete(w http.ResponseWriter, r *http.Request) {
	repoName, digest, ok := parseBlobPath(r)
	if !ok {
		http.NotFound(w, r)
		return
	}

	auth := s.authenticate(r)
	if !hasAccess(auth, repoName, "delete") {
		s.requireAuth(w, repoName, "delete")
		return
	}

	repoID, _, _, err := getRepoByName(r.Context(), s.db, repoName)
	if err == sql.ErrNoRows {
		ociError(w, http.StatusNotFound, "NAME_UNKNOWN", "repository not found")
		return
	}
	if err != nil {
		slog.Error("handleBlobDelete: getRepoByName", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	exists, _, err := blobExistsInRepo(r.Context(), s.db, repoID, digest)
	if err != nil {
		slog.Error("handleBlobDelete: blobExistsInRepo", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if !exists {
		ociError(w, http.StatusNotFound, "BLOB_UNKNOWN", "blob unknown to registry")
		return
	}

	// Mark the blob for GC rather than immediate GFS deletion; the GC sweep
	// will remove the GFS object once no manifests reference it.
	const q = `UPDATE repository_blobs SET gc_marked_at = NOW() WHERE repository_id = $1 AND digest = $2`
	if _, err := s.db.ExecContext(r.Context(), q, repoID, digest); err != nil {
		slog.Error("handleBlobDelete: mark gc", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// handleUploadStart handles POST /v2/{name}/blobs/uploads/
// Supports monolithic upload when ?digest= is present in the query.
func (s *server) handleUploadStart(w http.ResponseWriter, r *http.Request) {
	repoName, ok := parseUploadRepoName(r)
	if !ok {
		http.NotFound(w, r)
		return
	}

	auth := s.authenticate(r)
	if !hasAccess(auth, repoName, "push") {
		s.requireAuth(w, repoName, "push")
		return
	}

	repoID, err := getOrCreateRepo(r.Context(), s.db, repoName, auth.UserID)
	if err != nil {
		slog.Error("handleUploadStart: getOrCreateRepo", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Monolithic upload: POST with ?digest=sha256:... and body
	if digest := r.URL.Query().Get("digest"); digest != "" {
		s.completeMonolithicUpload(w, r, repoName, repoID, digest)
		return
	}

	// Chunked upload: create a session
	uploadUUID := uuid.New().String()

	// Create the GFS file for this upload
	if _, err := s.gfs.CreateFileWithNamespace(r.Context(), uploadGFSPath(uploadUUID), gfsNamespace); err != nil {
		slog.Error("handleUploadStart: CreateFileWithNamespace", "uuid", uploadUUID, "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if err := createUploadSession(r.Context(), s.db, uploadUUID, repoID); err != nil {
		slog.Error("handleUploadStart: createUploadSession", "uuid", uploadUUID, "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	uploadURL := fmt.Sprintf("/v2/%s/blobs/uploads/%s", repoName, uploadUUID)
	w.Header().Set("Location", uploadURL)
	w.Header().Set("Docker-Upload-UUID", uploadUUID)
	w.Header().Set("Range", "0-0")
	w.WriteHeader(http.StatusAccepted)
}

// completeMonolithicUpload handles a single-POST upload with body + ?digest=.
func (s *server) completeMonolithicUpload(w http.ResponseWriter, r *http.Request, repoName string, repoID int, digest string) {
	hasher := sha256.New()
	tee := io.TeeReader(r.Body, hasher)

	uploadUUID := uuid.New().String()
	gfsPath := uploadGFSPath(uploadUUID)

	if _, err := s.gfs.CreateFileWithNamespace(r.Context(), gfsPath, gfsNamespace); err != nil {
		slog.Error("completeMonolithicUpload: CreateFileWithNamespace", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	written, err := s.gfs.AppendFromWithNamespace(r.Context(), gfsPath, gfsNamespace, tee)
	if err != nil {
		slog.Error("completeMonolithicUpload: AppendFromWithNamespace", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	got := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	if got != digest {
		// Clean up the orphaned GFS file
		_ = s.gfs.DeleteFileWithNamespace(r.Context(), gfsPath, gfsNamespace)
		ociError(w, http.StatusBadRequest, "DIGEST_INVALID", fmt.Sprintf("digest mismatch: expected %s got %s", digest, got))
		return
	}

	finalPath := blobGFSPath(digest)
	if err := s.gfs.RenameFileWithNamespace(r.Context(), gfsPath, finalPath, gfsNamespace); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			// Concurrent dedup: another upload finished first, clean up ours
			_ = s.gfs.DeleteFileWithNamespace(r.Context(), gfsPath, gfsNamespace)
		} else {
			slog.Error("completeMonolithicUpload: RenameFileWithNamespace", "err", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}

	if err := insertRepoBlob(r.Context(), s.db, repoID, digest, written); err != nil {
		slog.Error("completeMonolithicUpload: insertRepoBlob", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/%s", repoName, digest))
	w.Header().Set("Docker-Content-Digest", digest)
	w.WriteHeader(http.StatusCreated)
}

// handleUploadPatch handles PATCH /v2/{name}/blobs/uploads/{uuid}
func (s *server) handleUploadPatch(w http.ResponseWriter, r *http.Request) {
	repoName, uploadUUID, ok := parseUploadPath(r)
	if !ok {
		http.NotFound(w, r)
		return
	}

	auth := s.authenticate(r)
	if !hasAccess(auth, repoName, "push") {
		s.requireAuth(w, repoName, "push")
		return
	}

	_, hashStateBytes, bytesReceived, err := getUploadSession(r.Context(), s.db, uploadUUID)
	if err == sql.ErrNoRows {
		ociError(w, http.StatusNotFound, "BLOB_UPLOAD_UNKNOWN", "upload session not found")
		return
	}
	if err != nil {
		slog.Error("handleUploadPatch: getUploadSession", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Restore or create hash state
	hasher := sha256.New()
	if len(hashStateBytes) > 0 {
		if um, ok := hasher.(interface {
			UnmarshalBinary([]byte) error
		}); ok {
			if err := um.UnmarshalBinary(hashStateBytes); err != nil {
				slog.Error("handleUploadPatch: UnmarshalBinary hash state", "err", err)
				// Fall back to a fresh hasher — hash verification will fail at PUT time
				hasher = sha256.New()
			}
		}
	}

	gfsPath := uploadGFSPath(uploadUUID)
	tee := io.TeeReader(r.Body, hasher)

	written, err := s.gfs.AppendFromWithNamespace(r.Context(), gfsPath, gfsNamespace, tee)
	if err != nil {
		slog.Error("handleUploadPatch: AppendFromWithNamespace", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	newBytesReceived := bytesReceived + written

	// Serialize hash state for checkpoint
	var newHashState []byte
	if m, ok := hasher.(interface {
		MarshalBinary() ([]byte, error)
	}); ok {
		if newHashState, err = m.MarshalBinary(); err != nil {
			slog.Error("handleUploadPatch: MarshalBinary hash state", "err", err)
			// Non-fatal: hash can't be resumed, but upload continues
		}
	}

	if err := updateUploadSession(r.Context(), s.db, uploadUUID, newHashState, newBytesReceived); err != nil {
		slog.Error("handleUploadPatch: updateUploadSession", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	uploadURL := fmt.Sprintf("/v2/%s/blobs/uploads/%s", repoName, uploadUUID)
	w.Header().Set("Location", uploadURL)
	w.Header().Set("Docker-Upload-UUID", uploadUUID)
	w.Header().Set("Range", fmt.Sprintf("0-%d", newBytesReceived-1))
	w.WriteHeader(http.StatusNoContent)
}

// handleUploadComplete handles PUT /v2/{name}/blobs/uploads/{uuid}?digest=sha256:...
func (s *server) handleUploadComplete(w http.ResponseWriter, r *http.Request) {
	repoName, uploadUUID, ok := parseUploadPath(r)
	if !ok {
		http.NotFound(w, r)
		return
	}

	auth := s.authenticate(r)
	if !hasAccess(auth, repoName, "push") {
		s.requireAuth(w, repoName, "push")
		return
	}

	digest := r.URL.Query().Get("digest")
	if digest == "" {
		ociError(w, http.StatusBadRequest, "DIGEST_INVALID", "digest parameter required")
		return
	}

	repoID, hashStateBytes, bytesReceived, err := getUploadSession(r.Context(), s.db, uploadUUID)
	if err == sql.ErrNoRows {
		ociError(w, http.StatusNotFound, "BLOB_UPLOAD_UNKNOWN", "upload session not found")
		return
	}
	if err != nil {
		slog.Error("handleUploadComplete: getUploadSession", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Restore hash state
	hasher := sha256.New()
	if len(hashStateBytes) > 0 {
		if um, ok := hasher.(interface {
			UnmarshalBinary([]byte) error
		}); ok {
			if err := um.UnmarshalBinary(hashStateBytes); err != nil {
				slog.Error("handleUploadComplete: UnmarshalBinary hash state", "err", err)
				hasher = sha256.New()
			}
		}
	}

	gfsPath := uploadGFSPath(uploadUUID)

	// If there is a body in this PUT, stream it into GFS as well
	if r.ContentLength != 0 {
		tee := io.TeeReader(r.Body, hasher)
		n, err := s.gfs.AppendFromWithNamespace(r.Context(), gfsPath, gfsNamespace, tee)
		if err != nil {
			slog.Error("handleUploadComplete: AppendFromWithNamespace", "err", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		bytesReceived += n
	}

	got := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	if got != digest {
		// Clean up upload artefacts
		_ = s.gfs.DeleteFileWithNamespace(r.Context(), gfsPath, gfsNamespace)
		_ = deleteUploadSession(r.Context(), s.db, uploadUUID)
		ociError(w, http.StatusBadRequest, "DIGEST_INVALID", fmt.Sprintf("digest mismatch: expected %s got %s", digest, got))
		return
	}

	finalPath := blobGFSPath(digest)
	if err := s.gfs.RenameFileWithNamespace(r.Context(), gfsPath, finalPath, gfsNamespace); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			_ = s.gfs.DeleteFileWithNamespace(r.Context(), gfsPath, gfsNamespace)
		} else {
			slog.Error("handleUploadComplete: RenameFileWithNamespace", "err", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}

	if err := insertRepoBlob(r.Context(), s.db, repoID, digest, bytesReceived); err != nil {
		slog.Error("handleUploadComplete: insertRepoBlob", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if err := deleteUploadSession(r.Context(), s.db, uploadUUID); err != nil {
		slog.Error("handleUploadComplete: deleteUploadSession (non-fatal)", "err", err)
	}

	w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/%s", repoName, digest))
	w.Header().Set("Docker-Content-Digest", digest)
	w.WriteHeader(http.StatusCreated)
}

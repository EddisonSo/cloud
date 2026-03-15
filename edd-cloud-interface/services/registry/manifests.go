package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

// ociManifest is used to decode and validate OCI/Docker image manifests.
type ociManifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`
	Config        struct {
		Digest string `json:"digest"`
		Size   int64  `json:"size"`
	} `json:"config"`
	Layers []struct {
		Digest string `json:"digest"`
		Size   int64  `json:"size"`
	} `json:"layers"`
}

// manifestGFSPath returns the GFS path for a stored manifest.
// Format: manifests/{repoName}/{digest_hex}
func manifestGFSPath(repoName, digest string) string {
	hex := strings.TrimPrefix(digest, "sha256:")
	return "manifests/" + repoName + "/" + hex
}

// parseManifestPath parses /v2/{name}/manifests/{reference} and returns
// (repoName, reference). The repo name may contain slashes.
func parseManifestPath(r *http.Request) (repoName, reference string, ok bool) {
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/v2/")
	idx := strings.LastIndex(path, "/manifests/")
	if idx < 0 {
		return "", "", false
	}
	repoName = path[:idx]
	reference = path[idx+len("/manifests/"):]
	if repoName == "" || reference == "" {
		return "", "", false
	}
	return repoName, reference, true
}

// handleManifestGet handles GET /v2/{name}/manifests/{reference}
func (s *server) handleManifestGet(w http.ResponseWriter, r *http.Request) {
	s.serveManifest(w, r, false)
}

// handleManifestHead handles HEAD /v2/{name}/manifests/{reference}
func (s *server) handleManifestHead(w http.ResponseWriter, r *http.Request) {
	s.serveManifest(w, r, true)
}

// serveManifest is the shared implementation for GET and HEAD manifest requests.
func (s *server) serveManifest(w http.ResponseWriter, r *http.Request, headOnly bool) {
	repoName, reference, ok := parseManifestPath(r)
	if !ok {
		http.NotFound(w, r)
		return
	}

	repoID, _, visibility, err := getRepoByName(r.Context(), s.db, repoName)
	if err == sql.ErrNoRows {
		ociError(w, http.StatusNotFound, "NAME_UNKNOWN", "repository not found")
		return
	}
	if err != nil {
		slog.Error("serveManifest: getRepoByName", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Auth: pull access required unless repo is public (visibility == 1)
	auth := s.authenticate(r)
	if visibility != 1 && !hasAccess(auth, repoName, "pull") {
		s.requireAuth(w, repoName, "pull")
		return
	}

	// Resolve reference to digest
	var digest string
	if strings.HasPrefix(reference, "sha256:") {
		digest = reference
		// Verify the manifest exists in the DB
		_, _, _, err := getManifest(r.Context(), s.db, repoID, digest)
		if err == sql.ErrNoRows {
			ociError(w, http.StatusNotFound, "MANIFEST_UNKNOWN", "manifest unknown")
			return
		}
		if err != nil {
			slog.Error("serveManifest: getManifest by digest", "err", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	} else {
		digest, err = getTagDigest(r.Context(), s.db, repoID, reference)
		if err == sql.ErrNoRows {
			ociError(w, http.StatusNotFound, "MANIFEST_UNKNOWN", "tag not found")
			return
		}
		if err != nil {
			slog.Error("serveManifest: getTagDigest", "err", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}

	_, mediaType, size, err := getManifest(r.Context(), s.db, repoID, digest)
	if err == sql.ErrNoRows {
		ociError(w, http.StatusNotFound, "MANIFEST_UNKNOWN", "manifest unknown")
		return
	}
	if err != nil {
		slog.Error("serveManifest: getManifest", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if mediaType == "" {
		mediaType = "application/vnd.docker.distribution.manifest.v2+json"
	}

	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Docker-Content-Digest", digest)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))

	if headOnly {
		w.WriteHeader(http.StatusOK)
		return
	}

	if _, err := s.gfs.ReadToWithNamespace(r.Context(), manifestGFSPath(repoName, digest), gfsNamespace, w); err != nil {
		slog.Error("serveManifest: ReadToWithNamespace", "repo", repoName, "digest", digest, "err", err)
		// Headers already sent; can't change status code
	}
}

// handleManifestPut handles PUT /v2/{name}/manifests/{reference}
func (s *server) handleManifestPut(w http.ResponseWriter, r *http.Request) {
	repoName, reference, ok := parseManifestPath(r)
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
		slog.Error("handleManifestPut: getOrCreateRepo", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Read body with a 10 MB limit
	body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024))
	if err != nil {
		ociError(w, http.StatusBadRequest, "MANIFEST_INVALID", "failed to read manifest body")
		return
	}

	// Compute digest
	sum := sha256.Sum256(body)
	digest := "sha256:" + hex.EncodeToString(sum[:])

	// Parse and validate manifest
	var m ociManifest
	if err := json.Unmarshal(body, &m); err != nil {
		ociError(w, http.StatusBadRequest, "MANIFEST_INVALID", "invalid JSON")
		return
	}
	if m.SchemaVersion != 2 {
		ociError(w, http.StatusBadRequest, "MANIFEST_INVALID", "schemaVersion must be 2")
		return
	}

	// Collect all blob digests referenced by this manifest
	var blobDigests []string
	if m.Config.Digest != "" {
		blobDigests = append(blobDigests, m.Config.Digest)
	}
	for _, layer := range m.Layers {
		if layer.Digest != "" {
			blobDigests = append(blobDigests, layer.Digest)
		}
	}

	// Validate that all referenced blobs exist in the repository
	for _, bd := range blobDigests {
		exists, _, err := blobExistsInRepo(r.Context(), s.db, repoID, bd)
		if err != nil {
			slog.Error("handleManifestPut: blobExistsInRepo", "digest", bd, "err", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !exists {
			ociError(w, http.StatusBadRequest, "BLOB_UNKNOWN",
				fmt.Sprintf("blob unknown to registry: %s", bd))
			return
		}
		// Clear any GC mark since this blob is now referenced
		if err := clearBlobGCMark(r.Context(), s.db, repoID, bd); err != nil {
			slog.Error("handleManifestPut: clearBlobGCMark", "digest", bd, "err", err)
		}
	}

	// Determine media type from Content-Type header or manifest field
	mediaType := r.Header.Get("Content-Type")
	if mediaType == "" {
		mediaType = m.MediaType
	}
	if mediaType == "" {
		mediaType = "application/vnd.docker.distribution.manifest.v2+json"
	}

	// Store manifest in GFS (skip if it already exists)
	gfsPath := manifestGFSPath(repoName, digest)
	if _, err := s.gfs.GetFileWithNamespace(r.Context(), gfsPath, gfsNamespace); err != nil {
		// File doesn't exist; create and write it
		if _, err := s.gfs.CreateFileWithNamespace(r.Context(), gfsPath, gfsNamespace); err != nil {
			slog.Error("handleManifestPut: CreateFileWithNamespace", "err", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if _, err := s.gfs.AppendFromWithNamespace(r.Context(), gfsPath, gfsNamespace,
			bytes.NewReader(body)); err != nil {
			slog.Error("handleManifestPut: AppendFromWithNamespace", "err", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}

	// Upsert manifest record in DB
	manifestID, err := upsertManifest(r.Context(), s.db, repoID, digest, mediaType, int64(len(body)))
	if err != nil {
		slog.Error("handleManifestPut: upsertManifest", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Populate manifest_blobs join table
	if err := insertManifestBlobs(r.Context(), s.db, manifestID, blobDigests); err != nil {
		slog.Error("handleManifestPut: insertManifestBlobs", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Handle tag: upsert and possibly clean up old manifest
	if !strings.HasPrefix(reference, "sha256:") {
		oldDigest, err := upsertTag(r.Context(), s.db, repoID, reference, digest)
		if err != nil {
			slog.Error("handleManifestPut: upsertTag", "err", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		// Eager cleanup: if old digest has no other tags pointing at it, remove it
		if oldDigest != "" && oldDigest != digest {
			if err := s.cleanupManifest(r.Context(), repoID, repoName, oldDigest, reference); err != nil {
				slog.Error("handleManifestPut: cleanupManifest (old digest)", "digest", oldDigest, "err", err)
				// Non-fatal: GC will handle it later
			}
		}
	}

	w.Header().Set("Location", fmt.Sprintf("/v2/%s/manifests/%s", repoName, digest))
	w.Header().Set("Docker-Content-Digest", digest)
	w.WriteHeader(http.StatusCreated)
}

// handleManifestDelete handles DELETE /v2/{name}/manifests/{reference}
func (s *server) handleManifestDelete(w http.ResponseWriter, r *http.Request) {
	repoName, reference, ok := parseManifestPath(r)
	if !ok {
		http.NotFound(w, r)
		return
	}

	auth := s.authenticate(r)
	if !hasAccess(auth, repoName, "push") {
		s.requireAuth(w, repoName, "push")
		return
	}

	repoID, _, _, err := getRepoByName(r.Context(), s.db, repoName)
	if err == sql.ErrNoRows {
		ociError(w, http.StatusNotFound, "NAME_UNKNOWN", "repository not found")
		return
	}
	if err != nil {
		slog.Error("handleManifestDelete: getRepoByName", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Resolve tag to digest if needed
	var digest string
	var isTag bool
	if strings.HasPrefix(reference, "sha256:") {
		digest = reference
	} else {
		isTag = true
		digest, err = getTagDigest(r.Context(), s.db, repoID, reference)
		if err == sql.ErrNoRows {
			ociError(w, http.StatusNotFound, "MANIFEST_UNKNOWN", "tag not found")
			return
		}
		if err != nil {
			slog.Error("handleManifestDelete: getTagDigest", "err", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}

	// Delete the tag if this is a tag reference
	if isTag {
		const q = `DELETE FROM tags WHERE repository_id = $1 AND name = $2`
		if _, err := s.db.ExecContext(r.Context(), q, repoID, reference); err != nil {
			slog.Error("handleManifestDelete: delete tag", "err", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}

	// Clean up manifest if no remaining tags point to it
	excludeTag := ""
	if isTag {
		excludeTag = reference
	}
	if err := s.cleanupManifest(r.Context(), repoID, repoName, digest, excludeTag); err != nil {
		slog.Error("handleManifestDelete: cleanupManifest", "err", err)
		// Non-fatal: GC will handle remaining cleanup
	}

	w.WriteHeader(http.StatusAccepted)
}

// handleTagsList handles GET /v2/{name}/tags/list
func (s *server) handleTagsList(w http.ResponseWriter, r *http.Request) {
	// Parse repo name: strip /v2/ prefix and /tags/list suffix
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/v2/")
	path = strings.TrimSuffix(path, "/tags/list")
	repoName := path
	if repoName == "" {
		http.NotFound(w, r)
		return
	}

	repoID, _, visibility, err := getRepoByName(r.Context(), s.db, repoName)
	if err == sql.ErrNoRows {
		ociError(w, http.StatusNotFound, "NAME_UNKNOWN", "repository not found")
		return
	}
	if err != nil {
		slog.Error("handleTagsList: getRepoByName", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	auth := s.authenticate(r)
	if visibility != 1 && !hasAccess(auth, repoName, "pull") {
		s.requireAuth(w, repoName, "pull")
		return
	}

	tags, err := listTags(r.Context(), s.db, repoID)
	if err != nil {
		slog.Error("handleTagsList: listTags", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// OCI spec: tags field must be an array (not null)
	if tags == nil {
		tags = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"name": repoName,
		"tags": tags,
	})
}

// handleCatalog handles GET /v2/_catalog with cursor-based pagination.
func (s *server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	auth := s.authenticate(r)

	q := r.URL.Query()
	last := q.Get("last")
	n := 100 // default page size
	if nStr := q.Get("n"); nStr != "" {
		if parsed, err := strconv.Atoi(nStr); err == nil && parsed > 0 {
			n = parsed
		}
	}

	// Fetch one extra row to know if there are more pages
	limit := n + 1

	var repos []string
	var err error
	if auth != nil && auth.UserID != "" {
		// Authenticated: own repos + public repos, cursor after `last`
		const qAuth = `
SELECT name FROM repositories
WHERE (owner_id = $1 OR visibility > 0)
  AND ($2 = '' OR name > $2)
ORDER BY name
LIMIT $3`
		rows, e := s.db.QueryContext(r.Context(), qAuth, auth.UserID, last, limit)
		if e != nil {
			slog.Error("handleCatalog: query (auth)", "err", e)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				slog.Error("handleCatalog: scan", "err", err)
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			repos = append(repos, name)
		}
		err = rows.Err()
	} else {
		// Anonymous: public repos only
		const qAnon = `
SELECT name FROM repositories
WHERE visibility > 0
  AND ($1 = '' OR name > $1)
ORDER BY name
LIMIT $2`
		rows, e := s.db.QueryContext(r.Context(), qAnon, last, limit)
		if e != nil {
			slog.Error("handleCatalog: query (anon)", "err", e)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				slog.Error("handleCatalog: scan", "err", err)
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			repos = append(repos, name)
		}
		err = rows.Err()
	}

	if err != nil {
		slog.Error("handleCatalog: rows.Err", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Determine if there is a next page
	hasMore := len(repos) > n
	if hasMore {
		repos = repos[:n]
	}

	if repos == nil {
		repos = []string{}
	}

	if hasMore {
		nextLast := repos[len(repos)-1]
		nStr := strconv.Itoa(n)
		w.Header().Set("Link", fmt.Sprintf(`</v2/_catalog?last=%s&n=%s>; rel="next"`, nextLast, nStr))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"repositories": repos,
	})
}

// cleanupManifest deletes a manifest from GFS and DB if no tags other than
// excludeTag still reference it. excludeTag may be empty to check all tags.
func (s *server) cleanupManifest(ctx context.Context, repoID int, repoName, digest, excludeTag string) error {
	hasOthers, err := manifestHasOtherTags(ctx, s.db, repoID, digest, excludeTag)
	if err != nil {
		return fmt.Errorf("manifestHasOtherTags: %w", err)
	}
	if hasOthers {
		return nil
	}

	// Delete from GFS
	gfsPath := manifestGFSPath(repoName, digest)
	if err := s.gfs.DeleteFileWithNamespace(ctx, gfsPath, gfsNamespace); err != nil {
		slog.Warn("cleanupManifest: DeleteFileWithNamespace (non-fatal)", "path", gfsPath, "err", err)
		// Non-fatal: continue to remove DB record
	}

	// Delete from DB (cascade deletes manifest_blobs rows)
	const q = `DELETE FROM manifests WHERE repository_id = $1 AND digest = $2`
	if _, err := s.db.ExecContext(ctx, q, repoID, digest); err != nil {
		return fmt.Errorf("delete manifest from db: %w", err)
	}

	return nil
}

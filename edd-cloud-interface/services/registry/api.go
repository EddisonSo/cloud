package main

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// setCORSHeaders sets CORS headers for requests from cloud.eddisonso.com origins.
func setCORSHeaders(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin != "" && (strings.HasSuffix(origin, ".cloud.eddisonso.com") || origin == "https://cloud.eddisonso.com") {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, DELETE, OPTIONS")
	}
}

// routeAPI is the catch-all handler for /api/ endpoints.
// Repo names may contain slashes so manual path parsing is used (same pattern as routeV2).
func (s *server) routeAPI(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w, r)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api")

	// GET /api/repos — list repos
	if path == "/repos" {
		if r.Method == http.MethodGet {
			s.handleAPIListRepos(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	if !strings.HasPrefix(path, "/repos/") {
		http.NotFound(w, r)
		return
	}

	// Strip /repos/ prefix; what remains is {name}[/tags[/{tag}]|/visibility]
	rest := strings.TrimPrefix(path, "/repos/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}

	// DELETE /api/repos/{name}/tags/{tag}
	// Check for /tags/ anywhere in the path (last occurrence to support slashes in name)
	if idx := strings.LastIndex(rest, "/tags/"); idx >= 0 {
		repoName := rest[:idx]
		tag := rest[idx+len("/tags/"):]
		if repoName != "" && tag != "" {
			if r.Method == http.MethodDelete {
				s.handleAPIDeleteTag(w, r, repoName, tag)
			} else {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
	}

	// GET /api/repos/{name}/tags — list tags (path ends with /tags)
	if strings.HasSuffix(rest, "/tags") {
		repoName := strings.TrimSuffix(rest, "/tags")
		if repoName != "" {
			if r.Method == http.MethodGet {
				s.handleAPIListTags(w, r, repoName)
			} else {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
	}

	// PUT /api/repos/{name}/visibility — toggle visibility (path ends with /visibility)
	if strings.HasSuffix(rest, "/visibility") {
		repoName := strings.TrimSuffix(rest, "/visibility")
		if repoName != "" {
			if r.Method == http.MethodPut {
				s.handleAPISetVisibility(w, r, repoName)
			} else {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
	}

	// GET /api/repos/{name} — repo detail; name is the entire rest value
	repoName := rest
	if r.Method == http.MethodGet {
		s.handleAPIGetRepo(w, r, repoName)
	} else {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// apiRepo is the JSON representation of a repository in list and detail responses.
type apiRepo struct {
	Name       string    `json:"name"`
	Visibility int       `json:"visibility"`
	OwnerID    string    `json:"owner_id"`
	TagCount   int64     `json:"tag_count"`
	TotalSize  int64     `json:"total_size"`
	LastPushed time.Time `json:"last_pushed"`
}

// apiTag is the JSON representation of a tag in the list-tags response.
type apiTag struct {
	Name     string    `json:"name"`
	Digest   string    `json:"digest"`
	Size     int64     `json:"size"`
	PushedAt time.Time `json:"pushed_at"`
}

// handleAPIListRepos handles GET /api/repos.
// Anonymous callers see only public repos; authenticated callers see own + public.
func (s *server) handleAPIListRepos(w http.ResponseWriter, r *http.Request) {
	auth := s.authenticate(r)

	var rows *sql.Rows
	var err error

	if auth != nil && auth.UserID != "" {
		const q = `
SELECT r.name, r.visibility, r.owner_id,
    COALESCE((SELECT COUNT(*) FROM tags WHERE repository_id = r.id), 0),
    COALESCE((SELECT SUM(rb.size) FROM repository_blobs rb WHERE rb.repository_id = r.id), 0),
    COALESCE((SELECT MAX(t.updated_at) FROM tags t WHERE t.repository_id = r.id), r.created_at)
FROM repositories r
WHERE r.owner_id = $1 OR r.visibility > 0
ORDER BY r.name`
		rows, err = s.db.QueryContext(r.Context(), q, auth.UserID)
	} else {
		const q = `
SELECT r.name, r.visibility, r.owner_id,
    COALESCE((SELECT COUNT(*) FROM tags WHERE repository_id = r.id), 0),
    COALESCE((SELECT SUM(rb.size) FROM repository_blobs rb WHERE rb.repository_id = r.id), 0),
    COALESCE((SELECT MAX(t.updated_at) FROM tags t WHERE t.repository_id = r.id), r.created_at)
FROM repositories r
WHERE r.visibility > 0
ORDER BY r.name`
		rows, err = s.db.QueryContext(r.Context(), q)
	}

	if err != nil {
		slog.Error("handleAPIListRepos: query", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	repos := []apiRepo{}
	for rows.Next() {
		var repo apiRepo
		if err := rows.Scan(&repo.Name, &repo.Visibility, &repo.OwnerID, &repo.TagCount, &repo.TotalSize, &repo.LastPushed); err != nil {
			slog.Error("handleAPIListRepos: scan", "err", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		repos = append(repos, repo)
	}
	if err := rows.Err(); err != nil {
		slog.Error("handleAPIListRepos: rows.Err", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(repos)
}

// handleAPIGetRepo handles GET /api/repos/{name}.
func (s *server) handleAPIGetRepo(w http.ResponseWriter, r *http.Request, repoName string) {
	repoID, ownerID, visibility, err := getRepoByName(r.Context(), s.db, repoName)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		slog.Error("handleAPIGetRepo: getRepoByName", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	auth := s.authenticate(r)
	if visibility != 1 {
		if auth == nil || (auth.UserID != ownerID && !auth.IsSession) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	var repo apiRepo
	const q = `
SELECT r.name, r.visibility, r.owner_id,
    COALESCE((SELECT COUNT(*) FROM tags WHERE repository_id = r.id), 0),
    COALESCE((SELECT SUM(rb.size) FROM repository_blobs rb WHERE rb.repository_id = r.id), 0),
    COALESCE((SELECT MAX(t.updated_at) FROM tags t WHERE t.repository_id = r.id), r.created_at)
FROM repositories r
WHERE r.id = $1`
	if err := s.db.QueryRowContext(r.Context(), q, repoID).Scan(
		&repo.Name, &repo.Visibility, &repo.OwnerID, &repo.TagCount, &repo.TotalSize, &repo.LastPushed,
	); err != nil {
		slog.Error("handleAPIGetRepo: scan", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(repo)
}

// handleAPIListTags handles GET /api/repos/{name}/tags.
func (s *server) handleAPIListTags(w http.ResponseWriter, r *http.Request, repoName string) {
	repoID, ownerID, visibility, err := getRepoByName(r.Context(), s.db, repoName)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		slog.Error("handleAPIListTags: getRepoByName", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	auth := s.authenticate(r)
	if visibility != 1 {
		if auth == nil || (auth.UserID != ownerID && !auth.IsSession) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	const q = `
SELECT t.name, t.manifest_digest, COALESCE(m.size, 0), t.updated_at
FROM tags t
LEFT JOIN manifests m ON m.repository_id = t.repository_id AND m.digest = t.manifest_digest
WHERE t.repository_id = $1
ORDER BY t.name`
	rows, err := s.db.QueryContext(r.Context(), q, repoID)
	if err != nil {
		slog.Error("handleAPIListTags: query", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	tags := []apiTag{}
	for rows.Next() {
		var tag apiTag
		if err := rows.Scan(&tag.Name, &tag.Digest, &tag.Size, &tag.PushedAt); err != nil {
			slog.Error("handleAPIListTags: scan", "err", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		slog.Error("handleAPIListTags: rows.Err", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tags)
}

// handleAPISetVisibility handles PUT /api/repos/{name}/visibility.
// Only the owner may change visibility. Session tokens do not bypass this check.
func (s *server) handleAPISetVisibility(w http.ResponseWriter, r *http.Request, repoName string) {
	auth := s.authenticate(r)
	if auth == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	_, ownerID, _, err := getRepoByName(r.Context(), s.db, repoName)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		slog.Error("handleAPISetVisibility: getRepoByName", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if auth.UserID != ownerID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	var body struct {
		Visibility int `json:"visibility"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	const q = `UPDATE repositories SET visibility = $1, updated_at = NOW() WHERE name = $2`
	if _, err := s.db.ExecContext(r.Context(), q, body.Visibility, repoName); err != nil {
		slog.Error("handleAPISetVisibility: update", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleAPIDeleteTag handles DELETE /api/repos/{name}/tags/{tag}.
// Only the owner may delete tags. Session tokens do not bypass this check.
func (s *server) handleAPIDeleteTag(w http.ResponseWriter, r *http.Request, repoName, tag string) {
	auth := s.authenticate(r)
	if auth == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	repoID, ownerID, _, err := getRepoByName(r.Context(), s.db, repoName)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		slog.Error("handleAPIDeleteTag: getRepoByName", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if auth.UserID != ownerID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	digest, err := getTagDigest(r.Context(), s.db, repoID, tag)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		slog.Error("handleAPIDeleteTag: getTagDigest", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	const delTag = `DELETE FROM tags WHERE repository_id = $1 AND name = $2`
	if _, err := s.db.ExecContext(r.Context(), delTag, repoID, tag); err != nil {
		slog.Error("handleAPIDeleteTag: delete tag", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if err := s.cleanupManifest(r.Context(), repoID, repoName, digest, tag); err != nil {
		slog.Error("handleAPIDeleteTag: cleanupManifest", "err", err)
		// Non-fatal: GC will handle remaining cleanup
	}

	w.WriteHeader(http.StatusNoContent)
}

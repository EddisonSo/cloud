package api

import (
	"net/http"
	"strings"
)

// actionForMethod maps an HTTP method to the scope action it requires.
// POST (add a domain/mapping, and the refresh/verify sub-actions) -> create.
func actionForMethod(r *http.Request) string {
	switch r.Method {
	case http.MethodGet:
		return "read"
	case http.MethodDelete:
		return "delete"
	default:
		return "create"
	}
}

// resourceIDFromPath extracts the resource id from a by-id networking path,
// e.g. resourceIDFromPath("/api/domains/abc/refresh", "domains") == "abc".
// It returns "" for collection paths (e.g. "/api/domains"), so the caller
// falls back to the resource-level scope.
func resourceIDFromPath(path, resource string) string {
	prefix := "/api/" + resource + "/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(path, prefix)
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i] // drop trailing /refresh, /verify, etc.
	}
	return rest
}

// hasPermission checks whether the granted scopes allow `action` on `scope`.
// It walks up the dot-separated path (e.g. networking.<uid>.domains ->
// networking.<uid>) checking for the action at each level, stopping before the
// bare root (networking), which is not assignable. Mirrors the compute
// service's RBAC cascade so all services authorize identically.
func hasPermission(granted map[string][]string, scope, action string) bool {
	current := scope
	for {
		if actions, ok := granted[current]; ok {
			for _, a := range actions {
				if a == action {
					return true
				}
			}
		}
		lastDot := strings.LastIndex(current, ".")
		if lastDot == -1 {
			break
		}
		parent := current[:lastDot]
		// Don't check the bare root (e.g. "networking") — roots aren't assignable.
		if !strings.Contains(parent, ".") {
			break
		}
		current = parent
	}
	return false
}

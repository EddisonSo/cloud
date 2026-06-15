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

package api

import (
	"context"
	"strings"
)

// requireScope checks if the current user has permission for the given scope and action.
// Session users (AuthType "session") always have full access.
// API token users are checked against their granted scopes with cascading.
func requireScope(ctx context.Context, scope, action string) bool {
	info := getUserInfoFromContext(ctx)
	if info == nil {
		return false
	}
	// Session tokens have full access
	if info.AuthType == "session" {
		return true
	}
	return hasPermission(info.Scopes, scope, action)
}

// hasPermission checks if the granted scopes contain the action for the given scope.
// It walks up the dot-separated path checking for the action at each level,
// stopping before bare roots (compute, storage).
func hasPermission(granted map[string][]string, scope, action string) bool {
	// Check exact scope first, then walk up
	current := scope
	for {
		if actions, ok := granted[current]; ok {
			for _, a := range actions {
				if a == action {
					return true
				}
			}
		}

		// Walk up: compute.abc.containers -> compute.abc
		lastDot := strings.LastIndex(current, ".")
		if lastDot == -1 {
			break
		}
		parent := current[:lastDot]
		// Don't check bare roots (compute, storage) - they're not assignable
		if !strings.Contains(parent, ".") {
			break
		}
		current = parent
	}
	return false
}

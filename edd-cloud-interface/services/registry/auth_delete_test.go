package main

import "testing"

func TestHasAccess_Delete(t *testing.T) {
	deleter := &authResult{Access: []registryAccess{{Type: "repository", Name: "eddison/resume", Actions: []string{"delete"}}}}
	puller := &authResult{Access: []registryAccess{{Type: "repository", Name: "eddison/resume", Actions: []string{"pull"}}}}

	if !hasAccess(deleter, "eddison/resume", "delete") {
		t.Error("delete token should be allowed to delete")
	}
	if hasAccess(puller, "eddison/resume", "delete") {
		t.Error("pull-only token must not be allowed to delete")
	}
}

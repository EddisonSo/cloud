package main

import "testing"

func TestHasAccess_Delete(t *testing.T) {
	deleter := &authResult{Access: []registryAccess{{Type: "repository", Name: "eddison/resume", Actions: []string{"delete"}}}}
	puller := &authResult{Access: []registryAccess{{Type: "repository", Name: "eddison/resume", Actions: []string{"pull"}}}}

	// OCI tokens are scoped by their Access claims and are independent of the
	// repository owner, so ownerID is irrelevant here.
	if !hasAccess(deleter, "eddison/resume", "alice", "delete") {
		t.Error("delete token should be allowed to delete")
	}
	if hasAccess(puller, "eddison/resume", "alice", "delete") {
		t.Error("pull-only token must not be allowed to delete")
	}
}

// TestHasAccess_SessionOwnerScoped verifies that a dashboard session token can
// only access repositories it owns, and never another user's repo.
func TestHasAccess_SessionOwnerScoped(t *testing.T) {
	owner := &authResult{UserID: "alice", IsSession: true}
	other := &authResult{UserID: "bob", IsSession: true}

	if !hasAccess(owner, "alice/app", "alice", "pull") {
		t.Error("session owner should be allowed to pull their own repo")
	}
	if hasAccess(other, "alice/app", "alice", "pull") {
		t.Error("session user must NOT access another user's repo")
	}
	// Unknown/non-existent repo (ownerID == "") must be denied for sessions.
	if hasAccess(owner, "alice/app", "", "pull") {
		t.Error("session must be denied when repo owner is unknown")
	}
}

// TestHasAccess_OCITokenIgnoresOwner verifies an OCI token with a matching
// Access scope still grants access regardless of the repo owner.
func TestHasAccess_OCITokenIgnoresOwner(t *testing.T) {
	tok := &authResult{Access: []registryAccess{{Type: "repository", Name: "alice/app", Actions: []string{"pull"}}}}
	if !hasAccess(tok, "alice/app", "alice", "pull") {
		t.Error("OCI token should pull when scope matches, regardless of owner")
	}
	if !hasAccess(tok, "alice/app", "", "pull") {
		t.Error("OCI token scope should grant access even when owner is unknown")
	}
}

// TestHasAccess_Anonymous verifies anonymous (nil) auth is always denied.
func TestHasAccess_Anonymous(t *testing.T) {
	if hasAccess(nil, "alice/app", "alice", "pull") {
		t.Error("anonymous access must be denied")
	}
}

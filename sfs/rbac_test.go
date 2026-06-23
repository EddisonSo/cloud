package main

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// captureAudit redirects the default slog logger to a buffer for the duration of
// fn, returning everything that was logged.
func captureAudit(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	old := slog.Default()
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(old)
	fn()
	return buf.String()
}

// TestRequireAuthWithScope_DenialEmitsAudit verifies that an RBAC denial (an API
// token lacking the required scope) returns 403 AND emits an audit event marked
// audit=true with action=authz.denied / outcome=denied.
func TestRequireAuthWithScope_DenialEmitsAudit(t *testing.T) {
	secret := []byte("test-secret")
	s := &server{
		jwtSecret: secret,
		tkCache:   newTokenCache(),
		idStore:   newIdentityStore(nil),
	}

	// Pre-seed the token cache so the revocation check is a hit (no network call):
	// valid token, no scopes -> scope check below must fail.
	const tokenID = "tok-test"
	s.tkCache.entries[tokenID] = tokenCacheEntry{valid: true, scopes: nil, checkedAt: time.Now()}

	// Build a legacy standalone API token (Type=api_token, no service account,
	// empty scopes) so requireAuthWithScope reaches the scope-denial branch.
	claims := &JWTClaims{
		Username: "alice",
		UserID:   "user-123",
		Type:     "api_token",
		TokenID:  tokenID,
		Scopes:   map[string][]string{},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/storage/secret-ns/file.txt", nil)
	req = req.WithContext(context.Background())
	req.Header.Set("Authorization", "Bearer "+signed)
	rec := httptest.NewRecorder()

	var (
		gotUser string
		gotOK   bool
	)
	out := captureAudit(t, func() {
		gotUser, gotOK = s.requireAuthWithScope(rec, req, "files", "delete", "secret-ns")
	})

	if gotOK {
		t.Fatalf("expected denial (ok=false), got ok=true user=%q", gotUser)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}

	for _, want := range []string{
		"audit=true",
		"action=authz.denied",
		"outcome=denied",
		"resource=secret-ns",
		"actor=user-123",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("audit output missing %q\nfull output:\n%s", want, out)
		}
	}
}

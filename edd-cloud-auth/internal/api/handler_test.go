package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// signToken signs an arbitrary claims object with the handler's secret.
func signToken(t *testing.T, secret []byte, claims jwt.Claims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString(secret)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return s
}

// requestWithToken builds a request carrying the given bearer token.
func requestWithToken(token string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/settings/keys", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	return r
}

// TestValidateToken_Rejects2FAChallengeToken guards against the JWT
// token-type-confusion vulnerability: a 2FA challenge token (issued after
// password verification but before the second factor) must NOT be accepted by
// the generic session validator, otherwise a password-only attacker bypasses
// 2FA and reaches session-protected endpoints.
func TestValidateToken_Rejects2FAChallengeToken(t *testing.T) {
	secret := []byte("test-secret")
	h := &Handler{jwtSecret: secret}

	challenge := signToken(t, secret, TwoFAClaims{
		UserID: "u1",
		Type:   "2fa_challenge",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})

	if _, ok := h.validateToken(requestWithToken(challenge)); ok {
		t.Fatal("validateToken must reject a 2fa_challenge token")
	}
}

// TestValidateToken_AcceptsSessionToken ensures the fix does not break the
// legitimate session flow: a normal session token (no type) is still accepted.
func TestValidateToken_AcceptsSessionToken(t *testing.T) {
	secret := []byte("test-secret")
	h := &Handler{jwtSecret: secret}

	session := signToken(t, secret, JWTClaims{
		Username: "alice",
		UserID:   "u1",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   "alice",
		},
	})

	claims, ok := h.validateToken(requestWithToken(session))
	if !ok {
		t.Fatal("validateToken must accept a normal session token")
	}
	if claims.Username != "alice" {
		t.Fatalf("expected username alice, got %q", claims.Username)
	}
}

// TestRequireAuth_Rejects2FAChallengeToken verifies the middleware-level
// behavior: a 2fa_challenge token must yield 401 and never invoke the handler.
func TestRequireAuth_Rejects2FAChallengeToken(t *testing.T) {
	secret := []byte("test-secret")
	h := &Handler{jwtSecret: secret}

	challenge := signToken(t, secret, TwoFAClaims{
		UserID: "u1",
		Type:   "2fa_challenge",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})

	called := false
	guarded := h.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	guarded(rec, requestWithToken(challenge))

	if called {
		t.Fatal("protected handler must not run for a 2fa_challenge token")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func signToken(t *testing.T, secret, userID string) string {
	claims := JWTClaims{
		UserID:   userID,
		Username: "alice",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

func TestValidateSession(t *testing.T) {
	v := &SessionValidator{jwtSecret: []byte("topsecret")}
	good := signToken(t, "topsecret", "u123")
	claims, err := v.ValidateSession(good)
	if err != nil || claims.UserID != "u123" {
		t.Fatalf("good token: %v %+v", err, claims)
	}
	if _, err := v.ValidateSession("ecloud_" + good); err != nil {
		t.Errorf("prefixed token: %v", err)
	}
	bad := signToken(t, "othersecret", "u123")
	if _, err := v.ValidateSession(bad); err == nil {
		t.Error("expected rejection of wrong-secret token")
	}
}

func TestValidateSessionExpired(t *testing.T) {
	v := &SessionValidator{jwtSecret: []byte("topsecret")}
	claims := JWTClaims{
		UserID: "u1",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)), // already expired
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString(v.jwtSecret)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := v.ValidateSession(s); err == nil {
		t.Error("expected expired token to be rejected")
	}
}

func TestValidateSessionRejectsNonHMAC(t *testing.T) {
	// An unsigned (alg: none) token must be rejected: the keyfunc only accepts
	// *jwt.SigningMethodHMAC, which is the abuse gate against algorithm confusion.
	v := &SessionValidator{jwtSecret: []byte("topsecret")}
	claims := JWTClaims{UserID: "u1"}
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	s, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none: %v", err)
	}
	if _, err := v.ValidateSession(s); err == nil {
		t.Error("expected alg:none token to be rejected")
	}
}

func TestExtractToken(t *testing.T) {
	if got := ExtractToken("Bearer abc", ""); got != "abc" {
		t.Errorf("bearer: %q", got)
	}
	if got := ExtractToken("", "cookieval"); got != "cookieval" {
		t.Errorf("cookie: %q", got)
	}
	if got := ExtractToken("", ""); got != "" {
		t.Errorf("empty: %q", got)
	}
}

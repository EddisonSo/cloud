// Package auth validates the HS256 JWTs issued by edd-cloud-auth. It mirrors
// the compute service's SessionValidator so all services accept the same tokens.
package auth

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// JWTClaims matches edd-cloud-auth's session claims (internal/api/handler.go)
// and the api-token claims (internal/api/tokens.go). Session tokens leave Type
// and Scopes empty; service-account (api_token) tokens set Type="api_token"
// and carry their granted Scopes.
type JWTClaims struct {
	Username    string              `json:"username"`
	DisplayName string              `json:"display_name"`
	UserID      string              `json:"user_id"`
	Type        string              `json:"type,omitempty"`
	Scopes      map[string][]string `json:"scopes,omitempty"`
	jwt.RegisteredClaims
}

// IsServiceAccount reports whether these claims came from a service-account
// (api_token) token rather than an interactive session.
func (c *JWTClaims) IsServiceAccount() bool { return c.Type == "api_token" }

// SessionValidator validates tokens against the shared JWT_SECRET.
type SessionValidator struct {
	jwtSecret []byte
}

// NewSessionValidator reads JWT_SECRET from the environment (same K8s secret as auth).
func NewSessionValidator() *SessionValidator {
	return &SessionValidator{jwtSecret: getJWTSecret()}
}

func getJWTSecret() []byte {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			slog.Error("failed to generate fallback JWT secret", "error", err)
			os.Exit(1)
		}
		slog.Warn("JWT_SECRET not set, using random secret (tokens will not validate)")
		return b
	}
	return []byte(secret)
}

// ValidateSession parses and verifies a token, returning its claims.
func (v *SessionValidator) ValidateSession(tokenString string) (*JWTClaims, error) {
	tokenString = strings.TrimPrefix(tokenString, "ecloud_")
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return v.jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	// Allowlist (default-deny): accept only interactive sessions (Type == "") and
	// API/service-account tokens (Type == "api_token"). A pre-auth 2fa_challenge
	// token (or any future intermediate type) is rejected — token-type-confusion
	// guard, matching the api server's Type switch.
	if claims.Type != "" && claims.Type != "api_token" {
		return nil, fmt.Errorf("invalid token type")
	}
	return claims, nil
}

// ExtractToken pulls the bearer token from an Authorization header or `token` cookie.
func ExtractToken(authHeader, cookie string) string {
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	return cookie
}

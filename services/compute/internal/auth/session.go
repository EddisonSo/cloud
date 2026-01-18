package auth

import (
	"crypto/rand"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// JWTClaims represents the claims in a JWT token
type JWTClaims struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	UserID      int64  `json:"user_id"`
	jwt.RegisteredClaims
}

type SessionValidator struct {
	jwtSecret []byte
}

func NewSessionValidator(_ string) *SessionValidator {
	return &SessionValidator{
		jwtSecret: getJWTSecret(),
	}
}

// getJWTSecret returns the JWT signing secret from environment variable
func getJWTSecret() []byte {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			log.Fatal("failed to generate JWT secret:", err)
		}
		log.Println("WARNING: JWT_SECRET not set, using random secret")
		return b
	}
	return []byte(secret)
}

// ValidateSession validates a JWT token
// Returns the username if valid, empty string if invalid
func (v *SessionValidator) ValidateSession(tokenString string) (string, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return v.jwtSecret, nil
	})
	if err != nil {
		return "", nil
	}
	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return "", nil
	}
	return claims.Username, nil
}

// GetSessionToken extracts the JWT token from Authorization header or query parameter
func GetSessionToken(r *http.Request) string {
	// Check Authorization header first
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	// Fall back to query parameter (for WebSocket connections)
	if token := r.URL.Query().Get("token"); token != "" {
		return token
	}
	return ""
}

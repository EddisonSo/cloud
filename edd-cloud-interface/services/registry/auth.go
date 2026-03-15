package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type registryAccess struct {
	Type    string   `json:"type"`
	Name    string   `json:"name"`
	Actions []string `json:"actions"`
}

type registryClaims struct {
	Access []registryAccess `json:"access,omitempty"`
	jwt.RegisteredClaims
}

// sessionClaims represents a standard session JWT from the auth service.
type sessionClaims struct {
	UserID string `json:"user_id"`
	Type   string `json:"type"`
	jwt.RegisteredClaims
}

type authResult struct {
	UserID    string
	Access    []registryAccess
	IsSession bool // true if authenticated via session token (full access to own repos)
}

// authenticate validates the JWT from Authorization header.
// Accepts both OCI registry tokens (from /v2/token) and session tokens (from auth service).
// Returns nil for anonymous requests.
func (s *server) authenticate(r *http.Request) *authResult {
	auth := r.Header.Get("Authorization")
	if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
		return nil
	}
	tokenStr := strings.TrimPrefix(auth, "Bearer ")

	// Try OCI registry token first
	token, err := jwt.ParseWithClaims(tokenStr, &registryClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err == nil {
		if claims, ok := token.Claims.(*registryClaims); ok {
			return &authResult{UserID: claims.Subject, Access: claims.Access}
		}
	}

	// Try session token (from auth service, used for internal service-to-service calls)
	sessToken, err := jwt.ParseWithClaims(tokenStr, &sessionClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err == nil {
		if claims, ok := sessToken.Claims.(*sessionClaims); ok && claims.UserID != "" {
			return &authResult{UserID: claims.UserID, IsSession: true}
		}
	}

	return nil
}

// hasAccess checks if the auth result grants the given action on the repository.
func hasAccess(auth *authResult, repoName, action string) bool {
	if auth == nil {
		return false
	}
	// Session tokens have full access (used by internal services forwarding user identity)
	if auth.IsSession {
		return true
	}
	for _, a := range auth.Access {
		if a.Type == "repository" && a.Name == repoName {
			for _, act := range a.Actions {
				if act == action {
					return true
				}
			}
		}
	}
	return false
}

// requireAuth sends 401 with WWW-Authenticate challenge header.
func (s *server) requireAuth(w http.ResponseWriter, repoName, action string) {
	scope := ""
	if repoName != "" {
		scope = fmt.Sprintf(`,scope="repository:%s:%s"`, repoName, action)
	}
	w.Header().Set("WWW-Authenticate",
		fmt.Sprintf(`Bearer realm="https://auth.cloud.eddisonso.com/v2/token",service="registry.cloud.eddisonso.com"%s`, scope))
	w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

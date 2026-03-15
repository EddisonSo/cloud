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

type authResult struct {
	UserID string
	Access []registryAccess
}

// authenticate validates the registry JWT from Authorization header.
// Returns nil for anonymous requests.
func (s *server) authenticate(r *http.Request) *authResult {
	auth := r.Header.Get("Authorization")
	if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
		return nil
	}
	tokenStr := strings.TrimPrefix(auth, "Bearer ")
	token, err := jwt.ParseWithClaims(tokenStr, &registryClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil
	}
	claims, ok := token.Claims.(*registryClaims)
	if !ok {
		return nil
	}
	return &authResult{UserID: claims.Subject, Access: claims.Access}
}

// hasAccess checks if the auth result grants the given action on the repository.
func hasAccess(auth *authResult, repoName, action string) bool {
	if auth == nil {
		return false
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

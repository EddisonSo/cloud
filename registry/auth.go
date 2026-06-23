package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"eddisonso.com/edd-cloud/pkg/auditlog"
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

	// Try session token first (from auth service, used by the frontend dashboard).
	// Must be checked before OCI registry tokens because both share the same
	// signing key and registryClaims would successfully parse a session JWT,
	// but would use Subject (username) as the UserID instead of the user_id claim.
	sessToken, err := jwt.ParseWithClaims(tokenStr, &sessionClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err == nil {
		if claims, ok := sessToken.Claims.(*sessionClaims); ok && claims.UserID != "" {
			// Allowlist (default-deny): only treat a token as a session when its
			// type is interactive ("") or an API/service-account token ("api_token").
			// A pre-auth 2fa_challenge token (or any future intermediate type) is
			// refused here and falls through to the OCI path, where it has no Access
			// claim and therefore grants no access either.
			if claims.Type == "" || claims.Type == "api_token" {
				return &authResult{UserID: claims.UserID, IsSession: true}
			}
		}
	}

	// Try OCI registry token (from /v2/token, used by docker push/pull)
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

	return nil
}

// hasAccess checks if the auth result grants the given action on the repository.
// ownerID is the owner of the repository being accessed; session tokens are
// scoped to repos they own, while OCI tokens are scoped by their Access claims.
func hasAccess(auth *authResult, repoName, ownerID, action string) bool {
	if auth == nil {
		return false
	}
	// Session tokens (dashboard users) may only access repositories they own.
	if auth.IsSession {
		return ownerID != "" && auth.UserID == ownerID
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

// requireAuth sends 401 with WWW-Authenticate challenge header and records a
// security audit event for the denied access. ctx must carry the request-scoped
// audit fields seeded by auditlog.HTTPMiddleware.
func (s *server) requireAuth(ctx context.Context, w http.ResponseWriter, repoName, action string) {
	scope := ""
	if repoName != "" {
		scope = fmt.Sprintf(`,scope="repository:%s:%s"`, repoName, action)
	}
	w.Header().Set("WWW-Authenticate",
		fmt.Sprintf(`Bearer realm="https://auth.cloud.eddisonso.com/v2/token",service="registry.cloud.eddisonso.com"%s`, scope))
	w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")

	resource := repoName
	if resource == "" {
		resource = "/v2/"
	}
	auditlog.Denied(ctx, "authz.denied", resource, "access", action)

	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

// imageRef formats a repository + reference into a canonical image reference
// for audit logs: "repo@sha256:..." for digest refs, "repo:tag" for tags.
func imageRef(repoName, reference string) string {
	if strings.HasPrefix(reference, "sha256:") {
		return repoName + "@" + reference
	}
	return repoName + ":" + reference
}

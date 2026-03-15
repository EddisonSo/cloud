package api

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type registryTokenClaims struct {
	Access []registryAccess `json:"access,omitempty"`
	jwt.RegisteredClaims
}

type registryAccess struct {
	Type    string   `json:"type"`
	Name    string   `json:"name"`
	Actions []string `json:"actions"`
}

type registryTokenResponse struct {
	Token     string `json:"token"`
	ExpiresIn int    `json:"expires_in"`
	IssuedAt  string `json:"issued_at"`
}

func (h *Handler) handleRegistryToken(w http.ResponseWriter, r *http.Request) {
	service := r.URL.Query().Get("service")
	scope := r.URL.Query().Get("scope")

	var requested []registryAccess
	if scope != "" {
		access, err := parseOCIScope(scope)
		if err == nil {
			requested = []registryAccess{access}
		}
		// If parse fails, treat as empty scope (anonymous / catalog access)
	}

	username, password, hasBasicAuth := r.BasicAuth()

	now := time.Now()
	expiry := now.Add(5 * time.Minute)

	var granted []registryAccess
	var tokenSubject string

	if hasBasicAuth && username != "" && password != "" {
		// Try user password auth first
		userID, err := h.authenticateRegistryUser(username, password)
		if err == nil {
			// Authenticated as a regular user
			tokenSubject = userID
			granted = h.filterAccessByUser(requested, userID)
		} else if strings.HasPrefix(password, "ecloud_") {
			// Try service account token auth (ecloud_ prefix)
			userID, saID, err := h.authenticateRegistryServiceAccount(username, password)
			if err == nil {
				tokenSubject = userID
				granted = h.filterAccessByServiceAccount(requested, saID, userID)
			} else {
				granted = nil
			}
		} else if access, sub, ok := h.validateInternalToken(password); ok {
			// Internal service token (JWT minted by compute/other services with shared secret)
			tokenSubject = sub
			granted = access
		} else {
			// Invalid credentials — issue empty token
			granted = nil
		}
	} else {
		// Anonymous — pull-only on public repos
		granted = h.filterAccessAnonymous(requested)
	}

	claims := registryTokenClaims{
		Access: granted,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiry),
			Issuer:    "auth.cloud.eddisonso.com",
			Subject:   tokenSubject,
			Audience:  jwt.ClaimStrings{service},
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(h.jwtSecret)
	if err != nil {
		writeError(w, "failed to sign token", http.StatusInternalServerError)
		return
	}

	writeJSON(w, registryTokenResponse{
		Token:     tokenString,
		ExpiresIn: 300,
		IssuedAt:  now.UTC().Format(time.RFC3339),
	})
}

// parseOCIScope parses a Docker registry scope string.
// Format: repository:<name>:<actions>
func parseOCIScope(scope string) (registryAccess, error) {
	parts := strings.SplitN(scope, ":", 3)
	if len(parts) != 3 {
		return registryAccess{}, fmt.Errorf("invalid scope format: %s", scope)
	}
	if parts[0] != "repository" {
		return registryAccess{}, fmt.Errorf("unsupported scope type: %s", parts[0])
	}
	name := parts[1]
	if name == "" {
		return registryAccess{}, fmt.Errorf("empty repository name in scope")
	}
	var actions []string
	for _, a := range strings.Split(parts[2], ",") {
		a = strings.TrimSpace(a)
		if a != "" {
			actions = append(actions, a)
		}
	}
	return registryAccess{
		Type:    "repository",
		Name:    name,
		Actions: actions,
	}, nil
}

// authenticateRegistryUser verifies a username/password pair against the users table.
// Returns the userID on success.
func (h *Handler) authenticateRegistryUser(username, password string) (string, error) {
	user, err := h.db.GetUserByUsername(username)
	if err != nil {
		return "", fmt.Errorf("db error: %w", err)
	}
	if user == nil {
		return "", fmt.Errorf("user not found")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", fmt.Errorf("invalid password")
	}
	return user.UserID, nil
}

// authenticateRegistryServiceAccount validates an ecloud_ service account token.
// Returns (userID, serviceAccountID, error).
func (h *Handler) authenticateRegistryServiceAccount(username, tokenStr string) (string, string, error) {
	// Strip the ecloud_ prefix and parse the JWT
	raw := strings.TrimPrefix(tokenStr, "ecloud_")

	var claims APITokenClaims
	token, err := jwt.ParseWithClaims(raw, &claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return h.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return "", "", fmt.Errorf("invalid token")
	}

	if claims.ServiceAccountID == "" {
		return "", "", fmt.Errorf("not a service account token")
	}

	// Verify the token hash exists in the DB (revocation check)
	hash := sha256.Sum256([]byte(tokenStr))
	hashHex := hex.EncodeToString(hash[:])
	exists, err := h.db.CheckTokenHash(hashHex)
	if err != nil || !exists {
		return "", "", fmt.Errorf("token not found or revoked")
	}

	return claims.UserID, claims.ServiceAccountID, nil
}

// filterAccessByUser grants access based on repo ownership/visibility.
// Pull is granted for owned or public repos; push only for owned repos.
func (h *Handler) filterAccessByUser(requested []registryAccess, userID string) []registryAccess {
	if len(requested) == 0 {
		return nil
	}

	var granted []registryAccess
	for _, req := range requested {
		if req.Type != "repository" {
			continue
		}

		repo, err := h.db.GetRepositoryByName(req.Name)
		if err != nil && err != sql.ErrNoRows {
			// DB error — deny access
			continue
		}

		var allowedActions []string
		if repo == nil {
			// Repo doesn't exist yet — grant full access so the push creates it
			allowedActions = req.Actions
		} else if repo.OwnerID == userID {
			// Owner — grant all requested actions
			allowedActions = req.Actions
		} else if repo.Visibility == 1 {
			// Public repo — pull only
			for _, action := range req.Actions {
				if action == "pull" {
					allowedActions = append(allowedActions, action)
				}
			}
		}
		// else: private repo, not owner — no access

		if len(allowedActions) > 0 {
			granted = append(granted, registryAccess{
				Type:    req.Type,
				Name:    req.Name,
				Actions: allowedActions,
			})
		}
	}
	return granted
}

// translateSAActionToOCI maps a service account scope action to an OCI registry action.
// SA scopes use read/create/update/delete; OCI uses pull/push.
func translateSAActionToOCI(saAction string) string {
	switch saAction {
	case "read":
		return "pull"
	case "create", "update", "delete":
		return "push"
	default:
		return saAction
	}
}

// filterAccessByServiceAccount grants access based on the SA's registry scopes.
// SA scopes for registry follow: storage.<userID>.registry.<repoName>
func (h *Handler) filterAccessByServiceAccount(requested []registryAccess, saID, userID string) []registryAccess {
	if len(requested) == 0 {
		return nil
	}

	sa, err := h.db.GetServiceAccountByID(saID)
	if err != nil || sa == nil {
		return nil
	}

	var granted []registryAccess
	for _, req := range requested {
		if req.Type != "repository" {
			continue
		}

		// Check if SA has a scope for this repo
		// Scope format: storage.<userID>.registry.<repoName>
		scopeKey := fmt.Sprintf("storage.%s.registry.%s", userID, req.Name)
		saActions, ok := sa.Scopes[scopeKey]
		if !ok {
			// Also check wildcard registry scope
			wildcardKey := fmt.Sprintf("storage.%s.registry", userID)
			saActions, ok = sa.Scopes[wildcardKey]
		}

		// Build OCI action set from translated SA scopes
		ociActionSet := make(map[string]bool)
		if ok {
			for _, a := range saActions {
				ociActionSet[translateSAActionToOCI(a)] = true
			}
		}

		// Also grant pull if the repo is public (Fix #6)
		repo, repoErr := h.db.GetRepositoryByName(req.Name)
		if repoErr == nil && repo != nil && repo.Visibility == 1 {
			ociActionSet["pull"] = true
		}

		if len(ociActionSet) == 0 {
			continue
		}

		var allowedActions []string
		for _, action := range req.Actions {
			if ociActionSet[action] {
				allowedActions = append(allowedActions, action)
			}
		}

		if len(allowedActions) > 0 {
			granted = append(granted, registryAccess{
				Type:    req.Type,
				Name:    req.Name,
				Actions: allowedActions,
			})
		}
	}
	return granted
}

// validateInternalToken checks if a password is a valid internal JWT with pre-authorized access.
// Used by compute service to create imagePullSecrets with scoped pull tokens.
func (h *Handler) validateInternalToken(tokenStr string) ([]registryAccess, string, bool) {
	type internalClaims struct {
		Access []registryAccess `json:"access,omitempty"`
		jwt.RegisteredClaims
	}
	token, err := jwt.ParseWithClaims(tokenStr, &internalClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return h.jwtSecret, nil
	})
	if err != nil {
		return nil, "", false
	}
	claims, ok := token.Claims.(*internalClaims)
	if !ok || claims.Subject == "" || len(claims.Access) == 0 {
		return nil, "", false
	}
	return claims.Access, claims.Subject, true
}

// filterAccessAnonymous grants pull-only access to public repos.
func (h *Handler) filterAccessAnonymous(requested []registryAccess) []registryAccess {
	if len(requested) == 0 {
		return nil
	}

	var granted []registryAccess
	for _, req := range requested {
		if req.Type != "repository" {
			continue
		}

		repo, err := h.db.GetRepositoryByName(req.Name)
		if err != nil || repo == nil {
			// Repo doesn't exist or DB error — no anonymous access
			continue
		}

		if repo.Visibility != 1 {
			continue
		}

		var allowedActions []string
		for _, action := range req.Actions {
			if action == "pull" {
				allowedActions = append(allowedActions, action)
			}
		}

		if len(allowedActions) > 0 {
			granted = append(granted, registryAccess{
				Type:    req.Type,
				Name:    req.Name,
				Actions: allowedActions,
			})
		}
	}
	return granted
}

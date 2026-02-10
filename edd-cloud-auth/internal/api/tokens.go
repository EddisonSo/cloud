package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// APITokenClaims represents the claims in an API token JWT
type APITokenClaims struct {
	UserID           string              `json:"user_id"`
	TokenID          string              `json:"token_id"`
	Type             string              `json:"type"` // "api_token"
	Scopes           map[string][]string `json:"scopes,omitempty"`
	ServiceAccountID string              `json:"service_account_id,omitempty"`
	jwt.RegisteredClaims
}

// Valid scope roots and allowed actions
var (
	validRoots   = map[string]bool{"compute": true, "storage": true}
	validActions = map[string]bool{"create": true, "read": true, "update": true, "delete": true}
)

type createTokenRequest struct {
	Name      string              `json:"name"`
	Scopes    map[string][]string `json:"scopes"`
	ExpiresIn string              `json:"expires_in"` // "30d", "90d", "365d", "never"
}

type tokenResponse struct {
	ID               string              `json:"id"`
	Name             string              `json:"name"`
	Scopes           map[string][]string `json:"scopes"`
	ExpiresAt        int64               `json:"expires_at"`
	LastUsedAt       int64               `json:"last_used_at"`
	CreatedAt        int64               `json:"created_at"`
	ServiceAccountID *string             `json:"service_account_id,omitempty"`
	Token            string              `json:"token,omitempty"` // Only on creation
}

func (h *Handler) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.validateToken(r)
	if !ok {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req createTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, "token name required", http.StatusBadRequest)
		return
	}
	if len(req.Name) > 64 {
		writeError(w, "token name too long (max 64 characters)", http.StatusBadRequest)
		return
	}

	if len(req.Scopes) == 0 {
		writeError(w, "at least one scope required", http.StatusBadRequest)
		return
	}

	// Validate scopes
	if err := validateScopes(req.Scopes, claims.UserID); err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Parse expiry
	var expiresAt int64
	var expiresDuration time.Duration
	switch req.ExpiresIn {
	case "30d":
		expiresDuration = 30 * 24 * time.Hour
	case "90d":
		expiresDuration = 90 * 24 * time.Hour
	case "365d":
		expiresDuration = 365 * 24 * time.Hour
	case "never", "":
		expiresDuration = 0
	default:
		writeError(w, "invalid expires_in (use 30d, 90d, 365d, or never)", http.StatusBadRequest)
		return
	}

	if expiresDuration > 0 {
		expiresAt = time.Now().Add(expiresDuration).Unix()
	}

	// Create a placeholder token to get the ID
	tokenHash := "pending" // temporary
	token, err := h.db.CreateAPIToken(claims.UserID, req.Name, tokenHash, req.Scopes, expiresAt)
	if err != nil {
		writeError(w, "failed to create token", http.StatusInternalServerError)
		return
	}

	// Sign JWT with token ID
	jwtClaims := APITokenClaims{
		UserID:  claims.UserID,
		TokenID: token.ID,
		Type:    "api_token",
		Scopes:  req.Scopes,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt: jwt.NewNumericDate(time.Now()),
			Subject:  claims.UserID,
		},
	}
	if expiresAt > 0 {
		jwtClaims.ExpiresAt = jwt.NewNumericDate(time.Unix(expiresAt, 0))
	}

	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims)
	tokenString, err := jwtToken.SignedString(h.jwtSecret)
	if err != nil {
		writeError(w, "failed to sign token", http.StatusInternalServerError)
		return
	}

	// Add ecloud_ prefix
	fullToken := "ecloud_" + tokenString

	// Store SHA-256 hash of the full token
	hash := sha256.Sum256([]byte(fullToken))
	hashHex := hex.EncodeToString(hash[:])

	// Update the token hash in DB
	_, err = h.db.Exec(`UPDATE api_tokens SET token_hash = $1 WHERE id = $2`, hashHex, token.ID)
	if err != nil {
		writeError(w, "failed to store token", http.StatusInternalServerError)
		return
	}

	writeJSON(w, tokenResponse{
		ID:        token.ID,
		Name:      token.Name,
		Scopes:    token.Scopes,
		ExpiresAt: token.ExpiresAt,
		CreatedAt: token.CreatedAt,
		Token:     fullToken,
	})
}

func (h *Handler) handleListTokens(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.validateToken(r)
	if !ok {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	tokens, err := h.db.ListAPITokensByUser(claims.UserID)
	if err != nil {
		writeError(w, "failed to list tokens", http.StatusInternalServerError)
		return
	}

	resp := make([]tokenResponse, 0, len(tokens))
	for _, t := range tokens {
		resp = append(resp, tokenResponse{
			ID:               t.ID,
			Name:             t.Name,
			Scopes:           t.Scopes,
			ExpiresAt:        t.ExpiresAt,
			LastUsedAt:       t.LastUsedAt,
			CreatedAt:        t.CreatedAt,
			ServiceAccountID: t.ServiceAccountID,
		})
	}

	writeJSON(w, resp)
}

func (h *Handler) handleDeleteToken(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.validateToken(r)
	if !ok {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, "token id required", http.StatusBadRequest)
		return
	}

	if err := h.db.DeleteAPIToken(id, claims.UserID); err != nil {
		writeError(w, "token not found", http.StatusNotFound)
		return
	}

	writeJSON(w, map[string]string{"status": "ok"})
}

// handleCheckToken is a service-to-service endpoint for revocation checks.
// Returns 200 if the token is valid and not expired, 404 otherwise.
// For service account tokens, includes the SA's current scopes in the response.
func (h *Handler) handleCheckToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	token, err := h.db.GetAPITokenByID(id)
	if err != nil || token == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Check expiry
	if token.ExpiresAt > 0 && time.Now().Unix() > token.ExpiresAt {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// If token belongs to a service account, return the SA's current scopes
	if token.ServiceAccountID != nil {
		sa, err := h.db.GetServiceAccountByID(*token.ServiceAccountID)
		if err != nil || sa == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]interface{}{
			"status": "valid",
			"scopes": sa.Scopes,
		})
		return
	}

	writeJSON(w, map[string]string{"status": "valid"})
}

// validateScopes checks that all scopes reference the caller's user_id,
// paths are valid, and actions are from the allowed set.
func validateScopes(scopes map[string][]string, userID string) error {
	validResources := map[string]map[string]bool{
		"compute": {"containers": true, "keys": true},
		"storage": {"namespaces": true, "files": true},
	}

	for scope, actions := range scopes {
		parts := strings.Split(scope, ".")
		if len(parts) < 2 || len(parts) > 4 {
			return fmt.Errorf("invalid scope path: %s (must be <root>.<userid>[.<resource>[.<id>]])", scope)
		}

		root := parts[0]
		if !validRoots[root] {
			return fmt.Errorf("invalid scope root: %s (must be compute or storage)", root)
		}

		scopeUserID := parts[1]
		if scopeUserID != userID {
			return fmt.Errorf("cannot create token for another user's resources")
		}

		if len(parts) >= 3 {
			resource := parts[2]
			if !validResources[root][resource] {
				return fmt.Errorf("invalid resource: %s for root %s", resource, root)
			}
		}

		if len(parts) == 4 {
			resourceID := parts[3]
			if resourceID == "" {
				return fmt.Errorf("resource ID cannot be empty in scope %s", scope)
			}
		}

		if len(actions) == 0 {
			return fmt.Errorf("at least one action required for scope %s", scope)
		}
		for _, action := range actions {
			if !validActions[action] {
				return fmt.Errorf("invalid action: %s (must be create, read, update, or delete)", action)
			}
		}
	}
	return nil
}

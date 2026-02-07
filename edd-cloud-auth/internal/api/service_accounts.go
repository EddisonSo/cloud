package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type createServiceAccountRequest struct {
	Name   string              `json:"name"`
	Scopes map[string][]string `json:"scopes"`
}

type serviceAccountResponse struct {
	ID         string              `json:"id"`
	Name       string              `json:"name"`
	Scopes     map[string][]string `json:"scopes"`
	TokenCount int                 `json:"token_count"`
	CreatedAt  int64               `json:"created_at"`
}

type createSATokenRequest struct {
	Name      string `json:"name"`
	ExpiresIn string `json:"expires_in"`
}

type saTokenResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ExpiresAt  int64  `json:"expires_at"`
	LastUsedAt int64  `json:"last_used_at"`
	CreatedAt  int64  `json:"created_at"`
	Token      string `json:"token,omitempty"`
}

func (h *Handler) handleCreateServiceAccount(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.validateToken(r)
	if !ok {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req createServiceAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, "service account name required", http.StatusBadRequest)
		return
	}
	if len(req.Name) > 64 {
		writeError(w, "name too long (max 64 characters)", http.StatusBadRequest)
		return
	}

	if len(req.Scopes) == 0 {
		writeError(w, "at least one scope required", http.StatusBadRequest)
		return
	}

	if err := validateScopes(req.Scopes, claims.UserID); err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	sa, err := h.db.CreateServiceAccount(claims.UserID, req.Name, req.Scopes)
	if err != nil {
		writeError(w, "failed to create service account", http.StatusInternalServerError)
		return
	}

	if h.publisher != nil {
		if err := h.publisher.PublishIdentityPermissionsUpdated(sa.ID, sa.UserID, sa.Scopes, sa.Version); err != nil {
			// Log but don't fail the request — event will be caught on next sync
			_ = err
		}
	}

	writeJSON(w, serviceAccountResponse{
		ID:         sa.ID,
		Name:       sa.Name,
		Scopes:     sa.Scopes,
		TokenCount: 0,
		CreatedAt:  sa.CreatedAt,
	})
}

func (h *Handler) handleListServiceAccounts(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.validateToken(r)
	if !ok {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	accounts, err := h.db.ListServiceAccountsByUser(claims.UserID)
	if err != nil {
		writeError(w, "failed to list service accounts", http.StatusInternalServerError)
		return
	}

	resp := make([]serviceAccountResponse, 0, len(accounts))
	for _, sa := range accounts {
		count, _ := h.db.CountServiceAccountTokens(sa.ID)
		resp = append(resp, serviceAccountResponse{
			ID:         sa.ID,
			Name:       sa.Name,
			Scopes:     sa.Scopes,
			TokenCount: count,
			CreatedAt:  sa.CreatedAt,
		})
	}

	writeJSON(w, resp)
}

func (h *Handler) handleGetServiceAccount(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.validateToken(r)
	if !ok {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, "service account id required", http.StatusBadRequest)
		return
	}

	sa, err := h.db.GetServiceAccountByID(id)
	if err != nil {
		writeError(w, "failed to get service account", http.StatusInternalServerError)
		return
	}
	if sa == nil || sa.UserID != claims.UserID {
		writeError(w, "service account not found", http.StatusNotFound)
		return
	}

	count, _ := h.db.CountServiceAccountTokens(sa.ID)

	writeJSON(w, serviceAccountResponse{
		ID:         sa.ID,
		Name:       sa.Name,
		Scopes:     sa.Scopes,
		TokenCount: count,
		CreatedAt:  sa.CreatedAt,
	})
}

func (h *Handler) handleUpdateServiceAccountScopes(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.validateToken(r)
	if !ok {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, "service account id required", http.StatusBadRequest)
		return
	}

	var req struct {
		Scopes map[string][]string `json:"scopes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.Scopes) == 0 {
		writeError(w, "at least one scope required", http.StatusBadRequest)
		return
	}

	if err := validateScopes(req.Scopes, claims.UserID); err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	newVersion, err := h.db.UpdateServiceAccountScopes(id, claims.UserID, req.Scopes)
	if err != nil {
		writeError(w, "service account not found", http.StatusNotFound)
		return
	}

	if h.publisher != nil {
		if err := h.publisher.PublishIdentityPermissionsUpdated(id, claims.UserID, req.Scopes, newVersion); err != nil {
			_ = err
		}
	}

	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *Handler) handleDeleteServiceAccount(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.validateToken(r)
	if !ok {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, "service account id required", http.StatusBadRequest)
		return
	}

	version, err := h.db.DeleteServiceAccount(id, claims.UserID)
	if err != nil {
		writeError(w, "service account not found", http.StatusNotFound)
		return
	}

	if h.publisher != nil {
		if err := h.publisher.PublishIdentityPermissionsDeleted(id, claims.UserID, version); err != nil {
			_ = err
		}
	}

	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *Handler) handleCreateServiceAccountToken(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.validateToken(r)
	if !ok {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	saID := r.PathValue("id")
	if saID == "" {
		writeError(w, "service account id required", http.StatusBadRequest)
		return
	}

	// Verify SA belongs to user
	sa, err := h.db.GetServiceAccountByID(saID)
	if err != nil {
		writeError(w, "failed to get service account", http.StatusInternalServerError)
		return
	}
	if sa == nil || sa.UserID != claims.UserID {
		writeError(w, "service account not found", http.StatusNotFound)
		return
	}

	var req createSATokenRequest
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

	// Create placeholder token to get the ID
	token, err := h.db.CreateServiceAccountToken(claims.UserID, saID, req.Name, "pending", expiresAt)
	if err != nil {
		writeError(w, "failed to create token", http.StatusInternalServerError)
		return
	}

	// Sign JWT — scopes left empty, service_account_id included
	jwtClaims := APITokenClaims{
		UserID:           claims.UserID,
		TokenID:          token.ID,
		Type:             "api_token",
		ServiceAccountID: saID,
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

	fullToken := "ecloud_" + tokenString

	// Store SHA-256 hash
	hash := sha256.Sum256([]byte(fullToken))
	hashHex := hex.EncodeToString(hash[:])

	_, err = h.db.Exec(`UPDATE api_tokens SET token_hash = $1 WHERE id = $2`, hashHex, token.ID)
	if err != nil {
		writeError(w, "failed to store token", http.StatusInternalServerError)
		return
	}

	writeJSON(w, saTokenResponse{
		ID:        token.ID,
		Name:      token.Name,
		ExpiresAt: token.ExpiresAt,
		CreatedAt: token.CreatedAt,
		Token:     fullToken,
	})
}

func (h *Handler) handleListServiceAccountTokens(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.validateToken(r)
	if !ok {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	saID := r.PathValue("id")
	if saID == "" {
		writeError(w, "service account id required", http.StatusBadRequest)
		return
	}

	// Verify SA belongs to user
	sa, err := h.db.GetServiceAccountByID(saID)
	if err != nil {
		writeError(w, "failed to get service account", http.StatusInternalServerError)
		return
	}
	if sa == nil || sa.UserID != claims.UserID {
		writeError(w, "service account not found", http.StatusNotFound)
		return
	}

	tokens, err := h.db.ListTokensByServiceAccount(saID, claims.UserID)
	if err != nil {
		writeError(w, "failed to list tokens", http.StatusInternalServerError)
		return
	}

	resp := make([]saTokenResponse, 0, len(tokens))
	for _, t := range tokens {
		resp = append(resp, saTokenResponse{
			ID:         t.ID,
			Name:       t.Name,
			ExpiresAt:  t.ExpiresAt,
			LastUsedAt: t.LastUsedAt,
			CreatedAt:  t.CreatedAt,
		})
	}

	writeJSON(w, resp)
}

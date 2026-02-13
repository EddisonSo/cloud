package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	"golang.org/x/crypto/bcrypt"
)

// --- Security Keys ---

type keyInfo struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	AuthenticatorType string `json:"authenticator_type"`
	CreatedAt         int64  `json:"created_at"`
}

func (h *Handler) handleListKeys(w http.ResponseWriter, r *http.Request) {
	claims := h.getClaims(r)

	creds, err := h.db.GetCredentialsByUserID(claims.UserID)
	if err != nil {
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	keys := make([]keyInfo, len(creds))
	for i, c := range creds {
		keys[i] = keyInfo{
			ID:                base64.RawURLEncoding.EncodeToString(c.ID),
			Name:              c.Name,
			AuthenticatorType: "Security Key",
			CreatedAt:         c.CreatedAt,
		}
	}

	writeJSON(w, map[string]interface{}{"keys": keys})
}

func (h *Handler) handleDeleteKey(w http.ResponseWriter, r *http.Request) {
	claims := h.getClaims(r)

	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request", http.StatusBadRequest)
		return
	}

	keyID, err := base64.RawURLEncoding.DecodeString(req.ID)
	if err != nil {
		writeError(w, "invalid key id", http.StatusBadRequest)
		return
	}

	count, err := h.db.CountCredentialsByUserID(claims.UserID)
	if err != nil {
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if count <= 1 {
		writeError(w, "cannot delete last security key", http.StatusBadRequest)
		return
	}

	if err := h.db.DeleteCredential(keyID, claims.UserID); err != nil {
		writeError(w, "key not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleRenameKey(w http.ResponseWriter, r *http.Request) {
	claims := h.getClaims(r)

	var req struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		writeError(w, "name is required", http.StatusBadRequest)
		return
	}

	keyID, err := base64.RawURLEncoding.DecodeString(req.ID)
	if err != nil {
		writeError(w, "invalid key id", http.StatusBadRequest)
		return
	}

	if err := h.db.UpdateCredentialName(keyID, claims.UserID, req.Name); err != nil {
		writeError(w, "key not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// --- Profile ---

func (h *Handler) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	claims := h.getClaims(r)

	var req struct {
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request", http.StatusBadRequest)
		return
	}

	if err := h.db.UpdateUser(claims.UserID, req.DisplayName); err != nil {
		writeError(w, "failed to update profile", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"username":     claims.Username,
		"display_name": req.DisplayName,
		"user_id":      claims.UserID,
	})
}

func (h *Handler) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	claims := h.getClaims(r)

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request", http.StatusBadRequest)
		return
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		writeError(w, "current and new password required", http.StatusBadRequest)
		return
	}

	user, err := h.db.GetUserByID(claims.UserID)
	if err != nil || user == nil {
		writeError(w, "user not found", http.StatusNotFound)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		writeError(w, "current password is incorrect", http.StatusUnauthorized)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, "failed to hash password", http.StatusInternalServerError)
		return
	}

	if err := h.db.UpdatePassword(claims.UserID, string(hash)); err != nil {
		writeError(w, "failed to update password", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// --- Sessions ---

type sessionInfo struct {
	ID        int64  `json:"id"`
	IPAddress string `json:"ip_address"`
	CreatedAt int64  `json:"created_at"`
	IsCurrent bool   `json:"is_current"`
}

func (h *Handler) handleListUserSessions(w http.ResponseWriter, r *http.Request) {
	claims := h.getClaims(r)
	currentToken := h.extractToken(r)

	sessions, err := h.db.ListSessionsByUserID(claims.UserID)
	if err != nil {
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	result := make([]sessionInfo, len(sessions))
	for i, s := range sessions {
		result[i] = sessionInfo{
			ID:        s.ID,
			IPAddress: s.IPAddress,
			CreatedAt: s.CreatedAt,
			IsCurrent: s.Token == currentToken,
		}
	}

	writeJSON(w, result)
}


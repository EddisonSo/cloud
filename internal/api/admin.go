package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

type adminUserResponse struct {
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

type createUserRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type sessionListResponse struct {
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	CreatedAt   int64  `json:"created_at"`
	IPAddress   string `json:"ip_address"`
}

func (h *Handler) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.db.ListUsers()
	if err != nil {
		writeError(w, "failed to list users", http.StatusInternalServerError)
		return
	}

	resp := make([]adminUserResponse, 0, len(users))
	for _, u := range users {
		resp = append(resp, adminUserResponse{
			UserID:      u.UserID,
			Username:    u.Username,
			DisplayName: u.DisplayName,
		})
	}

	writeJSON(w, resp)
}

func (h *Handler) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.Username == "" || req.Password == "" {
		writeError(w, "username and password required", http.StatusBadRequest)
		return
	}

	if req.DisplayName == "" {
		req.DisplayName = req.Username
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, "failed to hash password", http.StatusInternalServerError)
		return
	}

	user, err := h.db.CreateUser(req.Username, string(hash), req.DisplayName)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			writeError(w, "username already exists", http.StatusConflict)
			return
		}
		writeError(w, "failed to create user", http.StatusInternalServerError)
		return
	}

	// Publish user created event
	if h.publisher != nil {
		h.publisher.PublishUserCreated(user.UserID, user.Username, user.DisplayName)
	}

	writeJSON(w, adminUserResponse{
		UserID:      user.UserID,
		Username:    user.Username,
		DisplayName: user.DisplayName,
	})
}

func (h *Handler) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("id")
	if userID == "" {
		writeError(w, "id required", http.StatusBadRequest)
		return
	}

	// Get user info before deleting
	user, err := h.db.GetUserByID(userID)
	if err != nil {
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if user == nil {
		writeError(w, "user not found", http.StatusNotFound)
		return
	}

	// Prevent deleting self
	claims, _ := h.validateToken(r)
	if claims != nil && user.Username == claims.Username {
		writeError(w, "cannot delete yourself", http.StatusBadRequest)
		return
	}

	if err := h.db.DeleteUser(userID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, "user not found", http.StatusNotFound)
			return
		}
		writeError(w, "failed to delete user", http.StatusInternalServerError)
		return
	}

	// Publish user deleted event
	if h.publisher != nil {
		h.publisher.PublishUserDeleted(user.UserID, user.Username)
	}

	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *Handler) handleListSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := h.db.ListActiveSessions()
	if err != nil {
		writeError(w, "failed to list sessions", http.StatusInternalServerError)
		return
	}

	resp := make([]sessionListResponse, 0, len(sessions))
	for _, s := range sessions {
		resp = append(resp, sessionListResponse{
			UserID:      s.UserID,
			Username:    s.Username,
			DisplayName: s.DisplayName,
			CreatedAt:   s.CreatedAt,
			IPAddress:   s.IPAddress,
		})
	}

	writeJSON(w, resp)
}

// handleGetAllUsers returns all users for service-to-service sync
// This endpoint is used by other services during startup to populate their user caches
func (h *Handler) handleGetAllUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.db.ListUsers()
	if err != nil {
		writeError(w, "failed to list users", http.StatusInternalServerError)
		return
	}

	resp := make([]adminUserResponse, 0, len(users))
	for _, u := range users {
		resp = append(resp, adminUserResponse{
			UserID:      u.UserID,
			Username:    u.Username,
			DisplayName: u.DisplayName,
		})
	}

	writeJSON(w, resp)
}

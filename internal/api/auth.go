package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type sessionResponse struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	UserID      int64  `json:"user_id"`
	IsAdmin     bool   `json:"is_admin"`
	Token       string `json:"token,omitempty"`
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeError(w, "username and password required", http.StatusBadRequest)
		return
	}

	user, err := h.db.GetUserByUsername(req.Username)
	if err != nil {
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if user == nil {
		writeError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		writeError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	// Generate JWT token
	expires := time.Now().Add(h.sessionTTL)
	claims := JWTClaims{
		Username:    user.Username,
		DisplayName: user.DisplayName,
		UserID:      user.ID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expires),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   user.Username,
		},
	}

	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := jwtToken.SignedString(h.jwtSecret)
	if err != nil {
		writeError(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	// Store session in database
	clientIP := h.getClientIP(r)
	session, err := h.db.CreateSession(user.ID, tokenString, expires, clientIP)
	if err != nil {
		writeError(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	// Publish session created event
	if h.publisher != nil {
		h.publisher.PublishSessionCreated(session.ID, user.ID, expires)
	}

	writeJSON(w, sessionResponse{
		Username:    user.Username,
		DisplayName: user.DisplayName,
		UserID:      user.ID,
		IsAdmin:     h.isAdmin(user.Username),
		Token:       tokenString,
	})
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.validateToken(r)
	if ok && h.publisher != nil {
		// Publish session invalidated event
		token := h.extractToken(r)
		h.publisher.PublishSessionInvalidated(token, claims.UserID)
	}

	// With JWT, logout is handled client-side by removing the token
	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *Handler) handleSession(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.validateToken(r)
	if !ok {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	writeJSON(w, sessionResponse{
		Username:    claims.Username,
		DisplayName: claims.DisplayName,
		UserID:      claims.UserID,
		IsAdmin:     h.isAdmin(claims.Username),
	})
}

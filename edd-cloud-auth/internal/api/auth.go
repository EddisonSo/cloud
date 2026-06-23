package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"eddisonso.com/edd-cloud/pkg/auditlog"
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
	UserID      string `json:"user_id"` // public_id (nanoid)
	IsAdmin     bool   `json:"is_admin"`
	Token       string `json:"token,omitempty"`
}

type loginResponse struct {
	Username       string `json:"username,omitempty"`
	DisplayName    string `json:"display_name,omitempty"`
	UserID         string `json:"user_id,omitempty"`
	IsAdmin        bool   `json:"is_admin,omitempty"`
	Token          string `json:"token,omitempty"`
	Requires2FA    bool   `json:"requires_2fa,omitempty"`
	ChallengeToken string `json:"challenge_token,omitempty"`
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

	// Rate limit by IP and username
	clientIP := h.getClientIP(r)
	if !h.ipLimiter.allow(clientIP) {
		auditlog.Denied(r.Context(), "ratelimit.reject", req.Username)
		writeError(w, "too many login attempts, try again later", http.StatusTooManyRequests)
		return
	}
	if !h.userLimiter.allow(req.Username) {
		auditlog.Denied(r.Context(), "ratelimit.reject", req.Username)
		writeError(w, "too many login attempts for this account, try again later", http.StatusTooManyRequests)
		return
	}

	user, err := h.db.GetUserByUsername(req.Username)
	if err != nil {
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if user == nil {
		auditlog.Failure(r.Context(), "auth.login", req.Username)
		writeError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		auditlog.Failure(r.Context(), "auth.login", req.Username)
		writeError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	// Check if user has security keys (2FA required)
	credCount, err := h.db.CountCredentialsByUserID(user.UserID)
	if err != nil {
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	if credCount > 0 {
		// Issue short-lived challenge token for 2FA
		challengeClaims := TwoFAClaims{
			UserID: user.UserID,
			Type:   "2fa_challenge",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
			},
		}
		challengeToken := jwt.NewWithClaims(jwt.SigningMethodHS256, challengeClaims)
		challengeTokenStr, err := challengeToken.SignedString(h.jwtSecret)
		if err != nil {
			writeError(w, "failed to create challenge", http.StatusInternalServerError)
			return
		}

		auditlog.Success(auditlog.WithActor(r.Context(), user.UserID), "auth.2fa.challenge", user.Username)
		writeJSON(w, loginResponse{
			Requires2FA:    true,
			ChallengeToken: challengeTokenStr,
		})
		return
	}

	// No 2FA — proceed with normal login
	expires := time.Now().Add(h.sessionTTL)
	claims := JWTClaims{
		Username:    user.Username,
		DisplayName: user.DisplayName,
		UserID:      user.UserID,
		IsAdmin:     h.isAdmin(user.Username),
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
	session, err := h.db.CreateSession(user.UserID, tokenString, expires, clientIP)
	if err != nil {
		writeError(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	// Publish session created event
	if h.publisher != nil {
		h.publisher.PublishSessionCreated(session.ID, user.UserID, expires)
	}

	auditlog.Success(auditlog.WithActor(r.Context(), user.UserID), "auth.login", user.Username)

	writeJSON(w, loginResponse{
		Username:    user.Username,
		DisplayName: user.DisplayName,
		UserID:      user.UserID,
		IsAdmin:     h.isAdmin(user.Username),
		Token:       tokenString,
	})
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	token := h.extractToken(r)
	if token != "" {
		h.db.DeleteSession(token)
		// Best-effort: identify the session owner for the audit trail. The token
		// may be expired/invalid (logout is unauthenticated), in which case we
		// still record the invalidation with an anonymous actor.
		username := ""
		if claims, ok := h.validateToken(r); ok {
			username = claims.Username
			r = r.WithContext(auditlog.WithActor(r.Context(), claims.Username))
		}
		auditlog.Success(r.Context(), "session.invalidate", username)
	}
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

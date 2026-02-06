package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"eddisonso.com/edd-cloud-auth/internal/db"
	"eddisonso.com/edd-cloud-auth/internal/events"
	"github.com/golang-jwt/jwt/v5"
)

type Handler struct {
	db         *db.DB
	publisher  *events.Publisher
	jwtSecret  []byte
	sessionTTL time.Duration
	adminUser  string
}

type Config struct {
	DB         *db.DB
	Publisher  *events.Publisher
	JWTSecret  []byte
	SessionTTL time.Duration
	AdminUser  string
}

func NewHandler(cfg Config) *Handler {
	return &Handler{
		db:         cfg.DB,
		publisher:  cfg.Publisher,
		jwtSecret:  cfg.JWTSecret,
		sessionTTL: cfg.SessionTTL,
		adminUser:  cfg.AdminUser,
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Public auth endpoints
	mux.HandleFunc("POST /api/login", h.handleLogin)
	mux.HandleFunc("POST /api/logout", h.handleLogout)
	mux.HandleFunc("GET /api/session", h.handleSession)

	// Service-to-service endpoint (for initial sync)
	mux.HandleFunc("GET /api/users", h.handleGetAllUsers)

	// API token endpoints (session auth required)
	mux.HandleFunc("POST /api/tokens", h.handleCreateToken)
	mux.HandleFunc("GET /api/tokens", h.handleListTokens)
	mux.HandleFunc("DELETE /api/tokens/{id}", h.handleDeleteToken)

	// Service-to-service token check (no auth required)
	mux.HandleFunc("GET /api/tokens/{id}/check", h.handleCheckToken)

	// Admin endpoints
	mux.HandleFunc("GET /admin/users", h.adminOnly(h.handleListUsers))
	mux.HandleFunc("POST /admin/users", h.adminOnly(h.handleCreateUser))
	mux.HandleFunc("DELETE /admin/users", h.adminOnly(h.handleDeleteUser))
	mux.HandleFunc("GET /admin/sessions", h.adminOnly(h.handleListSessions))

	// Health check
	mux.HandleFunc("GET /healthz", h.handleHealthz)
}

// JWTClaims represents the claims in a JWT token
type JWTClaims struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	UserID      string `json:"user_id"` // nanoid
	jwt.RegisteredClaims
}

func (h *Handler) isAdmin(username string) bool {
	return username == h.adminUser || username == os.Getenv("ADMIN_USERNAME")
}

func (h *Handler) adminOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := h.validateToken(r)
		if !ok {
			writeError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !h.isAdmin(claims.Username) {
			writeError(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func (h *Handler) validateToken(r *http.Request) (*JWTClaims, bool) {
	tokenString := h.extractToken(r)
	if tokenString == "" {
		return nil, false
	}

	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return h.jwtSecret, nil
	})
	if err != nil {
		return nil, false
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, false
	}

	return claims, true
}

func (h *Handler) extractToken(r *http.Request) string {
	// Check Authorization header first
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}

	// Check cookie
	cookie, err := r.Cookie("token")
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}

	return ""
}

func (h *Handler) getClientIP(r *http.Request) string {
	clientIP := r.Header.Get("X-Forwarded-For")
	if clientIP == "" {
		clientIP = r.Header.Get("X-Real-IP")
	}
	if clientIP == "" {
		clientIP = r.RemoteAddr
	}
	// Take first IP if comma-separated
	if idx := strings.Index(clientIP, ","); idx != -1 {
		clientIP = strings.TrimSpace(clientIP[:idx])
	}
	// Remove port if present
	if idx := strings.LastIndex(clientIP, ":"); idx != -1 {
		if !strings.Contains(clientIP[idx:], "]") { // not IPv6
			clientIP = clientIP[:idx]
		}
	}
	return clientIP
}

func (h *Handler) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

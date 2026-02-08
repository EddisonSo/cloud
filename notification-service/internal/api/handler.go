package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/net/websocket"

	"eddisonso.com/notification-service/internal/db"
)

type JWTClaims struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	UserID      string `json:"user_id"`
	jwt.RegisteredClaims
}

type Handler struct {
	db        *db.DB
	jwtSecret []byte
	wsMu      sync.RWMutex
	wsConns   map[string][]*websocket.Conn // user_id -> connections
}

func NewHandler(database *db.DB, jwtSecret []byte) *Handler {
	return &Handler{
		db:        database,
		jwtSecret: jwtSecret,
		wsConns:   make(map[string][]*websocket.Conn),
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/notifications", h.handleList)
	mux.HandleFunc("GET /api/notifications/unread-count", h.handleUnreadCount)
	mux.HandleFunc("POST /api/notifications/{id}/read", h.handleMarkRead)
	mux.HandleFunc("POST /api/notifications/read-all", h.handleMarkAllRead)
	mux.HandleFunc("GET /api/notifications/mutes", h.handleListMutes)
	mux.HandleFunc("PUT /api/notifications/mutes", h.handleAddMute)
	mux.HandleFunc("DELETE /api/notifications/mutes", h.handleRemoveMute)
	mux.HandleFunc("GET /healthz", h.handleHealthz)
	mux.Handle("GET /ws/notifications", websocket.Handler(h.handleWS))
}

// BroadcastNotification sends a notification to all connected WebSocket clients for a user.
func (h *Handler) BroadcastNotification(n *db.Notification) {
	h.wsMu.RLock()
	conns := h.wsConns[n.UserID]
	h.wsMu.RUnlock()

	if len(conns) == 0 {
		return
	}

	data, err := json.Marshal(n)
	if err != nil {
		slog.Error("failed to marshal notification for ws", "error", err)
		return
	}

	h.wsMu.Lock()
	defer h.wsMu.Unlock()

	active := make([]*websocket.Conn, 0, len(conns))
	for _, conn := range conns {
		if _, err := conn.Write(data); err != nil {
			slog.Debug("ws send failed, removing connection", "user_id", n.UserID, "error", err)
			conn.Close()
			continue
		}
		active = append(active, conn)
	}
	h.wsConns[n.UserID] = active
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.authenticate(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	limit := 20
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	notifications, err := h.db.ListByUser(userID, limit, offset)
	if err != nil {
		slog.Error("failed to list notifications", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if notifications == nil {
		notifications = []db.Notification{}
	}

	writeJSON(w, notifications)
}

func (h *Handler) handleUnreadCount(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.authenticate(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	count, err := h.db.UnreadCount(userID)
	if err != nil {
		slog.Error("failed to get unread count", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]int{"count": count})
}

func (h *Handler) handleMarkRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.authenticate(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid notification id", http.StatusBadRequest)
		return
	}

	if err := h.db.MarkRead(id, userID); err != nil {
		slog.Error("failed to mark notification read", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *Handler) handleMarkAllRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.authenticate(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if err := h.db.MarkAllRead(userID); err != nil {
		slog.Error("failed to mark all notifications read", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *Handler) handleListMutes(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.authenticate(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	mutes, err := h.db.ListMutes(userID)
	if err != nil {
		slog.Error("failed to list mutes", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if mutes == nil {
		mutes = []db.NotificationMute{}
	}

	writeJSON(w, mutes)
}

func (h *Handler) handleAddMute(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.authenticate(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Category string `json:"category"`
		Scope    string `json:"scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Category == "" || req.Scope == "" {
		http.Error(w, "category and scope are required", http.StatusBadRequest)
		return
	}

	if err := h.db.AddMute(userID, req.Category, req.Scope); err != nil {
		slog.Error("failed to add mute", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *Handler) handleRemoveMute(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.authenticate(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Category string `json:"category"`
		Scope    string `json:"scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Category == "" || req.Scope == "" {
		http.Error(w, "category and scope are required", http.StatusBadRequest)
		return
	}

	if err := h.db.RemoveMute(userID, req.Category, req.Scope); err != nil {
		slog.Error("failed to remove mute", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *Handler) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (h *Handler) handleWS(ws *websocket.Conn) {
	r := ws.Request()

	// Validate origin
	origin := ws.Config().Origin
	if origin == nil || !isAllowedOrigin(origin.String()) {
		ws.Close()
		return
	}

	// Authenticate via query parameter or header
	token := r.URL.Query().Get("token")
	if token == "" {
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			token = strings.TrimPrefix(auth, "Bearer ")
		}
	}

	userID := h.validateJWT(token)
	if userID == "" {
		ws.Close()
		return
	}

	slog.Info("ws connected", "user_id", userID)

	const maxConnsPerUser = 5
	h.wsMu.Lock()
	if len(h.wsConns[userID]) >= maxConnsPerUser {
		h.wsMu.Unlock()
		ws.Close()
		return
	}
	h.wsConns[userID] = append(h.wsConns[userID], ws)
	h.wsMu.Unlock()

	defer func() {
		h.wsMu.Lock()
		conns := h.wsConns[userID]
		for i, c := range conns {
			if c == ws {
				h.wsConns[userID] = append(conns[:i], conns[i+1:]...)
				break
			}
		}
		if len(h.wsConns[userID]) == 0 {
			delete(h.wsConns, userID)
		}
		h.wsMu.Unlock()
		ws.Close()
		slog.Info("ws disconnected", "user_id", userID)
	}()

	// Keep connection alive with periodic pings
	buf := make([]byte, 512)
	for {
		if _, err := ws.Read(buf); err != nil {
			return
		}
	}
}

func (h *Handler) authenticate(r *http.Request) (string, bool) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return "", false
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	userID := h.validateJWT(token)
	if userID == "" {
		return "", false
	}
	return userID, true
}

func (h *Handler) validateJWT(tokenString string) string {
	if tokenString == "" {
		return ""
	}

	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return h.jwtSecret, nil
	})
	if err != nil {
		return ""
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return ""
	}

	return claims.UserID
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func isAllowedOrigin(origin string) bool {
	return origin == "https://cloud.eddisonso.com" ||
		(len(origin) > len("https://.cloud.eddisonso.com") &&
			strings.HasSuffix(origin, ".cloud.eddisonso.com") &&
			strings.HasPrefix(origin, "https://"))
}

func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && isAllowedOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func LogRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Debug("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}

package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"eddisonso.com/edd-cloud/services/compute/internal/auth"
	"eddisonso.com/edd-cloud/services/compute/internal/db"
	"eddisonso.com/edd-cloud/services/compute/internal/k8s"
)

type Handler struct {
	db        *db.DB
	k8s       *k8s.Client
	validator *auth.SessionValidator
	mux       *http.ServeMux
}

func NewHandler(database *db.DB, k8sClient *k8s.Client) http.Handler {
	h := &Handler{
		db:        database,
		k8s:       k8sClient,
		validator: auth.NewSessionValidator("http://simple-file-share-backend"),
		mux:       http.NewServeMux(),
	}

	// Health check (both paths for internal probes and external ingress access)
	h.mux.HandleFunc("GET /healthz", h.Healthz)
	h.mux.HandleFunc("GET /compute/healthz", h.Healthz)

	// Container endpoints
	h.mux.HandleFunc("GET /compute/containers", h.authMiddleware(h.ListContainers))
	h.mux.HandleFunc("POST /compute/containers", h.authMiddleware(h.CreateContainer))
	h.mux.HandleFunc("GET /compute/containers/{id}", h.authMiddleware(h.GetContainer))
	h.mux.HandleFunc("DELETE /compute/containers/{id}", h.authMiddleware(h.DeleteContainer))
	h.mux.HandleFunc("POST /compute/containers/{id}/stop", h.authMiddleware(h.StopContainer))
	h.mux.HandleFunc("POST /compute/containers/{id}/start", h.authMiddleware(h.StartContainer))

	// SSH key endpoints
	h.mux.HandleFunc("GET /compute/ssh-keys", h.authMiddleware(h.ListSSHKeys))
	h.mux.HandleFunc("POST /compute/ssh-keys", h.authMiddleware(h.AddSSHKey))
	h.mux.HandleFunc("DELETE /compute/ssh-keys/{id}", h.authMiddleware(h.DeleteSSHKey))

	// WebSocket endpoint for real-time updates
	h.mux.HandleFunc("GET /compute/ws", h.authMiddleware(h.HandleWebSocket))

	// Cloud terminal endpoint
	h.mux.HandleFunc("GET /compute/containers/{id}/terminal", h.authMiddleware(h.HandleTerminal))

	// Admin endpoints
	h.mux.HandleFunc("GET /admin/containers", h.adminMiddleware(h.AdminListContainers))

	return h
}

var adminUsername = os.Getenv("ADMIN_USERNAME")

// adminMiddleware validates session and checks admin status
func (h *Handler) adminMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := auth.GetSessionToken(r)
		if token != "" {
			username, err := h.validator.ValidateSession(token)
			if err != nil {
				slog.Error("session validation failed", "error", err)
				http.Error(w, "authentication error", http.StatusInternalServerError)
				return
			}
			if adminUsername != "" && username == adminUsername {
				r = r.WithContext(setUserContext(r.Context(), 1, username))
				next(w, r)
				return
			}
		}
		http.Error(w, "forbidden", http.StatusForbidden)
	}
}

// AdminListContainers lists all containers (admin only)
func (h *Handler) AdminListContainers(w http.ResponseWriter, r *http.Request) {
	containers, err := h.db.ListAllContainers()
	if err != nil {
		slog.Error("failed to list all containers", "error", err)
		writeError(w, "failed to list containers", http.StatusInternalServerError)
		return
	}

	type containerResponse struct {
		ID         string `json:"id"`
		UserID     int64  `json:"user_id"`
		Name       string `json:"name"`
		Status     string `json:"status"`
		ExternalIP string `json:"external_ip,omitempty"`
		MemoryMB   int    `json:"memory_mb"`
		StorageGB  int    `json:"storage_gb"`
		CreatedAt  int64  `json:"created_at"`
	}

	resp := make([]containerResponse, 0, len(containers))
	for _, c := range containers {
		ip := ""
		if c.ExternalIP.Valid {
			ip = c.ExternalIP.String
		}
		resp = append(resp, containerResponse{
			ID:         c.ID,
			UserID:     c.UserID,
			Name:       c.Name,
			Status:     c.Status,
			ExternalIP: ip,
			MemoryMB:   c.MemoryMB,
			StorageGB:  c.StorageGB,
			CreatedAt:  c.CreatedAt.Unix(),
		})
	}

	writeJSON(w, resp)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// authMiddleware validates session and injects user info into context
func (h *Handler) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := auth.GetSessionToken(r)
		if token != "" {
			username, err := h.validator.ValidateSession(token)
			if err != nil {
				slog.Error("session validation failed", "error", err)
				http.Error(w, "authentication error", http.StatusInternalServerError)
				return
			}
			if username != "" {
				// For now, use username as user ID (simplified)
				// In production, would lookup user ID from username
				r = r.WithContext(setUserContext(r.Context(), 1, username))
				next(w, r)
				return
			}
		}

		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to encode json response", "error", err)
	}
}

func writeError(w http.ResponseWriter, message string, code int) {
	http.Error(w, message, code)
}

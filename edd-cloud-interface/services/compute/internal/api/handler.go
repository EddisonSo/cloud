package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"eddisonso.com/edd-cloud/services/compute/internal/auth"
	"eddisonso.com/edd-cloud/services/compute/internal/db"
	"eddisonso.com/edd-cloud/services/compute/internal/k8s"
)

type Handler struct {
	db              *db.DB
	k8s             *k8s.Client
	validator       *auth.SessionValidator
	mux             *http.ServeMux
	tokenCache      *tokenCache
	permissionStore *permissionStore
}

func NewHandler(database *db.DB, k8sClient *k8s.Client) http.Handler {
	h := &Handler{
		db:              database,
		k8s:             k8sClient,
		validator:       auth.NewSessionValidator("http://simple-file-share-backend"),
		mux:             http.NewServeMux(),
		tokenCache:      newTokenCache(),
		permissionStore: newPermissionStore(database),
	}

	// Health check (both paths for internal probes and external ingress access)
	h.mux.HandleFunc("GET /healthz", h.Healthz)
	h.mux.HandleFunc("GET /compute/healthz", h.Healthz)

	// Container endpoints (list/create use broad scope, specific routes use container ID)
	h.mux.HandleFunc("GET /compute/containers", h.authMiddleware(h.scopeCheck("containers", "read", h.ListContainers)))
	h.mux.HandleFunc("POST /compute/containers", h.authMiddleware(h.scopeCheck("containers", "create", h.CreateContainer)))
	h.mux.HandleFunc("GET /compute/containers/{id}", h.authMiddleware(h.scopeCheckContainer("read", h.GetContainer)))
	h.mux.HandleFunc("DELETE /compute/containers/{id}", h.authMiddleware(h.scopeCheckContainer("delete", h.DeleteContainer)))
	h.mux.HandleFunc("POST /compute/containers/{id}/stop", h.authMiddleware(h.scopeCheckContainer("update", h.StopContainer)))
	h.mux.HandleFunc("POST /compute/containers/{id}/start", h.authMiddleware(h.scopeCheckContainer("update", h.StartContainer)))

	// SSH key endpoints
	h.mux.HandleFunc("GET /compute/ssh-keys", h.authMiddleware(h.scopeCheck("keys", "read", h.ListSSHKeys)))
	h.mux.HandleFunc("POST /compute/ssh-keys", h.authMiddleware(h.scopeCheck("keys", "create", h.AddSSHKey)))
	h.mux.HandleFunc("DELETE /compute/ssh-keys/{id}", h.authMiddleware(h.scopeCheck("keys", "delete", h.DeleteSSHKey)))

	// WebSocket endpoint for real-time updates
	h.mux.HandleFunc("GET /compute/ws", h.authMiddleware(h.scopeCheckRoot("read", h.HandleWebSocket)))

	// Cloud terminal endpoint
	h.mux.HandleFunc("GET /compute/containers/{id}/terminal", h.authMiddleware(h.scopeCheckContainer("update", h.HandleTerminal)))

	// SSH access toggle (for gateway SSH routing)
	h.mux.HandleFunc("GET /compute/containers/{id}/ssh", h.authMiddleware(h.scopeCheckContainer("read", h.GetSSHAccess)))
	h.mux.HandleFunc("PUT /compute/containers/{id}/ssh", h.authMiddleware(h.scopeCheckContainer("update", h.UpdateSSHAccess)))

	// Ingress rules (ports 80, 443, 8000-8999)
	h.mux.HandleFunc("GET /compute/containers/{id}/ingress", h.authMiddleware(h.scopeCheckContainer("read", h.ListIngressRules)))
	h.mux.HandleFunc("POST /compute/containers/{id}/ingress", h.authMiddleware(h.scopeCheckContainer("update", h.AddIngressRule)))
	h.mux.HandleFunc("DELETE /compute/containers/{id}/ingress/{port}", h.authMiddleware(h.scopeCheckContainer("update", h.RemoveIngressRule)))

	// Persistent storage mount paths
	h.mux.HandleFunc("GET /compute/containers/{id}/mounts", h.authMiddleware(h.scopeCheckContainer("read", h.GetMountPaths)))
	h.mux.HandleFunc("PUT /compute/containers/{id}/mounts", h.authMiddleware(h.scopeCheckContainer("update", h.UpdateMountPaths)))

	// Admin endpoints
	h.mux.HandleFunc("GET /compute/admin/containers", h.adminMiddleware(h.AdminListContainers))

	return h
}

var adminUsername = os.Getenv("ADMIN_USERNAME")

// adminMiddleware validates session and checks admin status
func (h *Handler) adminMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := auth.GetSessionToken(r)
		if token != "" {
			claims, err := h.validator.ValidateSession(token)
			if err != nil {
				slog.Error("session validation failed", "error", err)
				http.Error(w, "authentication error", http.StatusInternalServerError)
				return
			}
			if claims != nil && adminUsername != "" && claims.Username == adminUsername {
				r = r.WithContext(setUserContext(r.Context(), claims.UserID, claims.Username))
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

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	type containerResponse struct {
		ID            string   `json:"id"`
		UserID        string   `json:"user_id"`
		Owner         string   `json:"owner"`
		Name          string   `json:"name"`
		Hostname      string   `json:"hostname"`
		Status        string   `json:"status"`
		ExternalIP    string   `json:"external_ip,omitempty"`
		MemoryMB      int      `json:"memory_mb"`
		MemoryUsedMB  *int64   `json:"memory_used_mb,omitempty"`
		StorageGB     int      `json:"storage_gb"`
		StorageUsedGB *float64 `json:"storage_used_gb,omitempty"`
		CreatedAt     int64    `json:"created_at"`
		SSHEnabled    bool     `json:"ssh_enabled"`
		HTTPSEnabled  bool     `json:"https_enabled"`
	}

	resp := make([]containerResponse, 0, len(containers))
	for _, c := range containers {
		ip := ""
		if c.ExternalIP.Valid {
			ip = c.ExternalIP.String
		}
		// Construct hostname from container ID
		hostname := c.ID[:8] + ".cloud.eddisonso.com"
		cr := containerResponse{
			ID:           c.ID,
			UserID:       c.UserID,
			Owner:        c.Owner,
			Name:         c.Name,
			Hostname:     hostname,
			Status:       c.Status,
			ExternalIP:   ip,
			MemoryMB:     c.MemoryMB,
			StorageGB:    c.StorageGB,
			CreatedAt:    c.CreatedAt.Unix(),
			SSHEnabled:   c.SSHEnabled,
			HTTPSEnabled: c.HTTPSEnabled,
		}
		// Fetch usage for running containers
		if c.Status == "running" {
			if usage, err := h.k8s.GetResourceUsage(ctx, c.Namespace); err == nil {
				cr.MemoryUsedMB = &usage.MemoryUsedMB
				rounded := float64(int(usage.StorageUsedGB*10)) / 10
				cr.StorageUsedGB = &rounded
			}
		}
		resp = append(resp, cr)
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

// authMiddleware validates session or API token and injects user info into context
func (h *Handler) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := auth.GetSessionToken(r)
		if token == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Check if this is an API token (ecloud_ prefix)
		if strings.HasPrefix(token, "ecloud_") {
			claims, err := h.validator.ValidateSession(token)
			if err != nil {
				slog.Error("api token validation failed", "error", err)
				http.Error(w, "invalid api token", http.StatusUnauthorized)
				return
			}
			if claims == nil || claims.Type != "api_token" {
				http.Error(w, "invalid api token", http.StatusUnauthorized)
				return
			}

			// SA tokens: look up scopes from local identity_permissions store
			if claims.ServiceAccountID != "" {
				scopes := h.permissionStore.getScopes(claims.ServiceAccountID)
				if scopes == nil {
					http.Error(w, "forbidden: service account permissions not found", http.StatusForbidden)
					return
				}
				r = r.WithContext(setAPITokenContext(r.Context(), claims.UserID, scopes))
				next(w, r)
				return
			}

			// Legacy standalone tokens: check revocation and use JWT/cached scopes
			valid, cachedScopes := h.tokenCache.checkToken(claims.TokenID)
			if !valid {
				http.Error(w, "token revoked", http.StatusUnauthorized)
				return
			}
			scopes := claims.Scopes
			if cachedScopes != nil {
				scopes = cachedScopes
			}
			r = r.WithContext(setAPITokenContext(r.Context(), claims.UserID, scopes))
			next(w, r)
			return
		}

		// Regular session JWT
		claims, err := h.validator.ValidateSession(token)
		if err != nil {
			slog.Error("session validation failed", "error", err)
			http.Error(w, "authentication error", http.StatusInternalServerError)
			return
		}
		if claims != nil {
			r = r.WithContext(setUserContext(r.Context(), claims.UserID, claims.Username))
			next(w, r)
			return
		}

		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}
}

// scopeCheck returns middleware that checks if the user has the required scope for compute.<uid>.<resource>.
func (h *Handler) scopeCheck(resource, action string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, _, ok := getUserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		scope := fmt.Sprintf("compute.%s.%s", userID, resource)
		if !requireScope(r.Context(), scope, action) {
			http.Error(w, "forbidden: insufficient token scope", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// scopeCheckContainer returns middleware that checks compute.<uid>.containers.<id> for container-specific routes.
// The walk-up in hasPermission means a broad compute.<uid>.containers token still works.
func (h *Handler) scopeCheckContainer(action string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, _, ok := getUserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		containerID := r.PathValue("id")
		scope := fmt.Sprintf("compute.%s.containers.%s", userID, containerID)
		if !requireScope(r.Context(), scope, action) {
			http.Error(w, "forbidden: insufficient token scope", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// scopeCheckRoot checks compute.<uid> level scope (e.g., for WebSocket).
func (h *Handler) scopeCheckRoot(action string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, _, ok := getUserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		scope := fmt.Sprintf("compute.%s", userID)
		if !requireScope(r.Context(), scope, action) {
			http.Error(w, "forbidden: insufficient token scope", http.StatusForbidden)
			return
		}
		next(w, r)
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

package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	"eddisonso.com/edd-cloud/pkg/events"
	"eddisonso.com/edd-cloud/services/compute/internal/db"
	"eddisonso.com/edd-cloud/services/compute/internal/k8s"
)

// Handler handles user events for the compute service
type Handler struct {
	db  *db.DB
	k8s *k8s.Client
}

// NewHandler creates a new event handler
func NewHandler(db *db.DB, k8s *k8s.Client) *Handler {
	return &Handler{db: db, k8s: k8s}
}

// OnUserCreated upserts user to user_cache
func (h *Handler) OnUserCreated(ctx context.Context, event events.UserCreated) error {
	slog.Info("user created event received", "user_id", event.UserID, "username", event.Username)
	return h.db.UpsertUserCache(event.UserID, event.Username, event.DisplayName)
}

// OnUserDeleted removes user from cache and deletes all user resources
func (h *Handler) OnUserDeleted(ctx context.Context, event events.UserDeleted) error {
	slog.Info("user deleted event received", "user_id", event.UserID, "username", event.Username)

	// Get user's containers before deleting from DB
	containers, err := h.db.ListContainersByUser(event.UserID)
	if err != nil {
		slog.Error("failed to list user containers", "error", err, "user_id", event.UserID)
		// Continue anyway to clean up what we can
	}

	// Delete K8s namespaces for all user containers
	for _, c := range containers {
		slog.Info("deleting container namespace", "container_id", c.ID, "namespace", c.Namespace)
		if err := h.k8s.DeleteNamespace(ctx, c.Namespace); err != nil {
			slog.Error("failed to delete namespace", "error", err, "namespace", c.Namespace)
			// Continue to delete other resources
		}
	}

	// Delete all user data from DB (containers, SSH keys)
	if err := h.db.DeleteUserData(event.UserID); err != nil {
		slog.Error("failed to delete user data", "error", err, "user_id", event.UserID)
		return err
	}

	// Delete from user_cache
	if err := h.db.DeleteUserCache(event.UserID); err != nil {
		slog.Error("failed to delete user from cache", "error", err, "user_id", event.UserID)
		return err
	}

	slog.Info("user resources deleted", "user_id", event.UserID, "containers_deleted", len(containers))
	return nil
}

// OnUserUpdated updates user in cache
func (h *Handler) OnUserUpdated(ctx context.Context, event events.UserUpdated) error {
	slog.Info("user updated event received", "user_id", event.UserID, "username", event.Username)
	return h.db.UpsertUserCache(event.UserID, event.Username, event.DisplayName)
}

// CachedUser represents a user from the auth service API response
type CachedUser struct {
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

// SyncUsersFromAuthService fetches all users from auth-service and populates the cache
func SyncUsersFromAuthService(db *db.DB, authServiceURL string) error {
	if authServiceURL == "" {
		slog.Warn("AUTH_SERVICE_URL not set, skipping initial user sync")
		return nil
	}

	slog.Info("syncing users from auth-service", "url", authServiceURL)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, authServiceURL+"/api/users", nil)
	if err != nil {
		return err
	}
	if key := os.Getenv("SERVICE_API_KEY"); key != "" {
		req.Header.Set("X-Service-Key", key)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("failed to fetch users from auth-service", "error", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Error("auth-service returned non-200", "status", resp.StatusCode)
		return nil // Don't fail startup, just log warning
	}

	var users []CachedUser
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		slog.Error("failed to decode users response", "error", err)
		return err
	}

	// Upsert all users to cache
	for _, u := range users {
		if err := db.UpsertUserCache(u.UserID, u.Username, u.DisplayName); err != nil {
			slog.Error("failed to upsert user during sync", "error", err, "user_id", u.UserID)
		}
	}

	slog.Info("user sync complete", "count", len(users))
	return nil
}

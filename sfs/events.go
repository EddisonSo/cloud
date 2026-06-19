package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	"eddisonso.com/edd-cloud/pkg/events"
)

// userEventHandler implements events.EventHandler for SFS
type userEventHandler struct {
	db *sql.DB
}

func newUserEventHandler(db *sql.DB) *userEventHandler {
	return &userEventHandler{db: db}
}

// OnUserCreated upserts user to user_cache
func (h *userEventHandler) OnUserCreated(ctx context.Context, event events.UserCreated) error {
	slog.Info("user created event received", "user_id", event.UserID, "username", event.Username)
	return h.upsertUserCache(event.UserID, event.Username, event.DisplayName)
}

// OnUserDeleted removes user from cache and sets namespace owner_id to NULL
func (h *userEventHandler) OnUserDeleted(ctx context.Context, event events.UserDeleted) error {
	slog.Info("user deleted event received", "user_id", event.UserID, "username", event.Username)

	// Set namespace owner_id to NULL for namespaces owned by this user
	_, err := h.db.Exec(`UPDATE namespaces SET owner_id = NULL WHERE owner_id = $1`, event.UserID)
	if err != nil {
		slog.Error("failed to update namespace owner_id", "error", err, "user_id", event.UserID)
		return err
	}

	// Delete from user_cache
	_, err = h.db.Exec(`DELETE FROM user_cache WHERE user_id = $1`, event.UserID)
	if err != nil {
		slog.Error("failed to delete user from cache", "error", err, "user_id", event.UserID)
		return err
	}

	slog.Info("user deleted from cache", "user_id", event.UserID)
	return nil
}

// OnUserUpdated updates user in cache
func (h *userEventHandler) OnUserUpdated(ctx context.Context, event events.UserUpdated) error {
	slog.Info("user updated event received", "user_id", event.UserID, "username", event.Username)
	return h.upsertUserCache(event.UserID, event.Username, event.DisplayName)
}

func (h *userEventHandler) upsertUserCache(userID, username, displayName string) error {
	_, err := h.db.Exec(`
		INSERT INTO user_cache (user_id, username, display_name, synced_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id) DO UPDATE SET
			username = EXCLUDED.username,
			display_name = EXCLUDED.display_name,
			synced_at = CURRENT_TIMESTAMP
	`, userID, username, displayName)
	if err != nil {
		slog.Error("failed to upsert user cache", "error", err, "user_id", userID)
		return err
	}
	return nil
}

// syncUsersFromAuthService fetches all users from auth-service and populates the cache
func syncUsersFromAuthService(db *sql.DB, authServiceURL string) error {
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

	var users []events.CachedUser
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		slog.Error("failed to decode users response", "error", err)
		return err
	}

	// Upsert all users to cache
	for _, u := range users {
		_, err := db.Exec(`
			INSERT INTO user_cache (user_id, username, display_name, synced_at)
			VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
			ON CONFLICT (user_id) DO UPDATE SET
				username = EXCLUDED.username,
				display_name = EXCLUDED.display_name,
				synced_at = CURRENT_TIMESTAMP
		`, u.UserID, u.Username, u.DisplayName)
		if err != nil {
			slog.Error("failed to upsert user during sync", "error", err, "user_id", u.UserID)
		}
	}

	slog.Info("user sync complete", "count", len(users))
	return nil
}

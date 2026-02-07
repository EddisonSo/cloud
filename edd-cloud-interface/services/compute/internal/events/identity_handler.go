package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"eddisonso.com/edd-cloud/services/compute/internal/db"
)

// IdentityHandler handles identity permission events for the compute service
type IdentityHandler struct {
	db *db.DB
}

// NewIdentityHandler creates a new identity event handler
func NewIdentityHandler(db *db.DB) *IdentityHandler {
	return &IdentityHandler{db: db}
}

// OnIdentityUpdated filters to compute-relevant scopes and upserts identity permissions
func (h *IdentityHandler) OnIdentityUpdated(ctx context.Context, saID, userID string, scopes map[string][]string, version int64) error {
	slog.Info("identity updated event received", "sa_id", saID, "user_id", userID, "version", version)

	// Filter scopes to only compute.* prefixed keys
	filtered := make(map[string][]string)
	for key, actions := range scopes {
		if strings.HasPrefix(key, "compute.") {
			filtered[key] = actions
		}
	}

	// If no compute scopes, delete any existing permissions
	if len(filtered) == 0 {
		return h.db.DeleteIdentityPermissions(saID)
	}

	return h.db.UpsertIdentityPermissions(saID, userID, filtered, version)
}

// OnIdentityDeleted removes identity permissions for the deleted service account
func (h *IdentityHandler) OnIdentityDeleted(ctx context.Context, saID, userID string, version int64) error {
	slog.Info("identity deleted event received", "sa_id", saID, "user_id", userID, "version", version)
	return h.db.DeleteIdentityPermissions(saID)
}

type identityPermissionResponse struct {
	ServiceAccountID string              `json:"service_account_id"`
	UserID           string              `json:"user_id"`
	Scopes           map[string][]string `json:"scopes"`
	Version          int64               `json:"version"`
}

// SyncIdentityPermissions fetches all identity permissions from auth-service and populates the local store
func SyncIdentityPermissions(database *db.DB, authServiceURL string) error {
	if authServiceURL == "" {
		slog.Warn("AUTH_SERVICE_URL not set, skipping identity permissions sync")
		return nil
	}

	slog.Info("syncing identity permissions from auth-service", "url", authServiceURL)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, authServiceURL+"/api/identity-permissions", nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("failed to fetch identity permissions from auth-service", "error", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Error("auth-service returned non-200 for identity-permissions", "status", resp.StatusCode)
		return nil
	}

	var perms []identityPermissionResponse
	if err := json.NewDecoder(resp.Body).Decode(&perms); err != nil {
		slog.Error("failed to decode identity permissions response", "error", err)
		return err
	}

	for _, p := range perms {
		// Filter to compute scopes only
		filtered := make(map[string][]string)
		for key, actions := range p.Scopes {
			if strings.HasPrefix(key, "compute.") {
				filtered[key] = actions
			}
		}
		if len(filtered) == 0 {
			continue
		}
		if err := database.UpsertIdentityPermissions(p.ServiceAccountID, p.UserID, filtered, p.Version); err != nil {
			slog.Error("failed to upsert identity permission during sync", "error", err, "sa_id", p.ServiceAccountID)
		}
	}

	slog.Info("identity permissions sync complete", "count", len(perms))
	return nil
}

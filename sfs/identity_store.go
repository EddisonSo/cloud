package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type identityPermission struct {
	ServiceAccountID string
	UserID           string
	Scopes           map[string][]string
	Version          int64
}

type identityCacheEntry struct {
	scopes    map[string][]string
	fetchedAt time.Time
}

type identityStore struct {
	mu      sync.RWMutex
	entries map[string]identityCacheEntry
	ttl     time.Duration
	db      *sql.DB
}

func newIdentityStore(db *sql.DB) *identityStore {
	return &identityStore{
		entries: make(map[string]identityCacheEntry),
		ttl:     5 * time.Minute,
		db:      db,
	}
}

func (s *identityStore) getScopes(saID string) map[string][]string {
	s.mu.RLock()
	entry, ok := s.entries[saID]
	s.mu.RUnlock()

	if ok && time.Since(entry.fetchedAt) < s.ttl {
		return entry.scopes
	}

	// Cache miss or expired — read from DB
	perm, err := s.getFromDB(saID)
	if err != nil || perm == nil {
		s.mu.Lock()
		delete(s.entries, saID)
		s.mu.Unlock()
		return nil
	}

	s.mu.Lock()
	s.entries[saID] = identityCacheEntry{scopes: perm.Scopes, fetchedAt: time.Now()}
	s.mu.Unlock()

	return perm.Scopes
}

func (s *identityStore) invalidate(saID string) {
	s.mu.Lock()
	delete(s.entries, saID)
	s.mu.Unlock()
}

func (s *identityStore) getFromDB(saID string) (*identityPermission, error) {
	ip := &identityPermission{}
	var scopesJSON []byte
	err := s.db.QueryRow(`
		SELECT service_account_id, user_id, scopes, version
		FROM identity_permissions WHERE service_account_id = $1
	`, saID).Scan(&ip.ServiceAccountID, &ip.UserID, &scopesJSON, &ip.Version)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(scopesJSON, &ip.Scopes); err != nil {
		return nil, err
	}
	return ip, nil
}

func upsertIdentityPermissions(db *sql.DB, saID, userID string, scopes map[string][]string, version int64) error {
	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		INSERT INTO identity_permissions (service_account_id, user_id, scopes, version)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (service_account_id) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			scopes = EXCLUDED.scopes,
			version = EXCLUDED.version
		WHERE identity_permissions.version < EXCLUDED.version
	`, saID, userID, scopesJSON, version)
	return err
}

func deleteIdentityPermissions(db *sql.DB, saID string) error {
	_, err := db.Exec(`DELETE FROM identity_permissions WHERE service_account_id = $1`, saID)
	return err
}

// identityEventHandler implements events.IdentityHandler for SFS
type identityEventHandler struct {
	db    *sql.DB
	store *identityStore
}

func newIdentityEventHandler(db *sql.DB, store *identityStore) *identityEventHandler {
	return &identityEventHandler{db: db, store: store}
}

func (h *identityEventHandler) OnIdentityUpdated(ctx context.Context, saID, userID string, scopes map[string][]string, version int64) error {
	slog.Info("identity updated event received", "sa_id", saID, "user_id", userID, "version", version)

	// Filter to storage.* scopes only
	filtered := make(map[string][]string)
	for key, actions := range scopes {
		if strings.HasPrefix(key, "storage.") {
			filtered[key] = actions
		}
	}

	if len(filtered) == 0 {
		h.store.invalidate(saID)
		return deleteIdentityPermissions(h.db, saID)
	}

	h.store.invalidate(saID)
	return upsertIdentityPermissions(h.db, saID, userID, filtered, version)
}

func (h *identityEventHandler) OnIdentityDeleted(ctx context.Context, saID, userID string, version int64) error {
	slog.Info("identity deleted event received", "sa_id", saID, "user_id", userID, "version", version)
	h.store.invalidate(saID)
	return deleteIdentityPermissions(h.db, saID)
}

type identityPermissionResponse struct {
	ServiceAccountID string              `json:"service_account_id"`
	UserID           string              `json:"user_id"`
	Scopes           map[string][]string `json:"scopes"`
	Version          int64               `json:"version"`
}

func syncIdentityPermissions(db *sql.DB, authServiceURL string) error {
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
	if key := os.Getenv("SERVICE_API_KEY"); key != "" {
		req.Header.Set("X-Service-Key", key)
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
		filtered := make(map[string][]string)
		for key, actions := range p.Scopes {
			if strings.HasPrefix(key, "storage.") {
				filtered[key] = actions
			}
		}
		if len(filtered) == 0 {
			continue
		}
		if err := upsertIdentityPermissions(db, p.ServiceAccountID, p.UserID, filtered, p.Version); err != nil {
			slog.Error("failed to upsert identity permission during sync", "error", err, "sa_id", p.ServiceAccountID)
		}
	}

	slog.Info("identity permissions sync complete", "count", len(perms))
	return nil
}

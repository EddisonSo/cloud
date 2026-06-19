package api

import (
	"sync"
	"time"

	"eddisonso.com/edd-cloud/services/compute/internal/db"
)

type permissionCacheEntry struct {
	scopes    map[string][]string
	fetchedAt time.Time
}

type permissionStore struct {
	mu      sync.RWMutex
	entries map[string]permissionCacheEntry
	ttl     time.Duration
	db      *db.DB
}

func newPermissionStore(database *db.DB) *permissionStore {
	return &permissionStore{
		entries: make(map[string]permissionCacheEntry),
		ttl:     5 * time.Minute,
		db:      database,
	}
}

// getScopes returns the scopes for a service account, using in-memory cache with DB fallback.
func (ps *permissionStore) getScopes(saID string) map[string][]string {
	ps.mu.RLock()
	entry, ok := ps.entries[saID]
	ps.mu.RUnlock()

	if ok && time.Since(entry.fetchedAt) < ps.ttl {
		return entry.scopes
	}

	// Cache miss or expired — read from DB
	perm, err := ps.db.GetIdentityPermissions(saID)
	if err != nil || perm == nil {
		// Remove stale cache entry if DB has no record
		ps.mu.Lock()
		delete(ps.entries, saID)
		ps.mu.Unlock()
		return nil
	}

	ps.mu.Lock()
	ps.entries[saID] = permissionCacheEntry{scopes: perm.Scopes, fetchedAt: time.Now()}
	ps.mu.Unlock()

	return perm.Scopes
}

// invalidate removes a cached entry so the next lookup reads from DB.
func (ps *permissionStore) invalidate(saID string) {
	ps.mu.Lock()
	delete(ps.entries, saID)
	ps.mu.Unlock()
}

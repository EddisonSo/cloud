package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

type tokenCacheEntry struct {
	valid     bool
	scopes    map[string][]string // non-nil for service account tokens
	checkedAt time.Time
}

type tokenCache struct {
	mu      sync.RWMutex
	entries map[string]tokenCacheEntry
	ttl     time.Duration
	authURL string
}

func newTokenCache() *tokenCache {
	authURL := os.Getenv("AUTH_SERVICE_URL")
	if authURL == "" {
		authURL = "http://edd-cloud-auth:8080"
	}
	return &tokenCache{
		entries: make(map[string]tokenCacheEntry),
		ttl:     5 * time.Minute,
		authURL: authURL,
	}
}

// checkToken checks if the token is valid and returns cached scopes (if any).
// Results are cached for 5 minutes.
func (c *tokenCache) checkToken(tokenID string) (bool, map[string][]string) {
	c.mu.RLock()
	entry, ok := c.entries[tokenID]
	c.mu.RUnlock()

	if ok && time.Since(entry.checkedAt) < c.ttl {
		return entry.valid, entry.scopes
	}

	// Cache miss or expired — check with auth service
	valid, scopes := c.checkWithAuthService(tokenID)

	c.mu.Lock()
	c.entries[tokenID] = tokenCacheEntry{valid: valid, scopes: scopes, checkedAt: time.Now()}
	c.mu.Unlock()

	return valid, scopes
}

func (c *tokenCache) checkWithAuthService(tokenID string) (bool, map[string][]string) {
	url := fmt.Sprintf("%s/api/tokens/%s/check", c.authURL, tokenID)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		// On error, assume valid to avoid blocking legitimate requests
		return true, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	var body struct {
		Status string              `json:"status"`
		Scopes map[string][]string `json:"scopes,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return true, nil
	}

	return true, body.Scopes
}

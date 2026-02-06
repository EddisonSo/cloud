package api

import (
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

type tokenCacheEntry struct {
	valid     bool
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

// isValid checks if the token with the given ID has not been revoked.
// Results are cached for 5 minutes.
func (c *tokenCache) isValid(tokenID string) bool {
	c.mu.RLock()
	entry, ok := c.entries[tokenID]
	c.mu.RUnlock()

	if ok && time.Since(entry.checkedAt) < c.ttl {
		return entry.valid
	}

	// Cache miss or expired — check with auth service
	valid := c.checkWithAuthService(tokenID)

	c.mu.Lock()
	c.entries[tokenID] = tokenCacheEntry{valid: valid, checkedAt: time.Now()}
	c.mu.Unlock()

	return valid
}

func (c *tokenCache) checkWithAuthService(tokenID string) bool {
	url := fmt.Sprintf("%s/api/tokens/%s/check", c.authURL, tokenID)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		// On error, assume valid to avoid blocking legitimate requests
		return true
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// hasPermission checks if the granted scopes contain the action for the given scope.
// It walks up the dot-separated path checking for the action at each level,
// stopping before bare roots (compute, storage).
func hasPermission(granted map[string][]string, scope, action string) bool {
	current := scope
	for {
		if actions, ok := granted[current]; ok {
			for _, a := range actions {
				if a == action {
					return true
				}
			}
		}

		lastDot := strings.LastIndex(current, ".")
		if lastDot == -1 {
			break
		}
		parent := current[:lastDot]
		if !strings.Contains(parent, ".") {
			break
		}
		current = parent
	}
	return false
}

// tokenCacheEntry stores the result of a revocation check
type tokenCacheEntry struct {
	valid     bool
	scopes    map[string][]string // non-nil for service account tokens
	checkedAt time.Time
}

// tokenCache caches revocation checks with a 5-minute TTL
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
func (c *tokenCache) checkToken(tokenID string) (bool, map[string][]string) {
	c.mu.RLock()
	entry, ok := c.entries[tokenID]
	c.mu.RUnlock()

	if ok && time.Since(entry.checkedAt) < c.ttl {
		return entry.valid, entry.scopes
	}

	valid, scopes := c.checkWithAuthService(tokenID)

	c.mu.Lock()
	c.entries[tokenID] = tokenCacheEntry{valid: valid, scopes: scopes, checkedAt: time.Now()}
	c.mu.Unlock()

	return valid, scopes
}

func (c *tokenCache) checkWithAuthService(tokenID string) (bool, map[string][]string) {
	checkURL := fmt.Sprintf("%s/api/tokens/%s/check", c.authURL, tokenID)
	req, err := http.NewRequest(http.MethodGet, checkURL, nil)
	if err != nil {
		return true, nil // fail open on request creation errors
	}
	if key := os.Getenv("SERVICE_API_KEY"); key != "" {
		req.Header.Set("X-Service-Key", key)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return true, nil // fail open on network errors
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

// requireAuthWithScope wraps requireAuth to also handle API token auth and scope checking.
// For session tokens, returns (userID, true).
// For API tokens, verifies revocation and scope, returns (userID, true) if allowed.
// Optional resourceID narrows the scope to storage.<uid>.<resource>.<resourceID>.
func (s *server) requireAuthWithScope(w http.ResponseWriter, r *http.Request, resource, action string, resourceID ...string) (string, bool) {
	token := s.sessionToken(r)
	if token == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", false
	}

	claims, ok := s.parseJWT(token)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", false
	}

	// Session token — full access
	if claims.Type != "api_token" {
		return claims.UserID, true
	}

	// SA tokens: look up scopes from local identity_permissions store
	if claims.ServiceAccountID != "" {
		scopes := s.idStore.getScopes(claims.ServiceAccountID)
		if scopes == nil {
			http.Error(w, "forbidden: service account permissions not found", http.StatusForbidden)
			return "", false
		}
		scope := fmt.Sprintf("storage.%s.%s", claims.UserID, resource)
		if len(resourceID) > 0 && resourceID[0] != "" {
			scope = fmt.Sprintf("%s.%s", scope, resourceID[0])
		}
		if !hasPermission(scopes, scope, action) {
			http.Error(w, "forbidden: insufficient token scope", http.StatusForbidden)
			return "", false
		}
		return claims.UserID, true
	}

	// Legacy standalone tokens: check revocation and use JWT/cached scopes
	valid, cachedScopes := s.tkCache.checkToken(claims.TokenID)
	if !valid {
		http.Error(w, "token revoked", http.StatusUnauthorized)
		return "", false
	}

	scopes := claims.Scopes
	if cachedScopes != nil {
		scopes = cachedScopes
	}

	// Check scope — append resource ID if provided
	scope := fmt.Sprintf("storage.%s.%s", claims.UserID, resource)
	if len(resourceID) > 0 && resourceID[0] != "" {
		scope = fmt.Sprintf("%s.%s", scope, resourceID[0])
	}
	if !hasPermission(scopes, scope, action) {
		http.Error(w, "forbidden: insufficient token scope", http.StatusForbidden)
		return "", false
	}

	return claims.UserID, true
}

// isAPIToken checks if the request carries an API token (ecloud_ prefix).
func (s *server) isAPIToken(r *http.Request) bool {
	token := s.sessionToken(r)
	return strings.HasPrefix(token, "ecloud_")
}

// currentUserOrToken returns the user ID, handling both session and API tokens.
// Unlike requireAuthWithScope, this does not check scopes (used for namespace access checks).
func (s *server) currentUserOrToken(r *http.Request) (string, bool) {
	token := s.sessionToken(r)
	if token == "" {
		return "", false
	}
	claims, ok := s.parseJWT(token)
	if !ok {
		return "", false
	}
	if claims.Type == "api_token" {
		// SA tokens: verify permissions exist in local store
		if claims.ServiceAccountID != "" {
			scopes := s.idStore.getScopes(claims.ServiceAccountID)
			if scopes == nil {
				return "", false
			}
			return claims.UserID, true
		}
		// Legacy standalone tokens: check revocation via auth service
		valid, _ := s.tkCache.checkToken(claims.TokenID)
		if !valid {
			return "", false
		}
	}
	return claims.UserID, true
}

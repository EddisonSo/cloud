package proxy

import (
	"net/http"
	"sync"
	"time"
)

type cachedResponse struct {
	statusCode int
	header     http.Header
	body       []byte
	size       int
	createdAt  time.Time
}

type responseCache struct {
	mu      sync.RWMutex
	entries map[string]*cachedResponse
	maxSize int           // total byte cap across all entries
	maxAge  time.Duration // per-entry TTL
	curSize int
}

const maxEntrySize = 1 << 20 // 1 MB per entry

func newResponseCache(maxSize int, maxAge time.Duration) *responseCache {
	rc := &responseCache{
		entries: make(map[string]*cachedResponse),
		maxSize: maxSize,
		maxAge:  maxAge,
	}
	go rc.startCleanup()
	return rc
}

// Get returns a cached response if it exists and hasn't expired, nil otherwise.
func (rc *responseCache) Get(key string) *cachedResponse {
	rc.mu.RLock()
	entry, ok := rc.entries[key]
	rc.mu.RUnlock()
	if !ok {
		return nil
	}
	if time.Since(entry.createdAt) > rc.maxAge {
		// Expired — remove lazily
		rc.mu.Lock()
		if e, ok := rc.entries[key]; ok && e == entry {
			rc.curSize -= e.size
			delete(rc.entries, key)
		}
		rc.mu.Unlock()
		return nil
	}
	return entry
}

// Set stores a response in the cache if it fits within size limits.
// Only caches status 200 with body <= maxEntrySize and no Set-Cookie header.
func (rc *responseCache) Set(key string, resp *http.Response, body []byte) {
	if resp.StatusCode != 200 {
		return
	}
	if len(body) > maxEntrySize {
		return
	}
	if resp.Header.Get("Set-Cookie") != "" {
		return
	}
	if resp.Header.Get("Cache-Control") == "no-store" {
		return
	}

	entrySize := len(body)

	rc.mu.Lock()
	defer rc.mu.Unlock()

	// Remove existing entry for this key if present
	if old, ok := rc.entries[key]; ok {
		rc.curSize -= old.size
		delete(rc.entries, key)
	}

	// Skip if adding this entry would exceed total capacity
	if rc.curSize+entrySize > rc.maxSize {
		return
	}

	// Clone headers to avoid retaining the response
	hdr := make(http.Header, len(resp.Header))
	for k, v := range resp.Header {
		hdr[k] = append([]string(nil), v...)
	}

	rc.entries[key] = &cachedResponse{
		statusCode: resp.StatusCode,
		header:     hdr,
		body:       body,
		size:       entrySize,
		createdAt:  time.Now(),
	}
	rc.curSize += entrySize
}

// Delete removes a cache entry by key, freeing its space.
func (rc *responseCache) Delete(key string) {
	rc.mu.Lock()
	if entry, ok := rc.entries[key]; ok {
		rc.curSize -= entry.size
		delete(rc.entries, key)
	}
	rc.mu.Unlock()
}

// startCleanup runs a background goroutine that removes expired entries every 10s.
func (rc *responseCache) startCleanup() {
	ticker := time.NewTicker(10 * time.Second)
	for range ticker.C {
		now := time.Now()
		rc.mu.Lock()
		for key, entry := range rc.entries {
			if now.Sub(entry.createdAt) > rc.maxAge {
				rc.curSize -= entry.size
				delete(rc.entries, key)
			}
		}
		rc.mu.Unlock()
	}
}

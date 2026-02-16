package proxy

import (
	"container/list"
	"net/http"
	"sync"
	"time"
)

type cachedResponse struct {
	key        string
	statusCode int
	header     http.Header
	body       []byte
	size       int
	createdAt  time.Time
}

type responseCache struct {
	mu      sync.Mutex
	items   map[string]*list.Element // key → list element
	order   *list.List               // front = most recent, back = LRU
	maxSize int                      // total byte cap
	maxAge  time.Duration            // per-entry TTL
	curSize int
}

const maxEntrySize = 1 << 20 // 1 MB per entry

func newResponseCache(maxSize int, maxAge time.Duration) *responseCache {
	rc := &responseCache{
		items:   make(map[string]*list.Element),
		order:   list.New(),
		maxSize: maxSize,
		maxAge:  maxAge,
	}
	go rc.startCleanup()
	return rc
}

// Get returns a cached response if it exists and hasn't expired, nil otherwise.
// Moves the entry to the front (most recently used).
func (rc *responseCache) Get(key string) *cachedResponse {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	elem, ok := rc.items[key]
	if !ok {
		return nil
	}
	entry := elem.Value.(*cachedResponse)
	if time.Since(entry.createdAt) > rc.maxAge {
		rc.removeLocked(elem)
		return nil
	}
	rc.order.MoveToFront(elem)
	return entry
}

// Set stores a response in the cache. Evicts LRU entries if over capacity.
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

	// Clone headers to avoid retaining the response
	hdr := make(http.Header, len(resp.Header))
	for k, v := range resp.Header {
		hdr[k] = append([]string(nil), v...)
	}

	rc.mu.Lock()
	defer rc.mu.Unlock()

	// Remove existing entry for this key if present
	if elem, ok := rc.items[key]; ok {
		rc.removeLocked(elem)
	}

	// Evict LRU entries until there's room
	for rc.curSize+entrySize > rc.maxSize && rc.order.Len() > 0 {
		rc.removeLocked(rc.order.Back())
	}

	entry := &cachedResponse{
		key:        key,
		statusCode: resp.StatusCode,
		header:     hdr,
		body:       body,
		size:       entrySize,
		createdAt:  time.Now(),
	}
	rc.items[key] = rc.order.PushFront(entry)
	rc.curSize += entrySize
}

// Delete removes a cache entry by key, freeing its space.
func (rc *responseCache) Delete(key string) {
	rc.mu.Lock()
	if elem, ok := rc.items[key]; ok {
		rc.removeLocked(elem)
	}
	rc.mu.Unlock()
}

// removeLocked removes an element from both the list and map. Caller must hold mu.
func (rc *responseCache) removeLocked(elem *list.Element) {
	entry := rc.order.Remove(elem).(*cachedResponse)
	delete(rc.items, entry.key)
	rc.curSize -= entry.size
}

// startCleanup removes expired entries every 5 minutes.
func (rc *responseCache) startCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		now := time.Now()
		rc.mu.Lock()
		// Walk from back (LRU) — expired entries are likely older
		for elem := rc.order.Back(); elem != nil; {
			entry := elem.Value.(*cachedResponse)
			prev := elem.Prev()
			if now.Sub(entry.createdAt) > rc.maxAge {
				rc.removeLocked(elem)
			}
			elem = prev
		}
		rc.mu.Unlock()
	}
}

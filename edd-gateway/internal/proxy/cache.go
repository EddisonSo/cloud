package proxy

import (
	"container/list"
	"hash/fnv"
	"net/http"
	"sync"
	"time"
)

const (
	maxEntrySize = 1 << 20 // 1 MB per entry
	numShards    = 16
)

type cachedResponse struct {
	key        string
	statusCode int
	header     http.Header
	body       []byte
	size       int
	createdAt  time.Time
}

// cacheShard is a single LRU partition with its own lock.
type cacheShard struct {
	mu      sync.Mutex
	items   map[string]*list.Element
	order   *list.List // front = most recently used, back = least recently used
	maxSize int
	maxAge  time.Duration
	curSize int
}

// responseCache shards entries across independent LRU caches,
// reducing lock contention at high concurrency.
type responseCache struct {
	shards [numShards]*cacheShard
}

func newResponseCache(maxSize int, maxAge time.Duration) *responseCache {
	rc := &responseCache{}
	perShard := maxSize / numShards
	for i := range rc.shards {
		s := &cacheShard{
			items:   make(map[string]*list.Element),
			order:   list.New(),
			maxSize: perShard,
			maxAge:  maxAge,
		}
		rc.shards[i] = s
		go s.startCleanup()
	}
	return rc
}

func (rc *responseCache) shard(key string) *cacheShard {
	h := fnv.New32a()
	h.Write([]byte(key))
	return rc.shards[h.Sum32()%numShards]
}

// Get returns a cached response, promoting it to the front (true LRU).
func (rc *responseCache) Get(key string) *cachedResponse {
	s := rc.shard(key)
	s.mu.Lock()
	elem, ok := s.items[key]
	if !ok {
		s.mu.Unlock()
		return nil
	}
	entry := elem.Value.(*cachedResponse)
	if time.Since(entry.createdAt) > s.maxAge {
		s.removeLocked(elem)
		s.mu.Unlock()
		return nil
	}
	s.order.MoveToFront(elem)
	s.mu.Unlock()
	return entry
}

// Set stores a response. Evicts LRU entries if over capacity.
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

	s := rc.shard(key)
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove existing entry for this key if present
	if elem, ok := s.items[key]; ok {
		s.removeLocked(elem)
	}

	// Evict least recently used entries until there's room
	for s.curSize+entrySize > s.maxSize && s.order.Len() > 0 {
		s.removeLocked(s.order.Back())
	}

	entry := &cachedResponse{
		key:        key,
		statusCode: resp.StatusCode,
		header:     hdr,
		body:       body,
		size:       entrySize,
		createdAt:  time.Now(),
	}
	s.items[key] = s.order.PushFront(entry)
	s.curSize += entrySize
}

// Delete removes a cache entry by key.
func (rc *responseCache) Delete(key string) {
	s := rc.shard(key)
	s.mu.Lock()
	if elem, ok := s.items[key]; ok {
		s.removeLocked(elem)
	}
	s.mu.Unlock()
}

// removeLocked removes an element from both the list and map. Caller must hold shard lock.
func (s *cacheShard) removeLocked(elem *list.Element) {
	entry := s.order.Remove(elem).(*cachedResponse)
	delete(s.items, entry.key)
	s.curSize -= entry.size
}

// startCleanup removes expired entries every 5 minutes.
func (s *cacheShard) startCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		now := time.Now()
		s.mu.Lock()
		for elem := s.order.Back(); elem != nil; {
			entry := elem.Value.(*cachedResponse)
			prev := elem.Prev()
			if now.Sub(entry.createdAt) > s.maxAge {
				s.removeLocked(elem)
			}
			elem = prev
		}
		s.mu.Unlock()
	}
}

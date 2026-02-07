package api

import (
	"sync"
	"time"
)

// rateLimiter implements a sliding window rate limiter keyed by string.
// A background goroutine periodically sweeps stale keys to prevent unbounded growth.
type rateLimiter struct {
	mu      sync.Mutex
	windows map[string][]time.Time
	limit   int
	window  time.Duration
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{
		windows: make(map[string][]time.Time),
		limit:   limit,
		window:  window,
	}
	go rl.cleanup()
	return rl
}

// allow returns true if the key has not exceeded the rate limit.
func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Filter out expired entries
	entries := rl.windows[key]
	valid := entries[:0]
	for _, t := range entries {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.limit {
		rl.windows[key] = valid
		return false
	}

	rl.windows[key] = append(valid, now)
	return true
}

// cleanup periodically removes keys with no valid entries.
func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-rl.window)
		for key, entries := range rl.windows {
			valid := false
			for _, t := range entries {
				if t.After(cutoff) {
					valid = true
					break
				}
			}
			if !valid {
				delete(rl.windows, key)
			}
		}
		rl.mu.Unlock()
	}
}

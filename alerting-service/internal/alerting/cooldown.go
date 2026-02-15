package alerting

import (
	"sync"
	"time"
)

// CooldownTracker suppresses duplicate alerts within a cooldown window.
type CooldownTracker struct {
	mu        sync.Mutex
	lastFired map[string]time.Time
}

func NewCooldownTracker() *CooldownTracker {
	return &CooldownTracker{lastFired: make(map[string]time.Time)}
}

// Allow returns true if the alert key has not fired within the cooldown duration.
// If allowed, it records the current time as the last fire time.
func (c *CooldownTracker) Allow(key string, cooldown time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if last, ok := c.lastFired[key]; ok && time.Since(last) < cooldown {
		return false
	}
	c.lastFired[key] = time.Now()
	return true
}

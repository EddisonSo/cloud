package alerting

import (
	"testing"
	"time"
)

func TestCooldown_FirstFireAllowed(t *testing.T) {
	c := NewCooldownTracker()
	if !c.Allow("cpu:s0", 5*time.Minute) {
		t.Fatal("first fire should be allowed")
	}
}

func TestCooldown_SecondFireBlocked(t *testing.T) {
	c := NewCooldownTracker()
	c.Allow("cpu:s0", 5*time.Minute)
	if c.Allow("cpu:s0", 5*time.Minute) {
		t.Fatal("second immediate fire should be blocked")
	}
}

func TestCooldown_DifferentKeysIndependent(t *testing.T) {
	c := NewCooldownTracker()
	c.Allow("cpu:s0", 5*time.Minute)
	if !c.Allow("cpu:s1", 5*time.Minute) {
		t.Fatal("different key should be allowed")
	}
}

func TestCooldown_AllowsAfterExpiry(t *testing.T) {
	c := NewCooldownTracker()
	c.Allow("cpu:s0", 50*time.Millisecond)
	time.Sleep(60 * time.Millisecond)
	if !c.Allow("cpu:s0", 50*time.Millisecond) {
		t.Fatal("should allow after cooldown expires")
	}
}

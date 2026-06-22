package server

import (
	"testing"
	"time"
)

func TestExpiredLogPath(t *testing.T) {
	now := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		path string
		want bool
	}{
		{"/2026-06-01/edd-auth.jsonl", true},  // 20 days old > 14
		{"/2026-06-07/edd-auth.jsonl", false}, // exactly 14 days -> not strictly older
		{"/2026-06-08/edd-auth.jsonl", false}, // 13 days, keep
		{"/2026-06-21/edd-auth.jsonl", false}, // today, keep
		{"/garbage/x.jsonl", false},           // malformed, never delete
		{"not-a-path", false},
	}
	for _, c := range cases {
		if got := expiredLogPath(c.path, now, 14); got != c.want {
			t.Errorf("expiredLogPath(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

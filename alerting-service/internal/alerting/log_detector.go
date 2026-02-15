package alerting

import (
	"fmt"
	"sync"
	"time"
)

// LogEntry represents a single error log from any service.
type LogEntry struct {
	Source  string
	Message string
	Level   string
}

// LogDetectorConfig configures burst detection thresholds.
type LogDetectorConfig struct {
	BurstThreshold int
	BurstWindow    time.Duration
	DefaultCooldown time.Duration
}

// LogDetector detects bursts of errors from a single source.
type LogDetector struct {
	config   LogDetectorConfig
	cooldown *CooldownTracker
	onAlert  AlertFunc

	mu      sync.Mutex
	windows map[string][]time.Time // source → timestamps of recent errors
}

// NewLogDetector creates a LogDetector with the given config and alert callback.
func NewLogDetector(config LogDetectorConfig, onAlert AlertFunc) *LogDetector {
	return &LogDetector{
		config:   config,
		cooldown: NewCooldownTracker(),
		onAlert:  onAlert,
		windows:  make(map[string][]time.Time),
	}
}

// HandleLogEntry processes a single log entry, firing an alert if a burst is detected.
func (d *LogDetector) HandleLogEntry(entry LogEntry) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-d.config.BurstWindow)

	// Append and prune old entries
	times := d.windows[entry.Source]
	pruned := make([]time.Time, 0, len(times)+1)
	for _, t := range times {
		if t.After(cutoff) {
			pruned = append(pruned, t)
		}
	}
	pruned = append(pruned, now)
	d.windows[entry.Source] = pruned

	if len(pruned) >= d.config.BurstThreshold {
		key := fmt.Sprintf("log-burst:%s", entry.Source)
		if d.cooldown.Allow(key, d.config.DefaultCooldown) {
			d.onAlert(Alert{
				Title:    fmt.Sprintf("Error Burst: %s", entry.Source),
				Message:  fmt.Sprintf("Source %s produced %d errors in %s. Latest: %s", entry.Source, len(pruned), d.config.BurstWindow, entry.Message),
				Severity: SeverityWarning,
			})
		}
	}
}

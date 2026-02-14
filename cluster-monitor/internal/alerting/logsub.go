package alerting

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

var criticalKeywords = []string{"panic", "fatal", "crash"}

type LogDetectorConfig struct {
	BurstThreshold int
	BurstWindow    time.Duration
	Cooldown       time.Duration
}

type LogDetector struct {
	config   LogDetectorConfig
	cooldown *CooldownTracker
	onAlert  AlertFunc

	mu         sync.Mutex
	errorTimes map[string][]time.Time // source → timestamps of recent errors
}

func NewLogDetector(config LogDetectorConfig, onAlert AlertFunc) *LogDetector {
	return &LogDetector{
		config:     config,
		cooldown:   NewCooldownTracker(),
		onAlert:    onAlert,
		errorTimes: make(map[string][]time.Time),
	}
}

func (d *LogDetector) HandleLogEntry(source, message string) {
	lower := strings.ToLower(message)

	// Check critical keywords
	for _, kw := range criticalKeywords {
		if strings.Contains(lower, kw) {
			key := fmt.Sprintf("critical:%s", source)
			if d.cooldown.Allow(key, d.config.Cooldown) {
				d.onAlert(Alert{
					Title:    fmt.Sprintf("Critical Error: %s", source),
					Message:  fmt.Sprintf("Source %s logged critical error: %s", source, truncate(message, 200)),
					Severity: SeverityCritical,
				})
			}
			return
		}
	}

	// Track error burst
	d.mu.Lock()
	now := time.Now()
	cutoff := now.Add(-d.config.BurstWindow)

	times := d.errorTimes[source]
	pruned := make([]time.Time, 0, len(times)+1)
	for _, t := range times {
		if t.After(cutoff) {
			pruned = append(pruned, t)
		}
	}
	pruned = append(pruned, now)
	d.errorTimes[source] = pruned
	count := len(pruned)
	d.mu.Unlock()

	if count > d.config.BurstThreshold {
		key := fmt.Sprintf("error-burst:%s", source)
		if d.cooldown.Allow(key, d.config.Cooldown) {
			d.onAlert(Alert{
				Title:    fmt.Sprintf("Error Burst: %s", source),
				Message:  fmt.Sprintf("Source %s logged %d errors in %s", source, count, d.config.BurstWindow),
				Severity: SeverityWarning,
			})
		}
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

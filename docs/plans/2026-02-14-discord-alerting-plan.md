# Discord Alerting Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Discord webhook alerting to cluster-monitor for infrastructure health alerts and log error spike detection.

**Architecture:** All alerting logic lives inside the existing cluster-monitor binary. Two sources (metrics loop + log-service WebSocket) feed into a shared Discord webhook sender with per-alert-type cooldowns.

**Tech Stack:** Go, Discord Webhook API, gorilla/websocket (already a dependency), Kubernetes Secrets

---

### Task 1: Cooldown Tracker

**Files:**
- Create: `cluster-monitor/internal/alerting/cooldown.go`
- Create: `cluster-monitor/internal/alerting/cooldown_test.go`

**Step 1: Write the failing test**

```go
// cluster-monitor/internal/alerting/cooldown_test.go
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
```

**Step 2: Run test to verify it fails**

Run: `cd /home/eddison/cloud/cluster-monitor && go test ./internal/alerting/ -run TestCooldown -v`
Expected: FAIL — package does not exist

**Step 3: Write minimal implementation**

```go
// cluster-monitor/internal/alerting/cooldown.go
package alerting

import (
	"sync"
	"time"
)

// CooldownTracker suppresses duplicate alerts within a cooldown window.
type CooldownTracker struct {
	mu       sync.Mutex
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
```

**Step 4: Run test to verify it passes**

Run: `cd /home/eddison/cloud/cluster-monitor && go test ./internal/alerting/ -run TestCooldown -v`
Expected: PASS (all 4 tests)

**Step 5: Commit**

Commit message: `feat(cluster-monitor): add cooldown tracker for alert deduplication`

---

### Task 2: Discord Webhook Sender

**Files:**
- Create: `cluster-monitor/internal/alerting/discord.go`
- Create: `cluster-monitor/internal/alerting/discord_test.go`

**Step 1: Write the failing test**

```go
// cluster-monitor/internal/alerting/discord_test.go
package alerting

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscord_SendAlert(t *testing.T) {
	var received discordWebhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	d := NewDiscordSender(server.URL)
	err := d.Send(Alert{
		Title:    "High CPU",
		Message:  "Node s0 CPU at 95%",
		Severity: SeverityCritical,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(received.Embeds) != 1 {
		t.Fatalf("expected 1 embed, got %d", len(received.Embeds))
	}
	if received.Embeds[0].Title != "High CPU" {
		t.Fatalf("expected title 'High CPU', got '%s'", received.Embeds[0].Title)
	}
	if received.Embeds[0].Color != 0xFF0000 {
		t.Fatalf("expected red color for critical, got %d", received.Embeds[0].Color)
	}
}

func TestDiscord_SeverityColors(t *testing.T) {
	tests := []struct {
		severity Severity
		color    int
	}{
		{SeverityCritical, 0xFF0000},
		{SeverityWarning, 0xFFA500},
	}
	for _, tt := range tests {
		var received discordWebhookPayload
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewDecoder(r.Body).Decode(&received)
			w.WriteHeader(http.StatusNoContent)
		}))

		d := NewDiscordSender(server.URL)
		d.Send(Alert{Title: "test", Message: "msg", Severity: tt.severity})
		server.Close()

		if received.Embeds[0].Color != tt.color {
			t.Fatalf("severity %d: expected color %d, got %d", tt.severity, tt.color, received.Embeds[0].Color)
		}
	}
}

func TestDiscord_EmptyURL_Noop(t *testing.T) {
	d := NewDiscordSender("")
	err := d.Send(Alert{Title: "test", Message: "msg", Severity: SeverityCritical})
	if err != nil {
		t.Fatal("empty URL should be a no-op, not an error")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/eddison/cloud/cluster-monitor && go test ./internal/alerting/ -run TestDiscord -v`
Expected: FAIL — types not defined

**Step 3: Write minimal implementation**

```go
// cluster-monitor/internal/alerting/discord.go
package alerting

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Severity int

const (
	SeverityWarning  Severity = iota
	SeverityCritical
)

type Alert struct {
	Title    string
	Message  string
	Severity Severity
}

type DiscordSender struct {
	webhookURL string
	client     *http.Client
}

func NewDiscordSender(webhookURL string) *DiscordSender {
	return &DiscordSender{
		webhookURL: webhookURL,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

type discordEmbed struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Color       int    `json:"color"`
	Timestamp   string `json:"timestamp"`
}

type discordWebhookPayload struct {
	Embeds []discordEmbed `json:"embeds"`
}

func (d *DiscordSender) Send(alert Alert) error {
	if d.webhookURL == "" {
		return nil
	}

	color := 0xFFA500 // orange/warning
	if alert.Severity == SeverityCritical {
		color = 0xFF0000 // red
	}

	payload := discordWebhookPayload{
		Embeds: []discordEmbed{{
			Title:       alert.Title,
			Description: alert.Message,
			Color:       color,
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
		}},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal discord payload: %w", err)
	}

	resp, err := d.client.Post(d.webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("discord webhook POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("discord webhook returned %d", resp.StatusCode)
	}
	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/eddison/cloud/cluster-monitor && go test ./internal/alerting/ -run TestDiscord -v`
Expected: PASS (all 3 tests)

**Step 5: Commit**

Commit message: `feat(cluster-monitor): add Discord webhook sender with severity-based embeds`

---

### Task 3: Metrics Evaluator

The evaluator checks `ClusterInfo` snapshots against thresholds and fires alerts. It also tracks pod restart counts and OOMKilled events from the Kubernetes API.

**Files:**
- Create: `cluster-monitor/internal/alerting/evaluator.go`
- Create: `cluster-monitor/internal/alerting/evaluator_test.go`

**Step 1: Write the failing test**

```go
// cluster-monitor/internal/alerting/evaluator_test.go
package alerting

import (
	"testing"
	"time"
)

func TestEvaluator_HighCPU_SingleCheck_NoAlert(t *testing.T) {
	var fired []Alert
	e := NewEvaluator(EvaluatorConfig{
		CPUThreshold:    90,
		MemThreshold:    85,
		DiskThreshold:   90,
		DefaultCooldown: 5 * time.Minute,
	}, func(a Alert) { fired = append(fired, a) })

	e.EvaluateCluster(ClusterSnapshot{
		Nodes: []NodeSnapshot{{Name: "s0", CPUPercent: 95, MemPercent: 50, DiskPercent: 30}},
	})

	if len(fired) != 0 {
		t.Fatal("should not fire on single high CPU check (needs 2 consecutive)")
	}
}

func TestEvaluator_HighCPU_TwoConsecutive_Fires(t *testing.T) {
	var fired []Alert
	e := NewEvaluator(EvaluatorConfig{
		CPUThreshold:    90,
		MemThreshold:    85,
		DiskThreshold:   90,
		DefaultCooldown: 5 * time.Minute,
	}, func(a Alert) { fired = append(fired, a) })

	e.EvaluateCluster(ClusterSnapshot{
		Nodes: []NodeSnapshot{{Name: "s0", CPUPercent: 95, MemPercent: 50, DiskPercent: 30}},
	})
	e.EvaluateCluster(ClusterSnapshot{
		Nodes: []NodeSnapshot{{Name: "s0", CPUPercent: 92, MemPercent: 50, DiskPercent: 30}},
	})

	if len(fired) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(fired))
	}
	if fired[0].Severity != SeverityCritical {
		t.Fatal("CPU alert should be critical")
	}
}

func TestEvaluator_HighMemory_Fires(t *testing.T) {
	var fired []Alert
	e := NewEvaluator(EvaluatorConfig{
		CPUThreshold:    90,
		MemThreshold:    85,
		DiskThreshold:   90,
		DefaultCooldown: 5 * time.Minute,
	}, func(a Alert) { fired = append(fired, a) })

	e.EvaluateCluster(ClusterSnapshot{
		Nodes: []NodeSnapshot{{Name: "s0", CPUPercent: 10, MemPercent: 88, DiskPercent: 30}},
	})

	if len(fired) != 1 {
		t.Fatalf("expected 1 alert for high memory, got %d", len(fired))
	}
}

func TestEvaluator_HighDisk_Fires(t *testing.T) {
	var fired []Alert
	e := NewEvaluator(EvaluatorConfig{
		CPUThreshold:    90,
		MemThreshold:    85,
		DiskThreshold:   90,
		DefaultCooldown: 5 * time.Minute,
	}, func(a Alert) { fired = append(fired, a) })

	e.EvaluateCluster(ClusterSnapshot{
		Nodes: []NodeSnapshot{{Name: "s0", CPUPercent: 10, MemPercent: 50, DiskPercent: 95}},
	})

	if len(fired) != 1 {
		t.Fatalf("expected 1 alert for high disk, got %d", len(fired))
	}
}

func TestEvaluator_NodeCondition_Fires(t *testing.T) {
	var fired []Alert
	e := NewEvaluator(EvaluatorConfig{
		CPUThreshold:    90,
		MemThreshold:    85,
		DiskThreshold:   90,
		DefaultCooldown: 5 * time.Minute,
	}, func(a Alert) { fired = append(fired, a) })

	e.EvaluateCluster(ClusterSnapshot{
		Nodes: []NodeSnapshot{{
			Name: "s0", CPUPercent: 10, MemPercent: 50, DiskPercent: 30,
			Conditions: []string{"MemoryPressure"},
		}},
	})

	if len(fired) != 1 {
		t.Fatalf("expected 1 alert for node condition, got %d", len(fired))
	}
}

func TestEvaluator_CooldownPreventsRepeat(t *testing.T) {
	var fired []Alert
	e := NewEvaluator(EvaluatorConfig{
		CPUThreshold:    90,
		MemThreshold:    85,
		DiskThreshold:   90,
		DefaultCooldown: 5 * time.Minute,
	}, func(a Alert) { fired = append(fired, a) })

	// Fire high memory twice
	e.EvaluateCluster(ClusterSnapshot{
		Nodes: []NodeSnapshot{{Name: "s0", CPUPercent: 10, MemPercent: 88, DiskPercent: 30}},
	})
	e.EvaluateCluster(ClusterSnapshot{
		Nodes: []NodeSnapshot{{Name: "s0", CPUPercent: 10, MemPercent: 89, DiskPercent: 30}},
	})

	if len(fired) != 1 {
		t.Fatalf("expected 1 alert (cooldown should block second), got %d", len(fired))
	}
}

func TestEvaluator_PodRestart_Fires(t *testing.T) {
	var fired []Alert
	e := NewEvaluator(EvaluatorConfig{
		DefaultCooldown: 5 * time.Minute,
	}, func(a Alert) { fired = append(fired, a) })

	e.EvaluatePods(PodSnapshot{
		Pods: []PodStatus{{Name: "auth-svc-abc", Namespace: "default", RestartCount: 3}},
	})
	// First call records baseline, no alert
	if len(fired) != 0 {
		t.Fatal("first pod snapshot should record baseline, not alert")
	}

	e.EvaluatePods(PodSnapshot{
		Pods: []PodStatus{{Name: "auth-svc-abc", Namespace: "default", RestartCount: 5}},
	})
	if len(fired) != 1 {
		t.Fatalf("expected 1 alert for restart increase, got %d", len(fired))
	}
}

func TestEvaluator_PodOOMKilled_Fires(t *testing.T) {
	var fired []Alert
	e := NewEvaluator(EvaluatorConfig{
		DefaultCooldown: 5 * time.Minute,
	}, func(a Alert) { fired = append(fired, a) })

	e.EvaluatePods(PodSnapshot{
		Pods: []PodStatus{{Name: "gfs-master", Namespace: "default", OOMKilled: true}},
	})

	if len(fired) != 1 {
		t.Fatalf("expected 1 OOMKilled alert, got %d", len(fired))
	}
	if fired[0].Severity != SeverityCritical {
		t.Fatal("OOMKilled should be critical")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/eddison/cloud/cluster-monitor && go test ./internal/alerting/ -run TestEvaluator -v`
Expected: FAIL — types not defined

**Step 3: Write minimal implementation**

```go
// cluster-monitor/internal/alerting/evaluator.go
package alerting

import (
	"fmt"
	"time"
)

type EvaluatorConfig struct {
	CPUThreshold    float64       // e.g., 90.0
	MemThreshold    float64       // e.g., 85.0
	DiskThreshold   float64       // e.g., 90.0
	DefaultCooldown time.Duration // e.g., 5 * time.Minute
	DiskCooldown    time.Duration // e.g., 15 * time.Minute (0 = use default)
}

type NodeSnapshot struct {
	Name        string
	CPUPercent  float64
	MemPercent  float64
	DiskPercent float64
	Conditions  []string // e.g., ["MemoryPressure", "DiskPressure"]
}

type ClusterSnapshot struct {
	Nodes []NodeSnapshot
}

type PodStatus struct {
	Name         string
	Namespace    string
	RestartCount int32
	OOMKilled    bool
}

type PodSnapshot struct {
	Pods []PodStatus
}

type AlertFunc func(Alert)

type Evaluator struct {
	config   EvaluatorConfig
	cooldown *CooldownTracker
	onAlert  AlertFunc

	// Track previous CPU readings for 2-consecutive-check requirement
	prevCPUHigh map[string]bool // node name → was CPU high on previous check

	// Track pod restart baselines
	podRestarts map[string]int32 // "namespace/name" → last known restart count
}

func NewEvaluator(config EvaluatorConfig, onAlert AlertFunc) *Evaluator {
	if config.DiskCooldown == 0 {
		config.DiskCooldown = 15 * time.Minute
	}
	return &Evaluator{
		config:      config,
		cooldown:    NewCooldownTracker(),
		onAlert:     onAlert,
		prevCPUHigh: make(map[string]bool),
		podRestarts: make(map[string]int32),
	}
}

func (e *Evaluator) EvaluateCluster(snapshot ClusterSnapshot) {
	currentCPUHigh := make(map[string]bool)

	for _, node := range snapshot.Nodes {
		// CPU: requires 2 consecutive checks above threshold
		isHigh := node.CPUPercent > e.config.CPUThreshold
		currentCPUHigh[node.Name] = isHigh
		if isHigh && e.prevCPUHigh[node.Name] {
			key := fmt.Sprintf("cpu:%s", node.Name)
			if e.cooldown.Allow(key, e.config.DefaultCooldown) {
				e.onAlert(Alert{
					Title:    fmt.Sprintf("High CPU: %s", node.Name),
					Message:  fmt.Sprintf("Node %s CPU at %.1f%% (threshold: %.0f%%)", node.Name, node.CPUPercent, e.config.CPUThreshold),
					Severity: SeverityCritical,
				})
			}
		}

		// Memory: single check
		if node.MemPercent > e.config.MemThreshold {
			key := fmt.Sprintf("mem:%s", node.Name)
			if e.cooldown.Allow(key, e.config.DefaultCooldown) {
				e.onAlert(Alert{
					Title:    fmt.Sprintf("High Memory: %s", node.Name),
					Message:  fmt.Sprintf("Node %s memory at %.1f%% (threshold: %.0f%%)", node.Name, node.MemPercent, e.config.MemThreshold),
					Severity: SeverityWarning,
				})
			}
		}

		// Disk: single check, longer cooldown
		if node.DiskPercent > e.config.DiskThreshold {
			key := fmt.Sprintf("disk:%s", node.Name)
			if e.cooldown.Allow(key, e.config.DiskCooldown) {
				e.onAlert(Alert{
					Title:    fmt.Sprintf("High Disk: %s", node.Name),
					Message:  fmt.Sprintf("Node %s disk at %.1f%% (threshold: %.0f%%)", node.Name, node.DiskPercent, e.config.DiskThreshold),
					Severity: SeverityWarning,
				})
			}
		}

		// Node conditions
		for _, cond := range node.Conditions {
			key := fmt.Sprintf("condition:%s:%s", node.Name, cond)
			if e.cooldown.Allow(key, e.config.DefaultCooldown) {
				e.onAlert(Alert{
					Title:    fmt.Sprintf("Node Condition: %s", node.Name),
					Message:  fmt.Sprintf("Node %s has condition: %s", node.Name, cond),
					Severity: SeverityCritical,
				})
			}
		}
	}

	e.prevCPUHigh = currentCPUHigh
}

func (e *Evaluator) EvaluatePods(snapshot PodSnapshot) {
	for _, pod := range snapshot.Pods {
		podKey := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)

		// OOMKilled
		if pod.OOMKilled {
			alertKey := fmt.Sprintf("oom:%s", podKey)
			if e.cooldown.Allow(alertKey, e.config.DefaultCooldown) {
				e.onAlert(Alert{
					Title:    fmt.Sprintf("OOMKilled: %s", pod.Name),
					Message:  fmt.Sprintf("Pod %s in namespace %s was OOMKilled", pod.Name, pod.Namespace),
					Severity: SeverityCritical,
				})
			}
		}

		// Restart count increase
		if prev, ok := e.podRestarts[podKey]; ok {
			if pod.RestartCount > prev {
				alertKey := fmt.Sprintf("restart:%s", podKey)
				if e.cooldown.Allow(alertKey, e.config.DefaultCooldown) {
					e.onAlert(Alert{
						Title:    fmt.Sprintf("Pod Restarting: %s", pod.Name),
						Message:  fmt.Sprintf("Pod %s in namespace %s restarted (%d → %d)", pod.Name, pod.Namespace, prev, pod.RestartCount),
						Severity: SeverityWarning,
					})
				}
			}
		}
		e.podRestarts[podKey] = pod.RestartCount
	}
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/eddison/cloud/cluster-monitor && go test ./internal/alerting/ -run TestEvaluator -v`
Expected: PASS (all 8 tests)

**Step 5: Commit**

Commit message: `feat(cluster-monitor): add metrics evaluator with threshold-based alert rules`

---

### Task 4: Log Error Subscriber

Connects to the log-service WebSocket at `/ws/logs?level=ERROR` and detects error bursts (>5 from same source in 30s) and critical keywords (panic, fatal, crash).

**Files:**
- Create: `cluster-monitor/internal/alerting/logsub.go`
- Create: `cluster-monitor/internal/alerting/logsub_test.go`

**Step 1: Write the failing test**

Test the burst detection logic in isolation (not the WebSocket connection):

```go
// cluster-monitor/internal/alerting/logsub_test.go
package alerting

import (
	"testing"
	"time"
)

func TestLogDetector_CriticalKeyword_Fires(t *testing.T) {
	var fired []Alert
	d := NewLogDetector(LogDetectorConfig{
		BurstThreshold: 5,
		BurstWindow:    30 * time.Second,
		Cooldown:       5 * time.Minute,
	}, func(a Alert) { fired = append(fired, a) })

	d.HandleLogEntry("edd-gateway", "goroutine panic: runtime error")

	if len(fired) != 1 {
		t.Fatalf("expected 1 alert for panic keyword, got %d", len(fired))
	}
	if fired[0].Severity != SeverityCritical {
		t.Fatal("critical keyword should be critical severity")
	}
}

func TestLogDetector_BurstDetection_Fires(t *testing.T) {
	var fired []Alert
	d := NewLogDetector(LogDetectorConfig{
		BurstThreshold: 5,
		BurstWindow:    30 * time.Second,
		Cooldown:       5 * time.Minute,
	}, func(a Alert) { fired = append(fired, a) })

	for i := 0; i < 6; i++ {
		d.HandleLogEntry("auth-service", "database connection refused")
	}

	if len(fired) != 1 {
		t.Fatalf("expected 1 burst alert, got %d", len(fired))
	}
}

func TestLogDetector_BelowBurst_NoAlert(t *testing.T) {
	var fired []Alert
	d := NewLogDetector(LogDetectorConfig{
		BurstThreshold: 5,
		BurstWindow:    30 * time.Second,
		Cooldown:       5 * time.Minute,
	}, func(a Alert) { fired = append(fired, a) })

	for i := 0; i < 4; i++ {
		d.HandleLogEntry("auth-service", "some error")
	}

	if len(fired) != 0 {
		t.Fatalf("expected 0 alerts for below-threshold errors, got %d", len(fired))
	}
}

func TestLogDetector_BurstCooldown(t *testing.T) {
	var fired []Alert
	d := NewLogDetector(LogDetectorConfig{
		BurstThreshold: 5,
		BurstWindow:    30 * time.Second,
		Cooldown:       5 * time.Minute,
	}, func(a Alert) { fired = append(fired, a) })

	// First burst
	for i := 0; i < 6; i++ {
		d.HandleLogEntry("auth-service", "error")
	}
	// Second burst (should be suppressed by cooldown)
	for i := 0; i < 6; i++ {
		d.HandleLogEntry("auth-service", "error")
	}

	if len(fired) != 1 {
		t.Fatalf("expected 1 alert (cooldown should block second burst), got %d", len(fired))
	}
}

func TestLogDetector_DifferentSourcesBurstIndependently(t *testing.T) {
	var fired []Alert
	d := NewLogDetector(LogDetectorConfig{
		BurstThreshold: 5,
		BurstWindow:    30 * time.Second,
		Cooldown:       5 * time.Minute,
	}, func(a Alert) { fired = append(fired, a) })

	for i := 0; i < 6; i++ {
		d.HandleLogEntry("auth-service", "error")
	}
	for i := 0; i < 6; i++ {
		d.HandleLogEntry("edd-gateway", "error")
	}

	if len(fired) != 2 {
		t.Fatalf("expected 2 alerts (one per source), got %d", len(fired))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/eddison/cloud/cluster-monitor && go test ./internal/alerting/ -run TestLogDetector -v`
Expected: FAIL — types not defined

**Step 3: Write minimal implementation**

```go
// cluster-monitor/internal/alerting/logsub.go
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

	// Append and prune old entries
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
```

**Step 4: Run test to verify it passes**

Run: `cd /home/eddison/cloud/cluster-monitor && go test ./internal/alerting/ -run TestLogDetector -v`
Expected: PASS (all 5 tests)

**Step 5: Commit**

Commit message: `feat(cluster-monitor): add log error burst and critical keyword detection`

---

### Task 5: Log-Service WebSocket Client

Connects to log-service's WebSocket and feeds entries into the LogDetector. Handles reconnection.

**Files:**
- Create: `cluster-monitor/internal/alerting/logclient.go`

**Step 1: Write implementation**

No unit test for this — it's a WebSocket client with reconnect logic. Integration is verified in Task 7.

```go
// cluster-monitor/internal/alerting/logclient.go
package alerting

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/gorilla/websocket"
)

type logEntry struct {
	Source  string `json:"source"`
	Level  int    `json:"level"` // 0=DEBUG, 1=INFO, 2=WARN, 3=ERROR
	Message string `json:"message"`
}

// SubscribeLogService connects to the log-service WebSocket for ERROR-level logs
// and feeds them into the LogDetector. Reconnects on failure.
func SubscribeLogService(logServiceAddr string, detector *LogDetector) {
	url := "ws://" + logServiceAddr + "/ws/logs?level=ERROR"

	for {
		err := connectAndConsume(url, detector)
		if err != nil {
			slog.Error("log-service WebSocket disconnected", "error", err, "url", url)
		}
		slog.Info("reconnecting to log-service in 5s")
		time.Sleep(5 * time.Second)
	}
}

func connectAndConsume(url string, detector *LogDetector) error {
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	slog.Info("connected to log-service WebSocket", "url", url)

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		var entry logEntry
		if err := json.Unmarshal(msg, &entry); err != nil {
			slog.Warn("failed to parse log entry", "error", err)
			continue
		}

		// Only process ERROR level (3)
		if entry.Level >= 3 {
			detector.HandleLogEntry(entry.Source, entry.Message)
		}
	}
}
```

**Step 2: Commit**

Commit message: `feat(cluster-monitor): add log-service WebSocket client with auto-reconnect`

---

### Task 6: Wire Alerting into main.go

Add CLI flags, create the alerting components, hook them into the existing metrics loop, and add pod status collection.

**Files:**
- Modify: `cluster-monitor/main.go`

**Step 1: Add flags and imports**

Add to the flag block in `main()` (after line 316):

```go
discordWebhook := flag.String("discord-webhook", "", "Discord webhook URL for alerts")
alertCooldown := flag.Duration("alert-cooldown", 5*time.Minute, "Default alert cooldown duration")
logServiceHTTP := flag.String("log-service-http", "", "Log service HTTP address for WebSocket subscription (e.g., log-service:8080)")
```

Add import:
```go
"eddisonso.com/cluster-monitor/internal/alerting"
```

**Step 2: Initialize alerting after metrics cache setup**

After the `timeseries.NewMetricsStore(0)` call (after line 350), add:

```go
// Alerting
discord := alerting.NewDiscordSender(*discordWebhook)
fireAlert := func(a alerting.Alert) {
	slog.Warn("alert fired", "title", a.Title, "message", a.Message)
	if err := discord.Send(a); err != nil {
		slog.Error("failed to send Discord alert", "error", err)
	}
}

evaluator := alerting.NewEvaluator(alerting.EvaluatorConfig{
	CPUThreshold:    90,
	MemThreshold:    85,
	DiskThreshold:   90,
	DefaultCooldown: *alertCooldown,
}, fireAlert)

logDetector := alerting.NewLogDetector(alerting.LogDetectorConfig{
	BurstThreshold: 5,
	BurstWindow:    30 * time.Second,
	Cooldown:       *alertCooldown,
}, fireAlert)

if *logServiceHTTP != "" {
	go alerting.SubscribeLogService(*logServiceHTTP, logDetector)
}
```

**Step 3: Hook evaluator into metrics loop**

In `fetchClusterInfo()` — after `cache.SetClusterInfo(...)` (around line 575), add evaluation of the cluster snapshot. This requires passing the evaluator into the worker function.

Modify `clusterInfoWorker` signature to accept `*alerting.Evaluator`:
```go
func clusterInfoWorker(clientset *kubernetes.Clientset, cache *MetricsCache,
    store *timeseries.MetricsStore, interval time.Duration, evaluator *alerting.Evaluator)
```

At the end of `fetchClusterInfo`, after recording to time-series store, add:
```go
// Evaluate alert rules
if evaluator != nil {
	var nodes []alerting.NodeSnapshot
	for _, nm := range nodeMetrics {
		var conditions []string
		for _, c := range nm.Conditions {
			if c.Status == "True" {
				conditions = append(conditions, c.Type)
			}
		}
		nodes = append(nodes, alerting.NodeSnapshot{
			Name:        nm.Name,
			CPUPercent:  nm.CPUPercent,
			MemPercent:  nm.MemoryPercent,
			DiskPercent: nm.DiskPercent,
			Conditions:  conditions,
		})
	}
	evaluator.EvaluateCluster(alerting.ClusterSnapshot{Nodes: nodes})
}
```

**Step 4: Add pod status collection for restart/OOM detection**

In `fetchPodMetrics()`, after listing pods (around line 644 where `allPods` is built), add pod status extraction and evaluation. Pass `*alerting.Evaluator` to `podMetricsWorker` the same way.

After the pod metrics cache update, add:
```go
// Evaluate pod alerts
if evaluator != nil {
	var podStatuses []alerting.PodStatus
	for _, pod := range allPods {
		for _, cs := range pod.Status.ContainerStatuses {
			ps := alerting.PodStatus{
				Name:         pod.Name,
				Namespace:    pod.Namespace,
				RestartCount: cs.RestartCount,
			}
			if cs.LastTerminationState.Terminated != nil &&
				cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
				ps.OOMKilled = true
			}
			podStatuses = append(podStatuses, ps)
		}
	}
	evaluator.EvaluatePods(alerting.PodSnapshot{Pods: podStatuses})
}
```

**Step 5: Run full test suite**

Run: `cd /home/eddison/cloud/cluster-monitor && go build .`
Expected: Compiles successfully

**Step 6: Commit**

Commit message: `feat(cluster-monitor): wire alerting into metrics loop and pod status collection`

---

### Task 7: Update Kubernetes Manifest

**Files:**
- Modify: `manifests/cluster-monitor/cluster-monitor.yaml`

**Step 1: Create the Discord webhook Kubernetes secret**

Run (with user providing the actual webhook URL):
```bash
kubectl create secret generic discord-webhook-url \
  --from-literal=WEBHOOK_URL='<discord-webhook-url>' \
  --dry-run=client -o yaml | kubectl apply -f -
```

**Step 2: Update the deployment manifest**

Add the Discord webhook secret as an environment variable and the log-service HTTP address to the container args:

In the `env` section of the deployment, add:
```yaml
- name: DISCORD_WEBHOOK_URL
  valueFrom:
    secretKeyRef:
      name: discord-webhook-url
      key: WEBHOOK_URL
```

In the `args` section, add:
```yaml
- -discord-webhook
- $(DISCORD_WEBHOOK_URL)
- -log-service-http
- log-service:8080
```

**Step 3: Commit and push**

Commit message: `feat(manifests): add Discord webhook secret and log-service HTTP to cluster-monitor`

---

### Task 8: Update Roadmap and Documentation

**Files:**
- Modify: `edd-cloud-docs/docs/roadmap.md` — check the "Alerting" item under Monitoring and mark done or update

**Step 1: Update roadmap**

Mark the alerting item as complete in `edd-cloud-docs/docs/roadmap.md`:
```markdown
- [x] **Alerting** - Automated alerts for service health
```

**Step 2: Commit**

Commit message: `docs: mark alerting as complete on roadmap`

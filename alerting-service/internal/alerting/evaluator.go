package alerting

import (
	"fmt"
	"sync"
	"time"
)

type EvaluatorConfig struct {
	CPUThreshold    float64
	MemThreshold    float64
	DiskThreshold   float64
	DefaultCooldown time.Duration
	DiskCooldown    time.Duration // 0 = use 15 * time.Minute
}

type NodeSnapshot struct {
	Name        string
	CPUPercent  float64
	MemPercent  float64
	DiskPercent float64
	Conditions  []string // e.g., ["MemoryPressure"]
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
	mu          sync.Mutex
	config      EvaluatorConfig
	cooldown    *CooldownTracker
	onAlert     AlertFunc
	prevCPUHigh map[string]bool  // node → was CPU high on previous check
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
	e.mu.Lock()
	defer e.mu.Unlock()

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
	e.mu.Lock()
	defer e.mu.Unlock()

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

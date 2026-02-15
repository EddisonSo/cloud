package alerting

import (
	"testing"
	"time"
)

func TestEvaluator_HighCPU_SingleCheck_NoAlert(t *testing.T) {
	var fired []Alert
	e := NewEvaluator(EvaluatorConfig{
		CPUThreshold: 90, MemThreshold: 85, DiskThreshold: 90,
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
		CPUThreshold: 90, MemThreshold: 85, DiskThreshold: 90,
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
		CPUThreshold: 90, MemThreshold: 85, DiskThreshold: 90,
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
		CPUThreshold: 90, MemThreshold: 85, DiskThreshold: 90,
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
		CPUThreshold: 90, MemThreshold: 85, DiskThreshold: 90,
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
		CPUThreshold: 90, MemThreshold: 85, DiskThreshold: 90,
		DefaultCooldown: 5 * time.Minute,
	}, func(a Alert) { fired = append(fired, a) })

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

func TestEvaluator_PodOOMKilled_NoRepeatForSameEvent(t *testing.T) {
	var fired []Alert
	e := NewEvaluator(EvaluatorConfig{
		DefaultCooldown: 5 * time.Minute,
	}, func(a Alert) { fired = append(fired, a) })

	pod := PodStatus{Name: "gfs-master", Namespace: "default", OOMKilled: true, RestartCount: 1}

	// First snapshot: should fire
	e.EvaluatePods(PodSnapshot{Pods: []PodStatus{pod}})
	if len(fired) != 1 {
		t.Fatalf("expected 1 OOMKilled alert, got %d", len(fired))
	}

	// Repeated snapshots with same restart count: should NOT fire again
	e.EvaluatePods(PodSnapshot{Pods: []PodStatus{pod}})
	e.EvaluatePods(PodSnapshot{Pods: []PodStatus{pod}})
	e.EvaluatePods(PodSnapshot{Pods: []PodStatus{pod}})
	if len(fired) != 1 {
		t.Fatalf("expected still 1 alert (same OOM event), got %d", len(fired))
	}

	// New OOM (restart count increased): fires OOM alert + restart alert
	pod.RestartCount = 2
	e.EvaluatePods(PodSnapshot{Pods: []PodStatus{pod}})
	if len(fired) != 3 {
		t.Fatalf("expected 3 alerts (new OOM + restart increase), got %d", len(fired))
	}
}

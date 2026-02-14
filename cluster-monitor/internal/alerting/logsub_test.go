package alerting

import (
	"testing"
	"time"
)

func TestLogDetector_CriticalKeyword_Fires(t *testing.T) {
	var fired []Alert
	d := NewLogDetector(LogDetectorConfig{
		BurstThreshold: 5, BurstWindow: 30 * time.Second, Cooldown: 5 * time.Minute,
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
		BurstThreshold: 5, BurstWindow: 30 * time.Second, Cooldown: 5 * time.Minute,
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
		BurstThreshold: 5, BurstWindow: 30 * time.Second, Cooldown: 5 * time.Minute,
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
		BurstThreshold: 5, BurstWindow: 30 * time.Second, Cooldown: 5 * time.Minute,
	}, func(a Alert) { fired = append(fired, a) })

	for i := 0; i < 6; i++ {
		d.HandleLogEntry("auth-service", "error")
	}
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
		BurstThreshold: 5, BurstWindow: 30 * time.Second, Cooldown: 5 * time.Minute,
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

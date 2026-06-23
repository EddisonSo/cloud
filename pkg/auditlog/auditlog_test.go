package auditlog

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func capture(t *testing.T, fn func(l *slog.Logger)) string {
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	old := slog.Default()
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(old)
	fn(slog.Default())
	return buf.String()
}

func TestRecord_MarkerAndLevels(t *testing.T) {
	out := capture(t, func(l *slog.Logger) {
		Denied(context.Background(), "authz.denied", "container/abc")
		Success(context.Background(), "auth.login", "user/bob")
	})
	if !strings.Contains(out, "audit=true") {
		t.Fatal("missing audit marker")
	}
	if !strings.Contains(out, "level=WARN") || !strings.Contains(out, "outcome=denied") {
		t.Fatalf("denied should be WARN: %s", out)
	}
	if !strings.Contains(out, "level=INFO") || !strings.Contains(out, "outcome=success") {
		t.Fatalf("success should be INFO: %s", out)
	}
}

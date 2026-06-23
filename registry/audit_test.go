package main

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"testing"
)

// captureHandler is a slog.Handler that records emitted records for assertions.
type captureHandler struct {
	records []slog.Record
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}
func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler       { return h }

func attrMap(r slog.Record) map[string]string {
	m := map[string]string{}
	r.Attrs(func(a slog.Attr) bool {
		m[a.Key] = a.Value.String()
		return true
	})
	return m
}

// TestRequireAuthEmitsDeniedAudit verifies that the 401 WWW-Authenticate branch
// records a security audit event marked audit=true with action=authz.denied and
// outcome=denied, so denied registry access is captured in the durable archive.
func TestRequireAuthEmitsDeniedAudit(t *testing.T) {
	cap := &captureHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(cap))
	defer slog.SetDefault(prev)

	s := &server{}
	w := httptest.NewRecorder()
	s.requireAuth(context.Background(), w, "alice/app", "pull")

	if w.Code != 401 {
		t.Fatalf("expected status 401, got %d", w.Code)
	}

	var found bool
	for _, rec := range cap.records {
		m := attrMap(rec)
		if m["audit"] != "true" {
			continue
		}
		found = true
		if m["action"] != "authz.denied" {
			t.Errorf("action = %q, want authz.denied", m["action"])
		}
		if m["outcome"] != "denied" {
			t.Errorf("outcome = %q, want denied", m["outcome"])
		}
		if m["resource"] != "alice/app" {
			t.Errorf("resource = %q, want alice/app", m["resource"])
		}
	}
	if !found {
		t.Fatal("no audit event with audit=true was emitted by requireAuth")
	}
}

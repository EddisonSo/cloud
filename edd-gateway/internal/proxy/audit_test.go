package proxy

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"eddisonso.com/edd-cloud/pkg/auditlog"
)

// captureSlog redirects the default slog logger to a buffer for the duration of
// fn and returns everything written. Used to assert audit events carry the
// expected marker and fields.
func captureSlog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	old := slog.Default()
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(old)
	fn()
	return buf.String()
}

// TestNoRouteAuditEvent mirrors the NO-ROUTE audit call made on the HTTP/HTTPS
// proxy paths: a request id and client IP are threaded into the context and
// auditlog.Denied is invoked with the gateway.no_route action. It asserts the
// emitted event is marked audit=true, carries outcome=denied, the right action,
// the host+path resource, and the correlation fields.
func TestNoRouteAuditEvent(t *testing.T) {
	out := captureSlog(t, func() {
		ctx := auditlog.WithClientIP(auditlog.WithRequestID(context.Background(), "req-123"), "203.0.113.7:54321")
		auditlog.Denied(ctx, "gateway.no_route", "evil.example.com/admin")
	})

	for _, want := range []string{
		"audit=true",
		"level=WARN",
		"action=gateway.no_route",
		"outcome=denied",
		"resource=evil.example.com/admin",
		"request_id=req-123",
		"client_ip=203.0.113.7:54321",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("audit output missing %q\ngot: %s", want, out)
		}
	}
}

// TestSSHRejectAuditEvent mirrors the SSH key-rejected audit call: there is no
// request id on the raw SSH path, only the client IP. It asserts the denied
// event records the gateway.ssh.reject action with the rejection reason and the
// target container as the resource, never any secret material.
func TestSSHRejectAuditEvent(t *testing.T) {
	out := captureSlog(t, func() {
		ctx := auditlog.WithClientIP(context.Background(), "198.51.100.4:22")
		auditlog.Denied(ctx, "gateway.ssh.reject", "container-abc", "reason", "key_rejected")
	})

	for _, want := range []string{
		"audit=true",
		"level=WARN",
		"action=gateway.ssh.reject",
		"outcome=denied",
		"resource=container-abc",
		"reason=key_rejected",
		"client_ip=198.51.100.4:22",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("audit output missing %q\ngot: %s", want, out)
		}
	}
}

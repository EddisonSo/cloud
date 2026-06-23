// Package auditlog emits structured security audit events via log/slog.
//
// Every event carries the marker attribute "audit"="true" plus a standard set
// of fields (action, outcome, actor, client_ip, request_id, resource). The
// log-service persistence filter keys off the marker to guarantee that audit
// events reach the durable archive regardless of their slog level.
package auditlog

import (
	"context"
	"log/slog"
)

// Record emits a single audit event. The outcome determines the slog level:
// "denied"/"failure" log at Warn, everything else (e.g. "success") at Info.
// Extra key/value pairs are appended verbatim — NEVER pass secrets, only
// identifiers (key id, token id, username).
func Record(ctx context.Context, action, outcome, resource string, extra ...any) {
	args := []any{
		"audit", "true",
		"action", action,
		"outcome", outcome,
		"actor", actorFrom(ctx),
		"client_ip", clientIPFrom(ctx),
		"request_id", requestIDFrom(ctx),
	}
	if resource != "" {
		args = append(args, "resource", resource)
	}
	args = append(args, extra...)
	if outcome == "denied" || outcome == "failure" {
		slog.WarnContext(ctx, "audit", args...)
	} else {
		slog.InfoContext(ctx, "audit", args...)
	}
}

// Success records a successful security-relevant action (Info level).
func Success(ctx context.Context, action, resource string, extra ...any) {
	Record(ctx, action, "success", resource, extra...)
}

// Failure records a failed security-relevant action (Warn level).
func Failure(ctx context.Context, action, resource string, extra ...any) {
	Record(ctx, action, "failure", resource, extra...)
}

// Denied records a denied security-relevant action (Warn level).
func Denied(ctx context.Context, action, resource string, extra ...any) {
	Record(ctx, action, "denied", resource, extra...)
}

# Security Audit Trail — Implementation Plan (Stage 2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A structured security audit trail across all backend services, persisted to the 14-day GFS archive via an `audit=true` marker, answering "who did what, to what, from where, allowed or not."

**Architecture:** A shared `pkg/auditlog` package emits structured slog events (marker `audit=true`, standard actor/client_ip/request_id/action/outcome/resource fields). log-service's persistence filter is extended to keep `Warn+` OR audit-marked entries. Each service wires audit middleware (request_id + client_ip into context) and calls `auditlog.Success/Failure/Denied` at security-relevant points.

**Tech Stack:** Go, `log/slog`, existing `gfslog` (already copies slog attrs into the persisted `LogEntry.Attributes`). New module `eddisonso.com/edd-cloud/pkg/auditlog`, wired via `replace => ../pkg/auditlog` like `pkg/events`.

## Global Constraints

- Audit event = slog call carrying `"audit","true"` + standard fields. Level by outcome: `denied`/`failure` → `slog.Warn`; `success` → `slog.Info`. Marker guarantees persistence regardless of level.
- Standard fields (all string): `action` (namespaced verb), `outcome` (`success|failure|denied`), `actor` (id/username or `"anonymous"`), `client_ip` (or `""`), `request_id` (or `""`), `resource` (optional).
- NEVER log secrets/passwords/token values — only identifiers (key id, token id, username).
- Emit denial/failure audit events in the SAME branch that returns the 401/403/429, so they can't drift from the decision.
- `pkg/auditlog` depends only on stdlib + `log/slog`. Each consuming service adds `require eddisonso.com/edd-cloud/pkg/auditlog v0.0.0` + `replace eddisonso.com/edd-cloud/pkg/auditlog => ../pkg/auditlog`.
- Build each Go module from its own dir; tests needing GFS use `GFS_JWT_SECRET=test-secret`.

---

### Task 1: `pkg/auditlog` core — Record + outcome→level + marker

**Files:**
- Create: `pkg/auditlog/go.mod` (`module eddisonso.com/edd-cloud/pkg/auditlog`, `go 1.21`)
- Create: `pkg/auditlog/auditlog.go`
- Test: `pkg/auditlog/auditlog_test.go`

**Interfaces:**
- Produces: `Record(ctx, action, outcome, resource string, extra ...any)`, `Success/Failure/Denied(ctx, action, resource string, extra ...any)`.

- [ ] **Step 1: Write the failing test** (capture slog via a test handler; assert marker + level + outcome)

```go
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
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd pkg/auditlog && go test ./... -run TestRecord -v`
Expected: FAIL (undefined `Denied`/`Success`).

- [ ] **Step 3: Implement**

```go
// pkg/auditlog/auditlog.go
package auditlog

import (
	"context"
	"log/slog"
)

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

func Success(ctx context.Context, action, resource string, extra ...any) {
	Record(ctx, action, "success", resource, extra...)
}
func Failure(ctx context.Context, action, resource string, extra ...any) {
	Record(ctx, action, "failure", resource, extra...)
}
func Denied(ctx context.Context, action, resource string, extra ...any) {
	Record(ctx, action, "denied", resource, extra...)
}
```

- [ ] **Step 4: Run to verify pass** — `cd pkg/auditlog && go test ./... -v` → PASS (context helpers come in Task 2; this task may temporarily stub `actorFrom`/`clientIPFrom`/`requestIDFrom` — define them in Task 2's file but include minimal versions here if needed to compile. To avoid a split, implement Task 2's context.go in the same commit.)

- [ ] **Step 5: Commit** — `feat(auditlog): core Record/Success/Failure/Denied with audit marker` (combined with Task 2).

---

### Task 2: `pkg/auditlog` context + HTTP middleware

**Files:**
- Create: `pkg/auditlog/context.go`
- Test: `pkg/auditlog/context_test.go`

**Interfaces:**
- Produces: `WithRequestID/WithActor/WithClientIP(ctx, string) context.Context`; getters `actorFrom/clientIPFrom/requestIDFrom(ctx) string` (defaults: actor `"anonymous"`, others `""`); `HTTPMiddleware(next http.Handler) http.Handler`.

- [ ] **Step 1: Write the failing test**

```go
package auditlog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestContextGettersDefaults(t *testing.T) {
	ctx := context.Background()
	if actorFrom(ctx) != "anonymous" { t.Fatal("actor default") }
	if clientIPFrom(ctx) != "" || requestIDFrom(ctx) != "" { t.Fatal("empty defaults") }
	ctx = WithActor(WithRequestID(ctx, "rid"), "bob")
	if actorFrom(ctx) != "bob" || requestIDFrom(ctx) != "rid" { t.Fatal("set/get") }
}

func TestHTTPMiddleware_PopulatesContext(t *testing.T) {
	var gotRID, gotIP string
	h := HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRID = requestIDFrom(r.Context())
		gotIP = clientIPFrom(r.Context())
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-ID", "abc123")
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if gotRID != "abc123" { t.Fatalf("rid=%q", gotRID) }
	if gotIP != "1.2.3.4" { t.Fatalf("ip=%q", gotIP) }
}
```

- [ ] **Step 2: Run to verify fail** — `cd pkg/auditlog && go test ./... -run 'TestContext|TestHTTP' -v` → FAIL.

- [ ] **Step 3: Implement**

```go
// pkg/auditlog/context.go
package auditlog

import (
	"context"
	"net/http"
	"strings"
)

type ctxKey int

const (
	keyRequestID ctxKey = iota
	keyActor
	keyClientIP
)

func WithRequestID(ctx context.Context, id string) context.Context { return context.WithValue(ctx, keyRequestID, id) }
func WithActor(ctx context.Context, a string) context.Context      { return context.WithValue(ctx, keyActor, a) }
func WithClientIP(ctx context.Context, ip string) context.Context  { return context.WithValue(ctx, keyClientIP, ip) }

func requestIDFrom(ctx context.Context) string { s, _ := ctx.Value(keyRequestID).(string); return s }
func clientIPFrom(ctx context.Context) string  { s, _ := ctx.Value(keyClientIP).(string); return s }
func actorFrom(ctx context.Context) string {
	if s, ok := ctx.Value(keyActor).(string); ok && s != "" { return s }
	return "anonymous"
}

func clientIP(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		if i := strings.IndexByte(v, ','); i >= 0 { return strings.TrimSpace(v[:i]) }
		return strings.TrimSpace(v)
	}
	if v := r.Header.Get("X-Real-IP"); v != "" { return v }
	return r.RemoteAddr
}

// HTTPMiddleware seeds request_id and client_ip into the request context.
// Actor is added later by each service's auth middleware once identified.
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := WithRequestID(r.Context(), r.Header.Get("X-Request-ID"))
		ctx = WithClientIP(ctx, clientIP(r))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
```

- [ ] **Step 4: Run to verify pass** — `cd pkg/auditlog && go test ./... -v` → PASS; `go build ./...` clean.

- [ ] **Step 5: Commit** — `feat(auditlog): context propagation + HTTP middleware` (with Task 1).

---

### Task 3: log-service persistence filter — admit audit-marked entries

**Files:**
- Modify: `log-service/internal/server/server.go` (`enqueuePersist`)
- Test: `log-service/internal/server/persist_test.go` (extend)

- [ ] **Step 1: Add failing test**

```go
func TestEnqueuePersist_AuditMarkedInfoAdmitted(t *testing.T) {
	s := newTestServer(10)
	s.persistEnabled = true
	// Info WITHOUT marker -> not admitted
	s.enqueuePersist(context.Background(), &pb.LogEntry{Level: pb.LogLevel_INFO, Source: "t"})
	// Info WITH audit marker -> admitted
	s.enqueuePersist(context.Background(), &pb.LogEntry{Level: pb.LogLevel_INFO, Source: "t",
		Attributes: map[string]string{"audit": "true"}})
	if got := len(s.persistCh); got != 1 {
		t.Fatalf("expected 1 admitted (audit info), got %d", got)
	}
}
```
(Add `context` import to the test if missing.)

- [ ] **Step 2: Run to verify fail** — `cd log-service && GFS_JWT_SECRET=test-secret go test ./internal/server/ -run TestEnqueuePersist_Audit -v` → FAIL (Info dropped).

- [ ] **Step 3: Implement** — change the guard in `enqueuePersist`:

```go
	if !s.persistEnabled {
		return
	}
	if entry.Level < pb.LogLevel_WARN && entry.Attributes["audit"] != "true" {
		return
	}
```

- [ ] **Step 4: Run to verify pass** — `cd log-service && GFS_JWT_SECRET=test-secret go test ./internal/server/ -v && go build ./...` → PASS; clean.

- [ ] **Step 5: Commit** — `feat(log-service): persist audit-marked entries regardless of level`.

---

### Tasks 4–8: Per-service instrumentation

Each service task follows the SAME shape, so it is described once here and applied per service with that service's event list:

**Per-service steps (apply to each of Tasks 4–8):**
1. Add to the service `go.mod`: `require eddisonso.com/edd-cloud/pkg/auditlog v0.0.0` and `replace eddisonso.com/edd-cloud/pkg/auditlog => ../pkg/auditlog`; run `go mod tidy`.
2. Wrap the service's HTTP mux with `auditlog.HTTPMiddleware` (outermost, after CORS so OPTIONS still short-circuits; before auth so request_id/ip are present). For gateway, request_id already exists — instead add `auditlog.WithRequestID/WithClientIP` where it builds the upstream context, or skip middleware and pass the existing reqID into audit calls.
3. In the service's auth middleware, once the caller is identified, add `ctx = auditlog.WithActor(ctx, userID)` and continue with that context.
4. At each event site (file:line from the map), add one line: `auditlog.Success/Failure/Denied(ctx, "<action>", "<resource>", <extra k,v…>)`. Use the canonical action strings from the spec taxonomy. Place denial/failure calls in the exact branch returning 401/403/429.
5. NEVER pass secrets — only ids/usernames/key-ids.
6. Add ONE table test per service asserting a representative denial path emits an audit event with `outcome=denied` and the right action (capture via a test slog handler).
7. Verify: `cd <service> && GFS_JWT_SECRET=test-secret go test ./... && go build ./...`.

**Task 4 — edd-cloud-auth** (`internal/api/`): instrument `auth.login` (success + the existing failure → route through `auditlog`), `auth.logout`/`session.invalidate`, `user.create/update/delete`, `password.change`, `token.issue/revoke`, `sshkey.add/delete`, `passkey.register`, `auth.2fa.challenge`, `ratelimit.reject` (the 429 branch), `identity.create/update/delete`. Actor = authenticated user/SA id; reuse existing `getClientIP` semantics (now also in middleware).

**Task 5 — compute** (`internal/api/`): instrument `authz.denied` at the currently-Debug 403 branches (`handler.go` ~100/186/227 + scope checks ~251/270/287), `container.create`, `container.delete`, `terminal.start`, `terminal.end`. Resource = container id / namespace.

**Task 6 — sfs** (`main.go`, `events.go`): instrument RBAC `authz.denied` (the 403 branches), `ns.visibility.change`, `ns.grant`/`ns.revoke`, `file.delete`, and convert the `ws auth failed` `log.Printf` (main.go ~2079) to `auditlog.Denied(ctx, "auth.ws", …)`. Resource = namespace / path.

**Task 7 — edd-gateway** (`internal/proxy/`, `internal/api/`): instrument `gateway.ssh.reject` (ssh.go key-rejected + container-not-found/SSH-blocked) and `gateway.no_route` (the NO-ROUTE WARNs) as audit events, reusing the existing `request_id`. These are already WARN logs — add the `audit=true` marker + standard fields via `auditlog.Denied`.

**Task 8 — registry** (edd-registry): instrument `image.push`, `image.pull`, `image.delete` (success), and auth rejections → `auditlog.Denied(ctx, "authz.denied", <image ref>)`. Resource = image ref/tag.

Each of Tasks 4–8 ends with its own commit: `feat(<service>): emit security audit events`.

---

### Task 9: Docs

**Files:**
- Modify: `edd-cloud-docs/docs/services/logging.md`

- [ ] Add an "Audit Trail" section: the `audit=true` marker, the standard field set, the action taxonomy, outcome→level rule, that audit events always persist (14-day archive) and that denials/failures also appear in the live Warn view. Build docs (`cd edd-cloud-docs && npm run build`). Commit `docs: document the security audit trail`.

---

## Self-Review

- **Spec coverage:** shared package (T1–2) ✓; outcome→level + marker (T1) ✓; context/middleware + request_id propagation (T2) ✓; filter extension (T3) ✓; all 5 services' Tier 1+2 events incl. the silent compute/sfs 403s and registry ops (T4–8) ✓; no-secrets + same-branch-as-decision constraints (global + per-service step 4–5) ✓; docs (T9) ✓.
- **Placeholders:** foundation tasks (T1–3) carry full code; per-service tasks specify exact action strings, file:line anchors, and the identical one-line pattern + one worked test each — concrete given the implementer has the codebase + the 73-event map.
- **Type consistency:** `Record/Success/Failure/Denied(ctx, action[, outcome], resource, extra...)`, `With*`/`*From` helpers, `HTTPMiddleware`, and the `Attributes["audit"]=="true"` check are used consistently across tasks. SDK/proto: `entry.Attributes` is the existing `map[string]string` populated by gfslog.

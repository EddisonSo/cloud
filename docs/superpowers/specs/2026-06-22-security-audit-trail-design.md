# Security Audit Trail — Design (Stage 2)

**Date:** 2026-06-22
**Status:** Approved (design)
**Scope:** Stage 2 of the logging initiative. Adds a structured security audit
trail across the backend services, persisted durably via the Stage 1 archive.
Stage 1 (reliable Warn+ archive + 14-day retention) is already shipped.

**Services touched:** new `pkg/auditlog/` package; `log-service` (one-line filter
extension); instrumentation in `edd-cloud-auth`, `compute`, `sfs`, `edd-gateway`,
`registry`. Coverage: **Tier 1 + Tier 2** (full audit — authn, authz denials,
credential lifecycle, privileged/admin actions, AND routine success actions).

## Goal

Produce a durable, queryable record that answers "who did what, to what, from
where, and was it allowed" — detailed enough to debug incidents and identify/triage
security issues after the fact. All audit events land in the 14-day GFS archive.

## Key mechanism (verified)

`gfslog` already copies every slog attribute into the persisted `LogEntry.Attributes`
map (`go-gfs/pkg/gfslog/handler.go` `recordToEntry`). So an audit event is just an
slog call carrying an `audit=true` attribute — **no proto change needed**. The Stage 1
persistence filter is extended to keep an entry if `Level >= WARN` **OR**
`Attributes["audit"] == "true"`.

## Design

### 1. `pkg/auditlog/` — shared package

A single helper all services import. Standard event shape with these fields (all
strings in the slog attr set, so they round-trip through `gfslog` → archive):

- `audit` = `"true"` (the persistence marker)
- `action` — verb, dot-namespaced (e.g. `auth.login`, `authz.denied`,
  `token.issued`, `container.create`, `image.push`, `ns.visibility.change`)
- `outcome` — one of `success` | `failure` | `denied`
- `actor` — user id / service-account id / username if known, else `"anonymous"`
- `client_ip` — source IP if available, else `""`
- `request_id` — correlation id from the gateway's `X-Request-ID`, else `""`
- `resource` — the thing acted on (container id, namespace, image ref, key id…), optional
- plus any event-specific key/values passed by the caller

API:

```go
package auditlog

// Record emits a structured audit event. outcome determines the level:
// "failure"/"denied" -> Warn (also visible in the live Warn view + alertable),
// "success" -> Info (still archived via the audit marker). actor/client_ip/
// request_id are pulled from ctx (see context helpers); extra is appended as
// alternating key,value slog args.
func Record(ctx context.Context, action, outcome, resource string, extra ...any)

// Convenience wrappers (thin):
func Success(ctx context.Context, action, resource string, extra ...any) // outcome=success
func Failure(ctx context.Context, action, resource string, extra ...any) // outcome=failure
func Denied(ctx context.Context, action, resource string, extra ...any)  // outcome=denied
```

Level rule (inside `Record`): `denied`/`failure` → `slog.Warn`; `success` → `slog.Info`.
Every call includes `"audit","true"` so it is archived regardless of level.

### 2. Context propagation

`pkg/auditlog` also owns the context plumbing so call sites stay one-liners:

```go
// keys are unexported; set by middleware, read by Record.
func WithRequestID(ctx, id string) context.Context
func WithActor(ctx, actor string) context.Context
func WithClientIP(ctx, ip string) context.Context

// Middleware: reads X-Request-ID (or generates one if absent), extracts client IP
// (respecting the existing X-Forwarded-For / RemoteAddr conventions per service),
// and stores both in the request context. Actor is added later by each service's
// auth middleware once the caller is identified.
func HTTPMiddleware(next http.Handler) http.Handler
```

The gateway already generates `X-Request-ID` (Stage 1.5 work) and propagates it
upstream; each service's `HTTPMiddleware` reads it so a request correlates
end-to-end. Where a service has no IP (post-gateway termination), `client_ip` is
`""` and the `request_id` carries the correlation instead.

### 3. log-service filter extension (Stage 1 tie-in)

In `log-service/internal/server/server.go` `enqueuePersist`, change the guard from:

```go
if !s.persistEnabled || entry.Level < pb.LogLevel_WARN { return }
```

to also admit audit-marked entries:

```go
if !s.persistEnabled { return }
if entry.Level < pb.LogLevel_WARN && entry.Attributes["audit"] != "true" { return }
```

This is the only change to Stage 1 code. (Info-level audit successes now persist;
non-audit Info/Debug still do not.)

### 4. Event instrumentation (the bulk)

Instrument the events from the 73-event map, per service. Each is a one-line
`auditlog.Success/Failure/Denied(ctx, action, resource, …)` at the right point.
Notable required additions (currently silent):
- **compute** authorization denials (`internal/api/handler.go` scope/session checks
  returning 403) → `auditlog.Denied(ctx, "authz.denied", resource, "reason", …)`.
- **sfs** RBAC 403s and the `ws auth failed` (currently `log.Printf`) → `Denied`.
- **registry** push/pull/delete + auth rejections → `Success`/`Denied`.
- **auth** login success (currently only failure logged) → `Success`; keep the
  existing failure Warn but route it through `auditlog.Failure` for field consistency.

Actions taxonomy (canonical strings, namespaced):
`auth.login`, `auth.logout`, `auth.2fa.challenge`, `session.invalidate`,
`user.create`, `user.update`, `user.delete`, `password.change`,
`token.issue`, `token.revoke`, `sshkey.add`, `sshkey.delete`, `passkey.register`,
`identity.create`, `identity.update`, `identity.delete`, `ratelimit.reject`,
`authz.denied`, `container.create`, `container.delete`, `terminal.start`,
`terminal.end`, `file.delete`, `ns.visibility.change`, `ns.grant`, `ns.revoke`,
`image.push`, `image.pull`, `image.delete`, `gateway.ssh.reject`, `gateway.no_route`.

## Components & boundaries

- **`pkg/auditlog/auditlog.go`** — `Record`/`Success`/`Failure`/`Denied` + level rule.
- **`pkg/auditlog/context.go`** — context keys, `With*` setters, getters, `HTTPMiddleware`.
- **Per-service instrumentation** — call sites only; each service wires `HTTPMiddleware`
  into its mux and sets the actor in its existing auth middleware.
- **`log-service` filter** — the one-line `enqueuePersist` change.

Each service depends on `pkg/auditlog`, which depends only on stdlib + `log/slog`
(no circular deps; mirrors how `pkg/events` is shared today). Go module `replace`
directives are added per service `go.mod` as needed (same pattern as `pkg/events`).

## Data flow

```
request → gateway (mints X-Request-ID) → service HTTPMiddleware (ctx: request_id, client_ip)
   → auth middleware (ctx: actor) → handler
       → auditlog.Success/Failure/Denied(ctx, action, resource, …)
           → slog (Info for success / Warn for denied|failure) + audit=true + fields
               → gfslog → PushLog → enqueuePersist (admitted: Warn+ OR audit) → GFS archive (14d)
```

## Error handling

- auditlog never blocks the request path: it's a fire-and-forget slog call; gfslog
  ships async and drops on its own channel-full (live), while the durable path is the
  Stage 1 archive.
- Missing context (no actor/ip/request_id) → fields emit as `"anonymous"`/`""`; never
  panics on absent context values.
- A `denied`/`failure` audit event is emitted in the SAME branch that returns the
  403/401/429, so it cannot drift out of sync with the actual decision.

## Testing

- **auditlog unit:** `Record` emits the marker + correct level per outcome; context
  getters return defaults when unset; `Success/Failure/Denied` set the right outcome.
  Capture via a test `slog.Handler` and assert attributes.
- **filter unit (log-service):** an Info entry with `audit=true` is admitted; an Info
  entry without it is not; Warn+ still admitted. (Extends the Stage 1 persist test.)
- **per-service:** table-test that the denial branch emits an audit event with
  `outcome=denied` and the expected action/resource (using a captured handler).
- **integration smoke (manual/post-deploy):** trigger a failed login + a compute 403
  + an image push; confirm all three appear in the day's downloaded archive zip with
  request_id/actor/client_ip populated.

## Risks / notes

- Volume: audit successes add Info-level archived entries. Bounded (security/admin
  actions + container/file/image ops are not high-frequency) and capped by 14-day
  retention; storage headroom is ~195 GB/chunkserver.
- Field hygiene: never log secrets/passwords/token values — only identifiers (key id,
  token id, username). Enforced by call-site review + the commit security scan.
- Coverage is broad (5 services). Built incrementally: foundation (`pkg/auditlog` +
  filter) first and independently shippable; then one service at a time so each is a
  reviewable, deployable increment.
- `client_ip` is often unavailable after gateway TLS termination; `request_id` is the
  primary correlation key in that case. Acceptable.
```

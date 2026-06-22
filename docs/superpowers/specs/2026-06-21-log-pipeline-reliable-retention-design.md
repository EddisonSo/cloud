# Reliable Warn+ Log Archive + 14-Day Retention — Design (Stage 1)

**Date:** 2026-06-21
**Status:** Approved (design)
**Scope:** Stage 1 of a two-stage logging initiative. This stage makes the durable
log **archive reliably capture all `Warn+` records** (errors, warnings, and the
security events Stage 2 will add), retain them **14 days**, and **stop silently
dropping them**. Debug/Info keep their current live/stdout behavior and are *not*
archived. Stage 2 (a security-event taxonomy + request-ID/actor/namespace context in
producer services, emitted at Warn+ so it lands in this archive) is a separate spec.

**Services touched:** `log-service` (persistence redesign + retention GC) only.
Producer services and the dashboard are **unchanged** in this stage.

## Decision (the key constraint)

> **Persist only `Warn+`. `Debug+` is printed as usual (live stream + stdout), not archived.**

This decouples two concerns that were previously tangled:
- **Live/ephemeral** (ring buffers, dashboard, `kubectl logs`): everything, *exactly
  as it is today* — no `MinLevel` change, no dashboard change.
- **Durable/archived** (GFS `core-logs`, 14-day retention): **`Warn+` only**, captured
  reliably with no silent loss.

## Problem

The durable archive is lossy, so it can't support after-the-fact debugging or security
forensics even for the important (Warn+) records:

1. **Silent drops.** `PushLog` does a non-blocking send to `persistCh`; if full it
   drops with no record (`server.go:189-194`). Buffer is only **1000** (`server.go:102`).
2. **Synchronous flush stalls intake.** The persistence worker calls GFS
   `AppendWithNamespace` *inline in the same select loop that drains `persistCh`*
   (`server.go:475-488`), so a slow/failed write stops intake → buffer fills → drops.
3. **No retry; data discarded on error.** On append failure the batch is logged and
   **thrown away** (`server.go:466`) — this is why the `s1` outage lost almost everything.
4. **No retention.** Files accumulate forever in the `core-logs` GFS namespace.
5. **Everything is queued for persistence regardless of level** — today the archive
   tries to keep Debug/Info too (and loses most of it), which is both wasteful and the
   opposite of what we want.

## Goals

- Persist **every `Warn+` record** to GFS reliably — **never silently dropped**.
- **Do not persist Debug/Info** at all (they remain live/stdout only).
- Transient GFS slowness must **not** lose any Warn+ record (absorb + retry, don't drop).
- A sustained GFS outage must not lose Warn+ either — apply backpressure / retry until
  GFS recovers rather than discarding.
- **14-day retention**: auto-delete archived logs older than 14 days (UTC).
- **Zero change** to live behavior: producers' `MinLevel`, ring buffers, broadcast,
  NATS error publishing, and the dashboard all stay exactly as they are.

## Non-goals (Stage 2 / YAGNI)

- The security-event taxonomy and request-ID/actor/namespace context in producer
  services (separate spec — those events will be logged at Warn+ so they land here).
- Persisting Debug/Info, cross-replica sharing, ring-buffer replay on restart,
  full-text search of the archive, configurable per-service retention.

## Architecture (all in `log-service`)

`PushLog` keeps its existing fast paths **unchanged**: add to ring buffer, broadcast to
subscribers, publish Error+ to NATS. Only the persistence path changes.

### 1. Level filter at the persistence enqueue
Replace the `select { case persistCh<-entry: default: drop }` with:
- `entry.Level >= pb.LogLevel_WARN` → **guaranteed enqueue** (blocking send to
  `persistCh`). Warn/Error/Fatal must never be dropped; under extreme backup this applies
  brief backpressure to gfslog's async sender, not the producing service's request path.
- `entry.Level < WARN` (Debug/Info) → **not enqueued at all** (return immediately). They
  are never archived.

Because only Warn+ is admitted and that is low-volume, the buffer effectively never
fills in normal operation; the blocking send only matters during a real GFS outage.

### 2. Decoupled, resilient writer (async)
Split the single inline worker into two goroutines so a slow/failed GFS write never
stalls intake:
- **Drain goroutine**: reads `persistCh`, accumulates a batch; on `batchSize` (e.g. 200)
  or a **5s** ticker, hands the batch to a **bounded** `batchCh chan []*pb.LogEntry`
  (capacity ~64), then keeps draining. Never calls GFS. If `batchCh` is full (writer
  backed up), the send blocks → `persistCh` fills → the level filter's blocking send
  back-pressures gfslog. No Warn+ is lost.
- **Writer goroutine** (single, per-source-file ordered): pulls a batch, groups by
  `(date, source)`, and for each group calls `AppendWithNamespace` with **retry + capped
  exponential backoff** (1s→2s→4s→8s→…cap 30s), **retrying the same batch until it
  succeeds** (or shutdown) — never discarding. One writer keeps appends to each
  `/<date>/<source>.jsonl` serialized and ordered.

Enlarge `persistCh` 1000 → **10000** (ample for Warn+ volume; cheap headroom).

### 3. Retention GC
A goroutine runs once at startup, then every 24h:
- List the `core-logs` namespace (`ListFilesWithNamespace(ctx, "core-logs", "/")`).
- Parse the date from each path's first segment (`/YYYY-MM-DD/<source>.jsonl`).
- Delete every file whose date is strictly older than
  `time.Now().UTC().AddDate(0,0,-14)` via the SDK delete call.
- Emit one summary: `slog.Info("retention sweep", "deleted_files", n, "expired_days", d)`.
- A delete error logs a Warn and continues; the next sweep retries. Malformed/unparseable
  paths are skipped (never deleted).

## Data flow (persistence)

```
producer → gfslog → gRPC PushLog
   → ring buffer + broadcast + (Error→NATS)              [live, UNCHANGED — Debug+]
   → if Level >= WARN: enqueue persistCh(10k)            [archive path only]
       → drain goroutine → batch (200 or 5s) → batchCh(64)
           → writer goroutine → AppendWithNamespace (retry/backoff, per-file ordered) → GFS
GC goroutine (daily) → delete core-logs /<date>/ older than 14d
```

## Error handling

- **Debug/Info:** never enter the persistence path; unaffected.
- **GFS append fails:** writer retries the same batch with backoff indefinitely; never
  discards. Sustained failure → batchCh fills → drain blocks → persistCh fills → the
  Warn+ blocking enqueue back-pressures gfslog. **No Warn+ loss**, at the cost of
  back-pressure during a full outage.
- **Retention delete fails:** logged Warn, skipped, retried next sweep.
- **Shutdown:** drain flushes the pending batch to batchCh; writer makes a bounded final
  attempt then exits via the `done` channel (honors shutdown so termination can't hang).

## Components & boundaries

- **Level filter** — trivial guard at enqueue (`Level >= WARN`); unit-testable.
- **Drain goroutine** — batching only; depends on `persistCh`, `batchCh`, ticker.
- **Writer goroutine** — GFS I/O + retry/backoff + per-file ordering; depends on `batchCh`
  and the GFS client.
- **Retention sweeper** — date parsing + delete; depends on the GFS client and clock.

## Testing

- **Level filter:** Warn/Error/Fatal admitted; Debug/Info never enqueued.
- **Writer retry:** a GFS stub that fails N times then succeeds → batch eventually
  persisted, not lost; ordering within a source file preserved.
- **Backpressure:** with a blocked writer, Warn+ enqueue blocks (no loss) rather than
  dropping (fake clock / stub).
- **Retention cutoff:** `/2026-06-01/x.jsonl` older than 14d from a fixed "now" is
  deleted; `today` and `13-days-ago` are kept; malformed paths are skipped.

## Risks / notes

- Blocking enqueue for Warn+ can, in a total GFS outage, back-pressure gfslog's sender.
  Deliberate trade: we never lose error/security logs. Producers call log-service async,
  so this back-pressures the log shipper, not the service's request handling.
- Volume of Warn+ is small, so storage is a non-issue (~195 GB free per chunkserver;
  14-day retention caps it regardless).
- This stage intentionally makes the durable archive *sparser* than the live view (Warn+
  vs Debug+). Stage 2's value depends on emitting security-relevant events at **Warn+**
  so they reach this archive; that requirement is called out in the Stage 2 spec.
- The writer's indefinite retry must honor the shutdown `done` channel.

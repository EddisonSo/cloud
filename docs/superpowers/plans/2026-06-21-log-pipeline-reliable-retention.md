# Reliable Warn+ Log Archive + 14-Day Retention — Implementation Plan (Stage 1)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `log-service` reliably persist only `Warn+` records to GFS, never silently dropping them, and auto-delete archived logs older than 14 days — with no change to live/Debug behavior.

**Architecture:** Add a Warn+ filter at the persistence enqueue; split the inline persistence worker into a drain goroutine (batches, never touches GFS) and a single writer goroutine (GFS append with retry/backoff, per-file ordered) connected by a bounded `batchCh`; add a daily retention sweeper. All changes are in `log-service`.

**Tech Stack:** Go, `log/slog`, the go-gfs SDK (`AppendWithNamespace`, `ListFilesWithNamespace`, `DeleteFileWithNamespace`), `encoding/json`. Tests run with `GFS_JWT_SECRET=test-secret` (go-gfs `init()` guard).

## Global Constraints

- Persist **only `Level >= pb.LogLevel_WARN`**; Debug/Info are never enqueued for persistence.
- Warn+ is **never dropped**: enqueue blocks (with `done` escape) rather than dropping.
- Writer **retries the same batch until success or shutdown**; never discards on append error.
- One writer goroutine → appends to each `/<date>/<source>.jsonl` stay serialized/ordered.
- Retention cutoff: delete files strictly older than `now.UTC().AddDate(0,0,-14)`.
- No changes outside `log-service/` (producers, dashboard untouched in this stage).
- GFS namespace constant for logs: `"core-logs"`. Path layout: `/<YYYY-MM-DD>/<source>.jsonl`.

---

### Task 1: Warn+ filter at the persistence enqueue (never-drop)

**Files:**
- Modify: `log-service/internal/server/server.go` (the `PushLog` persistence block at `:188-194`; add `enqueuePersist` method)
- Test: `log-service/internal/server/persist_test.go` (new)

**Interfaces:**
- Produces: `func (s *LogServer) enqueuePersist(entry *pb.LogEntry)` — enqueues Warn+ to `persistCh` (blocking, with `done` escape); returns immediately for Debug/Info or when `gfsClient` is nil.

- [ ] **Step 1: Write the failing test**

```go
// log-service/internal/server/persist_test.go
package server

import (
	"testing"
	pb "github.com/eddisonso/log-service/pkg/pb/log"
)

func newTestServer(bufCap int) *LogServer {
	return &LogServer{
		buffers:     map[string]*RingBuffer{},
		sources:     map[string]struct{}{},
		subscribers: map[chan *pb.LogEntry]struct{}{},
		persistCh:   make(chan *pb.LogEntry, bufCap),
		done:        make(chan struct{}),
		gfsClient:   nil, // enqueue path is gated separately in tests below
	}
}

func TestEnqueuePersist_OnlyWarnPlus(t *testing.T) {
	s := newTestServer(10)
	s.persistEnabled = true // test seam: pretend GFS is configured
	cases := []struct {
		lvl     pb.LogLevel
		enqueue bool
	}{
		{pb.LogLevel_DEBUG, false},
		{pb.LogLevel_INFO, false},
		{pb.LogLevel_WARN, true},
		{pb.LogLevel_ERROR, true},
	}
	for _, c := range cases {
		s.enqueuePersist(&pb.LogEntry{Level: c.lvl, Source: "t", Message: "m"})
	}
	if got := len(s.persistCh); got != 2 {
		t.Fatalf("expected 2 enqueued (WARN+ERROR), got %d", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd log-service && GFS_JWT_SECRET=test-secret go test ./internal/server/ -run TestEnqueuePersist -v`
Expected: FAIL — `enqueuePersist` / `persistEnabled` undefined.

- [ ] **Step 3: Add the field + method, and call it from PushLog**

Add a `persistEnabled bool` field to the `LogServer` struct (set in `NewLogServer` to `gfsClient != nil` — this is the test seam so tests don't need a real client). Then add:

```go
// enqueuePersist queues Warn+ entries for durable persistence. Warn/Error/Fatal
// are never dropped: the send blocks (with a done escape) if the buffer is full,
// applying backpressure to the async log shipper rather than losing the entry.
// Debug/Info are intentionally not persisted.
func (s *LogServer) enqueuePersist(entry *pb.LogEntry) {
	if !s.persistEnabled || entry.Level < pb.LogLevel_WARN {
		return
	}
	select {
	case s.persistCh <- entry:
	case <-s.done:
	}
}
```

In `PushLog`, replace the existing block at `server.go:188-194`:

```go
	// Queue for persistence (non-blocking)
	if s.gfsClient != nil {
		select {
		case s.persistCh <- entry:
		default:
			// Channel full, drop (logs are best-effort persisted)
		}
	}
```

with:

```go
	// Durably persist Warn+ only (never dropped); Debug/Info stay live-only.
	s.enqueuePersist(entry)
```

In `NewLogServer`, set `persistEnabled: gfsClient != nil` in the struct literal.

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd log-service && GFS_JWT_SECRET=test-secret go test ./internal/server/ -run TestEnqueuePersist -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add log-service/internal/server/server.go log-service/internal/server/persist_test.go
git commit -m "feat(log-service): persist only Warn+ logs, never dropped"
```

---

### Task 2: Async decoupled writer with retry/backoff

**Files:**
- Modify: `log-service/internal/server/server.go` (refactor `persistenceWorker` `:430-489`; add `batchCh`/`appendFn` fields, `writerWorker`, `appendWithRetry`; bump `persistCh` cap; wire in `NewLogServer`)
- Test: `log-service/internal/server/persist_test.go` (extend)

**Interfaces:**
- Consumes: `enqueuePersist` (Task 1), `s.persistCh`, `s.done`.
- Produces:
  - field `appendFn func(ctx context.Context, path, namespace string, data []byte) (int, error)` (test seam; defaults to `gfsClient.AppendWithNamespace`).
  - field `batchCh chan []*pb.LogEntry`.
  - `func (s *LogServer) appendWithRetry(path string, data []byte)` — retries `appendFn` with capped backoff until success or `done`.
  - `func (s *LogServer) writerWorker()` — consumes `batchCh`, groups by `(date,source)`, writes via `appendWithRetry`.
  - `persistenceWorker` becomes the drain-only loop feeding `batchCh`.

- [ ] **Step 1: Write the failing test (retry succeeds after transient failures, ordering preserved)**

```go
func TestAppendWithRetry_RetriesThenSucceeds(t *testing.T) {
	s := newTestServer(10)
	s.persistEnabled = true
	var calls int
	var gotData []byte
	s.appendFn = func(_ context.Context, path, ns string, data []byte) (int, error) {
		calls++
		if calls < 3 {
			return 0, errors.New("gfs down")
		}
		gotData = append(gotData, data...)
		return len(data), nil
	}
	// shorten backoff for the test
	s.retryBase = time.Millisecond
	s.appendWithRetry("/2026-06-21/svc.jsonl", []byte("a\nb\n"))
	if calls != 3 {
		t.Fatalf("expected 3 attempts, got %d", calls)
	}
	if string(gotData) != "a\nb\n" {
		t.Fatalf("data mismatch: %q", gotData)
	}
}
```
(Add imports: `context`, `errors`, `time`.)

- [ ] **Step 2: Run to verify it fails**

Run: `cd log-service && GFS_JWT_SECRET=test-secret go test ./internal/server/ -run TestAppendWithRetry -v`
Expected: FAIL — `appendFn`/`appendWithRetry`/`retryBase` undefined.

- [ ] **Step 3: Implement fields, retry, writer, and split the worker**

Add fields to `LogServer`:
```go
	batchCh   chan []*pb.LogEntry
	appendFn  func(ctx context.Context, path, namespace string, data []byte) (int, error)
	retryBase time.Duration
```

In `NewLogServer`, set:
```go
		persistCh:   make(chan *pb.LogEntry, 10000),
		batchCh:     make(chan []*pb.LogEntry, 64),
		retryBase:   time.Second,
```
and after the struct literal (before `return s`), set `s.appendFn = nil` default wiring:
```go
	if gfsClient != nil {
		s.appendFn = gfsClient.AppendWithNamespace
		go s.persistenceWorker() // drain loop
		go s.writerWorker()      // GFS writer
	}
```
(Remove the old `go s.persistenceWorker()` block; it's replaced here.)

Add the retry helper (const namespace `"core-logs"`):
```go
func (s *LogServer) appendWithRetry(path string, data []byte) {
	const namespace = "core-logs"
	backoff := s.retryBase
	if backoff == 0 {
		backoff = time.Second
	}
	for {
		if _, err := s.appendFn(context.Background(), path, namespace, data); err == nil {
			return
		} else {
			slog.Warn("persist append failed, retrying", "path", path, "error", err, "backoff", backoff)
		}
		select {
		case <-time.After(backoff):
		case <-s.done:
			return
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}
```

Add the writer (moves the grouping/marshal logic out of the old `flush`):
```go
func (s *LogServer) writerWorker() {
	for {
		select {
		case batch := <-s.batchCh:
			groups := make(map[string][]*pb.LogEntry)
			for _, e := range batch {
				t := time.Unix(e.Timestamp, 0).UTC()
				key := fmt.Sprintf("/%s/%s.jsonl", t.Format("2006-01-02"), e.Source)
				groups[key] = append(groups[key], e)
			}
			for path, entries := range groups {
				var data []byte
				for _, e := range entries {
					line, err := json.Marshal(e)
					if err != nil {
						continue
					}
					data = append(data, line...)
					data = append(data, '\n')
				}
				if len(data) > 0 {
					s.appendWithRetry(path, data)
				}
			}
		case <-s.done:
			return
		}
	}
}
```

Replace `persistenceWorker` with the drain-only loop (batch then hand to `batchCh`):
```go
func (s *LogServer) persistenceWorker() {
	const (
		batchSize     = 200
		flushInterval = 5 * time.Second
	)
	batch := make([]*pb.LogEntry, 0, batchSize)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	hand := func() {
		if len(batch) == 0 {
			return
		}
		b := make([]*pb.LogEntry, len(batch))
		copy(b, batch)
		select {
		case s.batchCh <- b:
		case <-s.done:
		}
		batch = batch[:0]
	}

	for {
		select {
		case entry := <-s.persistCh:
			batch = append(batch, entry)
			if len(batch) >= batchSize {
				hand()
			}
		case <-ticker.C:
			hand()
		case <-s.done:
			hand()
			return
		}
	}
}
```
Ensure imports include `context` (add if missing).

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd log-service && GFS_JWT_SECRET=test-secret go test ./internal/server/ -v && go build ./...`
Expected: PASS; build OK.

- [ ] **Step 5: Commit**

```bash
git add log-service/internal/server/server.go log-service/internal/server/persist_test.go
git commit -m "feat(log-service): decouple GFS writer with retry/backoff, 10k buffer"
```

---

### Task 3: 14-day retention sweeper

**Files:**
- Modify: `log-service/internal/server/server.go` (add `deleteFn` field, `expiredLogPath` helper, `retentionWorker`; wire in `NewLogServer`)
- Test: `log-service/internal/server/retention_test.go` (new)

**Interfaces:**
- Produces:
  - `func expiredLogPath(path string, now time.Time, retentionDays int) bool` — pure; true if the path's `/<YYYY-MM-DD>/` is strictly older than `now - retentionDays`; false for malformed paths.
  - field `deleteFn func(ctx context.Context, path, namespace string) error` (defaults to `gfsClient.DeleteFileWithNamespace`).
  - `func (s *LogServer) retentionWorker()` — startup + every 24h sweep.

- [ ] **Step 1: Write the failing test**

```go
// log-service/internal/server/retention_test.go
package server

import (
	"testing"
	"time"
)

func TestExpiredLogPath(t *testing.T) {
	now := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		path string
		want bool
	}{
		{"/2026-06-01/edd-auth.jsonl", true},   // 20 days old > 14
		{"/2026-06-07/edd-auth.jsonl", false},  // exactly 14 days -> not strictly older
		{"/2026-06-08/edd-auth.jsonl", false},  // 13 days, keep
		{"/2026-06-21/edd-auth.jsonl", false},  // today, keep
		{"/garbage/x.jsonl", false},            // malformed, never delete
		{"not-a-path", false},
	}
	for _, c := range cases {
		if got := expiredLogPath(c.path, now, 14); got != c.want {
			t.Errorf("expiredLogPath(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd log-service && GFS_JWT_SECRET=test-secret go test ./internal/server/ -run TestExpiredLogPath -v`
Expected: FAIL — `expiredLogPath` undefined.

- [ ] **Step 3: Implement helper, worker, wiring**

```go
// expiredLogPath reports whether a core-logs file path ("/YYYY-MM-DD/source.jsonl")
// is strictly older than retentionDays before now. Malformed paths return false.
func expiredLogPath(path string, now time.Time, retentionDays int) bool {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 {
		return false
	}
	day, err := time.Parse("2006-01-02", parts[0])
	if err != nil {
		return false
	}
	cutoff := now.UTC().AddDate(0, 0, -retentionDays)
	return day.Before(cutoff)
}

func (s *LogServer) retentionWorker() {
	const namespace = "core-logs"
	sweep := func() {
		files, err := s.gfsClient.ListFilesWithNamespace(context.Background(), namespace, "/")
		if err != nil {
			slog.Warn("retention list failed", "error", err)
			return
		}
		deleted := 0
		for _, f := range files {
			if expiredLogPath(f.Path, time.Now(), 14) {
				if err := s.deleteFn(context.Background(), f.Path, namespace); err != nil {
					slog.Warn("retention delete failed", "path", f.Path, "error", err)
					continue
				}
				deleted++
			}
		}
		slog.Info("retention sweep", "deleted_files", deleted)
	}
	sweep()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			sweep()
		case <-s.done:
			return
		}
	}
}
```

Add `strings` to imports if missing. In `NewLogServer`, inside the `if gfsClient != nil` block, add:
```go
		s.deleteFn = gfsClient.DeleteFileWithNamespace
		go s.retentionWorker()
```
and add the field `deleteFn func(ctx context.Context, path, namespace string) error` to the struct.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd log-service && GFS_JWT_SECRET=test-secret go test ./internal/server/ -v && go build ./...`
Expected: PASS; build OK.

- [ ] **Step 5: Commit**

```bash
git add log-service/internal/server/server.go log-service/internal/server/retention_test.go
git commit -m "feat(log-service): 14-day retention sweep for archived logs"
```

---

### Task 4: Update docs + finalize

**Files:**
- Modify: `edd-cloud-docs/docs/services/logging.md` (Storage Model + retention)

- [ ] **Step 1: Update the logging doc**

In `edd-cloud-docs/docs/services/logging.md`, under the GFS Persistence section, change the description to state: only `Warn+` records are persisted to GFS (Debug/Info are live-only); Warn+ are never dropped (blocking enqueue + retry/backoff writer); and persisted logs are retained for **14 days** then auto-deleted by a daily sweep. Update the settings table (queue capacity 10000; add "Retention: 14 days"; "Persisted levels: WARN and above").

- [ ] **Step 2: Verify the docs build**

Run: `cd edd-cloud-docs && npm run build 2>&1 | tail -5`
Expected: build succeeds.

- [ ] **Step 3: Commit**

```bash
git add edd-cloud-docs/docs/services/logging.md
git commit -m "docs(log-service): document Warn+ persistence + 14-day retention"
```

---

## Self-Review

- **Spec coverage:** Warn+-only persistence (Task 1) ✓; never-drop blocking enqueue (Task 1) ✓; decoupled writer + retry/never-discard + per-file ordering (Task 2) ✓; 10k buffer (Task 2) ✓; 14-day retention GC (Task 3) ✓; no producer/dashboard changes ✓ (none in plan); docs (Task 4) ✓. Backpressure cascade is an emergent property of Tasks 1–2 (blocking enqueue + bounded batchCh + blocking hand-off).
- **Placeholders:** none — all steps contain concrete Go.
- **Type consistency:** `appendFn`/`deleteFn`/`appendWithRetry`/`writerWorker`/`persistenceWorker`/`enqueuePersist`/`expiredLogPath`/`persistEnabled`/`batchCh`/`retryBase` are defined once and used consistently. `pb.LogLevel_WARN`, `ListFilesWithNamespace(ctx, ns, prefix)`, `DeleteFileWithNamespace(ctx, path, ns)`, `AppendWithNamespace(ctx, path, ns, data)` match the real SDK signatures.

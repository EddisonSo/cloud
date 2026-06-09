package certstore

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// testStore opens a PostgresStorage against DATABASE_URL_TEST, skipping the
// test if the env var is unset. It registers cleanup that removes any rows
// created with the given key/lock prefix.
func testStore(t *testing.T) *PostgresStorage {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL_TEST")
	if dsn == "" {
		t.Skip("DATABASE_URL_TEST not set; skipping Postgres-backed tests")
	}
	s, err := New(dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() {
		// Remove test artifacts. Tests use the "test/" key prefix and
		// "test-lock" lock name prefix.
		s.db.Exec(`DELETE FROM certmagic_data WHERE key LIKE 'test/%'`)
		s.db.Exec(`DELETE FROM certmagic_locks WHERE name LIKE 'test-lock%'`)
		s.Close()
	})
	return s
}

func TestStoreLoadDeleteExists(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	key := "test/foo"
	val := []byte("hello world")

	if err := s.Store(ctx, key, val); err != nil {
		t.Fatalf("Store: %v", err)
	}

	if !s.Exists(ctx, key) {
		t.Fatalf("Exists: expected true after Store")
	}

	got, err := s.Load(ctx, key)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if string(got) != string(val) {
		t.Fatalf("Load: got %q, want %q", got, val)
	}

	// Overwrite.
	val2 := []byte("updated")
	if err := s.Store(ctx, key, val2); err != nil {
		t.Fatalf("Store overwrite: %v", err)
	}
	got, err = s.Load(ctx, key)
	if err != nil {
		t.Fatalf("Load after overwrite: %v", err)
	}
	if string(got) != string(val2) {
		t.Fatalf("Load after overwrite: got %q, want %q", got, val2)
	}

	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if s.Exists(ctx, key) {
		t.Fatalf("Exists: expected false after Delete")
	}
}

func TestLoadAfterDeleteNotExist(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	key := "test/gone"

	if err := s.Store(ctx, key, []byte("x")); err != nil {
		t.Fatalf("Store: %v", err)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := s.Load(ctx, key)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Load after delete: got %v, want fs.ErrNotExist", err)
	}

	// Delete again -> fs.ErrNotExist.
	if err := s.Delete(ctx, key); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Delete missing: got %v, want fs.ErrNotExist", err)
	}

	// Stat missing -> fs.ErrNotExist.
	if _, err := s.Stat(ctx, key); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Stat missing: got %v, want fs.ErrNotExist", err)
	}
}

func TestStat(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	key := "test/stat"
	val := []byte("abcdef")

	if err := s.Store(ctx, key, val); err != nil {
		t.Fatalf("Store: %v", err)
	}
	info, err := s.Stat(ctx, key)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Key != key {
		t.Fatalf("Stat: Key = %q, want %q", info.Key, key)
	}
	if info.Size != int64(len(val)) {
		t.Fatalf("Stat: Size = %d, want %d", info.Size, len(val))
	}
	if !info.IsTerminal {
		t.Fatalf("Stat: IsTerminal = false, want true")
	}
	if info.Modified.IsZero() {
		t.Fatalf("Stat: Modified is zero")
	}
}

func TestListPrefix(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	keys := []string{
		"test/list/a",
		"test/list/b",
		"test/list/sub/c",
	}
	for _, k := range keys {
		if err := s.Store(ctx, k, []byte("v")); err != nil {
			t.Fatalf("Store %q: %v", k, err)
		}
	}

	// Recursive: all keys present.
	got, err := s.List(ctx, "test/list/", true)
	if err != nil {
		t.Fatalf("List recursive: %v", err)
	}
	want := map[string]bool{
		"test/list/a":     true,
		"test/list/b":     true,
		"test/list/sub/c": true,
	}
	if len(got) != len(want) {
		t.Fatalf("List recursive: got %v, want keys %v", got, want)
	}
	for _, k := range got {
		if !want[k] {
			t.Fatalf("List recursive: unexpected key %q", k)
		}
	}

	// Non-recursive: collapse to direct children.
	got, err = s.List(ctx, "test/list/", false)
	if err != nil {
		t.Fatalf("List non-recursive: %v", err)
	}
	wantNR := map[string]bool{
		"test/list/a":   true,
		"test/list/b":   true,
		"test/list/sub": true,
	}
	if len(got) != len(wantNR) {
		t.Fatalf("List non-recursive: got %v, want %v", got, wantNR)
	}
	for _, k := range got {
		if !wantNR[k] {
			t.Fatalf("List non-recursive: unexpected key %q", k)
		}
	}
}

func TestLockUnlock(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	name := "test-lock-basic"

	if err := s.Lock(ctx, name); err != nil {
		t.Fatalf("Lock: %v", err)
	}
	if err := s.Unlock(ctx, name); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	// Re-lock after unlock should succeed quickly.
	if err := s.Lock(ctx, name); err != nil {
		t.Fatalf("Re-Lock: %v", err)
	}
	if err := s.Unlock(ctx, name); err != nil {
		t.Fatalf("Re-Unlock: %v", err)
	}
}

// TestLockMutualExclusion verifies that a second Lock on the same name (via the
// same store, simulating two callers) blocks until the first Unlocks, and that
// only one holder is in the critical section at a time.
func TestLockMutualExclusion(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	name := "test-lock-mutex"

	var inCritical int32
	var maxConcurrent int32
	done := make(chan struct{})

	// Goroutine 1 acquires first and holds for a bit.
	first := make(chan struct{})
	go func() {
		if err := s.Lock(ctx, name); err != nil {
			t.Errorf("g1 Lock: %v", err)
			close(first)
			close(done)
			return
		}
		close(first) // signal g2 to start trying
		atomic.AddInt32(&inCritical, 1)
		if c := atomic.LoadInt32(&inCritical); c > atomic.LoadInt32(&maxConcurrent) {
			atomic.StoreInt32(&maxConcurrent, c)
		}
		time.Sleep(500 * time.Millisecond)
		atomic.AddInt32(&inCritical, -1)
		if err := s.Unlock(ctx, name); err != nil {
			t.Errorf("g1 Unlock: %v", err)
		}
	}()

	<-first
	// Goroutine 2 tries to acquire; it must block until g1 unlocks.
	start := time.Now()
	go func() {
		if err := s.Lock(ctx, name); err != nil {
			t.Errorf("g2 Lock: %v", err)
			close(done)
			return
		}
		elapsed := time.Since(start)
		// g1 held the lock ~500ms; g2 should have waited a meaningful amount.
		if elapsed < 300*time.Millisecond {
			t.Errorf("g2 acquired lock too early (%v); mutual exclusion broken", elapsed)
		}
		atomic.AddInt32(&inCritical, 1)
		if c := atomic.LoadInt32(&inCritical); c > atomic.LoadInt32(&maxConcurrent) {
			atomic.StoreInt32(&maxConcurrent, c)
		}
		atomic.AddInt32(&inCritical, -1)
		if err := s.Unlock(ctx, name); err != nil {
			t.Errorf("g2 Unlock: %v", err)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("test timed out waiting for lock handoff")
	}

	if m := atomic.LoadInt32(&maxConcurrent); m > 1 {
		t.Fatalf("max concurrent lock holders = %d, want 1", m)
	}
}

func TestLockContextCancel(t *testing.T) {
	s := testStore(t)
	name := "test-lock-cancel"

	// Hold the lock.
	if err := s.Lock(context.Background(), name); err != nil {
		t.Fatalf("Lock: %v", err)
	}
	defer s.Unlock(context.Background(), name)

	// A second acquisition with a short-deadline context must fail rather than
	// block forever.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err := s.Lock(ctx, name)
	if err == nil {
		s.Unlock(context.Background(), name)
		t.Fatalf("Lock: expected context error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Lock: got %v, want context.DeadlineExceeded", err)
	}
}

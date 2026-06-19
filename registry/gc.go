package main

import (
	"context"
	"log/slog"
	"time"
)

// startGC launches the garbage collection loop in a background goroutine.
func (s *server) startGC(ctx context.Context, interval time.Duration) {
	go s.runGC(ctx, interval)
}

// runGC runs the garbage collection loop until ctx is cancelled.
// Each cycle performs three phases:
//  1. Sweep — delete blobs that were marked more than one interval ago.
//  2. Mark  — mark blobs that are not referenced by any manifest.
//  3. Clean — delete upload sessions older than 24 hours.
func (s *server) runGC(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	slog.Info("gc: started", "interval", interval)

	for {
		select {
		case <-ctx.Done():
			slog.Info("gc: stopping")
			return
		case <-ticker.C:
			s.gcCycle(ctx, interval)
		}
	}
}

// gcCycle runs a single GC cycle.
func (s *server) gcCycle(ctx context.Context, interval time.Duration) {
	// Phase 1: Sweep — remove blobs marked before (now - interval)
	sweepBefore := time.Now().Add(-interval)
	swept, err := sweepMarkedBlobs(ctx, s.db, sweepBefore)
	if err != nil {
		slog.Error("gc: sweepMarkedBlobs", "err", err)
	} else {
		slog.Info("gc: sweep phase", "blobs_swept", len(swept))
	}

	for _, blob := range swept {
		gfsPath := blobGFSPath(blob.Digest)
		if err := s.gfs.DeleteFileWithNamespace(ctx, gfsPath, gfsNamespace); err != nil {
			slog.Warn("gc: DeleteFileWithNamespace", "path", gfsPath, "err", err)
		}
	}

	// Phase 2: Mark — identify blobs not referenced by any manifest
	marked, err := markOrphanedBlobs(ctx, s.db)
	if err != nil {
		slog.Error("gc: markOrphanedBlobs", "err", err)
	} else {
		slog.Info("gc: mark phase", "blobs_marked", marked)
	}

	// Phase 3: Clean stale upload sessions (older than 24 hours)
	deleted, err := deleteStaleUploadSessions(ctx, s.db, 24*time.Hour)
	if err != nil {
		slog.Error("gc: deleteStaleUploadSessions", "err", err)
	} else {
		slog.Info("gc: clean stale uploads", "sessions_deleted", deleted)
	}
}

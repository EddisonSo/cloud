package server

import (
	"context"
	"errors"
	"testing"
	"time"

	pb "github.com/eddisonso/log-service/proto/logging"
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
		s.enqueuePersist(context.Background(), &pb.LogEntry{Level: c.lvl, Source: "t", Message: "m"})
	}
	if got := len(s.persistCh); got != 2 {
		t.Fatalf("expected 2 enqueued (WARN+ERROR), got %d", got)
	}
}

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

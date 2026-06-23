package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	gfs "eddisonso.com/go-gfs/pkg/go-gfs-sdk"
	pbcommon "github.com/eddisonso/log-service/pkg/pb/common"
	pblog "github.com/eddisonso/log-service/pkg/pb/log"
	pb "github.com/eddisonso/log-service/proto/logging"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
)

const bufferSize = 1000

// sourceRe restricts Source to safe path components so it can never traverse
// directories when used as the filename segment in a GFS path.
var sourceRe = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,128}$`)

// persistEnqueueTimeout bounds how long PushLog will block waiting for a slot in
// persistCh. Under a sustained GFS outage the channel fills; this cap lets the
// handler shed load gracefully instead of parking goroutines indefinitely.
const persistEnqueueTimeout = 2 * time.Second

// bufferKey creates a unique key for source+level combination
func bufferKey(source string, level pb.LogLevel) string {
	return source + ":" + level.String()
}

// RingBuffer is a simple circular buffer for log entries
type RingBuffer struct {
	entries []*pb.LogEntry
	head    int
	count   int
	mu      sync.RWMutex
}

func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		entries: make([]*pb.LogEntry, size),
	}
}

func (r *RingBuffer) Add(entry *pb.LogEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()

	idx := (r.head + r.count) % len(r.entries)
	r.entries[idx] = entry
	if r.count < len(r.entries) {
		r.count++
	} else {
		r.head = (r.head + 1) % len(r.entries)
	}
}

func (r *RingBuffer) GetAll() []*pb.LogEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*pb.LogEntry, 0, r.count)
	for i := 0; i < r.count; i++ {
		idx := (r.head + i) % len(r.entries)
		result = append(result, r.entries[idx])
	}
	return result
}

type LogServer struct {
	pb.UnimplementedLogServiceServer

	// Per-source, per-level ring buffers
	buffers   map[string]*RingBuffer
	buffersMu sync.RWMutex

	// Track known sources for enumeration
	sources   map[string]struct{}
	sourcesMu sync.RWMutex

	// WebSocket subscribers
	subscribers   map[chan *pb.LogEntry]struct{}
	subscribersMu sync.RWMutex

	// GFS client for persistence (optional)
	gfsClient *gfs.Client

	// NATS JetStream for publishing error logs
	js jetstream.JetStream

	// Channel for async persistence
	persistCh chan *pb.LogEntry

	// Test seam: true when GFS is configured (set to gfsClient != nil in NewLogServer)
	persistEnabled bool

	// Buffered channel connecting drain loop to writer goroutine
	batchCh chan []*pb.LogEntry

	// Test seam: injected append function (defaults to gfsClient.AppendWithNamespace)
	appendFn func(ctx context.Context, path, namespace string, data []byte) (int, error)

	// Test seam: injected delete function (defaults to gfsClient.DeleteFileWithNamespace)
	deleteFn func(ctx context.Context, path, namespace string) error

	// Base retry interval for appendWithRetry (shortened in tests)
	retryBase time.Duration

	// For graceful shutdown
	done chan struct{}
}

func NewLogServer(gfsClient *gfs.Client, js jetstream.JetStream) *LogServer {
	s := &LogServer{
		buffers:        make(map[string]*RingBuffer),
		sources:        make(map[string]struct{}),
		subscribers:    make(map[chan *pb.LogEntry]struct{}),
		gfsClient:      gfsClient,
		js:             js,
		persistCh:      make(chan *pb.LogEntry, 10000),
		batchCh:        make(chan []*pb.LogEntry, 64),
		retryBase:      time.Second,
		persistEnabled: gfsClient != nil,
		done:           make(chan struct{}),
	}

	if gfsClient != nil {
		s.appendFn = gfsClient.AppendWithNamespace
		s.deleteFn = gfsClient.DeleteFileWithNamespace
		go s.persistenceWorker() // drain loop
		go s.writerWorker()      // GFS writer
		go s.retentionWorker()
	}

	return s
}

func (s *LogServer) Close() {
	close(s.done)
}

// getOrCreateBuffer gets or creates a ring buffer for source+level
func (s *LogServer) getOrCreateBuffer(source string, level pb.LogLevel) *RingBuffer {
	key := bufferKey(source, level)

	s.buffersMu.RLock()
	buf, exists := s.buffers[key]
	s.buffersMu.RUnlock()

	if exists {
		return buf
	}

	s.buffersMu.Lock()
	defer s.buffersMu.Unlock()

	// Double-check after acquiring write lock
	if buf, exists = s.buffers[key]; exists {
		return buf
	}

	buf = NewRingBuffer(bufferSize)
	s.buffers[key] = buf
	return buf
}

// trackSource adds a source to the known sources set
func (s *LogServer) trackSource(source string) {
	s.sourcesMu.Lock()
	s.sources[source] = struct{}{}
	s.sourcesMu.Unlock()
}

// GetSources returns all known log sources
func (s *LogServer) GetSources() []string {
	s.sourcesMu.RLock()
	defer s.sourcesMu.RUnlock()

	result := make([]string, 0, len(s.sources))
	for src := range s.sources {
		result = append(result, src)
	}
	return result
}

// PushLog receives a log entry from a service
func (s *LogServer) PushLog(ctx context.Context, req *pb.PushLogRequest) (*pb.PushLogResponse, error) {
	if req.Entry == nil {
		return &pb.PushLogResponse{}, nil
	}

	entry := req.Entry
	if entry.Timestamp == 0 {
		entry.Timestamp = time.Now().Unix()
	}

	// Sanitize Source before it is used as a GFS path component.
	// Reject anything that could traverse directories or produce unexpected paths.
	if !sourceRe.MatchString(entry.Source) {
		entry.Source = "invalid-source"
	}

	// Track source
	s.trackSource(entry.Source)

	// Add to appropriate ring buffer
	buf := s.getOrCreateBuffer(entry.Source, entry.Level)
	buf.Add(entry)

	// Broadcast to subscribers
	s.broadcast(entry)

	// Publish error+ logs to NATS for alerting
	if entry.Level >= pb.LogLevel_ERROR && s.js != nil {
		go s.publishLogError(entry)
	}

	// Durably persist Warn+ only; Debug/Info stay live-only.
	s.enqueuePersist(ctx, entry)

	return &pb.PushLogResponse{}, nil
}

// StreamLogs streams log entries to clients (gRPC)
func (s *LogServer) StreamLogs(req *pb.StreamLogsRequest, stream pb.LogService_StreamLogsServer) error {
	ch := make(chan *pb.LogEntry, 100)

	// Register subscriber
	s.subscribersMu.Lock()
	s.subscribers[ch] = struct{}{}
	s.subscribersMu.Unlock()

	defer func() {
		s.subscribersMu.Lock()
		delete(s.subscribers, ch)
		s.subscribersMu.Unlock()
		close(ch)
	}()

	// Send recent entries from ring buffers first
	recent := s.getRecentEntries(req.Source, req.MinLevel)
	for _, entry := range recent {
		if err := stream.Send(entry); err != nil {
			return err
		}
	}

	// Stream new entries
	for {
		select {
		case entry, ok := <-ch:
			if !ok {
				return nil
			}
			if matchesFilter(entry, req.Source, req.MinLevel) {
				if err := stream.Send(entry); err != nil {
					return err
				}
			}
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
}

// GetLogs returns historical log entries
func (s *LogServer) GetLogs(ctx context.Context, req *pb.GetLogsRequest) (*pb.GetLogsResponse, error) {
	entries := s.getRecentEntries(req.Source, req.MinLevel)

	// Apply since filter
	if req.Since > 0 {
		filtered := make([]*pb.LogEntry, 0)
		for _, e := range entries {
			if e.Timestamp >= req.Since {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	// Apply limit
	if req.Limit > 0 && int(req.Limit) < len(entries) {
		entries = entries[len(entries)-int(req.Limit):]
	}

	return &pb.GetLogsResponse{Entries: entries}, nil
}

// Subscribe creates a channel for receiving log entries (used by WebSocket handler)
func (s *LogServer) Subscribe(source string, minLevel pb.LogLevel) (<-chan *pb.LogEntry, func()) {
	// Large buffer to hold all initial entries without dropping
	ch := make(chan *pb.LogEntry, 10000)
	done := make(chan struct{})

	s.subscribersMu.Lock()
	s.subscribers[ch] = struct{}{}
	s.subscribersMu.Unlock()

	unsubscribe := func() {
		s.subscribersMu.Lock()
		delete(s.subscribers, ch)
		s.subscribersMu.Unlock()
		close(done)
	}

	// Send recent entries in background
	go func() {
		recent := s.getRecentEntries(source, minLevel)
		for _, entry := range recent {
			select {
			case ch <- entry:
			case <-done:
				return // Subscriber disconnected
			}
		}
	}()

	return ch, unsubscribe
}

// getRecentEntries retrieves entries from ring buffers matching the filter
func (s *LogServer) getRecentEntries(source string, minLevel pb.LogLevel) []*pb.LogEntry {
	s.buffersMu.RLock()
	defer s.buffersMu.RUnlock()

	var entries []*pb.LogEntry

	// Collect from matching buffers
	for _, buf := range s.buffers {
		bufEntries := buf.GetAll()
		for _, entry := range bufEntries {
			if matchesFilter(entry, source, minLevel) {
				entries = append(entries, entry)
			}
		}
	}

	// Sort by timestamp
	sortByTimestamp(entries)

	return entries
}

func (s *LogServer) broadcast(entry *pb.LogEntry) {
	s.subscribersMu.RLock()
	defer s.subscribersMu.RUnlock()

	for ch := range s.subscribers {
		select {
		case ch <- entry:
		default:
			// Drop if subscriber is slow
		}
	}
}

func matchesFilter(entry *pb.LogEntry, source string, minLevel pb.LogLevel) bool {
	if entry == nil {
		return false
	}
	if source != "" && entry.Source != source {
		return false
	}
	if entry.Level < minLevel {
		return false
	}
	return true
}

// sortByTimestamp sorts entries by timestamp (oldest first)
func sortByTimestamp(entries []*pb.LogEntry) {
	// Simple insertion sort - entries are mostly sorted already
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].Timestamp < entries[j-1].Timestamp; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

// publishLogError publishes an error log entry to NATS JetStream
func (s *LogServer) publishLogError(entry *pb.LogEntry) {
	logErr := &pblog.LogError{
		Metadata: &pbcommon.EventMetadata{
			EventId:   generateUUID(),
			Timestamp: &pbcommon.Timestamp{Seconds: entry.Timestamp},
			Source:    "log-service",
		},
		Source:  entry.Source,
		Message: entry.Message,
		Level:   entry.Level.String(),
	}
	data, err := proto.Marshal(logErr)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	subject := fmt.Sprintf("log.error.%s", entry.Source)
	if _, err := s.js.Publish(ctx, subject, data); err != nil {
		slog.Error("failed to publish log error to NATS", "error", err, "source", entry.Source)
	}
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// FetchDayLogs reads all persisted log entries for the given date (YYYY-MM-DD)
// from GFS, merges entries from all source files, and returns them sorted
// ascending by timestamp.
//
// If a single source file is unreadable (e.g. chunkserver down), a WARN is
// logged and the remaining sources are still included (partial-read tolerance).
// Returns an error only if GFS is unavailable or the directory listing fails.
func (s *LogServer) FetchDayLogs(ctx context.Context, date string) ([]*pb.LogEntry, error) {
	const namespace = "core-logs"

	if s.gfsClient == nil {
		return nil, fmt.Errorf("GFS client not available")
	}

	prefix := "/" + date + "/"
	files, err := s.gfsClient.ListFilesWithNamespace(ctx, namespace, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list log files for %s: %w", date, err)
	}

	var all []*pb.LogEntry
	for _, f := range files {
		data, err := s.gfsClient.ReadWithNamespace(ctx, f.Path, namespace)
		if err != nil {
			slog.Warn("failed to read log file for download", "path", f.Path, "error", err)
			continue
		}

		for _, line := range bytes.Split(data, []byte("\n")) {
			if len(line) == 0 {
				continue
			}
			var entry pb.LogEntry
			if err := json.Unmarshal(line, &entry); err != nil {
				continue
			}
			all = append(all, &entry)
		}
	}

	sortByTimestamp(all)
	return all, nil
}

// enqueuePersist queues Warn+ entries for durable persistence. Debug/Info are
// intentionally not persisted. Under a sustained GFS outage the channel fills;
// the handler sheds the entry (with a Warn log) after persistEnqueueTimeout so
// gRPC goroutines never park indefinitely. In normal operation the channel has
// ample headroom and the timeout is never hit.
func (s *LogServer) enqueuePersist(ctx context.Context, entry *pb.LogEntry) {
	if !s.persistEnabled {
		return
	}
	if entry.Level < pb.LogLevel_WARN && entry.Attributes["audit"] != "true" {
		return
	}
	select {
	case s.persistCh <- entry:
	case <-ctx.Done():
	case <-time.After(persistEnqueueTimeout):
		slog.Warn("persist enqueue timeout, shedding entry under GFS backpressure", "source", entry.Source)
	case <-s.done:
	}
}

// persistenceWorker is the drain-only loop: it reads from persistCh, batches
// entries, and hands each batch to batchCh for the writer goroutine.
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

// appendWithRetry retries appendFn with capped exponential backoff until success
// or shutdown. The same batch is never discarded on error.
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

// writerWorker consumes batches from batchCh, groups entries by (date, source),
// and appends each group to GFS via appendWithRetry.
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

// retentionWorker runs a sweep on startup and every 24h, deleting log files
// strictly older than 14 days from the core-logs namespace.
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

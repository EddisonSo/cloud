package main

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pb "github.com/eddisonso/log-service/proto/logging"
)

// ---------------------------------------------------------------------------
// formatLogLine
// ---------------------------------------------------------------------------

func TestFormatLogLine(t *testing.T) {
	// Pin a known UTC timestamp so the expected string is deterministic.
	ts := time.Date(2026, 6, 20, 12, 34, 56, 0, time.UTC).Unix()
	entry := &pb.LogEntry{
		Source:    "edd-gateway",
		Level:     pb.LogLevel_ERROR,
		Message:   "connection refused",
		Timestamp: ts,
	}
	got := formatLogLine(entry)
	want := "2026-06-20 12:34:56  ERROR  edd-gateway  connection refused\n"
	if got != want {
		t.Errorf("formatLogLine =\n  %q\nwant\n  %q", got, want)
	}
}

func TestFormatLogLine_AllLevels(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	cases := []struct {
		level pb.LogLevel
		want  string
	}{
		{pb.LogLevel_DEBUG, "DEBUG"},
		{pb.LogLevel_INFO, "INFO"},
		{pb.LogLevel_WARN, "WARN"},
		{pb.LogLevel_ERROR, "ERROR"},
	}
	for _, c := range cases {
		e := &pb.LogEntry{Source: "svc", Level: c.level, Message: "msg", Timestamp: ts}
		got := formatLogLine(e)
		if fmt.Sprintf("2026-01-01 00:00:00  %s  svc  msg\n", c.want) != got {
			t.Errorf("level %s: got %q", c.want, got)
		}
	}
}

// ---------------------------------------------------------------------------
// date validation
// ---------------------------------------------------------------------------

func TestDoHandleDownload_DateValidation(t *testing.T) {
	// A fetcher that always errors so we never depend on GFS.
	errFetch := func(_ context.Context, _ string) ([]*pb.LogEntry, error) {
		return nil, fmt.Errorf("no GFS in test")
	}

	// Invalid dates must all produce 400.
	invalid := []string{"2026-13-99", "garbage", "", "20260620", "2026/06/20"}
	for _, bad := range invalid {
		req := httptest.NewRequest(http.MethodGet, "/logs/download?date="+bad, nil)
		rr := httptest.NewRecorder()
		doHandleDownload(rr, req, errFetch)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("date %q: got %d, want 400", bad, rr.Code)
		}
	}

	// A valid date must not return 400 (it will return 502 because errFetch errors).
	validDates := []string{"2026-06-20", "2000-01-01", "1999-12-31"}
	for _, good := range validDates {
		req := httptest.NewRequest(http.MethodGet, "/logs/download?date="+good, nil)
		rr := httptest.NewRecorder()
		doHandleDownload(rr, req, errFetch)
		if rr.Code == http.StatusBadRequest {
			t.Errorf("valid date %q: got 400, should not be 400", good)
		}
	}
}

// ---------------------------------------------------------------------------
// empty day → 404
// ---------------------------------------------------------------------------

func TestDoHandleDownload_EmptyDay(t *testing.T) {
	emptyFetch := func(_ context.Context, _ string) ([]*pb.LogEntry, error) {
		return nil, nil // no entries, no error
	}

	req := httptest.NewRequest(http.MethodGet, "/logs/download?date=2026-06-20", nil)
	rr := httptest.NewRecorder()
	doHandleDownload(rr, req, emptyFetch)

	if rr.Code != http.StatusNotFound {
		t.Errorf("empty day: got %d, want 404", rr.Code)
	}
}

func TestDoHandleDownload_EmptySlice(t *testing.T) {
	emptyFetch := func(_ context.Context, _ string) ([]*pb.LogEntry, error) {
		return []*pb.LogEntry{}, nil // explicitly empty slice
	}

	req := httptest.NewRequest(http.MethodGet, "/logs/download?date=2026-06-20", nil)
	rr := httptest.NewRecorder()
	doHandleDownload(rr, req, emptyFetch)

	if rr.Code != http.StatusNotFound {
		t.Errorf("empty slice: got %d, want 404", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// zip contains exactly the two expected member names
// ---------------------------------------------------------------------------

func TestDoHandleDownload_ZipMemberNames(t *testing.T) {
	ts := time.Date(2026, 6, 20, 9, 0, 0, 0, time.UTC).Unix()
	fakeFetch := func(_ context.Context, _ string) ([]*pb.LogEntry, error) {
		return []*pb.LogEntry{
			{Source: "svc-a", Level: pb.LogLevel_INFO, Message: "boot", Timestamp: ts},
			{Source: "svc-b", Level: pb.LogLevel_WARN, Message: "slow", Timestamp: ts + 1},
		}, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/logs/download?date=2026-06-20", nil)
	rr := httptest.NewRecorder()
	doHandleDownload(rr, req, fakeFetch)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify Content-Type and Content-Disposition headers.
	if ct := rr.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", ct)
	}
	wantCD := `attachment; filename="edd-cloud-logs-2026-06-20.zip"`
	if cd := rr.Header().Get("Content-Disposition"); cd != wantCD {
		t.Errorf("Content-Disposition = %q, want %q", cd, wantCD)
	}

	// Open the zip and check member names.
	body := rr.Body.Bytes()
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("cannot open response as zip: %v", err)
	}

	wantNames := []string{
		"edd-cloud-logs-2026-06-20.log",
		"edd-cloud-logs-2026-06-20.jsonl",
	}
	if len(zr.File) != len(wantNames) {
		t.Fatalf("zip has %d members, want %d", len(zr.File), len(wantNames))
	}
	for i, f := range zr.File {
		if f.Name != wantNames[i] {
			t.Errorf("zip member[%d] = %q, want %q", i, f.Name, wantNames[i])
		}
	}
}

// ---------------------------------------------------------------------------
// GFS error → 502
// ---------------------------------------------------------------------------

func TestDoHandleDownload_GFSError(t *testing.T) {
	errFetch := func(_ context.Context, _ string) ([]*pb.LogEntry, error) {
		return nil, fmt.Errorf("chunkserver unreachable")
	}

	req := httptest.NewRequest(http.MethodGet, "/logs/download?date=2026-06-20", nil)
	rr := httptest.NewRecorder()
	doHandleDownload(rr, req, errFetch)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("GFS error: got %d, want 502", rr.Code)
	}
}

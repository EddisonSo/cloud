package server

import (
	"testing"

	pb "github.com/eddisonso/log-service/proto/logging"
)

// TestSortByTimestamp_MultiSource verifies that entries from multiple sources
// with interleaved timestamps are merged into a single globally-ascending slice.
func TestSortByTimestamp_MultiSource(t *testing.T) {
	entries := []*pb.LogEntry{
		{Source: "svc-a", Timestamp: 300},
		{Source: "svc-b", Timestamp: 100},
		{Source: "svc-a", Timestamp: 200},
		{Source: "svc-b", Timestamp: 400},
		{Source: "svc-a", Timestamp: 150},
	}

	sortByTimestamp(entries)

	want := []int64{100, 150, 200, 300, 400}
	if len(entries) != len(want) {
		t.Fatalf("got %d entries, want %d", len(entries), len(want))
	}
	for i, e := range entries {
		if e.Timestamp != want[i] {
			t.Errorf("entry[%d].Timestamp = %d, want %d", i, e.Timestamp, want[i])
		}
	}
}

// TestSortByTimestamp_AlreadySorted confirms that an already-sorted slice is
// left unchanged (insertion sort is stable for pre-sorted input).
func TestSortByTimestamp_AlreadySorted(t *testing.T) {
	entries := []*pb.LogEntry{
		{Source: "svc-a", Timestamp: 1},
		{Source: "svc-b", Timestamp: 2},
		{Source: "svc-a", Timestamp: 3},
	}

	sortByTimestamp(entries)

	for i := 1; i < len(entries); i++ {
		if entries[i].Timestamp < entries[i-1].Timestamp {
			t.Errorf("entry[%d] (%d) < entry[%d] (%d): not sorted ascending",
				i, entries[i].Timestamp, i-1, entries[i-1].Timestamp)
		}
	}
}

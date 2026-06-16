package cli

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestDone(t *testing.T) {
	// On failure, done must return the error unchanged (so the top-level
	// handler prints it) and print nothing.
	boom := errors.New("boom")
	if got := done(boom, "deleted %s", "x"); got != boom {
		t.Errorf("done(err) = %v, want the original error", got)
	}

	// On success, done prints the confirmation and returns nil.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := done(nil, "deleted token %s", "abc123")
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	if err != nil {
		t.Fatalf("done(nil) = %v, want nil", err)
	}
	if !strings.Contains(string(out), "deleted token abc123") {
		t.Errorf("expected confirmation message, got %q", out)
	}
}

package cli

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestBuildCli2faURL_NoCallback(t *testing.T) {
	got := buildCli2faURL("cloud.eddisonso.com", "abc.def", 0, "")
	want := "https://cloud.eddisonso.com/cli-2fa?challenge=abc.def"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestBuildCli2faURL_WithCallback(t *testing.T) {
	got := buildCli2faURL("cloud.eddisonso.com", "a+b/c", 54221, "deadbeef")
	if !strings.Contains(got, "challenge=a%2Bb%2Fc") {
		t.Errorf("challenge not escaped in %q", got)
	}
	if !strings.Contains(got, "127.0.0.1%3A54221") {
		t.Errorf("cb host:port not present/escaped in %q", got)
	}
	if !strings.Contains(got, "state%3Ddeadbeef") {
		t.Errorf("state not present/escaped in cb in %q", got)
	}
}

func TestIsSSHSession(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_TTY", "")
	if isSSHSession() {
		t.Error("expected non-SSH when both unset")
	}
	t.Setenv("SSH_CONNECTION", "10.0.0.1 22 10.0.0.2 22")
	if !isSSHSession() {
		t.Error("expected SSH when SSH_CONNECTION set")
	}
	// SSH_TTY alone is also sufficient evidence.
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_TTY", "/dev/pts/0")
	if !isSSHSession() {
		t.Error("expected SSH when SSH_TTY set")
	}
}

func TestCallbackListenerReceivesToken(t *testing.T) {
	port, state, tokenCh, stop, err := startCallbackListener()
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	resp, err := http.Post(
		"http://127.0.0.1:"+strconv.Itoa(port)+"?state="+state, "text/plain", strings.NewReader("sess-token-xyz"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	select {
	case got := <-tokenCh:
		if got != "sess-token-xyz" {
			t.Errorf("got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for callback token")
	}
}

func TestCallbackListenerRejectsBadState(t *testing.T) {
	port, _, tokenCh, stop, err := startCallbackListener()
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	resp, err := http.Post(
		"http://127.0.0.1:"+strconv.Itoa(port)+"?state=wrong", "text/plain", strings.NewReader("attacker-token"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}

	select {
	case tok := <-tokenCh:
		t.Errorf("should not have received token, got %q", tok)
	case <-time.After(200 * time.Millisecond):
		// pass — nothing arrived on the channel
	}
}

func TestAwaitToken_PasteWins(t *testing.T) {
	cbCh := make(chan string) // never fires
	pasteCh := make(chan string, 1)
	pasteCh <- "pasted-token"
	got, err := awaitToken(cbCh, pasteCh, time.Second)
	if err != nil || got != "pasted-token" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestAwaitToken_Timeout(t *testing.T) {
	cbCh := make(chan string)
	pasteCh := make(chan string)
	_, err := awaitToken(cbCh, pasteCh, 10*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

package cli

import (
	"strings"
	"testing"
)

func TestBuildCli2faURL_NoCallback(t *testing.T) {
	got := buildCli2faURL("cloud.eddisonso.com", "abc.def", 0)
	want := "https://cloud.eddisonso.com/cli-2fa?challenge=abc.def"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestBuildCli2faURL_WithCallback(t *testing.T) {
	got := buildCli2faURL("cloud.eddisonso.com", "a+b/c", 54221)
	if !strings.Contains(got, "challenge=a%2Bb%2Fc") {
		t.Errorf("challenge not escaped in %q", got)
	}
	if !strings.Contains(got, "cb=http%3A%2F%2F127.0.0.1%3A54221") {
		t.Errorf("cb not present/escaped in %q", got)
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

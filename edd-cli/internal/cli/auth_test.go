package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLogoutClearsToken(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	os.WriteFile(p, []byte(`{"token":"x","base_domain":"cloud.eddisonso.com"}`), 0o600)
	if err := doLogout(p); err != nil {
		t.Fatal(err)
	}
	if loadConfig(p).Token != "" {
		t.Error("token not cleared")
	}
	// base domain preserved
	if loadConfig(p).BaseDomain != "cloud.eddisonso.com" {
		t.Error("base domain should be preserved")
	}
}

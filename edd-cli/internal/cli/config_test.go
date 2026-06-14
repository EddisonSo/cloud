package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveToken_Precedence(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	os.WriteFile(cfgPath, []byte(`{"token":"from_file","base_domain":"cloud.eddisonso.com"}`), 0o600)

	t.Setenv("EDD_TOKEN", "from_env")
	if tok, _ := resolveToken("", cfgPath); tok != "from_env" {
		t.Errorf("env should win, got %q", tok)
	}
	if tok, _ := resolveToken("from_flag", cfgPath); tok != "from_flag" {
		t.Errorf("flag should win, got %q", tok)
	}
	os.Unsetenv("EDD_TOKEN")
	if tok, _ := resolveToken("", cfgPath); tok != "from_file" {
		t.Errorf("file fallback, got %q", tok)
	}
}

package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"eddisonso.com/edd-cli/pkg/eddsdk"
)

func TestIsAuthError(t *testing.T) {
	if !isAuthError(&eddsdk.APIError{Status: 401}) {
		t.Error("401 should be an auth error")
	}
	if !isAuthError(&eddsdk.APIError{Status: 403}) {
		t.Error("403 should be an auth error")
	}
	if isAuthError(&eddsdk.APIError{Status: 500}) {
		t.Error("500 should not be an auth error")
	}
	if isAuthError(errors.New("network down")) {
		t.Error("plain error should not be an auth error")
	}
}

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

func TestScopeMeRewrite(t *testing.T) {
	in := map[string][]string{
		"networking.me.domains":            {"read"},
		"networking.me.domain-mappings.m1": {"read", "delete"},
		"compute.me":                       {"read"},
		"storage.BINrYi.files":             {"read"}, // already-qualified, untouched
	}
	if !scopesUseMe(in) {
		t.Fatal("scopesUseMe should be true")
	}
	out := rewriteScopeMe(in, "BINrYi")
	want := []string{
		"networking.BINrYi.domains",
		"networking.BINrYi.domain-mappings.m1",
		"compute.BINrYi",
		"storage.BINrYi.files",
	}
	for _, k := range want {
		if _, ok := out[k]; !ok {
			t.Errorf("missing rewritten key %q (got %v)", k, out)
		}
	}
	if scopesUseMe(out) {
		t.Error("no 'me' should remain after rewrite")
	}
	// A fully-qualified-only set needs no expansion.
	if scopesUseMe(map[string][]string{"compute.u1.containers": {"read"}}) {
		t.Error("scopesUseMe should be false when no 'me' present")
	}
}

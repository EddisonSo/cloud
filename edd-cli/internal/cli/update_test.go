package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAssetName(t *testing.T) {
	want := "ec_" + runtime.GOOS + "_" + runtime.GOARCH
	if got := assetName(); got != want {
		t.Errorf("assetName() = %q, want %q", got, want)
	}
}

func TestPickRelease(t *testing.T) {
	releases := []ghRelease{
		// Newest first (as the API returns). A draft must be skipped even though
		// it carries the asset.
		{TagName: "ec-draft", Draft: true, Assets: []ghAsset{{Name: "ec_linux_amd64", URL: "draft"}}},
		{TagName: "ec-newest", Assets: []ghAsset{
			{Name: "ec_linux_amd64", URL: "u-amd64"},
			{Name: "ec_darwin_arm64", URL: "u-darwin"},
		}},
		{TagName: "ec-older", Assets: []ghAsset{{Name: "ec_linux_amd64", URL: "old"}}},
	}

	tag, url, ok := pickRelease(releases, "ec_linux_amd64")
	if !ok || tag != "ec-newest" || url != "u-amd64" {
		t.Errorf("got tag=%q url=%q ok=%v, want ec-newest/u-amd64/true", tag, url, ok)
	}

	// An asset the releases don't carry → not found.
	if _, _, ok := pickRelease(releases, "ec_windows_arm64"); ok {
		t.Error("expected not-found for an unpublished platform asset")
	}
}

func TestReplaceBinary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ec")
	if err := os.WriteFile(path, []byte("OLD"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := replaceBinary(path, []byte("NEW-BINARY")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "NEW-BINARY" {
		t.Errorf("content = %q, want NEW-BINARY", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("replaced binary is not executable: mode %v", info.Mode())
	}
	// No stray temp files left behind in the directory.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("expected only the replaced binary, found %d entries", len(entries))
	}
}

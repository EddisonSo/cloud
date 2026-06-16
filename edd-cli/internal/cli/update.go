package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"eddisonso.com/edd-cli/pkg/eddsdk"
)

func init() { register(command{name: "update", run: cmdUpdate}) }

// ec is published as GitHub Releases on the (public) monorepo; release assets
// are named ec_<goos>_<goarch>. The repo being public means downloads need no
// auth.
const releasesAPI = "https://api.github.com/repos/EddisonSo/cloud/releases"

// assetName is the release asset for the running platform, e.g. ec_linux_amd64.
func assetName() string {
	return fmt.Sprintf("ec_%s_%s", runtime.GOOS, runtime.GOARCH)
}

type ghAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName    string    `json:"tag_name"`
	Draft      bool      `json:"draft"`
	Prerelease bool      `json:"prerelease"`
	Assets     []ghAsset `json:"assets"`
}

// pickRelease returns the newest release (the API lists newest first) that
// carries an asset named `asset`, along with that asset's download URL.
func pickRelease(releases []ghRelease, asset string) (tag, url string, ok bool) {
	for _, rel := range releases {
		if rel.Draft || rel.Prerelease {
			continue
		}
		for _, a := range rel.Assets {
			if a.Name == asset {
				return rel.TagName, a.URL, true
			}
		}
	}
	return "", "", false
}

func fetchReleases(ctx context.Context) ([]ghRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesAPI+"?per_page=20", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("github releases API returned %d: %s", resp.StatusCode, body)
	}
	var releases []ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decoding releases: %w", err)
	}
	return releases, nil
}

func download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// replaceBinary atomically replaces the file at path with data. It writes a
// temp file in the same directory (so the rename stays on one filesystem) and
// renames it over path — which works even while path is the running binary on
// Linux/macOS.
func replaceBinary(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".ec-update-*")
	if err != nil {
		return fmt.Errorf("cannot write to %s (insufficient permissions? try sudo): %w", dir, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("cannot replace %s (insufficient permissions? try sudo): %w", path, err)
	}
	return nil
}

func cmdUpdate(_ *eddsdk.Client, _ string, args []string) error {
	force := false
	for _, a := range args {
		if a == "--force" || a == "-f" {
			force = true
		}
	}

	ctx := context.Background()
	releases, err := fetchReleases(ctx)
	if err != nil {
		return err
	}
	asset := assetName()
	tag, url, ok := pickRelease(releases, asset)
	if !ok {
		return fmt.Errorf("no release asset %q found (unsupported platform?)", asset)
	}

	if tag == Version && !force {
		fmt.Printf("ec is already up to date (%s)\n", Version)
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	fmt.Printf("Updating ec %s -> %s ...\n", Version, tag)
	data, err := download(ctx, url)
	if err != nil {
		return err
	}
	if err := replaceBinary(exe, data); err != nil {
		return err
	}
	fmt.Printf("Updated to %s (%s)\n", tag, exe)
	return nil
}

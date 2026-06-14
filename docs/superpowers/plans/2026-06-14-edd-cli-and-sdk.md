# edd-cli + eddsdk Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Go SDK (`eddsdk`) for all edd-cloud services and a non-interactive CLI (`edd`) that drives them, in a new monorepo module `~/cloud/edd-cli`.

**Architecture:** `pkg/eddsdk` is a typed HTTP client (one root `Client` with per-service methods, bearer-auth, host resolution, typed `APIError`). `internal/cli` is a hand-rolled stdlib-`flag` command dispatcher consuming the SDK, with config/token resolution and table/`--json` output. `cmd/edd` is the entrypoint. No TUI, no external deps.

**Tech Stack:** Go 1.22, stdlib only (`net/http`, `flag`, `text/tabwriter`, `encoding/json`, `httptest` for tests).

**Spec:** `docs/superpowers/specs/2026-06-14-edd-cli-and-sdk-design.md`
**Repo/branch:** `~/cloud`, branch `feat/edd-cli` (exists). New module dir `edd-cli/`.
**Commit policy:** No `Co-Authored-By` trailers. Per `~/cloud/CLAUDE.md`, pushes go through the `commit-organizer` agent — but this plan only `git commit`s locally on the branch; pushing/merging is a separate finishing step.

## Service facts (verified)
- Hosts: `https://<svc>.cloud.eddisonso.com`; service names `compute`, `storage`, `registry`, `auth`; **networking → `net`**.
- Auth: `Authorization: Bearer <jwt>` (session JWT or `ecloud_` SA token).
- Compute routes: `GET/POST /compute/containers`, `GET/DELETE /compute/containers/{id}`, `POST .../{id}/start|stop`, `GET .../{id}/logs`, `GET/PUT .../{id}/ssh`, `GET/POST .../{id}/ingress`, `DELETE .../{id}/ingress/{port}`, `GET/PUT .../{id}/mounts`, `PUT .../{id}/pull-policy`.
- `GET /compute/containers` returns `{"containers":[...]}`. Container JSON: `id,name,status,hostname,memory_mb,storage_gb,instance_type,created_at,ssh_enabled,https_enabled,pull_policy`.
- Create body: `{name,memory_mb,storage_gb,instance_type,ssh_key_ids,ssh_enabled,mount_paths,image,pull_policy}`.
- Auth: `POST /api/login {username,password}` → `{token,username,display_name,user_id,is_admin,requires_2fa}`; `GET /api/session`.

## File structure

| File | Responsibility |
|---|---|
| `edd-cli/go.mod` | module `eddisonso.com/edd-cli`, go 1.22 |
| `edd-cli/pkg/eddsdk/client.go` | `Client`, `NewClient`, host resolution, `doJSON`, auth header |
| `edd-cli/pkg/eddsdk/errors.go` | `APIError` |
| `edd-cli/pkg/eddsdk/types.go` | DTOs |
| `edd-cli/pkg/eddsdk/compute.go` | compute methods |
| `edd-cli/pkg/eddsdk/auth.go` | `Login`, `Session` |
| `edd-cli/pkg/eddsdk/{storage,registry,networking}.go` | other services (Task 6) |
| `edd-cli/internal/cli/app.go` | argv parse, global flags, command dispatch, usage |
| `edd-cli/internal/cli/config.go` | config file + token resolution |
| `edd-cli/internal/cli/output.go` | table + `--json` |
| `edd-cli/internal/cli/{auth,compute,...}.go` | subcommands |
| `edd-cli/cmd/edd/main.go` | entrypoint |

---

### Task 1: Module scaffold + SDK client core

**Files:** Create `edd-cli/go.mod`, `edd-cli/pkg/eddsdk/errors.go`, `edd-cli/pkg/eddsdk/client.go`, `edd-cli/pkg/eddsdk/client_test.go`

- [ ] **Step 1: Init module**
```bash
cd /home/eddison/cloud/edd-cli 2>/dev/null || mkdir -p /home/eddison/cloud/edd-cli && cd /home/eddison/cloud/edd-cli
go mod init eddisonso.com/edd-cli
go mod edit -go=1.22
```

- [ ] **Step 2: Write the failing test** — `pkg/eddsdk/client_test.go`:
```go
package eddsdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"errors"
	"strings"
	"testing"
)

func TestServiceURL(t *testing.T) {
	c := NewClient(Options{BaseDomain: "cloud.eddisonso.com", Token: "t"})
	cases := map[string]string{
		"compute":    "https://compute.cloud.eddisonso.com",
		"storage":    "https://storage.cloud.eddisonso.com",
		"networking": "https://net.cloud.eddisonso.com",
	}
	for svc, want := range cases {
		if got := c.serviceURL(svc); got != want {
			t.Errorf("serviceURL(%q) = %q, want %q", svc, got, want)
		}
	}
}

func TestDoJSON_SendsAuthAndDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("missing/wrong auth header: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"hello": "world"})
	}))
	defer srv.Close()
	c := NewClient(Options{Token: "tok"})
	var out struct{ Hello string `json:"hello"` }
	if err := c.doJSON(context.Background(), "GET", srv.URL, "/x", nil, &out); err != nil {
		t.Fatalf("doJSON: %v", err)
	}
	if out.Hello != "world" {
		t.Errorf("decoded %q", out.Hello)
	}
}

func TestDoJSON_MapsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer srv.Close()
	c := NewClient(Options{Token: "tok"})
	err := c.doJSON(context.Background(), "GET", srv.URL, "/x", nil, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want *APIError, got %v", err)
	}
	if apiErr.Status != 403 || !strings.Contains(apiErr.Message, "nope") {
		t.Errorf("got status=%d msg=%q", apiErr.Status, apiErr.Message)
	}
}
```

- [ ] **Step 3: Run — expect FAIL (undefined)**
`cd /home/eddison/cloud/edd-cli && go test ./pkg/eddsdk/` → FAIL (undefined NewClient/Options/serviceURL/doJSON/APIError).

- [ ] **Step 4: Implement `errors.go`**
```go
package eddsdk

import "fmt"

// APIError is a non-2xx HTTP response from an edd-cloud service.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("edd-cloud API error %d: %s", e.Status, e.Message) }
```

- [ ] **Step 5: Implement `client.go`**
```go
package eddsdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Options configures a Client.
type Options struct {
	BaseDomain string       // default "cloud.eddisonso.com"
	Token      string       // bearer JWT (session or ecloud_ SA token)
	HTTPClient *http.Client // optional
}

// Client is a typed client for edd-cloud services.
type Client struct {
	baseDomain string
	token      string
	http       *http.Client
}

func NewClient(o Options) *Client {
	base := o.BaseDomain
	if base == "" {
		base = "cloud.eddisonso.com"
	}
	hc := o.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{baseDomain: base, token: o.Token, http: hc}
}

// serviceSubdomain maps a logical service name to its subdomain prefix.
func serviceSubdomain(svc string) string {
	if svc == "networking" {
		return "net"
	}
	return svc
}

func (c *Client) serviceURL(svc string) string {
	return fmt.Sprintf("https://%s.%s", serviceSubdomain(svc), c.baseDomain)
}

// doJSON performs a JSON request to baseURL+path. body (if non-nil) is JSON-encoded;
// out (if non-nil) receives the decoded JSON response. Non-2xx → *APIError.
func (c *Client) doJSON(ctx context.Context, method, baseURL, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, rdr)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{Status: resp.StatusCode, Message: strings.TrimSpace(string(data))}
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
```
Note: the `Client.urlOverride` field referenced by tests is added in Task 2 Step 3 — it's fine that it doesn't exist yet in Task 1 (Task 1 tests don't use it).

- [ ] **Step 6: Run — expect PASS**
`go test ./pkg/eddsdk/ -v` → all 3 pass. `go vet ./...`.

- [ ] **Step 7: Commit**
```bash
cd /home/eddison/cloud
git add edd-cli/go.mod edd-cli/pkg/eddsdk/
git commit -m "feat(edd-cli): scaffold module + eddsdk client core (auth, host resolution, errors)"
```

---

### Task 2: SDK compute methods

**Files:** Create `edd-cli/pkg/eddsdk/types.go`, `edd-cli/pkg/eddsdk/compute.go`, `edd-cli/pkg/eddsdk/compute_test.go`

- [ ] **Step 1: Write the failing test** — `compute_test.go`:
```go
package eddsdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestClient points all service URLs at srv.
func newTestClient(srv *httptest.Server) *Client {
	c := NewClient(Options{Token: "tok"})
	c.urlOverride = srv.URL // test hook
	return c
}

func TestListContainers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/compute/containers" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"containers": []map[string]any{
			{"id": "abc", "name": "web", "status": "running", "pull_policy": "Always"},
		}})
	}))
	defer srv.Close()
	cs, err := newTestClient(srv).ListContainers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 || cs[0].ID != "abc" || cs[0].PullPolicy != "Always" {
		t.Fatalf("got %+v", cs)
	}
}

func TestStartStopPullPolicy(t *testing.T) {
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.Method+" "+r.URL.Path)
		w.WriteHeader(200)
	}))
	defer srv.Close()
	c := newTestClient(srv)
	ctx := context.Background()
	if err := c.StartContainer(ctx, "abc"); err != nil { t.Fatal(err) }
	if err := c.StopContainer(ctx, "abc"); err != nil { t.Fatal(err) }
	if err := c.SetPullPolicy(ctx, "abc", "Always"); err != nil { t.Fatal(err) }
	want := []string{"POST /compute/containers/abc/start", "POST /compute/containers/abc/stop", "PUT /compute/containers/abc/pull-policy"}
	for i, w := range want {
		if i >= len(hits) || hits[i] != w {
			t.Fatalf("hits=%v want %v", hits, want)
		}
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (`urlOverride`, `ListContainers`, `Container`, etc. undefined).
`go test ./pkg/eddsdk/ -run 'Container|StartStop'`

- [ ] **Step 3: Add a test hook to `client.go`** — add field `urlOverride string` to `Client` and change `serviceURL` to honor it:
```go
func (c *Client) serviceURL(svc string) string {
	if c.urlOverride != "" {
		return c.urlOverride
	}
	return fmt.Sprintf("https://%s.%s", serviceSubdomain(svc), c.baseDomain)
}
```
(Add `urlOverride string` to the `Client` struct.)

- [ ] **Step 4: Implement `types.go`**
```go
package eddsdk

// Container mirrors the compute service's container JSON.
type Container struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	Hostname     string `json:"hostname"`
	MemoryMB     int    `json:"memory_mb"`
	StorageGB    int    `json:"storage_gb"`
	InstanceType string `json:"instance_type"`
	CreatedAt    string `json:"created_at"`
	SSHEnabled   bool   `json:"ssh_enabled"`
	HTTPSEnabled bool   `json:"https_enabled"`
	PullPolicy   string `json:"pull_policy"`
}

// CreateContainerRequest is the body for creating a container.
type CreateContainerRequest struct {
	Name         string   `json:"name"`
	MemoryMB     int      `json:"memory_mb"`
	StorageGB    int      `json:"storage_gb"`
	InstanceType string   `json:"instance_type"`
	SSHKeyIDs    []int64  `json:"ssh_key_ids,omitempty"`
	SSHEnabled   bool     `json:"ssh_enabled"`
	MountPaths   []string `json:"mount_paths,omitempty"`
	Image        string   `json:"image,omitempty"`
	PullPolicy   string   `json:"pull_policy,omitempty"`
}

// IngressRule mirrors a container ingress rule.
type IngressRule struct {
	Port       int `json:"port"`
	TargetPort int `json:"target_port"`
}
```

- [ ] **Step 5: Implement `compute.go`**
```go
package eddsdk

import (
	"context"
	"fmt"
)

const computeSvc = "compute"

func (c *Client) ListContainers(ctx context.Context) ([]Container, error) {
	var out struct {
		Containers []Container `json:"containers"`
	}
	if err := c.doJSON(ctx, "GET", c.serviceURL(computeSvc), "/compute/containers", nil, &out); err != nil {
		return nil, err
	}
	return out.Containers, nil
}

func (c *Client) GetContainer(ctx context.Context, id string) (*Container, error) {
	var out Container
	if err := c.doJSON(ctx, "GET", c.serviceURL(computeSvc), "/compute/containers/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CreateContainer(ctx context.Context, req CreateContainerRequest) (*Container, error) {
	var out Container
	if err := c.doJSON(ctx, "POST", c.serviceURL(computeSvc), "/compute/containers", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) StartContainer(ctx context.Context, id string) error {
	return c.doJSON(ctx, "POST", c.serviceURL(computeSvc), "/compute/containers/"+id+"/start", nil, nil)
}

func (c *Client) StopContainer(ctx context.Context, id string) error {
	return c.doJSON(ctx, "POST", c.serviceURL(computeSvc), "/compute/containers/"+id+"/stop", nil, nil)
}

func (c *Client) DeleteContainer(ctx context.Context, id string) error {
	return c.doJSON(ctx, "DELETE", c.serviceURL(computeSvc), "/compute/containers/"+id, nil, nil)
}

func (c *Client) SetPullPolicy(ctx context.Context, id, policy string) error {
	return c.doJSON(ctx, "PUT", c.serviceURL(computeSvc), "/compute/containers/"+id+"/pull-policy",
		map[string]string{"pull_policy": policy}, nil)
}

func (c *Client) SetSSH(ctx context.Context, id string, enabled bool) error {
	return c.doJSON(ctx, "PUT", c.serviceURL(computeSvc), "/compute/containers/"+id+"/ssh",
		map[string]bool{"ssh_enabled": enabled}, nil)
}

func (c *Client) ListIngress(ctx context.Context, id string) ([]IngressRule, error) {
	var out []IngressRule
	if err := c.doJSON(ctx, "GET", c.serviceURL(computeSvc), "/compute/containers/"+id+"/ingress", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) AddIngress(ctx context.Context, id string, port, target int) error {
	return c.doJSON(ctx, "POST", c.serviceURL(computeSvc), "/compute/containers/"+id+"/ingress",
		IngressRule{Port: port, TargetPort: target}, nil)
}

func (c *Client) RemoveIngress(ctx context.Context, id string, port int) error {
	return c.doJSON(ctx, "DELETE", c.serviceURL(computeSvc),
		fmt.Sprintf("/compute/containers/%s/ingress/%d", id, port), nil, nil)
}

func (c *Client) GetMounts(ctx context.Context, id string) ([]string, error) {
	var out struct {
		MountPaths []string `json:"mount_paths"`
	}
	if err := c.doJSON(ctx, "GET", c.serviceURL(computeSvc), "/compute/containers/"+id+"/mounts", nil, &out); err != nil {
		return nil, err
	}
	return out.MountPaths, nil
}

func (c *Client) SetMounts(ctx context.Context, id string, paths []string) error {
	return c.doJSON(ctx, "PUT", c.serviceURL(computeSvc), "/compute/containers/"+id+"/mounts",
		map[string][]string{"mount_paths": paths}, nil)
}
```
Note: `ListIngress`/`GetMounts` response shapes are best-effort from the routes; when implementing, quickly confirm the JSON the handlers return (read `internal/api/ingress.go` / mounts handler in the compute service) and adjust the wrapper struct if needed. If a shape differs, fix the struct — do not invent fields.

- [ ] **Step 6: Run — expect PASS**
`go test ./pkg/eddsdk/ -v` → all pass. `go vet ./...`.

- [ ] **Step 7: Commit**
```bash
cd /home/eddison/cloud
git add edd-cli/pkg/eddsdk/
git commit -m "feat(eddsdk): compute methods (list/get/create/start/stop/rm/ssh/ingress/mounts/pull-policy)"
```

---

### Task 3: SDK auth + CLI core (config, output, dispatch)

**Files:** Create `edd-cli/pkg/eddsdk/auth.go`, `edd-cli/pkg/eddsdk/auth_test.go`, `edd-cli/internal/cli/config.go`, `edd-cli/internal/cli/config_test.go`, `edd-cli/internal/cli/output.go`, `edd-cli/internal/cli/app.go`, `edd-cli/cmd/edd/main.go`

- [ ] **Step 1: Write failing SDK auth test** — `auth_test.go`:
```go
package eddsdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLogin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/login" || r.Method != "POST" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"token": "sess123", "user_id": "u1", "requires_2fa": false})
	}))
	defer srv.Close()
	c := NewClient(Options{Token: ""})
	c.urlOverride = srv.URL
	res, err := c.Login(context.Background(), "eddison", "pw")
	if err != nil {
		t.Fatal(err)
	}
	if res.Token != "sess123" || res.Requires2FA {
		t.Fatalf("got %+v", res)
	}
}
```

- [ ] **Step 2: Run — FAIL** (`Login`, `LoginResult` undefined). `go test ./pkg/eddsdk/ -run TestLogin`

- [ ] **Step 3: Implement `auth.go`**
```go
package eddsdk

import "context"

const authSvc = "auth"

// LoginResult is the response from POST /api/login.
type LoginResult struct {
	Token       string `json:"token"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	UserID      string `json:"user_id"`
	IsAdmin     bool   `json:"is_admin"`
	Requires2FA bool   `json:"requires_2fa"`
}

// Session is the response from GET /api/session.
type Session struct {
	Username string `json:"username"`
	UserID   string `json:"user_id"`
	IsAdmin  bool   `json:"is_admin"`
}

func (c *Client) Login(ctx context.Context, username, password string) (*LoginResult, error) {
	var out LoginResult
	body := map[string]string{"username": username, "password": password}
	if err := c.doJSON(ctx, "POST", c.serviceURL(authSvc), "/api/login", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Session(ctx context.Context) (*Session, error) {
	var out Session
	if err := c.doJSON(ctx, "GET", c.serviceURL(authSvc), "/api/session", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
```
Note: confirm the `GET /api/session` JSON field names against `edd-cloud-auth/internal/api/auth.go`'s `sessionResponse` when implementing; adjust `Session` tags if they differ.

- [ ] **Step 4: Run — PASS.** `go test ./pkg/eddsdk/ -run TestLogin -v`

- [ ] **Step 5: Write failing CLI config test** — `internal/cli/config_test.go`:
```go
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

	// env beats file
	t.Setenv("EDD_TOKEN", "from_env")
	tok, _ := resolveToken("", cfgPath)
	if tok != "from_env" {
		t.Errorf("env should win, got %q", tok)
	}
	// flag beats env
	tok, _ = resolveToken("from_flag", cfgPath)
	if tok != "from_flag" {
		t.Errorf("flag should win, got %q", tok)
	}
	// file when no flag/env
	os.Unsetenv("EDD_TOKEN")
	tok, _ = resolveToken("", cfgPath)
	if tok != "from_file" {
		t.Errorf("file fallback, got %q", tok)
	}
}
```

- [ ] **Step 6: Run — FAIL** (`resolveToken` undefined). `go test ./internal/cli/`

- [ ] **Step 7: Implement `config.go`**
```go
package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Token      string `json:"token,omitempty"`
	BaseDomain string `json:"base_domain,omitempty"`
}

func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".edd-config.json"
	}
	return filepath.Join(home, ".config", "edd", "config.json")
}

func loadConfig(path string) Config {
	var c Config
	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, &c)
	}
	return c
}

func saveConfig(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// resolveToken: flag > EDD_TOKEN env > config file. Returns token and base domain.
func resolveToken(flagToken, cfgPath string) (string, string) {
	cfg := loadConfig(cfgPath)
	base := cfg.BaseDomain
	if base == "" {
		base = "cloud.eddisonso.com"
	}
	if flagToken != "" {
		return flagToken, base
	}
	if env := os.Getenv("EDD_TOKEN"); env != "" {
		return env, base
	}
	return cfg.Token, base
}
```

- [ ] **Step 8: Run — PASS.** `go test ./internal/cli/ -v`

- [ ] **Step 9: Implement `output.go`** (used by later tasks; minimal now)
```go
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
)

// printJSON writes v as indented JSON to stdout.
func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// printTable writes headers + rows as an aligned table.
func printTable(w io.Writer, headers []string, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	for i, h := range headers {
		if i > 0 { fmt.Fprint(tw, "\t") }
		fmt.Fprint(tw, h)
	}
	fmt.Fprintln(tw)
	for _, row := range rows {
		for i, cell := range row {
			if i > 0 { fmt.Fprint(tw, "\t") }
			fmt.Fprint(tw, cell)
		}
		fmt.Fprintln(tw)
	}
	tw.Flush()
}
```

- [ ] **Step 10: Implement `app.go` (dispatch + global flags) and `cmd/edd/main.go`**
`app.go`:
```go
package cli

import (
	"fmt"
	"os"

	"eddisonso.com/edd-cli/pkg/eddsdk"
)

// command is a single CLI command (e.g. "compute").
type command struct {
	name string
	run  func(c *eddsdk.Client, cfgPath string, args []string) error
}

var commands = map[string]command{} // registered by init() in each file

func register(cmd command) { commands[cmd.name] = cmd }

// Run is the CLI entrypoint. Returns process exit code.
func Run(argv []string) int {
	// Global flags can appear before the command: --token, --base, --json.
	var flagToken, flagBase string
	var jsonOut bool
	args := argv
	for len(args) > 0 && len(args[0]) > 2 && args[0][:2] == "--" {
		switch {
		case args[0] == "--json":
			jsonOut = true; args = args[1:]
		case args[0] == "--token" && len(args) > 1:
			flagToken = args[1]; args = args[2:]
		case args[0] == "--base" && len(args) > 1:
			flagBase = args[1]; args = args[2:]
		default:
			fmt.Fprintf(os.Stderr, "unknown global flag: %s\n", args[0]); return 2
		}
	}
	if len(args) == 0 || args[0] == "help" {
		usage(); return 2
	}
	cmd, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0]); usage(); return 2
	}
	cfgPath := defaultConfigPath()
	tok, base := resolveToken(flagToken, cfgPath)
	if flagBase != "" {
		base = flagBase
	}
	client := eddsdk.NewClient(eddsdk.Options{BaseDomain: base, Token: tok})
	jsonOutput = jsonOut // package-level, read by output helpers
	if err := cmd.run(client, cfgPath, args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

var jsonOutput bool

func usage() {
	fmt.Fprintln(os.Stderr, `edd — edd-cloud CLI

Usage: edd [--json] [--token T] [--base D] <command> [args]

Commands:
  login | logout | whoami
  compute   manage containers
(more commands added incrementally)`)
}
```
`cmd/edd/main.go`:
```go
package main

import (
	"os"

	"eddisonso.com/edd-cli/internal/cli"
)

func main() { os.Exit(cli.Run(os.Args[1:])) }
```

- [ ] **Step 11: Build + test**
`cd /home/eddison/cloud/edd-cli && go build ./... && go test ./... && go vet ./...` → builds (with zero registered commands yet), tests pass.

- [ ] **Step 12: Commit**
```bash
cd /home/eddison/cloud
git add edd-cli/pkg/eddsdk/ edd-cli/internal/cli/ edd-cli/cmd/
git commit -m "feat(edd-cli): SDK auth (login/session) + CLI core (config, token resolution, output, dispatch)"
```

---

### Task 4: CLI auth commands (login / logout / whoami)

**Files:** Create `edd-cli/internal/cli/auth.go`, `edd-cli/internal/cli/auth_test.go`

- [ ] **Step 1: Write failing test** — `auth_test.go` (tests logout clears the config file):
```go
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
}
```

- [ ] **Step 2: Run — FAIL** (`doLogout` undefined). `go test ./internal/cli/ -run Logout`

- [ ] **Step 3: Implement `auth.go`**
```go
package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"eddisonso.com/edd-cli/pkg/eddsdk"
	"golang.org/x/term"
)

func init() {
	register(command{name: "login", run: cmdLogin})
	register(command{name: "logout", run: cmdLogout})
	register(command{name: "whoami", run: cmdWhoami})
}

func cmdLogin(c *eddsdk.Client, cfgPath string, args []string) error {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Username: ")
	user, _ := reader.ReadString('\n')
	user = strings.TrimSpace(user)
	fmt.Print("Password: ")
	pwBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return err
	}
	res, err := c.Login(context.Background(), user, strings.TrimSpace(string(pwBytes)))
	if err != nil {
		return err
	}
	if res.Requires2FA {
		return fmt.Errorf("this account requires 2FA/WebAuthn, which the CLI can't do interactively; create a service-account token in the dashboard and set EDD_TOKEN instead")
	}
	cfg := loadConfig(cfgPath)
	cfg.Token = res.Token
	if cfg.BaseDomain == "" {
		cfg.BaseDomain = "cloud.eddisonso.com"
	}
	if err := saveConfig(cfgPath, cfg); err != nil {
		return err
	}
	fmt.Printf("Logged in as %s\n", res.Username)
	return nil
}

func doLogout(cfgPath string) error {
	cfg := loadConfig(cfgPath)
	cfg.Token = ""
	return saveConfig(cfgPath, cfg)
}

func cmdLogout(c *eddsdk.Client, cfgPath string, args []string) error {
	if err := doLogout(cfgPath); err != nil {
		return err
	}
	fmt.Println("Logged out")
	return nil
}

func cmdWhoami(c *eddsdk.Client, cfgPath string, args []string) error {
	s, err := c.Session(context.Background())
	if err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(s)
	}
	fmt.Printf("%s (%s)%s\n", s.Username, s.UserID, map[bool]string{true: " [admin]", false: ""}[s.IsAdmin])
	return nil
}
```

- [ ] **Step 4: Add the term dependency**
```bash
cd /home/eddison/cloud/edd-cli && go get golang.org/x/term
```
(This is the one allowed external dep — stdlib has no secure password reader. If you prefer zero deps, replace `term.ReadPassword` with a plain `reader.ReadString('\n')` and drop the import + `go get`.)

- [ ] **Step 5: Run — PASS + build.** `go test ./internal/cli/ -v && go build ./...`

- [ ] **Step 6: Commit**
```bash
cd /home/eddison/cloud
git add edd-cli/
git commit -m "feat(edd-cli): login/logout/whoami commands"
```

---

### Task 5: CLI compute commands

**Files:** Create `edd-cli/internal/cli/compute.go`, `edd-cli/internal/cli/compute_test.go`

- [ ] **Step 1: Write failing test** — `compute_test.go` (table formatting of containers):
```go
package cli

import (
	"bytes"
	"strings"
	"testing"

	"eddisonso.com/edd-cli/pkg/eddsdk"
)

func TestContainerTable(t *testing.T) {
	var buf bytes.Buffer
	containerTable(&buf, []eddsdk.Container{
		{ID: "abc", Name: "web", Status: "running", Image: "", PullPolicy: "Always"},
	})
	out := buf.String()
	if !strings.Contains(out, "abc") || !strings.Contains(out, "web") || !strings.Contains(out, "Always") {
		t.Fatalf("table missing data:\n%s", out)
	}
	if !strings.Contains(out, "ID") {
		t.Fatalf("table missing header:\n%s", out)
	}
}
```
Note: `Container` has no `Image` field in `types.go` — drop `Image` from the literal (it was illustrative). Use only real fields: ID, Name, Status, PullPolicy, InstanceType.

- [ ] **Step 2: Run — FAIL** (`containerTable` undefined). Fix the test literal to real fields, then run `go test ./internal/cli/ -run ContainerTable`.

- [ ] **Step 3: Implement `compute.go`**
```go
package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"eddisonso.com/edd-cli/pkg/eddsdk"
)

func init() { register(command{name: "compute", run: cmdCompute}) }

func containerTable(w io.Writer, cs []eddsdk.Container) {
	rows := make([][]string, len(cs))
	for i, c := range cs {
		rows[i] = []string{c.ID, c.Name, c.Status, c.InstanceType, c.PullPolicy}
	}
	printTable(w, []string{"ID", "NAME", "STATUS", "TYPE", "PULL"}, rows)
}

func cmdCompute(c *eddsdk.Client, cfgPath string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: edd compute <ls|get|create|start|stop|rm|logs|ssh|ingress|mounts|pull-policy> [args]")
	}
	ctx := context.Background()
	sub, rest := args[0], args[1:]
	switch sub {
	case "ls":
		cs, err := c.ListContainers(ctx)
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(cs)
		}
		containerTable(os.Stdout, cs)
		return nil
	case "get":
		if len(rest) < 1 {
			return fmt.Errorf("usage: edd compute get <id>")
		}
		ct, err := c.GetContainer(ctx, rest[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(ct)
		}
		containerTable(os.Stdout, []eddsdk.Container{*ct})
		return nil
	case "start":
		return needID(rest, func(id string) error { return c.StartContainer(ctx, id) })
	case "stop":
		return needID(rest, func(id string) error { return c.StopContainer(ctx, id) })
	case "rm":
		return needID(rest, func(id string) error { return c.DeleteContainer(ctx, id) })
	case "pull-policy":
		fs := flag.NewFlagSet("pull-policy", flag.ContinueOnError)
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if fs.NArg() != 2 {
			return fmt.Errorf("usage: edd compute pull-policy <id> <Always|IfNotPresent>")
		}
		return c.SetPullPolicy(ctx, fs.Arg(0), fs.Arg(1))
	case "ssh":
		if len(rest) != 2 || (rest[1] != "on" && rest[1] != "off") {
			return fmt.Errorf("usage: edd compute ssh <id> <on|off>")
		}
		return c.SetSSH(ctx, rest[0], rest[1] == "on")
	case "create":
		return cmdComputeCreate(ctx, c, rest)
	default:
		return fmt.Errorf("unknown compute subcommand: %s", sub)
	}
}

func needID(args []string, fn func(string) error) error {
	if len(args) < 1 {
		return fmt.Errorf("a container id is required")
	}
	return fn(args[0])
}

func cmdComputeCreate(ctx context.Context, c *eddsdk.Client, args []string) error {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	name := fs.String("name", "", "container name")
	image := fs.String("image", "", "image (registry.cloud.eddisonso.com/...)")
	itype := fs.String("type", "nano", "instance type")
	mem := fs.Int("memory", 256, "memory MB")
	storage := fs.Int("storage", 5, "storage GB")
	pull := fs.String("pull-policy", "IfNotPresent", "Always|IfNotPresent")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	ct, err := c.CreateContainer(ctx, eddsdk.CreateContainerRequest{
		Name: *name, Image: *image, InstanceType: *itype,
		MemoryMB: *mem, StorageGB: *storage, PullPolicy: *pull,
	})
	if err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(ct)
	}
	fmt.Printf("created %s (%s)\n", ct.Name, ct.ID)
	return nil
}
```
Note: `logs`/`ingress`/`mounts` subcommands follow the same pattern using the SDK methods (`ContainerLogs`, `ListIngress`/`AddIngress`/`RemoveIngress`, `GetMounts`/`SetMounts`). Add them here; if `ContainerLogs` isn't yet in the SDK (Task 2 didn't include it), add `func (c *Client) ContainerLogs(ctx, id string) (string, error)` to `compute.go` returning the raw body, with a test, before wiring the `logs` subcommand. Wire each remaining subcommand minimally (id arg → SDK call → print).

- [ ] **Step 4: Run — PASS + build.** `go test ./... && go build ./...`

- [ ] **Step 5: Manual smoke (if a token is available)**
`EDD_TOKEN=<token> go run ./cmd/edd compute ls` (or `--json`). If no token/cluster reachable, skip and note it.

- [ ] **Step 6: Commit**
```bash
cd /home/eddison/cloud
git add edd-cli/
git commit -m "feat(edd-cli): compute subcommands (ls/get/create/start/stop/rm/ssh/pull-policy/ingress/mounts/logs)"
```

---

### Task 6: SDK + CLI for storage, registry, service-accounts, networking

**Files:** Create `edd-cli/pkg/eddsdk/{storage,registry,networking}.go` (+ tests), extend `auth.go` (SA/token methods); create `edd-cli/internal/cli/{storage,registry,accounts,networking}.go`

This task replicates the Task-2/Task-5 pattern for the remaining services. Before each, read the service's routes + JSON shapes (compute pattern: `grep -oE '"(GET|POST|PUT|DELETE) /[^"]*"' <service>/internal/api/*.go` and the response structs) and define DTOs + methods to match — do not invent fields.

- [ ] **Step 1: Storage SDK + commands.** Routes under `storage.cloud.eddisonso.com` (read `edd-cloud-interface/services/storage` or the gateway routes). SDK: `ListNamespaces`, `CreateNamespace(name)`, `DeleteNamespace(name)`, `ListFiles(ns)`, `UploadFile`, `DownloadFile`, `DeleteFile`. Test each against `httptest`. CLI: `edd storage ns ls|create|rm`, `edd storage ls|cp|rm`. Commit.

- [ ] **Step 2: Registry SDK + commands.** Read `edd-cloud-interface/services/registry` `/api/repos` + tags routes (the compute service's `ListImages` showed `GET /api/repos` and `/api/repos/{name}/tags`). SDK: `ListRepos`, `ListTags(repo)`, `DeleteTag(repo, ref)`. CLI: `edd registry repos|tags|rm`. Commit.

- [ ] **Step 3: Service accounts + tokens SDK + commands.** Routes in `edd-cloud-auth/internal/api/handler.go`: `GET/POST /api/service-accounts`, `.../{id}/tokens`, `POST/GET /api/tokens`. SDK: `ListServiceAccounts`, `CreateServiceAccount(name, scopes)`, `DeleteServiceAccount(id)`, `CreateToken`, `ListTokens`, `DeleteToken`. CLI: `edd sa ls|create|rm`, `edd token create|ls|rm`. Commit.

- [ ] **Step 4: Networking SDK + commands.** Gateway routes `/api/domains`, `/api/cloudflare-connections` on `net.cloud.eddisonso.com`. SDK: `ListDomains`, `AddDomain`, `DeleteDomain`, `ListConnections`, `AddConnection`, `DeleteConnection`. CLI: `edd domains ls|add|rm`, `edd net connections ls|add|rm`. Commit.

Each sub-step: write the SDK method + `httptest` test (FAIL→implement→PASS), then the CLI subcommand reusing `printTable`/`printJSON`, then `go test ./... && go build ./... && go vet ./...`, then commit with a `feat(edd-cli): <service> SDK + commands` message.

---

### Task 7: README + install target

**Files:** Create `edd-cli/README.md`, `edd-cli/Makefile`

- [ ] **Step 1: README** documenting install (`go build -o edd ./cmd/edd`), auth (`edd login` or `EDD_TOKEN`), and the command surface with examples (`edd compute ls`, `edd --json compute get <id>`, `edd compute pull-policy <id> Always`).

- [ ] **Step 2: Makefile**
```makefile
BINARY=edd
build:
	go build -o $(BINARY) ./cmd/edd
install: build
	install -m 0755 $(BINARY) $(HOME)/bin/$(BINARY)
test:
	go test ./...
```

- [ ] **Step 3: Final verify**
`cd /home/eddison/cloud/edd-cli && go build ./... && go test ./... && go vet ./...` — all pass.

- [ ] **Step 4: Commit**
```bash
cd /home/eddison/cloud
git add edd-cli/README.md edd-cli/Makefile
git commit -m "docs(edd-cli): README + Makefile install target"
```

---

## Self-review notes
- **Spec coverage:** SDK core+auth+compute → Tasks 1-3,5; CLI core/config/output/dispatch → Task 3; login/SA-token auth → Tasks 3-4; compute commands → Task 5; other services (SDK+CLI) → Task 6; README/install → Task 7. No-TUI honored (none present).
- **Type consistency:** `Client.urlOverride` added in Task 2 Step 3 and used by all SDK tests; `eddsdk.Container`/`CreateContainerRequest`/`IngressRule`/`LoginResult`/`Session` defined in Tasks 2-3 and consumed in Task 5; `command`/`register`/`commands`/`jsonOutput`/`resolveToken`/`printTable`/`printJSON` defined in Task 3 and used in 4-6.
- **Known follow-ups flagged inline:** confirm exact JSON shapes for ingress/mounts/session/storage/registry when implementing (don't invent fields); `ContainerLogs` SDK method to be added in Task 5 if used; the `golang.org/x/term` dep is optional (Task 4 Step 4).
- **Caveat:** the `Options.token()` indirection in Task 1 Step 5 should be simplified to `token: o.Token` — noted in that step.

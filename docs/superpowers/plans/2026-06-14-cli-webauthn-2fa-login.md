# CLI WebAuthn 2FA Login Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `ec auth login` complete login for security-key (2FA) accounts by delegating the WebAuthn ceremony to a browser page on the dashboard, returning the session token to the CLI via a localhost callback (local) or copy-paste (SSH).

**Architecture:** No auth-service changes — the existing `POST /api/login` → `POST /api/webauthn/login/begin` → `POST /api/webauthn/login/finish` chain already issues a session token. We add (1) a small SDK field + helper, (2) CLI 2FA-login logic that prints a verification URL and races a localhost callback against a paste prompt, and (3) an unauthenticated dashboard page `/cli-2fa` that runs the passkey ceremony and surfaces the token.

**Tech Stack:** Go 1.22 (`edd-cli`, stdlib + `golang.org/x/term`), React + React Router v7 + TypeScript (`edd-cloud-interface/frontend`, no test runner — type-check + manual).

**Spec:** `docs/superpowers/specs/2026-06-14-cli-webauthn-2fa-login-design.md`

---

## Reference: existing shapes (do not re-derive)

- `POST /api/login` returns `{requires_2fa: true, challenge_token: "<jwt>"}` for 2FA accounts (`edd-cloud-auth/internal/api/auth.go:84-105`). The `challenge_token` is a 5-minute HS256 JWT.
- `POST /api/webauthn/login/begin`, header `Authorization: Bearer <challenge_token>`, returns `{options, state}` (`webauthn.go:203`).
- `POST /api/webauthn/login/finish`, body `{state, credential}`, returns `{token, username, display_name, user_id, is_admin}` (`webauthn.go:258-345`). `token` is the session JWT to store.
- Frontend WebAuthn helpers already exist in `frontend/src/lib/webauthn.ts`: `parseRequestOptions(options)`, `getCredential(opts)`, `serializeAssertionResponse(cred)`. The dashboard's `AuthContext.complete2FA` (`frontend/src/contexts/AuthContext.tsx:157-195`) is the reference sequence.
- `buildAuthBase()` (`frontend/src/lib/api.ts`) → `https://auth.cloud.eddisonso.com`.
- `copyToClipboard(text)` exists in `frontend/src/lib/api.ts:137`.

---

## File Structure

| File | Responsibility | Action |
|---|---|---|
| `edd-cli/pkg/eddsdk/auth.go` | Add `ChallengeToken` to `LoginResult`; add `Client.SetToken` | Modify |
| `edd-cli/pkg/eddsdk/auth_test.go` | Test challenge-token decode | Modify |
| `edd-cli/internal/cli/login2fa.go` | SSH detection, URL builder, callback listener, token race, browser open | Create |
| `edd-cli/internal/cli/login2fa_test.go` | Unit tests for the above | Create |
| `edd-cli/internal/cli/auth.go` | Replace 2FA dead-end in `cmdLogin` with the new flow | Modify |
| `frontend/src/pages/Cli2faPage.tsx` | Unauthenticated page: run ceremony, show token, optional loopback POST | Create |
| `frontend/src/pages/index.ts` | Export `Cli2faPage` | Modify |
| `frontend/src/App.tsx` | Register `/cli-2fa` route outside `<AppLayout>` | Modify |

---

## Task 1: SDK — capture challenge token + allow token swap

**Files:**
- Modify: `edd-cli/pkg/eddsdk/auth.go:7-15` (LoginResult), `edd-cli/pkg/eddsdk/client.go` (add SetToken)
- Test: `edd-cli/pkg/eddsdk/auth_test.go`

- [ ] **Step 1: Write the failing test**

Add to `edd-cli/pkg/eddsdk/auth_test.go`:

```go
func TestLoginCapturesChallengeToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"requires_2fa": true, "challenge_token": "ch123"})
	}))
	defer srv.Close()

	c := NewClient(Options{Token: "", BaseDomain: "example.com"})
	c.SetBaseURLForTest(srv.URL) // see Step 3 note; if a test seam already exists, reuse it

	res, err := c.Login(context.Background(), "u", "p")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Requires2FA || res.ChallengeToken != "ch123" {
		t.Errorf("got requires_2fa=%v challenge=%q", res.Requires2FA, res.ChallengeToken)
	}
}
```

NOTE: Look at the top of `auth_test.go` / `client_test.go` first to see how existing tests point the client at an `httptest` server (the existing `TestLogin` in `auth_test.go:16` already does this). Reuse that exact seam instead of inventing `SetBaseURLForTest` — match the established pattern. The assertion on `ChallengeToken` is the part that must fail initially.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ~/cloud/edd-cli && go test ./pkg/eddsdk/ -run TestLoginCapturesChallengeToken -v`
Expected: FAIL — `res.ChallengeToken` is empty (field does not exist / not decoded).

- [ ] **Step 3: Add the field and the SetToken method**

In `edd-cli/pkg/eddsdk/auth.go`, add to `LoginResult` (after `Requires2FA`):

```go
	Requires2FA    bool   `json:"requires_2fa"`
	ChallengeToken string `json:"challenge_token"`
```

In `edd-cli/pkg/eddsdk/client.go`, add after the constructor:

```go
// SetToken updates the bearer token used for subsequent requests.
// Used after an interactive login obtains a fresh session token.
func (c *Client) SetToken(token string) { c.token = token }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd ~/cloud/edd-cli && go test ./pkg/eddsdk/ -v`
Expected: PASS (all eddsdk tests, including the new one).

- [ ] **Step 5: Commit**

```bash
git add edd-cli/pkg/eddsdk/auth.go edd-cli/pkg/eddsdk/client.go edd-cli/pkg/eddsdk/auth_test.go
git commit -m "feat(eddsdk): decode challenge_token on login and add SetToken"
```

---

## Task 2: CLI — SSH detection and verification-URL builder

**Files:**
- Create: `edd-cli/internal/cli/login2fa.go`
- Test: `edd-cli/internal/cli/login2fa_test.go`

- [ ] **Step 1: Write the failing test**

Create `edd-cli/internal/cli/login2fa_test.go`:

```go
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
	// challenge must be query-escaped; cb must be present and escaped
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
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ~/cloud/edd-cli && go test ./internal/cli/ -run 'TestBuildCli2faURL|TestIsSSHSession' -v`
Expected: FAIL — `buildCli2faURL` / `isSSHSession` undefined.

- [ ] **Step 3: Implement the functions**

Create `edd-cli/internal/cli/login2fa.go`:

```go
package cli

import (
	"fmt"
	"net/url"
	"os"
)

// isSSHSession reports whether ec appears to be running over SSH, in which
// case the localhost callback cannot reach the user's browser and we fall
// back to copy-paste only.
func isSSHSession() bool {
	return os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != ""
}

// buildCli2faURL builds the dashboard verification URL. cbPort == 0 omits the
// localhost callback parameter.
func buildCli2faURL(baseDomain, challenge string, cbPort int) string {
	q := url.Values{}
	q.Set("challenge", challenge)
	if cbPort != 0 {
		q.Set("cb", fmt.Sprintf("http://127.0.0.1:%d", cbPort))
	}
	return fmt.Sprintf("https://%s/cli-2fa?%s", baseDomain, q.Encode())
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd ~/cloud/edd-cli && go test ./internal/cli/ -run 'TestBuildCli2faURL|TestIsSSHSession' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add edd-cli/internal/cli/login2fa.go edd-cli/internal/cli/login2fa_test.go
git commit -m "feat(ec): add SSH detection and cli-2fa URL builder"
```

---

## Task 3: CLI — localhost callback listener and token race

**Files:**
- Modify: `edd-cli/internal/cli/login2fa.go`
- Test: `edd-cli/internal/cli/login2fa_test.go`

- [ ] **Step 1: Write the failing test**

Add to `edd-cli/internal/cli/login2fa_test.go`:

```go
import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCallbackListenerReceivesToken(t *testing.T) {
	port, tokenCh, stop, err := startCallbackListener()
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	resp, err := http.Post(
		"http://127.0.0.1:"+itoa(port), "text/plain", strings.NewReader("sess-token-xyz"))
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
```

NOTE: use `strconv.Itoa` directly in the test rather than a helper — replace `itoa(port)` with `strconv.Itoa(port)` and import `strconv`. (The helper is not part of the implementation.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ~/cloud/edd-cli && go test ./internal/cli/ -run 'TestCallbackListener|TestAwaitToken' -v`
Expected: FAIL — `startCallbackListener` / `awaitToken` undefined.

- [ ] **Step 3: Implement the listener and the race**

Append to `edd-cli/internal/cli/login2fa.go` (add `"io"`, `"net"`, `"net/http"`, `"strings"`, `"time"`, `"errors"` to imports):

```go
// startCallbackListener binds 127.0.0.1 on an OS-assigned port and returns the
// port plus a channel that receives the first non-empty token POSTed to it.
// The browser page POSTs the session token as a plain-text body. CORS is
// permitted so the page's fetch does not error, though only the server-side
// receipt matters.
func startCallbackListener() (port int, tokenCh chan string, stop func(), err error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, nil, nil, err
	}
	tokenCh = make(chan string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 8192))
		tok := strings.TrimSpace(string(body))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		if tok != "" {
			select {
			case tokenCh <- tok:
			default:
			}
		}
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	port = ln.Addr().(*net.TCPAddr).Port
	stop = func() { _ = srv.Close() }
	return port, tokenCh, stop, nil
}

// awaitToken returns the first token from either channel, or an error on timeout.
func awaitToken(cbCh, pasteCh <-chan string, timeout time.Duration) (string, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case tok := <-cbCh:
			if strings.TrimSpace(tok) != "" {
				return strings.TrimSpace(tok), nil
			}
		case tok := <-pasteCh:
			if strings.TrimSpace(tok) != "" {
				return strings.TrimSpace(tok), nil
			}
		case <-timer.C:
			return "", errors.New("timed out waiting for 2FA token")
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd ~/cloud/edd-cli && go test ./internal/cli/ -run 'TestCallbackListener|TestAwaitToken' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add edd-cli/internal/cli/login2fa.go edd-cli/internal/cli/login2fa_test.go
git commit -m "feat(ec): add localhost callback listener and token race for 2FA"
```

---

## Task 4: CLI — wire the 2FA flow into `ec auth login`

**Files:**
- Modify: `edd-cli/internal/cli/login2fa.go` (orchestrator + browser open + paste reader)
- Modify: `edd-cli/internal/cli/auth.go:51-72` (replace the dead-end)

- [ ] **Step 1: Implement browser-open, paste reader, and orchestrator**

Append to `edd-cli/internal/cli/login2fa.go` (add `"bufio"`, `"context"`, `"os/exec"`, `"runtime"` to imports; `eddsdk` is the existing module import `eddisonso.com/edd-cli/pkg/eddsdk`):

```go
// openBrowser best-effort opens url in the default browser. Failure is ignored.
func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd, args = "open", []string{url}
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		cmd, args = "xdg-open", []string{url}
	}
	_ = exec.Command(cmd, args...).Start()
}

// readPastedToken reads a single line from r and sends the trimmed value to ch.
func readPastedToken(r io.Reader, ch chan<- string) {
	br := bufio.NewReader(r)
	line, _ := br.ReadString('\n')
	if s := strings.TrimSpace(line); s != "" {
		select {
		case ch <- s:
		default:
		}
	}
}

// complete2FALogin runs the browser-delegated WebAuthn flow: print a
// verification URL, race a localhost callback (local only) against a paste
// prompt, store the resulting session token, and confirm identity.
func complete2FALogin(c *eddsdk.Client, cfgPath, challenge string) error {
	cfg := loadConfig(cfgPath)
	baseDomain := cfg.BaseDomain
	if baseDomain == "" {
		baseDomain = "cloud.eddisonso.com"
	}

	cbPort := 0
	cbCh := make(chan string, 1)
	ssh := isSSHSession()
	if !ssh {
		port, ch, stop, err := startCallbackListener()
		if err == nil {
			cbPort = port
			cbCh = ch
			defer stop()
		}
	}

	verifyURL := buildCli2faURL(baseDomain, challenge, cbPort)
	fmt.Println("\nThis account uses a security key.")
	fmt.Println("Open this URL in a browser and verify your security key:")
	fmt.Printf("\n  %s\n\n", verifyURL)
	if !ssh {
		openBrowser(verifyURL)
	}

	pasteCh := make(chan string, 1)
	fmt.Print("Paste the token shown after verifying (or press Enter to keep waiting): ")
	go readPastedToken(os.Stdin, pasteCh)

	token, err := awaitToken(cbCh, pasteCh, 5*time.Minute)
	if err != nil {
		return fmt.Errorf("%w; re-run 'ec auth login'", err)
	}

	cfg.Token = token
	if cfg.BaseDomain == "" {
		cfg.BaseDomain = "cloud.eddisonso.com"
	}
	if err := saveConfig(cfgPath, cfg); err != nil {
		return err
	}

	c.SetToken(token)
	sess, err := c.Session(context.Background())
	if err != nil {
		fmt.Println("\nLogged in (could not fetch profile).")
		return nil
	}
	fmt.Printf("\nLogged in as %s\n", sess.Username)
	return nil
}
```

- [ ] **Step 2: Replace the dead-end in `cmdLogin`**

In `edd-cli/internal/cli/auth.go`, replace:

```go
	if res.Requires2FA {
		return fmt.Errorf("this account requires 2FA/WebAuthn, which the CLI can't do interactively; use 'ec auth set-token' with a service-account token instead")
	}
```

with:

```go
	if res.Requires2FA {
		return complete2FALogin(c, cfgPath, res.ChallengeToken)
	}
```

- [ ] **Step 3: Build and run the full CLI test suite**

Run: `cd ~/cloud/edd-cli && go build ./... && go test ./...`
Expected: PASS (build clean, all tests green). The orchestrator's interactive parts (browser open, real stdin paste) are not unit-tested; its building blocks (`buildCli2faURL`, `startCallbackListener`, `awaitToken`, `isSSHSession`) are covered by Tasks 2–3.

- [ ] **Step 4: Manual smoke (non-2FA regression)**

Run: `cd ~/cloud/edd-cli && go run . auth login` against a password-only account.
Expected: unchanged behavior — logs in directly, prints `Logged in as <user>`. (Full 2FA path is verified end-to-end after Task 5.)

- [ ] **Step 5: Commit**

```bash
git add edd-cli/internal/cli/login2fa.go edd-cli/internal/cli/auth.go
git commit -m "feat(ec): complete 2FA login via browser-delegated WebAuthn"
```

---

## Task 5: Frontend — `/cli-2fa` verification page

**Files:**
- Create: `frontend/src/pages/Cli2faPage.tsx`
- Modify: `frontend/src/pages/index.ts`, `frontend/src/App.tsx`

No frontend test runner exists in this repo; this task is verified by `npm run type-check` plus manual end-to-end. Keep the page small and reuse existing helpers.

- [ ] **Step 1: Create the page component**

Create `frontend/src/pages/Cli2faPage.tsx`:

```tsx
import { useState } from "react";
import { useSearchParams } from "react-router-dom";
import { buildAuthBase, copyToClipboard } from "@/lib/api";
import { parseRequestOptions, getCredential, serializeAssertionResponse } from "@/lib/webauthn";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

type Phase = "idle" | "verifying" | "done" | "error";

// Only post the captured token to a loopback callback (the local ec listener).
function isLoopbackCallback(cb: string): boolean {
  try {
    const u = new URL(cb);
    return u.protocol === "http:" && (u.hostname === "127.0.0.1" || u.hostname === "localhost");
  } catch {
    return false;
  }
}

export function Cli2faPage() {
  const [params] = useSearchParams();
  const challenge = params.get("challenge") || "";
  const cb = params.get("cb") || "";
  const [phase, setPhase] = useState<Phase>("idle");
  const [token, setToken] = useState("");
  const [error, setError] = useState("");

  async function verify() {
    if (!challenge) {
      setPhase("error");
      setError("Missing challenge. Re-run `ec auth login`.");
      return;
    }
    setPhase("verifying");
    try {
      const beginRes = await fetch(`${buildAuthBase()}/api/webauthn/login/begin`, {
        method: "POST",
        headers: { Authorization: `Bearer ${challenge}` },
      });
      if (!beginRes.ok) throw new Error("Challenge expired or invalid. Re-run `ec auth login`.");
      const { options, state } = await beginRes.json();

      const parsed = parseRequestOptions(options);
      const credential = await getCredential(parsed);
      const serialized = serializeAssertionResponse(credential);

      const finishRes = await fetch(`${buildAuthBase()}/api/webauthn/login/finish`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ state, credential: serialized }),
      });
      if (!finishRes.ok) throw new Error("Security key verification failed.");
      const data = await finishRes.json();
      const sessionToken: string = data.token;

      setToken(sessionToken);
      setPhase("done");

      if (cb && isLoopbackCallback(cb)) {
        // Best-effort handoff to the local ec listener; ignore failures (e.g. over SSH).
        fetch(cb, { method: "POST", body: sessionToken }).catch(() => {});
      }
    } catch (e) {
      setPhase("error");
      setError((e as Error).message);
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>CLI Security Key Verification</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {phase === "idle" && (
            <>
              <p className="text-sm text-muted-foreground">
                Verify your security key to finish logging in to the <code>ec</code> CLI.
              </p>
              <Button onClick={verify} disabled={!challenge}>Verify security key</Button>
              {!challenge && (
                <p className="text-sm text-destructive">
                  Missing challenge. Re-run <code>ec auth login</code>.
                </p>
              )}
            </>
          )}
          {phase === "verifying" && (
            <p className="text-sm text-muted-foreground">Waiting for your security key…</p>
          )}
          {phase === "done" && (
            <>
              <p className="text-sm text-muted-foreground">
                Verified. Paste this token into your terminal:
              </p>
              <div className="flex gap-2">
                <Input readOnly value={token} className="font-mono text-xs" />
                <Button onClick={() => copyToClipboard(token)}>Copy</Button>
              </div>
              <p className="text-xs text-muted-foreground">
                If <code>ec</code> logged you in automatically, you can close this tab.
              </p>
            </>
          )}
          {phase === "error" && <p className="text-sm text-destructive">{error}</p>}
        </CardContent>
      </Card>
    </div>
  );
}
```

- [ ] **Step 2: Export the page**

In `frontend/src/pages/index.ts`, add an export alongside the others:

```ts
export { Cli2faPage } from "./Cli2faPage";
```

(Match the existing export style in that file — if it re-exports named components, add the line above; if it uses `export * from`, follow that pattern instead.)

- [ ] **Step 3: Register the route outside `<AppLayout>`**

In `frontend/src/App.tsx`, add `Cli2faPage` to the `@/pages` import, then add the route as a **sibling of** the `<Route element={<AppLayout />}>` block (so it is NOT wrapped by the authenticated layout):

```tsx
          <Routes>
            <Route path="/cli-2fa" element={<Cli2faPage />} />
            <Route element={<AppLayout />}>
              {/* existing routes unchanged */}
```

- [ ] **Step 4: Type-check and build**

Run: `cd ~/cloud/edd-cloud-interface/frontend && npm run type-check && npm run build`
Expected: no TypeScript errors; build succeeds.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/pages/Cli2faPage.tsx frontend/src/pages/index.ts frontend/src/App.tsx
git commit -m "feat(dashboard): add /cli-2fa security-key verification page"
```

---

## Task 6: End-to-end verification

**Files:** none (verification only)

- [ ] **Step 1: Local (callback) path**

On a local machine (no SSH), with the dashboard build deployed: run `ec auth login` for a 2FA account. Expected: browser auto-opens to `/cli-2fa?challenge=…&cb=http://127.0.0.1:PORT`; after tapping the security key, `ec` completes automatically and prints `Logged in as <user>` without any paste.

- [ ] **Step 2: SSH (paste) path**

Over an SSH session (`SSH_CONNECTION` set): run `ec auth login` for the same account. Expected: no browser auto-open, URL has no `cb=` param; copy the URL into the laptop browser, tap the key, copy the displayed token, paste into the `ec` prompt → `Logged in as <user>`.

- [ ] **Step 3: Confirm**

Run: `ec auth whoami`
Expected: prints the logged-in user — confirming the stored session token works.

---

## Self-Review Notes

- **Spec coverage:** origin constraint → Task 5 serves from dashboard; no backend change → no auth task; copy-paste always present → Tasks 4 (paste prompt) + 5 (copy box); local callback convenience → Tasks 3+4; SSH detection → Task 2; loopback-only `cb` validation → Task 5 `isLoopbackCallback`; 5-min timeout → Task 4 `awaitToken`; post-login identity via `Session` with fallback → Task 4. All covered.
- **No backend tasks:** intentional — the auth service already returns the token.
- **Type consistency:** `buildCli2faURL(baseDomain, challenge string, cbPort int)`, `startCallbackListener() (int, chan string, func(), error)`, `awaitToken(cbCh, pasteCh <-chan string, timeout) (string, error)`, `complete2FALogin(c *eddsdk.Client, cfgPath, challenge string) error`, `Client.SetToken(string)` — names/signatures consistent across Tasks 1–4.

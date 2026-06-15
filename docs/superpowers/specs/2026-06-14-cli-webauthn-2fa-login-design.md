# CLI WebAuthn 2FA Login — Design

**Date:** 2026-06-14
**Status:** Approved (design phase)

## Goal

Let `ec auth login` complete login for accounts that have a security key, by delegating the WebAuthn passkey ceremony to a browser and handing the resulting session token back to the CLI. The flow must work over an SSH connection (the CLI's primary use case), where copy-paste of the token is the guaranteed return path.

## Problem

`ec auth login` does `POST /api/login` (username + password). For accounts with ≥1 registered credential, the auth service returns `{requires_2fa: true, challenge_token}` instead of a session token. The CLI currently dead-ends here with an error telling the user to fall back to a service-account token (`ec auth set-token`). A passkey ceremony needs `navigator.credentials.get`, which only a browser can perform — and only on the credential's registered origin.

## Key Constraint: WebAuthn Origin

The auth service configures the relying party as `RPID: cloud.eddisonso.com`, origin `https://cloud.eddisonso.com` (`edd-cloud-auth/cmd/auth/main.go:98-113`). A passkey can only be exercised on its registered origin. Therefore the browser verification page **must** be served from the dashboard (`cloud.eddisonso.com`), not from `auth.cloud.eddisonso.com`. The original sketch's `auth.eddisonso.com/2fa` URL is not viable.

## Backend: No Changes Required

The full ceremony already exists and already returns a pasteable session token:

- `POST /api/login {username, password}` → on 2FA accounts: `loginResponse{requires_2fa: true, challenge_token}` (`internal/api/auth.go:84-105`). The `challenge_token` is a 5-minute HS256 JWT with `Type: "2fa_challenge"`.
- `POST /api/webauthn/login/begin` with header `Authorization: Bearer <challenge_token>` → `{options, state}` (`internal/api/webauthn.go:203-258`).
- `POST /api/webauthn/login/finish {state, credential}` → `loginResponse{token, username, display_name, user_id, is_admin}` (`internal/api/webauthn.go:258-345`). `token` is the standard session JWT — exactly what the CLI stores.

This is the same begin→finish sequence the dashboard's `AuthContext.complete2FA` already drives (`frontend/src/contexts/AuthContext.tsx:157-195`). The new page reuses it verbatim.

## Architecture

Three pieces. Two get built; the backend is untouched.

```
ec auth login                    cloud.eddisonso.com/cli-2fa            auth.cloud.eddisonso.com
─────────────                    ──────────────────────────            ────────────────────────
1. prompt user/pass  ───POST /api/login──────────────────────────────────────────────▶
2. ◀── {requires_2fa, challenge_token} ───────────────────────────────────────────────
3. print link with challenge (+ optional cb=127.0.0.1:PORT when local)
4. (local only) start callback listener + auto-open browser
        │
        ▼ user opens link in browser (laptop browser when over SSH)
                                 5. read ?challenge, ?cb from URL
                                 6. POST /login/begin (Bearer challenge) ──────────────▶
                                 7. ◀── {options, state}
                                 8. navigator.credentials.get(...)  ← tap security key
                                 9. POST /login/finish {state, cred} ──────────────────▶
                                 10. ◀── {token, username, ...}
                                 11. show token in copy box  AND  (if cb) POST token→cb
        │                                    │
        ▼ callback fires (local)             ▼ user copies token (remote/SSH)
12. CLI captures token (whichever path lands first) → save to config → "Logged in as …"
```

### Return-path policy

- **Copy-paste (always works, the SSH path):** the page always renders the session token in a copy box. The CLI always keeps a `Paste token:` prompt open. This needs no open ports and no forwarding, so it works identically whether `ec` runs locally or on a remote SSH host.
- **Localhost callback (local-only convenience):** when the CLI detects it is **not** in an SSH session, it starts a short-lived `http://127.0.0.1:PORT` listener, embeds `&cb=http://127.0.0.1:PORT` in the link, and auto-opens the browser. The page best-effort `POST`s the token to `cb`. The CLI races the callback against the paste prompt and takes whichever arrives first. Over SSH the callback is omitted entirely (it could not reach the laptop's browser), and the flow falls back to paste.

SSH detection: presence of `SSH_CONNECTION` or `SSH_TTY` in the environment. When set, skip callback + auto-open.

## Component 1: Dashboard page `/cli-2fa`

**File:** `frontend/src/pages/Cli2faPage.tsx` (new), route registered in `frontend/src/App.tsx`.

**Routing:** add as a top-level route **outside** the `<Route element={<AppLayout />}>` group (and outside any auth guard), because the user holds only a challenge token, not a session. e.g. a sibling `<Route path="/cli-2fa" element={<Cli2faPage />} />` directly under `<Routes>`.

**Responsibility:**
1. Read `challenge` (required) and `cb` (optional) from `useSearchParams`.
2. On a button press ("Verify security key" — must be user-initiated; `navigator.credentials.get` requires a user gesture), run the begin→finish ceremony against `buildAuthBase()` using the existing helpers in `src/lib/webauthn.ts` (`parseRequestOptions`, `getCredential`, `serializeAssertionResponse`). This mirrors `AuthContext.complete2FA` but does **not** touch the dashboard's own auth state (it must not log the browser into the dashboard session — it only surfaces the token for the CLI).
3. On success, display the returned `token` in a read-only field with a copy-to-clipboard button and an instruction to paste it into the terminal.
4. If `cb` is present, additionally `fetch(cb, {method: "POST", body: token})` as a best-effort, errors ignored (a non-reachable `cb`, e.g. over SSH, is expected and must not surface an error).

**States to render:** prompt-to-verify (initial), verifying (spinner), success (token + copy box), error (challenge expired / verification failed, with a clear "restart `ec auth login`" message). No styling beyond the dashboard's existing component primitives.

**Security notes:**
- The page is unauthenticated by design but is inert without a valid, unexpired `challenge_token` — `/login/begin` rejects anything that isn't a live `2fa_challenge` JWT.
- The displayed token is a real session JWT. The copy box is read-only; the page does not persist it anywhere (no localStorage), only renders it for manual transfer.
- The `cb` POST goes only to a `127.0.0.1` loopback address; the page should validate `cb` parses as an `http://127.0.0.1:` or `http://localhost:` URL before posting, and ignore anything else (prevents the page from being coerced into POSTing the token to an arbitrary origin).

## Component 2: CLI `ec auth login`

**File:** `edd-cli/internal/cli/auth.go` — replace the `if res.Requires2FA { return error }` dead-end in `cmdLogin` (`auth.go:57-59`) with the 2FA flow. Helper functions may live in a new `edd-cli/internal/cli/login2fa.go`.

**Flow when `res.Requires2FA`:**
1. Build the verification URL: `https://<BaseDomain>/cli-2fa?challenge=<challenge_token>` (BaseDomain defaults to `cloud.eddisonso.com`).
2. Determine SSH: `isSSH := os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != ""`.
3. If **not** SSH: bind `127.0.0.1:0` (OS-assigned port), append `&cb=http://127.0.0.1:<port>` to the URL, start a one-shot HTTP handler that captures the POSTed token, and attempt to open the browser (`xdg-open`/`open`, best-effort — failure is fine).
4. Print the URL and instructions regardless of mode (so SSH users can copy it).
5. Wait on two paths concurrently and take the first:
   - callback handler receives a non-empty token, OR
   - the user types/pastes a token at the `Paste token (press Enter after pasting): ` prompt.
   Implement as two goroutines feeding a single channel; first non-empty token wins, then stop the listener.
6. Validate the received token is non-empty (it is a session JWT, not an `ecloud_` token — so no prefix check), store via the same path `cmdLogin` already uses (`cfg.Token = token; cfg.BaseDomain ||= "cloud.eddisonso.com"; saveConfig`). Then call `c.Session(ctx)` with the freshly stored token to fetch and confirm identity: on success print `Logged in as <username>`; if `Session` errors (e.g. token already expired), still report success storing the token but print `Logged in (could not fetch profile)` so the user knows the token was saved.

**Timeout:** the challenge JWT expires in 5 minutes; the CLI should give up with a clear message if no token arrives within ~5 minutes, telling the user to re-run `ec auth login`.

**No new SDK surface required** beyond what exists (`c.Login`, `c.Session`); the begin/finish ceremony lives entirely in the browser page, not the SDK.

## Error Handling

| Failure | Behavior |
|---|---|
| Wrong password / no such user | Existing `POST /api/login` 401 → CLI prints "invalid credentials" (unchanged). |
| Challenge expired (>5 min) before browser verifies | `/login/begin` returns 401 → page shows "challenge expired, re-run `ec auth login`". |
| Security key verification fails | `/login/finish` 401 → page shows "verification failed". |
| `cb` unreachable (SSH) | Silent; user uses the copy box. |
| CLI 5-min timeout, no token | CLI exits non-zero: "timed out waiting for 2FA; re-run `ec auth login`". |
| User has no security keys | `POST /api/login` returns a full session token directly; `Requires2FA` is false; existing path stores it (unchanged). |

## Testing

- **CLI (`edd-cli`):** unit-test the SSH-detection branch (env set vs unset → cb present/absent in URL); unit-test the two-path token race (callback-first and paste-first both resolve); unit-test the URL builder (challenge + optional cb encoding). Use the stdlib `httptest` loopback for the callback handler.
- **Frontend:** the page is thin glue over already-tested `webauthn.ts` helpers; verify it reads both query params, renders the token on a mocked successful `finish`, validates/ignores a non-loopback `cb`, and shows the error state on a mocked 401. No new e2e harness.
- **Manual:** local (auto-open + callback completes hands-free) and over SSH (copy link → laptop browser → tap key → paste token).

## Out of Scope

- Device-authorization/polling flow (would require a new auth endpoint to stash completed tokens; copy-paste already covers SSH without it).
- Any auth-service code change.
- Registering/managing security keys from the CLI.
- WebAuthn for the SDK (`eddsdk`) — the ceremony stays browser-only.

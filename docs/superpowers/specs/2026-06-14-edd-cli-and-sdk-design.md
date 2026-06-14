# edd-cli + eddsdk — Design

**Date:** 2026-06-14
**Status:** Approved

## Goal

A Go SDK and a command-line tool for edd-cloud. The **SDK** (`eddsdk`) is a
reusable, typed client for every edd-cloud service. The **CLI** (`edd`) is a
non-interactive command-line tool (subcommands per service) for both interactive
terminal use and scripting/CI. Both sit on the SDK. No TUI.

## Decisions (from brainstorming)

- **Language/home:** Go, new module `~/cloud/edd-cli/` (`eddisonso.com/edd-cli`)
  in the monorepo.
- **SDK scope:** all five services (compute, storage, registry, auth, networking).
- **CLI:** non-interactive subcommands, hand-rolled with stdlib `flag` (no cobra
  or other CLI framework), matching the ecosystem's CLI style (`go-gfs`'s
  `internal/clientcli`). No interactive TUI.
- **Auth:** `edd login` (password → session) AND `ecloud_` SA token via env/flag.
- **No external CLI/TUI dependencies** — stdlib only (the SDK uses stdlib
  `net/http`).

## Background

- Services live behind the edd-gateway at `<service>.cloud.eddisonso.com`
  (`compute`, `storage`, `registry`, `auth`; networking is `net.cloud…` per the
  frontend's `SERVICE_SUBDOMAIN_MAP`). `cloud-api.eddisonso.com` is deprecated.
- Auth is a bearer JWT: a session JWT (from `POST /api/login`) or an `ecloud_`
  service-account token. The same token authenticates every service.
- Services are separate Go modules with `internal/` types — the SDK cannot import
  them, so it defines its own DTOs against the JSON contracts.

## Module layout

```
edd-cli/
├── go.mod                      # module eddisonso.com/edd-cli, stdlib only
├── cmd/edd/main.go             # entry: parse argv → route to subcommand; no args → usage
├── pkg/eddsdk/
│   ├── client.go               # Client, NewClient(Options), host resolution, doJSON, auth
│   ├── errors.go               # APIError{Status, Message}
│   ├── types.go                # DTOs (Container, Namespace, FileInfo, Repo, ServiceAccount, Token, Domain, ...)
│   ├── compute.go              # container + ssh/ingress/mounts/pull-policy/logs methods
│   ├── storage.go              # namespaces + files
│   ├── registry.go             # repos, tags, delete
│   ├── auth.go                 # login, session, service accounts, tokens
│   └── networking.go           # domains, cloudflare connections
└── internal/cli/
    ├── app.go                  # argv parse + route to a registered command; usage/help
    ├── config.go               # ~/.config/edd/config.json, token resolution
    ├── output.go               # table (text/tabwriter) + --json
    ├── auth.go                 # login, logout, whoami
    ├── compute.go              # compute subcommands
    ├── storage.go              # storage subcommands
    ├── registry.go             # registry subcommands
    ├── accounts.go             # service-accounts + tokens subcommands
    └── networking.go           # domains + connections subcommands
```

## SDK (`pkg/eddsdk`)

- `NewClient(Options{BaseDomain string, Token string, HTTPClient *http.Client})`.
  Default `BaseDomain = cloud.eddisonso.com`. Resolves per-service base URLs
  (`https://<svc>.<baseDomain>`, with `networking→net`).
- Every request sets `Authorization: Bearer <token>` and `Content-Type:
  application/json`; a private `doJSON(ctx, method, serviceURL, path, body, out)`
  centralizes encode/decode and maps non-2xx to `APIError{Status, Message}`
  (Message parsed from the body when present).
- Methods take `context.Context` first (matching `go-gfs` SDK). Surface:
  - Compute: `ListContainers`, `GetContainer(id)`, `CreateContainer(req)`,
    `StartContainer(id)`, `StopContainer(id)`, `DeleteContainer(id)`,
    `ContainerLogs(id)`, `GetSSH(id)`/`SetSSH(id, bool)`, `ListIngress(id)`/
    `AddIngress(id, port, target)`/`RemoveIngress(id, port)`, `GetMounts(id)`/
    `SetMounts(id, paths)`, `SetPullPolicy(id, policy)`.
  - Storage: `ListNamespaces`/`CreateNamespace`/`DeleteNamespace`; `ListFiles`/
    `UploadFile`/`DownloadFile`/`DeleteFile`.
  - Registry: `ListRepos`, `ListTags(repo)`, `DeleteTag(repo, ref)`.
  - Auth: `Login(user, pass) (LoginResult, error)`, `Session()`,
    `ListServiceAccounts`/`CreateServiceAccount`/`DeleteServiceAccount`,
    `CreateToken`/`ListTokens`/`DeleteToken`.
  - Networking: `ListDomains`/`AddDomain`/`DeleteDomain`;
    `ListConnections`/`AddConnection`/`DeleteConnection`.
- DTOs in `types.go` mirror the JSON the services return — e.g. `Container`:
  `ID, Name, Status, Hostname, MemoryMB, StorageGB, InstanceType, CreatedAt,
  SSHEnabled, HTTPSEnabled, PullPolicy` (json `id,name,status,hostname,memory_mb,
  storage_gb,instance_type,created_at,ssh_enabled,https_enabled,pull_policy`).
  `CreateContainer` request: `Name, MemoryMB, StorageGB, InstanceType, SSHKeyIDs,
  SSHEnabled, MountPaths, Image, PullPolicy`. `ListContainers` decodes the
  `{"containers":[...]}` wrapper. Drift risk mitigated by same-repo + SDK tests.

## CLI (`cmd/edd` + `internal/cli`)

- `edd` with **no args** → prints usage/help and exits non-zero. `edd help`
  prints the same.
- `edd <command> [subcommand] [flags/args]` → dispatched to a registered command.
  Surface:
  - `edd login | logout | whoami`
  - `edd compute ls|get|create|start|stop|rm|logs|ssh|ingress|mounts|pull-policy`
  - `edd storage ns ls|create|rm` · `edd storage ls|cp|rm`
  - `edd registry repos|tags|rm`
  - `edd sa ls|create|rm` · `edd token create|ls|rm`
  - `edd domains ls|add|rm` · `edd net connections ls|add|rm`
- Global flags (parsed before the command, available to all): `--json` (raw JSON
  instead of tables), `--token`, `--base`.
- **Config & token resolution** (`config.go`): order is `--token` flag →
  `EDD_TOKEN` env → `~/.config/edd/config.json` session token. `edd login` writes
  the session token (and `base_domain`) to that file (mode 0600); `edd logout`
  clears it. `whoami` calls `Session()`. If `Login` returns `Requires2FA`, the CLI
  prints a message directing the user to use an `ecloud_` SA token via `EDD_TOKEN`
  (interactive WebAuthn is out of scope for the CLI).
- **Output** (`output.go`): default human tables via `text/tabwriter`; `--json`
  prints the SDK DTO as indented JSON. Non-2xx → friendly stderr message + non-zero
  exit; `--json` errors still go to stderr so stdout stays clean for piping.
- Parsing is hand-rolled with stdlib `flag` per subcommand (no cobra), matching
  the ecosystem's CLI style.

## Error handling

- SDK: every method returns `(*T, error)`; transport/decoding errors wrapped;
  HTTP non-2xx → `APIError{Status, Message}` (callers can `errors.As`).
- CLI: print `APIError.Message` (or wrapped error) to stderr, exit non-zero.

## Testing

- **SDK:** table-driven tests per method against an `httptest.Server` asserting
  method/path/headers/body and decoding, plus `APIError` mapping for non-2xx and a
  401 case.
- **CLI:** arg-parsing tests (flags, missing args → usage/error) and `output.go`
  table-vs-`--json` formatting tests; `config.go` token-resolution precedence test.

## Build & distribution

- `go build -o edd ./cmd/edd` → static binary; a `Makefile`/`install.sh` target
  copying to `~/bin` or `/usr/local/bin`. (`go install` from the private monorepo
  path is possible locally but not the primary path.)

## Build order (for the plan)

1. Module scaffold + SDK core (`client.go`, `errors.go`, host resolution, auth).
2. SDK compute methods + tests.
3. CLI: `app.go` (dispatch) + `config.go` (token resolution) + `output.go`.
4. CLI auth (`login`/`logout`/`whoami`) + SDK auth `Login`/`Session`.
5. CLI compute subcommands (ls/get/create/start/stop/rm/logs/pull-policy/ssh/
   ingress/mounts) + tests.
6. SDK + CLI subcommands for storage, registry, auth(SA/token), networking.
7. README + install target.

## Out of scope (v1)

- An interactive TUI (explicitly dropped).
- Shell completion, packaging/release automation, a shared services↔SDK types
  package (would require refactoring the services), and streaming/`exec` over the
  WebSocket terminal endpoint (logs are fetched, not streamed, in v1).

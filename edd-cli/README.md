# edd-cli

Go SDK and CLI for [edd-cloud](https://cloud.eddisonso.com) — manage compute containers, storage, registry images, service accounts, API tokens, custom domains, and Cloudflare network connections.

## Install

```sh
# Build the binary
make build          # produces ./edd
# or
go build -o edd ./cmd/edd

# Install to ~/bin
make install
```

Requires Go 1.22+. `~/bin` must be on your `$PATH`.

## Auth

Two ways to authenticate:

**Interactive login** — prompts for username and password, stores a session token in `~/.config/edd/config.json`:

```sh
edd login
```

> Accounts with 2FA/WebAuthn enabled cannot use interactive login. Create a service-account token in the dashboard and set `EDD_TOKEN` instead.

**Service-account token** — set `EDD_TOKEN` to an `ecloud_` token minted in the dashboard. Suitable for scripting and CI:

```sh
export EDD_TOKEN=ecloud_...
```

**Token precedence:** `--token` flag > `EDD_TOKEN` env > `~/.config/edd/config.json`

## Global Flags

| Flag | Description |
|------|-------------|
| `--json` | Print raw JSON output |
| `--token T` | Use token `T` for this invocation |
| `--base D` | Override base domain (default: `cloud.eddisonso.com`) |

Global flags must appear before the command name:

```sh
edd --json --token ecloud_... compute ls
```

## Commands

### Auth

```sh
edd login             # prompt for credentials, save session token
edd logout            # clear saved session token
edd whoami            # print current user (id, name, admin flag)
```

### Compute

```sh
edd compute ls
edd --json compute get <id>

edd compute create \
  --name web \
  --image registry.cloud.eddisonso.com/eddison/foo:v1.0 \
  --type nano \
  --memory 256 \
  --storage 5 \
  --pull-policy IfNotPresent

edd compute start <id>
edd compute stop <id>
edd compute rm <id>

edd compute pull-policy <id> Always      # Always | IfNotPresent
edd compute ssh <id> on                  # on | off
edd compute logs <id>

# Ingress rules
edd compute ingress ls <id>
edd compute ingress add <id> <port> <target-port>
edd compute ingress rm <id> <port>

# Storage mounts
edd compute mounts ls <id>
edd compute mounts set <id> [path ...]   # replaces current mount list
```

`compute create` flags: `--name` (required), `--image`, `--type` (default `nano`), `--memory` MB (default `256`), `--storage` GB (default `5`), `--pull-policy` (default `IfNotPresent`).

### Storage

```sh
edd storage ns ls
edd storage ns create <name>
edd storage ns rm <name>

edd storage ls <namespace>
edd storage rm <namespace> <path>
```

### Registry

```sh
edd registry repos
edd registry tags <repo>
edd registry rm <repo> <tag>
```

### Service Accounts

```sh
edd sa ls
edd sa create --name ci-bot \
  --scope compute.u.containers=read,write
edd sa rm <id>
```

`sa create` flags: `--name` (required), `--scope root.uid.resource=action1,action2` (repeatable), `--scopes-json '{"key":["action"]}'`.

### Tokens

```sh
edd token ls
edd token create --name deploy --expires-in 90d \
  --scope compute.u.containers=read
edd token rm <id>   # note: DELETE /api/tokens/{id} not yet registered in the auth service
```

`token create` flags: `--name` (required), `--expires-in` `30d|90d|365d|never` (default `never`), `--scope`, `--scopes-json`.

### Domains

```sh
edd domains ls
edd domains add <container-id> <domain> <target-port>
edd domains rm <id>
```

### Networking

```sh
edd net connections ls
edd net connections add <cloudflare-api-token>
edd net connections rm <id>
```

## SDK Usage

```go
import (
    "context"
    "fmt"
    "eddisonso.com/edd-cli/pkg/eddsdk"
)

client := eddsdk.NewClient(eddsdk.Options{
    Token: "ecloud_...",
})

containers, err := client.ListContainers(context.Background())
if err != nil {
    // *eddsdk.APIError carries Status and Message
    panic(err)
}
for _, c := range containers {
    fmt.Println(c.ID, c.Name, c.Status)
}
```

`eddsdk.Options` fields: `Token` (required for authenticated calls), `BaseDomain` (default `cloud.eddisonso.com`), `HTTPClient` (optional custom `*http.Client`).

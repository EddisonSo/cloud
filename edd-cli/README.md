# edd-cli

Go SDK and CLI for [edd-cloud](https://cloud.eddisonso.com) — manage compute containers, storage, registry images, service accounts, API tokens, custom domains, and Cloudflare network connections.

## Install

```sh
# Build the binary
make build          # produces ./ec
# or
go build -o ec ./cmd/ec

# Install to ~/bin
make install
```

Requires Go 1.22+. `~/bin` must be on your `$PATH`.

## Auth

Two ways to authenticate:

**Interactive login** — prompts for username and password, stores a session token in `~/.config/ec/config.json`:

```sh
ec login
```

> Accounts with 2FA/WebAuthn enabled cannot use interactive login. Create a service-account token in the dashboard and set `EC_TOKEN` instead.

**Service-account token** — set `EC_TOKEN` to an `ecloud_` token minted in the dashboard. Suitable for scripting and CI:

```sh
export EC_TOKEN=ecloud_...
```

**Token precedence:** `--token` flag > `EC_TOKEN` env > `~/.config/ec/config.json`

## Global Flags

| Flag | Description |
|------|-------------|
| `--json` | Print raw JSON output |
| `--token T` | Use token `T` for this invocation |
| `--base D` | Override base domain (default: `cloud.eddisonso.com`) |

Global flags must appear before the command name:

```sh
ec --json --token ecloud_... compute ls
```

## Commands

### Auth

```sh
ec login             # prompt for credentials, save session token
ec logout            # clear saved session token
ec whoami            # print current user (id, name, admin flag)
```

### Compute

```sh
ec compute ls
ec --json compute get <id>

ec compute create \
  --name web \
  --image registry.cloud.eddisonso.com/eddison/foo:v1.0 \
  --type nano \
  --memory 256 \
  --storage 5 \
  --pull-policy IfNotPresent

ec compute start <id>
ec compute stop <id>
ec compute rm <id>

ec compute pull-policy <id> Always      # Always | IfNotPresent
ec compute ssh <id> on                  # on | off
ec compute logs <id>

# Ingress rules
ec compute ingress ls <id>
ec compute ingress add <id> <port> <target-port>
ec compute ingress rm <id> <port>

# Storage mounts
ec compute mounts ls <id>
ec compute mounts set <id> [path ...]   # replaces current mount list
```

`compute create` flags: `--name` (required), `--image`, `--type` (default `nano`), `--memory` MB (default `256`), `--storage` GB (default `5`), `--pull-policy` (default `IfNotPresent`).

### Storage

```sh
ec storage ns ls
ec storage ns create <name>
ec storage ns rm <name>

ec storage ls <namespace>
ec storage rm <namespace> <path>
```

### Registry

```sh
ec registry repos
ec registry tags <repo>
ec registry rm <repo> <tag>
```

### Service Accounts

```sh
ec sa ls
ec sa create --name ci-bot \
  --scope compute.u.containers=read,write
ec sa rm <id>
```

`sa create` flags: `--name` (required), `--scope root.uid.resource=action1,action2` (repeatable), `--scopes-json '{"key":["action"]}'`.

### Tokens

```sh
ec token ls
ec token create --name deploy --expires-in 90d \
  --scope compute.u.containers=read
ec token rm <id>   # note: DELETE /api/tokens/{id} not yet registered in the auth service
```

`token create` flags: `--name` (required), `--expires-in` `30d|90d|365d|never` (default `never`), `--scope`, `--scopes-json`.

### Domains

```sh
ec domains ls
ec domains add <container-id> <domain> <target-port>
ec domains rm <id>
```

### Networking

```sh
ec net connections ls
ec net connections add <cloudflare-api-token>
ec net connections rm <id>
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

# edd-cli

Go SDK and CLI for [edd-cloud](https://cloud.eddisonso.com) — manage compute containers, SSH keys, storage, registry images, service accounts, API tokens, custom domains, and Cloudflare network connections.

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

Three ways to authenticate:

**Interactive login** — prompts for username and password, stores a session token in `~/.config/ec/config.json`:

```sh
ec auth login
```

> Accounts with 2FA/WebAuthn enabled cannot use interactive login. Use `ec auth set-token` with a service-account token instead.

**Set a service-account token** — store an `ecloud_` token to the config file for persistent use:

```sh
ec auth set-token --token ecloud_...
# or pipe it in:
echo "ecloud_..." | ec auth set-token
```

**Per-invocation token** — set `EC_TOKEN` or use `--token` for scripting and CI without touching the config:

```sh
export EC_TOKEN=ecloud_...
# or
ec --token ecloud_... compute containers ls
```

**Token precedence:** `--token` flag > `EC_TOKEN` env > `~/.config/ec/config.json`

## Global Flags

| Flag | Description |
|------|-------------|
| `--json` | Print raw JSON output |
| `--token T` | Use token `T` for this invocation |
| `--base D` | Override base domain (default: `cloud.eddisonso.com`) |

Global flags must appear before the category:

```sh
ec --json --token ecloud_... compute containers ls
```

## Commands

The CLI is organized as `ec <category> <resource> <action>`.

### Auth

```sh
ec auth login             # prompt for credentials, save session token
ec auth logout            # clear saved session token
ec auth whoami            # print current user (id, name, admin flag)
ec auth set-token         # store an ecloud_ token interactively (stdin)
ec auth set-token --token ecloud_...   # store an ecloud_ token via flag
```

#### Service Accounts

```sh
ec auth service-accounts ls
ec auth service-accounts create --name ci-bot \
  --scope compute.u.containers=read,write
ec auth service-accounts rm <id>
```

`service-accounts create` flags: `--name` (required), `--scope root.uid.resource=action1,action2` (repeatable), `--scopes-json '{"key":["action"]}'`.

#### Tokens

```sh
ec auth tokens ls
ec auth tokens create --name deploy --expires-in 90d \
  --scope compute.u.containers=read
ec auth tokens rm <id>   # note: DELETE /api/tokens/{id} not yet registered in the auth service
```

`tokens create` flags: `--name` (required), `--expires-in` `30d|90d|365d|never` (default `never`), `--scope`, `--scopes-json`.

### Compute

#### Containers

```sh
ec compute containers ls
ec --json compute containers get <id>

ec compute containers create \
  --name web \
  --image registry.cloud.eddisonso.com/eddison/foo:v1.0 \
  --type nano \
  --memory 256 \
  --storage 5 \
  --pull-policy IfNotPresent

ec compute containers start <id>
ec compute containers stop <id>
ec compute containers rm <id>

ec compute containers pull-policy <id> Always      # Always | IfNotPresent
ec compute containers ssh <id> on                  # on | off
ec compute containers logs <id>

# Ingress rules
ec compute containers ingress ls <id>
ec compute containers ingress add <id> <port> <target-port>
ec compute containers ingress rm <id> <port>

# Storage mounts
ec compute containers mounts ls <id>
ec compute containers mounts set <id> [path ...]   # replaces current mount list
```

`containers create` flags: `--name` (required), `--image`, `--type` (default `nano`), `--memory` MB (default `256`), `--storage` GB (default `5`), `--pull-policy` (default `IfNotPresent`).

#### SSH Keys

```sh
ec compute keys ls
ec compute keys add --name laptop --key "ssh-ed25519 AAAA..."
ec compute keys rm <id>
```

### Storage

#### Namespaces

```sh
ec storage namespaces ls
ec storage namespaces create <name>
ec storage namespaces rm <name>
```

#### Files

```sh
ec storage files ls <namespace>
ec storage files rm <namespace> <path>
```

#### Registry

```sh
ec storage registry ls
ec storage registry tags <repo>
ec storage registry rm <repo> <tag>
```

### Networking

#### Domains

```sh
ec networking domains ls
ec networking domains add <container-id> <domain> <target-port>
ec networking domains rm <id>
```

#### Connections

```sh
ec networking connections ls
ec networking connections add <cloudflare-api-token>
ec networking connections rm <id>
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

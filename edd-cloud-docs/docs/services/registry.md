---
sidebar_position: 9
---

# Registry Service

The Registry Service is an OCI Distribution Specification-compliant container registry backed by GFS for blob and manifest storage, and PostgreSQL for metadata. It allows users to push, pull, and manage container images using standard Docker tooling.

## Overview

- **OCI compliant**: Implements the [OCI Distribution Spec v1.1](https://github.com/opencontainers/distribution-spec) at `/v2/`
- **Storage backend**: All blobs and manifests are stored in GFS under the `core-registry` namespace — no external object storage required
- **Authentication**: Docker Token Auth (RFC 6750) flow via `auth.cloud.eddisonso.com/v2/token`
- **Garbage collection**: Background two-phase mark-and-sweep GC running on a 24-hour cycle

## Architecture

The registry sits within the Storage grouping alongside SFS (Simple File Share), both backed by GFS:

```
docker push/pull
      │
      ▼
registry.cloud.eddisonso.com  (edd-registry deployment, 2 pods)
      │
      ├── PostgreSQL (metadata: repos, manifests, tags, blobs, upload sessions)
      │
      └── GFS (blobs + manifests stored under core-registry namespace)
               ├── blobs/<sha256-hex>
               ├── uploads/<uuid>            ← in-progress chunked uploads
               └── manifests/<repo>/<sha256> ← final manifests
```

Auth tokens are issued by `auth.cloud.eddisonso.com/v2/token` and carry repository-scoped access grants (`pull`, `push`, `delete`). The registry validates these tokens using the shared `JWT_SECRET`.

## Configuration

### Environment Variables

| Variable | Required | Source | Description |
|----------|----------|--------|-------------|
| `DATABASE_URL` | Yes | K8s Secret `registry-db-credentials` | PostgreSQL connection string |
| `JWT_SECRET` | Yes | K8s Secret `edd-cloud-auth` | Shared HMAC secret for JWT validation |
| `NATS_URL` | No | Manifest env | NATS server URL (e.g. `nats://nats:4222`). When set, push and delete events are published via the notification publisher. |

### Command-line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `0.0.0.0:8080` | Listen address |
| `-master` | `gfs-master:9000` | GFS master gRPC address |

## Endpoints

All OCI Distribution API endpoints are served under `https://registry.cloud.eddisonso.com/v2/`.

### Version Check

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/v2/` | Optional | Returns `{}` with `200` if authenticated, `401` challenge if not |

### Catalog

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/v2/_catalog` | Optional | List visible repositories (paginated with `?n=&last=`). Anonymous callers see public repos only; authenticated callers see their own repos plus public repos. Never lists all repositories. |

### Blobs

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| HEAD | `/v2/{name}/blobs/{digest}` | pull | Check blob existence and size |
| GET | `/v2/{name}/blobs/{digest}` | pull | Download a blob |
| DELETE | `/v2/{name}/blobs/{digest}` | delete | Mark blob for GC |
| POST | `/v2/{name}/blobs/uploads/` | push | Start a chunked or monolithic upload |
| PATCH | `/v2/{name}/blobs/uploads/{uuid}` | push | Append a chunk to an in-progress upload |
| PUT | `/v2/{name}/blobs/uploads/{uuid}` | push | Finalize an upload (with `?digest=sha256:...`) |

### Manifests

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| HEAD | `/v2/{name}/manifests/{ref}` | pull | Check manifest by tag or digest |
| GET | `/v2/{name}/manifests/{ref}` | pull | Fetch manifest by tag or digest |
| PUT | `/v2/{name}/manifests/{ref}` | push | Push a manifest (creates or updates a tag) |
| DELETE | `/v2/{name}/manifests/{ref}` | delete | Delete a manifest or untag a tag |

### Tags

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/v2/{name}/tags/list` | pull | List tags for a repository (paginated) |

### Management API

Separate from the OCI `/v2/` API, a JSON REST API under `/api/` powers the dashboard's registry views. These endpoints are authenticated with the caller's auth-service **session token** (see [Authentication](#authentication)). Repo names may contain slashes and are parsed positionally.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/repos` | Optional | List repositories. Anonymous callers see public repos only; authenticated callers see their own repos plus public repos. |
| GET | `/api/repos/{name}` | Optional | Repo detail (name, visibility, owner, tag count, total size, last pushed). Private repos require the owner (or a session token for that user). |
| GET | `/api/repos/{name}/tags` | Optional | List tags with digest, size, and push time. Private repos require the owner (or a session token for that user). |
| PUT | `/api/repos/{name}/visibility` | Owner | Set repository visibility. Owner-only — session tokens do **not** bypass the owner check. Body: `{"visibility": <int>}`. |
| DELETE | `/api/repos/{name}/tags/{tag}` | Owner | Delete a tag (and clean up its manifest if unreferenced). Owner-only — session tokens do **not** bypass the owner check. |

CORS is enabled for `*.cloud.eddisonso.com` origins (and `https://cloud.eddisonso.com`), allowing `GET`, `PUT`, `DELETE`, and `OPTIONS`.

## Authentication

The registry uses the [Docker Token Auth](https://distribution.github.io/distribution/spec/auth/token/) flow:

1. Client sends an unauthenticated request to `/v2/`.
2. Registry returns `401 Unauthorized` with a `WWW-Authenticate` header pointing to `https://auth.cloud.eddisonso.com/v2/token`.
3. Docker client sends its credentials to the token endpoint and receives a short-lived JWT scoped to the requested repository and actions.
4. Client retries the original request with `Authorization: Bearer <token>`.

The JWT encodes the scope as `repository:<name>:<actions>` (e.g. `repository:myuser/myimage:pull,push`).

### Token Types

The registry accepts **two** kinds of bearer token, both signed with the shared `JWT_SECRET`:

1. **OCI registry tokens** — short-lived, repository-scoped tokens issued by `auth.cloud.eddisonso.com/v2/token` for `docker push`/`pull`. Access is limited to the `repository:<name>:<actions>` grants encoded in the token.
2. **Session tokens** — standard auth-service session JWTs (the `user_id`/`type` claims used by the frontend dashboard). A session token grants the caller **full access to their own repositories** (`hasAccess` returns true for any action). It carries no per-repo scope.

Because both token types share the same signing key, `authenticate` tries the **session token first**: a session JWT would also parse as a registry token, but registry parsing would wrongly use the `Subject` (username) as the user ID instead of the `user_id` claim. Order therefore matters — session validation must run before OCI validation. The resulting `authResult` sets `IsSession` so downstream checks can distinguish the two.

Note: session-token "full access to own repos" applies to the OCI `/v2/` actions via `hasAccess`. The owner-only management endpoints (`PUT .../visibility`, `DELETE .../tags/{tag}`) are *not* bypassed by `IsSession` — they compare `UserID` against the repo owner directly.

### Repository Visibility

| Value | Behavior |
|-------|----------|
| `0` (private) | Owner only — pull, push, and catalog listing all require authentication |
| `1` (public) | Pull and manifest/tags fetch allowed without a token; push and delete still require auth |

## Storage Layout

GFS namespace: `core-registry`

| Path pattern | Contents |
|---|---|
| `blobs/<sha256-hex>` | Finalized layer and config blobs |
| `uploads/<uuid>` | In-progress chunked upload data |
| `manifests/<repo-name>/<sha256-hex>` | Finalized manifest JSON |

Blob data is stored exactly once in GFS regardless of how many repositories reference it. The `repository_blobs` and `manifest_blobs` tables track reference relationships in PostgreSQL.

## Garbage Collection

GC runs as a background goroutine on a 24-hour interval using a two-phase mark-and-sweep strategy:

1. **Sweep** — delete blobs that were marked for GC (`gc_marked_at IS NOT NULL`) more than one interval ago and remove the corresponding GFS objects.
2. **Mark** — set `gc_marked_at` on blobs not referenced by any manifest (i.e. absent from `manifest_blobs`).
3. **Clean** — delete upload sessions older than 24 hours to reclaim GFS space for abandoned uploads.

The API `DELETE /v2/{name}/blobs/{digest}` only sets the GC mark; actual GFS object removal happens asynchronously during the next sweep phase. This ensures concurrent pulls are not interrupted.

## Deployment

- **Replicas**: 2 pods spread across backend nodes (`backend: "true"`)
- **Topology**: `topologySpreadConstraints` with `maxSkew: 1` ensures one pod per node
- **Manifest**: `manifests/edd-registry/edd-registry.yaml`
- **Liveness/readiness**: HTTP GET `/healthz` on port 8080 (an unauthenticated `200 OK` handler — `/v2/` is not used for probes because it requires auth and would return `401`)

## Database Schema

| Table | Purpose |
|---|---|
| `repositories` | Registry repositories — owner ID, name, visibility |
| `manifests` | Manifest records — digest, media type, size — per repository |
| `tags` | Tag-to-digest mappings per repository |
| `repository_blobs` | Blobs tracked per repository with optional `gc_marked_at` timestamp |
| `manifest_blobs` | Join table linking manifests to their referenced blob digests |
| `upload_sessions` | In-progress chunked upload state: UUID, hash checkpoint, byte count |

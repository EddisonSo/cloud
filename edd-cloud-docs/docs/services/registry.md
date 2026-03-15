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
| GET | `/v2/_catalog` | Required | List all repositories (paginated with `?n=&last=`) |

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

## Authentication

The registry uses the [Docker Token Auth](https://distribution.github.io/distribution/spec/auth/token/) flow:

1. Client sends an unauthenticated request to `/v2/`.
2. Registry returns `401 Unauthorized` with a `WWW-Authenticate` header pointing to `https://auth.cloud.eddisonso.com/v2/token`.
3. Docker client sends its credentials to the token endpoint and receives a short-lived JWT scoped to the requested repository and actions.
4. Client retries the original request with `Authorization: Bearer <token>`.

The JWT encodes the scope as `repository:<name>:<actions>` (e.g. `repository:myuser/myimage:pull,push`).

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
- **Liveness/readiness**: HTTP GET `/v2/` on port 8080

## Database Schema

| Table | Purpose |
|---|---|
| `repositories` | Registry repositories — owner ID, name, visibility |
| `manifests` | Manifest records — digest, media type, size — per repository |
| `tags` | Tag-to-digest mappings per repository |
| `repository_blobs` | Blobs tracked per repository with optional `gc_marked_at` timestamp |
| `manifest_blobs` | Join table linking manifests to their referenced blob digests |
| `upload_sessions` | In-progress chunked upload state: UUID, hash checkpoint, byte count |

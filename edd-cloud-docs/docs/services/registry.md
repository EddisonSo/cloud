---
sidebar_position: 9
---

# Registry Service

The registry service is an OCI Distribution Specification-compliant container registry backed by GFS for blob/manifest storage and PostgreSQL for metadata.

## Endpoint

`registry.cloud.eddisonso.com` (OCI v2 API at `/v2/`)

## Architecture

- **Blob storage**: Blobs and manifests are stored in GFS under the `core-registry` namespace.
  - Finalized blobs: `blobs/<sha256-hex>`
  - In-progress uploads: `uploads/<uuid>`
  - Manifests: `manifests/<repo-name>/<sha256-hex>`
- **Metadata**: Repository, manifest, tag, blob, and upload-session records live in PostgreSQL.
- **Auth**: Bearer JWT tokens issued by `auth.cloud.eddisonso.com/v2/token`. Tokens encode repository-scoped access grants (`pull`, `push`, `delete`).

## Database Schema

| Table | Purpose |
|---|---|
| `repositories` | Registry repositories with owner and visibility |
| `manifests` | Manifest records (digest, media type, size) per repository |
| `tags` | Tag-to-manifest-digest mappings |
| `repository_blobs` | Blobs tracked per repository with optional GC mark |
| `manifest_blobs` | Join table: manifests → blob digests |
| `upload_sessions` | In-progress chunked upload state (UUID, hash checkpoint, byte count) |

## OCI API Coverage

| Method | Path | Description |
|---|---|---|
| GET | `/v2/` | API version check |
| GET | `/v2/_catalog` | Repository catalog (paginated) |
| HEAD/GET | `/v2/{name}/blobs/{digest}` | Check / download a blob |
| DELETE | `/v2/{name}/blobs/{digest}` | Mark a blob for GC |
| POST | `/v2/{name}/blobs/uploads/` | Start a chunked or monolithic upload |
| PATCH | `/v2/{name}/blobs/uploads/{uuid}` | Append a chunk |
| PUT | `/v2/{name}/blobs/uploads/{uuid}` | Finalize an upload |
| HEAD/GET | `/v2/{name}/manifests/{ref}` | Check / fetch a manifest by tag or digest |
| PUT | `/v2/{name}/manifests/{ref}` | Push a manifest |
| DELETE | `/v2/{name}/manifests/{ref}` | Delete a manifest or tag |
| GET | `/v2/{name}/tags/list` | List tags for a repository |

## Garbage Collection

GC runs as a background goroutine on a 24-hour interval with three phases:

1. **Sweep** — delete blobs that were marked for GC more than one interval ago and remove the corresponding GFS objects.
2. **Mark** — set `gc_marked_at` on blobs not referenced by any manifest.
3. **Clean** — delete upload sessions older than 24 hours.

Blob deletion via the API only marks the blob for GC; actual GFS object removal happens during the sweep phase.

## Configuration

| Environment Variable | Required | Description |
|---|---|---|
| `DATABASE_URL` | Yes | PostgreSQL connection string (from K8s Secret) |
| `JWT_SECRET` | Yes | Shared HMAC secret for JWT validation (from K8s Secret) |
| `NATS_URL` | No | NATS server URL (e.g. `nats://nats:4222`). When set, push and delete events are published via the notification publisher. |

| Flag | Default | Description |
|---|---|---|
| `-addr` | `0.0.0.0:8080` | Listen address |
| `-master` | `gfs-master:9000` | GFS master address |

## Visibility

Repository visibility is stored as an integer in `repositories.visibility`:
- `0` — private (owner only)
- `1` — public (pull without authentication)

Public repositories appear in the catalog for anonymous requests and do not require a token for pull/manifest/tags operations.

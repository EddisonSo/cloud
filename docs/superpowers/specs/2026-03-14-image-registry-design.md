# Image Registry Service — Design Spec

**Date:** 2026-03-14
**Status:** Draft
**Service:** `edd-cloud-registry`
**Domain:** `registry.cloud.eddisonso.com`

## Overview

A self-hosted OCI-compatible container image registry backed by GFS. Sits alongside SFS as a sibling under the "Storage" umbrella. CI/CD pushes built images, the Kubernetes cluster pulls from it, and users can push/pull their own images.

The compute service is augmented to let users select registry images when creating containers, providing a real end-to-end proof of concept.

## Service Identity & Routing

- **Service name:** `edd-cloud-registry`
- **Location:** `edd-cloud-interface/services/registry/`
- **Domain:** `registry.cloud.eddisonso.com`
- **GFS namespace:** `core-registry`
- **Gateway route:** `registry.cloud.eddisonso.com/* -> edd-cloud-registry:80`
- **Deployment:** 2 replicas on `backend=true` nodes (rp2-rp4), scales to 4

```
Storage (conceptual grouping)
+-- SFS (storage.cloud.eddisonso.com)       -> GFS namespace: /sfs/{user-namespace}
+-- Registry (registry.cloud.eddisonso.com)  -> GFS namespace: core-registry
```

GFS namespace follows the `core-` convention used by internal services (e.g., `core-logs` for the log service).

## OCI Distribution Spec — Endpoints

Implements the full OCI Distribution Specification. The `/v2/` prefix is mandated by the spec — all OCI clients (docker, podman, crictl) expect it.

### API Version Check
- `GET /v2/` — Returns 200 OK (required handshake for all OCI clients)

### Blob Operations (image layers, config)
- `HEAD /v2/{name}/blobs/{digest}` — Check blob exists, return size
- `GET /v2/{name}/blobs/{digest}` — Download blob
- `DELETE /v2/{name}/blobs/{digest}` — Delete blob
- `POST /v2/{name}/blobs/uploads/` — Initiate chunked upload, returns UUID
- `PATCH /v2/{name}/blobs/uploads/{uuid}` — Stream chunk data
- `PUT /v2/{name}/blobs/uploads/{uuid}?digest=sha256:...` — Complete upload, verify digest

### Manifest Operations
- `GET /v2/{name}/manifests/{reference}` — Get manifest by tag or digest
- `HEAD /v2/{name}/manifests/{reference}` — Check manifest exists
- `PUT /v2/{name}/manifests/{reference}` — Push manifest
- `DELETE /v2/{name}/manifests/{reference}` — Delete manifest

### Catalog & Tags
- `GET /v2/_catalog` — List repositories (supports `Link` header pagination)
- `GET /v2/{name}/tags/list` — List tags for a repository

`{name}` is the repository path (e.g., `ecloud-auth` or `eddison/ecloud-auth`).

## GFS Storage Layout

All files use GFS namespace `core-registry`. Paths below are relative to that namespace:

```
Namespace: core-registry
  blobs/sha256/{digest}                    # Immutable content-addressable blobs
  manifests/{repository}/sha256/{digest}   # Manifest by digest
  uploads/{uuid}                           # In-progress chunked uploads (temporary)
```

Tag-to-digest mappings live in PostgreSQL only (not in GFS) to avoid dual-source-of-truth consistency issues.

Container image layers are content-addressable and immutable — a natural fit for GFS's append-oriented, large sequential I/O design. Layers are typically 10-200MB compressed, mapping to 1-3 GFS chunks (64MB each).

## PostgreSQL Schema

```sql
-- Repository catalog
CREATE TABLE repositories (
    id SERIAL PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    owner_id INT,
    visibility INT DEFAULT 0,       -- 0=private, 1=public
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Tag -> digest mapping (mutable — tags can be re-pushed)
CREATE TABLE tags (
    id SERIAL PRIMARY KEY,
    repository_id INT REFERENCES repositories(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    manifest_digest TEXT NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(repository_id, name)
);

-- Manifest metadata (for digest-based lookups and GC enumeration)
CREATE TABLE manifests (
    id SERIAL PRIMARY KEY,
    repository_id INT REFERENCES repositories(id) ON DELETE CASCADE,
    digest TEXT NOT NULL,
    media_type TEXT NOT NULL,
    size BIGINT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(repository_id, digest)
);

-- Blob references per repository (for GC)
CREATE TABLE repository_blobs (
    repository_id INT REFERENCES repositories(id) ON DELETE CASCADE,
    digest TEXT NOT NULL,
    size BIGINT,
    gc_marked_at TIMESTAMPTZ,           -- Set by GC mark phase; blob deleted on next cycle
                                        -- if still marked. NULL = not marked.
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY(repository_id, digest)
);

-- Which blobs each manifest references (populated on manifest push)
CREATE TABLE manifest_blobs (
    manifest_id INT REFERENCES manifests(id) ON DELETE CASCADE,
    blob_digest TEXT NOT NULL,
    PRIMARY KEY(manifest_id, blob_digest)
);

-- In-progress upload sessions
CREATE TABLE upload_sessions (
    uuid TEXT PRIMARY KEY,
    repository_id INT REFERENCES repositories(id) ON DELETE CASCADE,
    hash_state BYTEA,               -- Serialized SHA256 state (Go's crypto/sha256
                                    -- implements encoding.BinaryMarshaler since Go 1.13)
    bytes_received BIGINT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

- `tags` — fast tag-to-digest lookups, sole source of truth for tag mappings
- `manifests` — enables digest-based lookups (`GET /v2/{name}/manifests/sha256:...`) and GC enumeration without scanning GFS
- `repository_blobs` — tracks which repos reference which blobs for garbage collection; `gc_marked_at` supports two-cycle mark-then-delete
- `manifest_blobs` — join table linking manifests to their referenced blobs, populated during manifest push. Enables SQL-only GC without re-parsing manifests from GFS.
- `upload_sessions` — persists upload state across PATCH requests; hash state serialized via Go's `encoding.BinaryMarshaler`

All foreign keys use `ON DELETE CASCADE` so repository deletion automatically cleans up related rows.

## Authentication

Uses the Docker Token Authentication protocol, integrated with `edd-cloud-auth`.

### Flow

1. Client calls any endpoint without credentials
2. Registry returns `401` with `WWW-Authenticate: Bearer realm="https://auth.cloud.eddisonso.com/v2/token",service="registry.cloud.eddisonso.com",scope="repository:{name}:{action}"`
3. Client calls auth service token endpoint with credentials (Basic auth)
4. Auth service validates credentials, returns short-lived JWT with granted scopes
5. Client retries with `Authorization: Bearer <token>`
6. Registry validates JWT and checks scopes

### New Endpoint on edd-cloud-auth

`GET /v2/token?service=...&scope=...` — Issues registry-scoped JWTs.

Accepts both:
- **User credentials** — username + password
- **Service account credentials** — name + `ecloud_` API token

### Scope Format

The OCI spec requires scopes in the format `repository:{name}:{actions}` (e.g., `repository:ecloud-auth:pull,push`). This is a different format from the existing API token scope system (`root.userid.resource.id` with `[create, read, update, delete]` actions).

**Approach:** The `/v2/token` endpoint on `edd-cloud-auth` is a self-contained handler that speaks the Docker token protocol. It:
1. Validates credentials against the existing user/identity store
2. Checks the user's access to the requested repository (owner check or public visibility)
3. Issues a short-lived JWT with OCI-formatted scopes in the claims

This keeps OCI scopes isolated to the `/v2/token` endpoint. The registry validates these JWTs directly — it does not go through the existing API token scope system. The existing `validRoots` and hierarchical scope format are unaffected.

For service accounts, the existing `ecloud_` token is used for authentication only. The `/v2/token` endpoint maps the service account's existing scopes (e.g., `storage.<uid>.registry.<repo>.read`) to OCI scopes (`repository:<repo>:pull`) in the issued JWT.

### Access Control

| Identity | Pull (public) | Pull (private) | Push |
|----------|--------------|----------------|------|
| Anonymous | Yes | No | No |
| Authenticated user | Yes | Own repos | Own repos |
| Service account | Yes | Scoped repos | Scoped repos |

### Catalog Permissions

`GET /v2/_catalog` returns a filtered list:
- **Anonymous** — public repositories only
- **Authenticated user** — own repositories + public repositories
- **Service account** — repositories within their scopes + public repositories

### Docker Login

```bash
# User login
docker login registry.cloud.eddisonso.com
Username: eddison
Password: ********

# Service account login
docker login registry.cloud.eddisonso.com
Username: <service-account-name>
Password: ecloud_xxxxxxxxxxxxxxxx
```

## Push Flow

1. `POST /v2/{name}/blobs/uploads/` — Create GFS file at `uploads/{uuid}` in namespace `core-registry`, initialize SHA256 hasher, create upload session in PostgreSQL.
2. `PATCH /v2/{name}/blobs/uploads/{uuid}` — Stream through `io.TeeReader(body, hasher)` -> `AppendFromWithNamespace` to GFS. Hash state updates incrementally. Hash state checkpointed to `upload_sessions.hash_state` after each PATCH so uploads survive pod restarts.
3. `PUT /v2/{name}/blobs/uploads/{uuid}?digest=sha256:abc...` — Finalize hash, verify against provided digest. If match:
   - Check if `blobs/sha256/abc...` already exists in namespace `core-registry` (dedup). If so, delete upload file and record reference only.
   - Otherwise, `RenameWithNamespace("uploads/{uuid}", "blobs/sha256/abc...", "core-registry")` — instant metadata-only operation on GFS master. No data copy.
   - **Concurrent dedup race:** If rename fails with "already exists" (two concurrent pushes of the same blob), treat as successful dedup — delete the upload file and record the reference.
   - Insert into `repository_blobs`. Clean up upload session (best-effort — stale session reaper at 24h is the safety net if cleanup fails mid-transaction).
4. Repeat for each layer blob and config blob.
5. `PUT /v2/{name}/manifests/{tag}` — Validate manifest (check `mediaType`, `schemaVersion`, verify referenced blobs exist in `repository_blobs`). Store manifest in GFS, upsert into `manifests` and `tags` tables, populate `manifest_blobs` join table in PostgreSQL.
6. Emit NATS event `registry.image.pushed`.

**Memory per concurrent push:** ~128KB (two 64KB streaming buffers for GFS double-buffering) + SHA256 state (112 bytes). With 100 concurrent uploads: ~12.5MB total. This requires a new GFS SDK option `WithUploadBufferSize(64 * 1024)` to override the default 64MB buffer (see GFS Enhancements section).

**Resumable uploads:** `Content-Range` header support (OCI spec for resumable chunked uploads) is deferred to v2. In v1, a failed PATCH requires restarting the upload from the last checkpointed position (tracked via `bytes_received` in `upload_sessions`).

## Pull Flow

1. Client authenticates (token auth flow).
2. `GET /v2/{name}/manifests/{reference}` — If `reference` is a tag, look up digest in `tags` table. If `reference` is a digest, look up directly in `manifests` table. Read manifest content from GFS, return with content type.
3. Client parses manifest, identifies layer digests.
4. For each layer: `GET /v2/{name}/blobs/sha256:abc...` — Stream from GFS via `ReadToWithNamespace` directly to HTTP response. Bounded memory, same pattern as SFS downloads.

## GFS Enhancements

Two enhancements to the GFS SDK and master:

### 1. Rename (cross-path within namespace)

The push flow requires renaming upload temp files to their final blob path. GFS master already tracks file->chunk mappings. Rename is a metadata-only operation: update the file path key in the master's file table. Chunks don't move.

`RenameFileWithNamespace` already exists in the GFS SDK (`go-gfs/pkg/go-gfs-sdk/metadata.go`), backed by the `RenameFile` RPC in `master.proto`. The enhancement needed is to verify it supports cross-directory paths within the same namespace (e.g., renaming from `uploads/{uuid}` to `blobs/sha256/{digest}`). If cross-path rename already works, no GFS changes are needed for this item.

### 2. Configurable Upload Buffer Size

The current GFS SDK uses two 64MB buffers for double-buffering during `AppendFromWithNamespace` (128MB per concurrent upload). For the registry, where many concurrent uploads are expected, this is too expensive.

**New client option:** `WithUploadBufferSize(size int)` — adds a new `uploadBufferSize` field to `clientConfig` that only affects the double-buffer allocation in `AppendFrom`. The existing `maxChunkSize` continues to govern chunk boundary calculations and chunk allocation RPCs. When `uploadBufferSize` is set, `AppendFrom` uses it instead of `maxChunkSize` for `make([]byte, ...)`. Default remains `maxChunkSize` (64MB) for backward compatibility. Registry uses 64KB (128KB total per upload).

```go
client, err := gfs.NewClient(masterAddr,
    gfs.WithUploadBufferSize(64 * 1024),  // 64KB per buffer (independent of chunk size)
)
```

## Garbage Collection

### Blob Cleanup (periodic, batched)

Background goroutine runs on configurable interval (default: daily):

1. Query all blob digests referenced by active manifests via the `manifest_blobs` join table (SQL-only, no GFS reads)
2. Compare against `repository_blobs` — any blob not in the referenced set is orphaned
3. **Mark phase:** Set `gc_marked_at = NOW()` on orphaned blobs (do NOT delete immediately)
4. **Sweep phase:** Delete blobs where `gc_marked_at` is non-NULL and older than the previous GC cycle (i.e., marked in a prior run). Remove from both `repository_blobs` and GFS.
5. Clean up stale upload sessions (older than 24h)

The two-cycle mark-then-delete approach prevents a race condition where a concurrent push references a blob between the mark and delete phases. A blob must be orphaned for two consecutive GC cycles before deletion. If a blob is re-referenced between cycles, clear its `gc_marked_at` back to NULL during manifest push.

### Manifest Cleanup (eager)

- On tag update: if old manifest digest has no other tags pointing to it, delete manifest from GFS and `manifests` table immediately
- On tag delete: same check

Manifests are small (few KB), so eager deletion is efficient. Note: after a manifest is deleted, `GET /v2/{name}/manifests/sha256:{old-digest}` will return 404. Digest-based access to untagged manifests is not preserved in v1.

### Future (Scale C)

Move GC to a separate CronJob to avoid competing with serve traffic.

## NATS Events

Proto definition in `proto/registry/events.proto`, same pattern as `proto/sfs/events.proto`.

| Event | Fields |
|-------|--------|
| `registry.image.pushed` | repository, tag, digest, user/service-account |
| `registry.image.deleted` | repository, tag, digest, user/service-account |
| `registry.repository.created` | repository name, owner |
| `registry.repository.deleted` | repository name |

Consumed by the notification service for user-facing alerts. Future: webhook delivery for CI/CD triggers.

## Compute Service Integration

The compute service is augmented to let users create containers from registry images.

### Current State

- All containers use hardcoded `eddisonso/ecloud-compute-base:latest`
- `containers.image` column exists in DB but is always set to the default
- Frontend has no image picker

### Changes

**Backend (`edd-cloud-interface/services/compute/`):**
- New endpoint to list available images: query registry for repos/tags the user can access, combine with built-in images
- Accept `image` field in `POST /compute/containers` — validate against allowed list
- On pod creation with a registry image: create `imagePullSecret` in the container's namespace with the user's registry credentials

**Frontend (`CreateContainerForm.tsx`):**
- Add image picker: show built-in images (Debian base) alongside registry images the user has access to
- Selected image passed in container creation request

**Image Pull Authentication:**
- Per-namespace `imagePullSecret` (Kubernetes secret type `kubernetes.io/dockerconfigjson`)
- Created during container provisioning, scoped to the user's registry access
- Follows existing security model: per-user namespaces, strict NetworkPolicy, RBAC scopes

### Why imagePullSecret over node-level containerd config

Node-level config gives blanket access to all images for any pod. `imagePullSecret` per namespace maintains isolation — each user's containers can only pull images they have access to. Consistent with the existing security model.

## Limits & Quotas

Upload size limits and per-user/per-repository storage quotas are deferred to v2. For v1, the registry accepts any upload size that GFS can handle. Monitoring via logs and `repository_blobs` table aggregation will inform quota thresholds.

## Testing Strategy

### Unit Tests
- Digest verification logic (SHA256 streaming hash)
- Auth token scope parsing and OCI scope translation
- Manifest parsing and validation (mediaType, schemaVersion, blob reference check)

### Integration Tests
- Full push/pull cycle against running registry with GFS
- Auth flow end-to-end (docker login -> push -> pull)
- Deduplication (push same layer from two repos, verify single blob in GFS)
- Concurrent dedup race (two simultaneous pushes of same blob)
- GC blob cleanup (push, delete tag, run two GC cycles, verify blob removed)
- Upload session recovery after pod restart

### Smoke Test
```bash
docker login registry.cloud.eddisonso.com
docker tag alpine:latest registry.cloud.eddisonso.com/test:v1
docker push registry.cloud.eddisonso.com/test:v1
docker pull registry.cloud.eddisonso.com/test:v1
```

### Compute Integration Test
1. Push image to registry
2. Create container via compute API selecting that image
3. Verify pod runs with the correct image

## Out of Scope (Follow-up)

- **CI/CD integration** — Updating existing pipelines to push to the registry
- **Cluster-wide pull** — Configuring all workloads to pull from the registry instead of Docker Hub
- **Image vulnerability scanning**
- **Webhook delivery** on push events
- **Multi-architecture manifest lists**
- **Registry UI** in the dashboard (browsing images, tags, layers)
- **Content-Range resumable uploads** — v1 restarts from last checkpoint
- **Upload size limits / storage quotas** — v1 relies on monitoring

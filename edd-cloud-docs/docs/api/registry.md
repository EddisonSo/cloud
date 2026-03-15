---
sidebar_position: 5
---

# Registry API

Base URL: `https://registry.cloud.eddisonso.com`

The Registry implements the [OCI Distribution Specification v1.1](https://github.com/opencontainers/distribution-spec). All endpoints are under `/v2/`.

## Authentication

The registry uses the Docker Token Auth flow. Docker handles this automatically when you run `docker login`.

### Login

```bash
docker login registry.cloud.eddisonso.com
# Prompts for username and password (same credentials as cloud.eddisonso.com)
```

Under the hood:
1. Docker sends a request to `/v2/` — the registry returns `401` with a `WWW-Authenticate: Bearer realm="https://auth.cloud.eddisonso.com/v2/token"` header.
2. Docker exchanges your credentials at `auth.cloud.eddisonso.com/v2/token?service=registry&scope=repository:<name>:<actions>`.
3. Docker uses the returned JWT for subsequent requests.

### Manual Token Fetch

```bash
TOKEN=$(curl -s \
  "https://auth.cloud.eddisonso.com/v2/token?service=registry&scope=repository:myuser/myimage:pull,push" \
  -u "myuser:mypassword" | jq -r .token)
```

---

## Version Check

### GET /v2/

Returns `{}` with HTTP 200 when authenticated. Returns `401` with a `WWW-Authenticate` challenge for unauthenticated requests.

**Auth:** Optional (triggers auth challenge when absent)

```bash
curl https://registry.cloud.eddisonso.com/v2/ \
  -H "Authorization: Bearer $TOKEN"
# Response: {}
```

---

## Catalog

### GET /v2/_catalog

List all repositories visible to the authenticated user.

**Auth:** Required
**Query params:** `n` (page size), `last` (pagination cursor)

```bash
curl https://registry.cloud.eddisonso.com/v2/_catalog \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
{
  "repositories": ["myuser/myimage", "myuser/anotherimage"]
}
```

**Paginated:**
```bash
curl "https://registry.cloud.eddisonso.com/v2/_catalog?n=10&last=myuser/myimage" \
  -H "Authorization: Bearer $TOKEN"
```

---

## Blobs

### HEAD /v2/\{name\}/blobs/\{digest\}

Check if a blob exists. Returns `200` with `Content-Length` if found, `404` if not.

**Auth:** `pull` on `\{name\}`

```bash
curl -I "https://registry.cloud.eddisonso.com/v2/myuser/myimage/blobs/sha256:abc123..." \
  -H "Authorization: Bearer $TOKEN"
```

---

### GET /v2/\{name\}/blobs/\{digest\}

Download a blob by its content digest.

**Auth:** `pull` on `\{name\}`

```bash
curl -o layer.tar.gz \
  "https://registry.cloud.eddisonso.com/v2/myuser/myimage/blobs/sha256:abc123..." \
  -H "Authorization: Bearer $TOKEN"
```

**Response:** Raw blob bytes with `Content-Type: application/octet-stream`.

---

### DELETE /v2/\{name\}/blobs/\{digest\}

Mark a blob for garbage collection. The blob is not removed from GFS immediately — it is swept on the next GC cycle (every 24 hours).

**Auth:** `delete` on `\{name\}`

```bash
curl -X DELETE \
  "https://registry.cloud.eddisonso.com/v2/myuser/myimage/blobs/sha256:abc123..." \
  -H "Authorization: Bearer $TOKEN"
# Response: 202 Accepted
```

---

### POST /v2/\{name\}/blobs/uploads/

Start a blob upload session. Supports both chunked and monolithic (single-PUT) uploads.

**Auth:** `push` on `\{name\}`

**Monolithic upload** (include `digest` query param and full body):
```bash
curl -X POST \
  "https://registry.cloud.eddisonso.com/v2/myuser/myimage/blobs/uploads/?digest=sha256:abc123..." \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @layer.tar.gz
# Response: 201 Created, Location: /v2/myuser/myimage/blobs/sha256:abc123...
```

**Chunked upload** (no digest, returns upload UUID):
```bash
curl -X POST \
  "https://registry.cloud.eddisonso.com/v2/myuser/myimage/blobs/uploads/" \
  -H "Authorization: Bearer $TOKEN"
# Response: 202 Accepted
# Location: /v2/myuser/myimage/blobs/uploads/<uuid>
# Range: 0-0
```

---

### PATCH /v2/\{name\}/blobs/uploads/\{uuid\}

Append a chunk to an in-progress upload.

**Auth:** `push` on `\{name\}`

| Param | Type | In | Description |
|-------|------|----|-------------|
| uuid | string | path | Upload session UUID |
| Content-Range | string | header | Optional byte range (e.g. `0-1023`) |

```bash
curl -X PATCH \
  "https://registry.cloud.eddisonso.com/v2/myuser/myimage/blobs/uploads/<uuid>" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @chunk1.bin
# Response: 202 Accepted
# Range: 0-<bytes_received>
```

---

### PUT /v2/\{name\}/blobs/uploads/\{uuid\}

Finalize a chunked upload. Optionally include the last chunk in the body.

**Auth:** `push` on `\{name\}`

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| uuid | string | path | Yes | Upload session UUID |
| digest | string | query | Yes | Expected `sha256:...` digest of the full blob |

```bash
curl -X PUT \
  "https://registry.cloud.eddisonso.com/v2/myuser/myimage/blobs/uploads/<uuid>?digest=sha256:abc123..." \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/octet-stream"
# Response: 201 Created
# Location: /v2/myuser/myimage/blobs/sha256:abc123...
```

---

## Manifests

### HEAD /v2/\{name\}/manifests/\{ref\}

Check if a manifest exists. `\{ref\}` can be a tag name or a `sha256:...` digest.

**Auth:** `pull` on `\{name\}`

```bash
curl -I \
  "https://registry.cloud.eddisonso.com/v2/myuser/myimage/manifests/latest" \
  -H "Authorization: Bearer $TOKEN"
```

---

### GET /v2/\{name\}/manifests/\{ref\}

Fetch a manifest by tag or digest.

**Auth:** `pull` on `\{name\}`

```bash
curl \
  "https://registry.cloud.eddisonso.com/v2/myuser/myimage/manifests/latest" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/vnd.oci.image.manifest.v1+json"
```

**Response:**
```json
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "config": {
    "mediaType": "application/vnd.oci.image.config.v1+json",
    "digest": "sha256:...",
    "size": 1234
  },
  "layers": [
    {
      "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
      "digest": "sha256:...",
      "size": 5678
    }
  ]
}
```

---

### PUT /v2/\{name\}/manifests/\{ref\}

Push a manifest. Creates a new tag or updates an existing one. All referenced blobs must already be uploaded.

**Auth:** `push` on `\{name\}`

```bash
curl -X PUT \
  "https://registry.cloud.eddisonso.com/v2/myuser/myimage/manifests/v1.0.0" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/vnd.oci.image.manifest.v1+json" \
  -d @manifest.json
# Response: 201 Created
# Location: /v2/myuser/myimage/manifests/sha256:<digest>
# Docker-Content-Digest: sha256:<digest>
```

---

### DELETE /v2/\{name\}/manifests/\{ref\}

Delete a manifest by tag or digest. Deleting a tag removes only the tag. Deleting a digest removes the manifest record and all tags pointing to it.

**Auth:** `delete` on `\{name\}`

```bash
curl -X DELETE \
  "https://registry.cloud.eddisonso.com/v2/myuser/myimage/manifests/v1.0.0" \
  -H "Authorization: Bearer $TOKEN"
# Response: 202 Accepted
```

---

## Tags

### GET /v2/\{name\}/tags/list

List all tags for a repository.

**Auth:** `pull` on `\{name\}`
**Query params:** `n` (page size), `last` (pagination cursor)

```bash
curl \
  "https://registry.cloud.eddisonso.com/v2/myuser/myimage/tags/list" \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
{
  "name": "myuser/myimage",
  "tags": ["latest", "v1.0.0", "v1.1.0"]
}
```

---

## Dashboard API

These endpoints are used by the cloud dashboard (`cloud.eddisonso.com`) and accept session JWTs issued by the auth service. They are separate from the OCI `/v2/` endpoints used by Docker.

**Auth:** Session JWT in `Authorization: Bearer <token>` header.

CORS is enabled for `cloud.eddisonso.com` and `*.cloud.eddisonso.com` origins.

---

### GET /api/repos

List repositories. Authenticated users see their own repositories plus all public repositories. Unauthenticated callers see only public repositories.

**Auth:** Optional (session)

```bash
curl https://registry.cloud.eddisonso.com/api/repos \
  -H "Authorization: Bearer $SESSION_TOKEN"
```

**Response:**
```json
{
  "repositories": [
    {
      "name": "myuser/myapp",
      "visibility": 1,
      "owner_id": "usr_abc123",
      "tag_count": 3,
      "total_size": 52428800,
      "last_pushed": "2026-03-15T17:00:00Z"
    }
  ]
}
```

`visibility`: `1` = public, `0` = private.

---

### GET /api/repos/\{name\}

Get details for a single repository.

**Auth:** Session (required for private repositories; optional for public)

```bash
curl https://registry.cloud.eddisonso.com/api/repos/myuser/myapp \
  -H "Authorization: Bearer $SESSION_TOKEN"
```

---

### GET /api/repos/\{name\}/tags

List tags for a repository, including digest, size, and push date.

**Auth:** Session (required for private repositories; optional for public)

```bash
curl https://registry.cloud.eddisonso.com/api/repos/myuser/myapp/tags \
  -H "Authorization: Bearer $SESSION_TOKEN"
```

**Response:**
```json
{
  "name": "myuser/myapp",
  "tags": [
    {
      "name": "v1.0.0",
      "digest": "sha256:611fec88...",
      "size": 25165824,
      "pushed_at": "2026-03-15T17:00:00Z"
    }
  ]
}
```

---

### PUT /api/repos/\{name\}/visibility

Toggle a repository between public and private. Only the owner may change visibility.

**Auth:** Session (owner only)

```bash
curl -X PUT https://registry.cloud.eddisonso.com/api/repos/myuser/myapp/visibility \
  -H "Authorization: Bearer $SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"visibility": 1}'
```

`visibility`: `1` = public, `0` = private.

**Response:** `204 No Content`

---

### DELETE /api/repos/\{name\}/tags/\{tag\}

Delete a tag. Only the owner may delete tags. If no other tag points to the same manifest, the manifest and unreferenced blobs are scheduled for garbage collection.

**Auth:** Session (owner only)

```bash
curl -X DELETE \
  "https://registry.cloud.eddisonso.com/api/repos/myuser/myapp/tags/v1.0.0" \
  -H "Authorization: Bearer $SESSION_TOKEN"
```

**Response:** `204 No Content`

---

## Push/Pull Examples

### Push an image

```bash
# Tag an existing local image
docker tag myapp:latest registry.cloud.eddisonso.com/myuser/myapp:v1.0.0

# Push
docker push registry.cloud.eddisonso.com/myuser/myapp:v1.0.0
```

### Pull an image

```bash
docker pull registry.cloud.eddisonso.com/myuser/myapp:v1.0.0
```

### Use as base image in Dockerfile

```dockerfile
FROM registry.cloud.eddisonso.com/myuser/mybase:latest
RUN apt-get install -y mypackage
```

### Pull in a Kubernetes pod

Kubernetes nodes must have the `regcred` image pull secret configured to pull from the registry:

```yaml
spec:
  imagePullSecrets:
    - name: regcred
  containers:
    - name: myapp
      image: registry.cloud.eddisonso.com/myuser/myapp:v1.0.0
```

---

## Smoke Test Commands

Run these to verify the registry is healthy after deployment:

```bash
# 1. Check API version endpoint
curl -s -o /dev/null -w "%{http_code}" \
  https://registry.cloud.eddisonso.com/v2/
# Expected: 401 (auth challenge — registry is up)

# 2. Get a token
TOKEN=$(curl -s \
  "https://auth.cloud.eddisonso.com/v2/token?service=registry&scope=repository:myuser/test:pull,push" \
  -u "myuser:mypassword" | jq -r .token)

# 3. Confirm authenticated access
curl -s -o /dev/null -w "%{http_code}" \
  https://registry.cloud.eddisonso.com/v2/ \
  -H "Authorization: Bearer $TOKEN"
# Expected: 200

# 4. List catalog
curl -s \
  https://registry.cloud.eddisonso.com/v2/_catalog \
  -H "Authorization: Bearer $TOKEN"

# 5. Push and pull a test image
docker login registry.cloud.eddisonso.com
docker pull alpine:3.20
docker tag alpine:3.20 registry.cloud.eddisonso.com/myuser/test:smoke
docker push registry.cloud.eddisonso.com/myuser/test:smoke
docker pull registry.cloud.eddisonso.com/myuser/test:smoke
```

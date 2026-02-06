---
sidebar_position: 2
---

# API Reference

Edd Cloud exposes REST APIs across three services. All authenticated endpoints accept a JWT in the `Authorization: Bearer <token>` header — either a session JWT from login or an [API token](./api-tokens).

## Base URLs

| Service | URL |
|---------|-----|
| Auth | `https://auth.cloud.eddisonso.com` |
| Compute | `https://compute.cloud.eddisonso.com` |
| Storage | `https://storage.cloud.eddisonso.com` |

---

## Authentication

### Login

```
POST /api/login
```

**Host:** `auth.cloud.eddisonso.com`

Authenticate with username and password. Returns a session JWT.

**Request:**

```json
{
  "username": "eddison",
  "password": "secret"
}
```

**Response:**

```json
{
  "username": "eddison",
  "display_name": "Eddison",
  "user_id": "XyZ123",
  "is_admin": false,
  "token": "eyJhbGciOiJIUzI1NiIs..."
}
```

Store the `token` and pass it as `Authorization: Bearer <token>` on subsequent requests. The `user_id` is needed when creating [API tokens](./api-tokens) with scoped permissions.

**Example:**

```bash
TOKEN=$(curl -s -X POST https://auth.cloud.eddisonso.com/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"eddison","password":"secret"}' | jq -r '.token')
```

### Get Session

```
GET /api/session
```

**Host:** `auth.cloud.eddisonso.com`
**Auth:** Required

Returns the current user's session info.

**Response:**

```json
{
  "username": "eddison",
  "display_name": "Eddison",
  "user_id": "XyZ123",
  "is_admin": false
}
```

### Logout

```
POST /api/logout
```

**Host:** `auth.cloud.eddisonso.com`

Server acknowledges the logout. Token removal is handled client-side.

---

## Compute

All compute endpoints are on `compute.cloud.eddisonso.com` and require authentication.

### List Containers

```
GET /compute/containers
```

Returns all containers owned by the authenticated user.

**Response:**

```json
{
  "containers": [
    {
      "id": "a1b2c3d4",
      "name": "dev-box",
      "status": "running",
      "hostname": "a1b2c3d4.compute.eddisonso.com",
      "ssh_command": "ssh root@a1b2c3d4.compute.eddisonso.com",
      "memory_mb": 512,
      "memory_used_mb": 128,
      "storage_gb": 5,
      "storage_used_gb": 1.2,
      "instance_type": "nano",
      "created_at": "2026-01-15T10:30:00Z",
      "ssh_enabled": true,
      "https_enabled": false
    }
  ]
}
```

**Example:**

```bash
curl https://compute.cloud.eddisonso.com/compute/containers \
  -H "Authorization: Bearer $TOKEN"
```

### Create Container

```
POST /compute/containers
```

Creates a new container. Maximum 3 per user. Requires at least one SSH key.

**Request:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Container display name |
| `instance_type` | string | No | `nano` (default), `micro`, `mini` (arm64) or `tiny`, `small`, `medium` (amd64) |
| `memory_mb` | int | No | Memory in MB (default: 512) |
| `storage_gb` | int | No | Storage in GB (default: 5) |
| `ssh_key_ids` | int[] | Yes | IDs of SSH keys to inject |
| `ssh_enabled` | bool | No | Enable SSH access (default: false) |
| `mount_paths` | string[] | No | Directories to persist (default: `["/root"]`) |

**Instance types:**

| Type | Architecture | CPU Cores |
|------|-------------|-----------|
| `nano` | arm64 | 0.5 |
| `micro` | arm64 | 1 |
| `mini` | arm64 | 2 |
| `tiny` | amd64 | 1 |
| `small` | amd64 | 2 |
| `medium` | amd64 | 4 |

**Example:**

```bash
curl -X POST https://compute.cloud.eddisonso.com/compute/containers \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "dev-box",
    "instance_type": "nano",
    "ssh_key_ids": [1],
    "ssh_enabled": true,
    "mount_paths": ["/root", "/var/data"]
  }'
```

**Response:** Same shape as a single container object. Status starts as `"pending"` and transitions through `"initializing"` to `"running"` (observable via [WebSocket](#websocket)).

### Get Container

```
GET /compute/containers/:id
```

Returns a single container with live status and resource usage.

### Delete Container

```
DELETE /compute/containers/:id
```

Deletes the container and all its Kubernetes resources (namespace, PVC, pods).

**Response:**

```json
{ "status": "ok" }
```

### Start Container

```
POST /compute/containers/:id/start
```

Starts a stopped container. Returns the container with status `"pending"`.

### Stop Container

```
POST /compute/containers/:id/stop
```

Stops a running container (deletes the pod, preserves storage).

---

### SSH Keys

#### List SSH Keys

```
GET /compute/ssh-keys
```

**Response:**

```json
{
  "ssh_keys": [
    {
      "id": 1,
      "name": "laptop",
      "public_key": "ssh-ed25519 AAAAC3Nza...",
      "created_at": "2026-01-15T10:00:00Z"
    }
  ]
}
```

#### Add SSH Key

```
POST /compute/ssh-keys
```

Maximum 10 keys per user.

**Request:**

```json
{
  "name": "laptop",
  "public_key": "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA..."
}
```

Supported key types: `ssh-rsa`, `ssh-ed25519`, `ecdsa-sha2-nistp256`, `ecdsa-sha2-nistp384`, `ecdsa-sha2-nistp521`.

#### Delete SSH Key

```
DELETE /compute/ssh-keys/:id
```

---

### SSH Access

#### Get SSH Status

```
GET /compute/containers/:id/ssh
```

**Response:**

```json
{ "ssh_enabled": true }
```

#### Toggle SSH Access

```
PUT /compute/containers/:id/ssh
```

**Request:**

```json
{ "ssh_enabled": true }
```

When enabled, the container is accessible via:

```bash
ssh root@<container-id>.compute.eddisonso.com
```

---

### Ingress Rules

Expose container ports to the internet. Allowed external ports: 80, 443, 8000-8999.

#### List Ingress Rules

```
GET /compute/containers/:id/ingress
```

**Response:**

```json
{
  "rules": [
    { "id": 1, "port": 8080, "target_port": 8080, "created_at": 1736000000 }
  ]
}
```

#### Add Ingress Rule

```
POST /compute/containers/:id/ingress
```

**Request:**

```json
{
  "port": 8080,
  "target_port": 8080
}
```

`target_port` defaults to `port` if not specified. Adding port 443 enables HTTPS routing through the gateway.

#### Remove Ingress Rule

```
DELETE /compute/containers/:id/ingress/:port
```

---

### Mount Paths

Persistent directories that survive container restarts.

#### Get Mount Paths

```
GET /compute/containers/:id/mounts
```

**Response:**

```json
{ "mount_paths": ["/root", "/var/data"] }
```

#### Update Mount Paths

```
PUT /compute/containers/:id/mounts
```

**Request:**

```json
{ "mount_paths": ["/root", "/var/data", "/opt/app"] }
```

If the container is running, it will be **restarted** to apply the new mounts.

**Response:**

```json
{ "mount_paths": ["/root", "/var/data", "/opt/app"], "restarted": true }
```

---

### WebSocket

```
GET /compute/ws
```

**Host:** `compute.cloud.eddisonso.com`
**Auth:** Bearer token via query parameter `?token=<jwt>`

Provides real-time container status updates.

**Connection:**

```bash
websocat "wss://compute.cloud.eddisonso.com/compute/ws?token=$TOKEN"
```

**Messages received:**

Initial full container list:

```json
{ "type": "containers", "data": [ /* array of container objects */ ] }
```

Status updates (when containers start, stop, get IPs, etc.):

```json
{
  "type": "container_status",
  "data": {
    "container_id": "a1b2c3d4",
    "status": "running",
    "external_ip": "192.168.1.100"
  }
}
```

---

### Terminal

```
GET /compute/containers/:id/terminal
```

WebSocket endpoint for an interactive terminal session inside the container. Used by the dashboard's built-in terminal.

---

## Storage

All storage endpoints are on `storage.cloud.eddisonso.com`. Some endpoints require authentication, others allow public access for public namespaces.

### List Files

```
GET /storage/files?namespace=<name>
```

**Auth:** Required for private namespaces, optional for public ones.

**Query parameters:**

| Param | Required | Description |
|-------|----------|-------------|
| `namespace` | No | Namespace to list (default: `default`) |

**Response:**

```json
[
  {
    "name": "report.pdf",
    "path": "report.pdf",
    "namespace": "default",
    "size": 1048576,
    "created_at": 1736000000,
    "modified_at": 1736000000
  }
]
```

**Example:**

```bash
curl https://storage.cloud.eddisonso.com/storage/files?namespace=default \
  -H "Authorization: Bearer $TOKEN"
```

### Upload File

```
POST /storage/upload?namespace=<name>&overwrite=<bool>
```

**Auth:** Required

Upload a file via multipart form data.

**Query parameters:**

| Param | Required | Description |
|-------|----------|-------------|
| `namespace` | No | Target namespace (default: `default`) |
| `overwrite` | No | Overwrite existing file (default: `false`) |

**Headers:**

| Header | Description |
|--------|-------------|
| `X-File-Size` | Total file size in bytes (enables progress tracking) |

**Example:**

```bash
curl -X POST "https://storage.cloud.eddisonso.com/storage/upload?namespace=default" \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-File-Size: 1048576" \
  -F "file=@report.pdf"
```

**Response:**

```json
{ "status": "ok", "name": "report.pdf" }
```

Returns `409 Conflict` if the file exists and `overwrite` is not `true`.

### Download File

```
GET /storage/download?name=<filename>&namespace=<name>
```

**Auth:** Required for private namespaces.

Returns the file as an `application/octet-stream` with `Content-Disposition: attachment`.

**Example:**

```bash
curl -o report.pdf \
  "https://storage.cloud.eddisonso.com/storage/download?name=report.pdf&namespace=default" \
  -H "Authorization: Bearer $TOKEN"
```

### Delete File

```
DELETE /storage/delete?name=<filename>&namespace=<name>
```

**Auth:** Required

**Example:**

```bash
curl -X DELETE \
  "https://storage.cloud.eddisonso.com/storage/delete?name=report.pdf&namespace=default" \
  -H "Authorization: Bearer $TOKEN"
```

### Direct File Access

Files can also be accessed by path for embedding and sharing:

```
GET /storage/<namespace>/<filename>
```

Serves the file inline with the correct `Content-Type` based on extension. Accessible without auth for public/visible namespaces.

```
GET /storage/download/<namespace>/<filename>
```

Same as above but forces download via `Content-Disposition: attachment`.

---

### Namespaces

#### List Namespaces

```
GET /storage/namespaces
```

Returns namespaces visible to the authenticated user. Public namespaces are shown to everyone. Private namespaces are only shown to their owner.

**Response:**

```json
[
  {
    "name": "default",
    "count": 5,
    "hidden": false,
    "visibility": 2,
    "owner_id": null
  },
  {
    "name": "my-files",
    "count": 12,
    "hidden": false,
    "visibility": 0,
    "owner_id": "XyZ123"
  }
]
```

**Visibility levels:**

| Value | Name | Description |
|-------|------|-------------|
| 0 | Private | Only owner can see and access |
| 1 | Visible | Not listed, but accessible via direct URL |
| 2 | Public | Listed and accessible to everyone |

#### Create Namespace

```
POST /storage/namespaces
```

**Auth:** Required

**Request:**

```json
{
  "name": "my-files",
  "visibility": 0
}
```

Namespace names can contain letters, numbers, hyphens, underscores, and dots.

#### Delete Namespace

```
DELETE /storage/namespaces/:name
```

**Auth:** Required (owner only)

Deletes the namespace and all files in it.

#### Update Namespace

```
PUT /storage/namespaces/:name
```

**Auth:** Required (owner only)

**Request:**

```json
{ "visibility": 1 }
```

---

### Cluster Status

```
GET /storage/status
```

Returns GFS cluster health.

**Response:**

```json
{
  "chunkserver_count": 3,
  "total_servers": 3
}
```

---

## Common Patterns

### Error Responses

All errors return JSON:

```json
{ "error": "description of what went wrong" }
```

| Status | Meaning |
|--------|---------|
| `400` | Invalid request (bad JSON, missing fields, validation failure) |
| `401` | Missing or invalid authentication |
| `403` | Authenticated but not authorized (wrong owner, insufficient scope) |
| `404` | Resource not found |
| `409` | Conflict (e.g., file already exists, namespace already exists) |
| `500` | Internal server error |

### Authentication

Every authenticated request uses the same header:

```
Authorization: Bearer <token>
```

Where `<token>` is either:
- A **session JWT** from `POST /api/login` (full access to your resources)
- An **API token** (`ecloud_...`) from `POST /api/tokens` (scoped access — see [API Tokens](./api-tokens))

Both token types work identically on compute and storage endpoints. The only difference is that API tokens are checked against their granted scopes and can be revoked independently.

### Pagination

The API currently does not paginate. All list endpoints return the full result set.

### Rate Limiting

There is no rate limiting at the API level. Abuse is mitigated by resource limits (3 containers per user, 10 SSH keys per user).

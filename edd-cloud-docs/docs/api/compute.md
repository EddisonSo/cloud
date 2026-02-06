---
sidebar_position: 2
---

# Compute

**Base URL:** `https://compute.cloud.eddisonso.com`

All compute endpoints require authentication.

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/compute/containers` | List containers |
| `POST` | `/compute/containers` | Create container |
| `GET` | `/compute/containers/:id` | Get container |
| `DELETE` | `/compute/containers/:id` | Delete container |
| `POST` | `/compute/containers/:id/start` | Start container |
| `POST` | `/compute/containers/:id/stop` | Stop container |
| `GET` | `/compute/ssh-keys` | List SSH keys |
| `POST` | `/compute/ssh-keys` | Add SSH key |
| `DELETE` | `/compute/ssh-keys/:id` | Delete SSH key |
| `GET` | `/compute/containers/:id/ssh` | Get SSH status |
| `PUT` | `/compute/containers/:id/ssh` | Toggle SSH access |
| `GET` | `/compute/containers/:id/ingress` | List ingress rules |
| `POST` | `/compute/containers/:id/ingress` | Add ingress rule |
| `DELETE` | `/compute/containers/:id/ingress/:port` | Remove ingress rule |
| `GET` | `/compute/containers/:id/mounts` | Get mount paths |
| `PUT` | `/compute/containers/:id/mounts` | Update mount paths |
| `GET` | `/compute/ws` | WebSocket updates |
| `GET` | `/compute/containers/:id/terminal` | Terminal WebSocket |

---

## Containers

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
| `instance_type` | string | No | Instance type (default: `nano`) |
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

## SSH Keys

### List SSH Keys

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

### Add SSH Key

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

### Delete SSH Key

```
DELETE /compute/ssh-keys/:id
```

---

## SSH Access

### Get SSH Status

```
GET /compute/containers/:id/ssh
```

**Response:**

```json
{ "ssh_enabled": true }
```

### Toggle SSH Access

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

## Ingress Rules

Expose container ports to the internet. Allowed external ports: 80, 443, 8000-8999.

### List Ingress Rules

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

### Add Ingress Rule

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

### Remove Ingress Rule

```
DELETE /compute/containers/:id/ingress/:port
```

---

## Mount Paths

Persistent directories that survive container restarts.

### Get Mount Paths

```
GET /compute/containers/:id/mounts
```

**Response:**

```json
{ "mount_paths": ["/root", "/var/data"] }
```

### Update Mount Paths

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

## WebSocket

```
GET /compute/ws
```

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

## Terminal

```
GET /compute/containers/:id/terminal
```

WebSocket endpoint for an interactive terminal session inside the container. Used by the dashboard's built-in terminal.

---

## Health Check

```
GET /healthz
```

Returns `200 OK` with body `ok`.

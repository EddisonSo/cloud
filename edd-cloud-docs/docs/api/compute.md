---
sidebar_position: 3
---

# Compute API

Base URL: `https://compute.cloud.eddisonso.com`

## Reference

### Instance Types

| Type | Arch | CPU Cores |
|------|------|-----------|
| `nano` | arm64 | 0.5 |
| `micro` | arm64 | 1 |
| `mini` | arm64 | 2 |
| `tiny` | amd64 | 1 |
| `small` | amd64 | 2 |
| `medium` | amd64 | 4 |

### Default Limits

| Setting | Value |
|---------|-------|
| Max containers per user | 3 |
| Max SSH keys per user | 10 |
| Default memory | 512 MB |
| Default storage | 5 GB |

### Ingress Ports

Allowed external ports: `80`, `443`, `8000–8999`. Setting port `443` automatically enables HTTPS routing via the gateway.

---

## Containers

### GET /compute/containers

List all containers for the authenticated user.

**Auth:** Session / API token
**Token Scope:** `compute.<uid>.containers` with `read`

**Example request:**
```bash
curl https://compute.cloud.eddisonso.com/compute/containers \
  -H "Authorization: Bearer eyJhbGci..."
```

**Response:**
```json
{
  "containers": [
    {
      "id": "abc12345",
      "name": "my-app",
      "status": "running",
      "hostname": "abc12345.compute.eddisonso.com",
      "memory_mb": 512,
      "memory_used_mb": 256,
      "storage_gb": 5,
      "storage_used_gb": 2.3,
      "instance_type": "nano",
      "created_at": "2024-01-15T10:30:00Z",
      "ssh_enabled": true,
      "https_enabled": false,
      "ssh_command": "ssh root@abc12345.compute.eddisonso.com"
    }
  ]
}
```

---

### POST /compute/containers

Create a new container.

**Auth:** Session / API token
**Token Scope:** `compute.<uid>.containers` with `create`

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| name | string | body | Yes | Container name |
| instance_type | string | body | No | Instance type (default `nano`) |
| memory_mb | int | body | No | Memory in MB (default 512) |
| storage_gb | int | body | No | Storage in GB (default 5) |
| ssh_key_ids | int[] | body | Yes | SSH key IDs to inject (at least one) |
| ssh_enabled | bool | body | No | Enable SSH access (default false) |
| mount_paths | string[] | body | No | Absolute paths to persist (default `["/root"]`) |

**Example request:**
```bash
curl -X POST https://compute.cloud.eddisonso.com/compute/containers \
  -H "Authorization: Bearer eyJhbGci..." \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-app",
    "instance_type": "nano",
    "ssh_key_ids": [1],
    "ssh_enabled": true
  }'
```

**Response:**
```json
{
  "id": "abc12345",
  "name": "my-app",
  "status": "pending",
  "hostname": "abc12345.compute.eddisonso.com",
  "memory_mb": 512,
  "storage_gb": 5,
  "instance_type": "nano",
  "created_at": "2024-01-15T10:30:00Z",
  "ssh_enabled": true,
  "https_enabled": false
}
```

---

### GET /compute/containers/:id

Get a single container by ID.

**Auth:** Session / API token
**Token Scope:** `compute.<uid>.containers.<id>` with `read`

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| id | string | path | Yes | Container ID |

**Example request:**
```bash
curl https://compute.cloud.eddisonso.com/compute/containers/abc12345 \
  -H "Authorization: Bearer eyJhbGci..."
```

**Response:**
```json
{
  "id": "abc12345",
  "name": "my-app",
  "status": "running",
  "hostname": "abc12345.compute.eddisonso.com",
  "memory_mb": 512,
  "memory_used_mb": 256,
  "storage_gb": 5,
  "storage_used_gb": 2.3,
  "instance_type": "nano",
  "created_at": "2024-01-15T10:30:00Z",
  "ssh_enabled": true,
  "https_enabled": false,
  "ssh_command": "ssh root@abc12345.compute.eddisonso.com"
}
```

---

### DELETE /compute/containers/:id

Delete a container and its resources.

**Auth:** Session / API token
**Token Scope:** `compute.<uid>.containers.<id>` with `delete`

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| id | string | path | Yes | Container ID |

**Example request:**
```bash
curl -X DELETE https://compute.cloud.eddisonso.com/compute/containers/abc12345 \
  -H "Authorization: Bearer eyJhbGci..."
```

**Response:**
```json
{
  "status": "ok"
}
```

---

### POST /compute/containers/:id/start

Start a stopped container.

**Auth:** Session / API token
**Token Scope:** `compute.<uid>.containers.<id>` with `update`

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| id | string | path | Yes | Container ID |

**Example request:**
```bash
curl -X POST https://compute.cloud.eddisonso.com/compute/containers/abc12345/start \
  -H "Authorization: Bearer eyJhbGci..."
```

**Response:**
```json
{
  "id": "abc12345",
  "name": "my-app",
  "status": "pending",
  "hostname": "abc12345.compute.eddisonso.com",
  "memory_mb": 512,
  "storage_gb": 5,
  "instance_type": "nano",
  "created_at": "2024-01-15T10:30:00Z",
  "ssh_enabled": true,
  "https_enabled": false
}
```

---

### POST /compute/containers/:id/stop

Stop a running container.

**Auth:** Session / API token
**Token Scope:** `compute.<uid>.containers.<id>` with `update`

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| id | string | path | Yes | Container ID |

**Example request:**
```bash
curl -X POST https://compute.cloud.eddisonso.com/compute/containers/abc12345/stop \
  -H "Authorization: Bearer eyJhbGci..."
```

**Response:**
```json
{
  "id": "abc12345",
  "name": "my-app",
  "status": "stopped",
  "hostname": "abc12345.compute.eddisonso.com",
  "memory_mb": 512,
  "storage_gb": 5,
  "instance_type": "nano",
  "created_at": "2024-01-15T10:30:00Z",
  "ssh_enabled": true,
  "https_enabled": false
}
```

---

## SSH Keys

### GET /compute/ssh-keys

List all SSH keys for the authenticated user.

**Auth:** Session / API token
**Token Scope:** `compute.<uid>.keys` with `read`

**Example request:**
```bash
curl https://compute.cloud.eddisonso.com/compute/ssh-keys \
  -H "Authorization: Bearer eyJhbGci..."
```

**Response:**
```json
{
  "ssh_keys": [
    {
      "id": 1,
      "name": "laptop",
      "public_key": "ssh-ed25519 AAAA...",
      "created_at": "2024-01-10T15:20:00Z"
    }
  ]
}
```

---

### POST /compute/ssh-keys

Add a new SSH key.

**Auth:** Session / API token
**Token Scope:** `compute.<uid>.keys` with `create`

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| name | string | body | Yes | Key name |
| public_key | string | body | Yes | SSH public key (OpenSSH format) |

Accepted key types: `ssh-rsa`, `ssh-ed25519`, `ecdsa-sha2-nistp256`, `ecdsa-sha2-nistp384`, `ecdsa-sha2-nistp521`.

**Example request:**
```bash
curl -X POST https://compute.cloud.eddisonso.com/compute/ssh-keys \
  -H "Authorization: Bearer eyJhbGci..." \
  -H "Content-Type: application/json" \
  -d '{
    "name": "laptop",
    "public_key": "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA... user@laptop"
  }'
```

**Response:**
```json
{
  "id": 1,
  "name": "laptop",
  "public_key": "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA... user@laptop",
  "created_at": "2024-01-10T15:20:00Z"
}
```

---

### DELETE /compute/ssh-keys/:id

Delete an SSH key.

**Auth:** Session / API token
**Token Scope:** `compute.<uid>.keys` with `delete`

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| id | int | path | Yes | SSH key ID |

**Example request:**
```bash
curl -X DELETE https://compute.cloud.eddisonso.com/compute/ssh-keys/1 \
  -H "Authorization: Bearer eyJhbGci..."
```

**Response:**
```json
{
  "status": "ok"
}
```

---

## SSH Access

### GET /compute/containers/:id/ssh

Get SSH access status for a container.

**Auth:** Session / API token
**Token Scope:** `compute.<uid>.containers.<id>` with `read`

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| id | string | path | Yes | Container ID |

**Example request:**
```bash
curl https://compute.cloud.eddisonso.com/compute/containers/abc12345/ssh \
  -H "Authorization: Bearer eyJhbGci..."
```

**Response:**
```json
{
  "ssh_enabled": true
}
```

---

### PUT /compute/containers/:id/ssh

Enable or disable SSH access for a container.

**Auth:** Session / API token
**Token Scope:** `compute.<uid>.containers.<id>` with `update`

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| id | string | path | Yes | Container ID |
| ssh_enabled | bool | body | Yes | Enable or disable SSH |

**Example request:**
```bash
curl -X PUT https://compute.cloud.eddisonso.com/compute/containers/abc12345/ssh \
  -H "Authorization: Bearer eyJhbGci..." \
  -H "Content-Type: application/json" \
  -d '{"ssh_enabled": true}'
```

**Response:**
```json
{
  "ssh_enabled": true
}
```

---

## Ingress Rules

### GET /compute/containers/:id/ingress

List ingress rules for a container.

**Auth:** Session / API token
**Token Scope:** `compute.<uid>.containers.<id>` with `read`

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| id | string | path | Yes | Container ID |

**Example request:**
```bash
curl https://compute.cloud.eddisonso.com/compute/containers/abc12345/ingress \
  -H "Authorization: Bearer eyJhbGci..."
```

**Response:**
```json
{
  "rules": [
    {
      "id": 1,
      "port": 8080,
      "target_port": 8080,
      "created_at": 1705326600
    }
  ]
}
```

---

### POST /compute/containers/:id/ingress

Add an ingress rule. Allowed external ports: `80`, `443`, `8000–8999`.

**Auth:** Session / API token
**Token Scope:** `compute.<uid>.containers.<id>` with `update`

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| id | string | path | Yes | Container ID |
| port | int | body | Yes | External port |
| target_port | int | body | No | Internal container port (default: same as `port`) |

**Example request:**
```bash
curl -X POST https://compute.cloud.eddisonso.com/compute/containers/abc12345/ingress \
  -H "Authorization: Bearer eyJhbGci..." \
  -H "Content-Type: application/json" \
  -d '{"port": 8080, "target_port": 3000}'
```

**Response:**
```json
{
  "id": 1,
  "port": 8080,
  "target_port": 3000,
  "created_at": 1705326600
}
```

---

### DELETE /compute/containers/:id/ingress/:port

Remove an ingress rule.

**Auth:** Session / API token
**Token Scope:** `compute.<uid>.containers.<id>` with `update`

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| id | string | path | Yes | Container ID |
| port | int | path | Yes | External port to remove |

**Example request:**
```bash
curl -X DELETE https://compute.cloud.eddisonso.com/compute/containers/abc12345/ingress/8080 \
  -H "Authorization: Bearer eyJhbGci..."
```

**Response:**
```json
{
  "status": "ok"
}
```

---

## Mount Paths

### GET /compute/containers/:id/mounts

Get persistent mount paths for a container.

**Auth:** Session / API token
**Token Scope:** `compute.<uid>.containers.<id>` with `read`

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| id | string | path | Yes | Container ID |

**Example request:**
```bash
curl https://compute.cloud.eddisonso.com/compute/containers/abc12345/mounts \
  -H "Authorization: Bearer eyJhbGci..."
```

**Response:**
```json
{
  "mount_paths": ["/root", "/var/data"]
}
```

---

### PUT /compute/containers/:id/mounts

Update mount paths. Restarts the container if it is running.

**Auth:** Session / API token
**Token Scope:** `compute.<uid>.containers.<id>` with `update`

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| id | string | path | Yes | Container ID |
| mount_paths | string[] | body | Yes | Absolute paths to persist (at least one) |

**Example request:**
```bash
curl -X PUT https://compute.cloud.eddisonso.com/compute/containers/abc12345/mounts \
  -H "Authorization: Bearer eyJhbGci..." \
  -H "Content-Type: application/json" \
  -d '{"mount_paths": ["/root", "/var/data", "/data"]}'
```

**Response:**
```json
{
  "mount_paths": ["/root", "/var/data", "/data"],
  "restarted": true
}
```

---

## WebSocket

### GET /compute/ws

WebSocket connection for real-time container status updates.

**Auth:** Session / API token
**Token Scope:** `compute.<uid>` with `read`

**Connection:**
```
wss://compute.cloud.eddisonso.com/compute/ws
```

On connect the server sends the full container list, then pushes status changes as they happen.

**Initial message:**
```json
{
  "type": "containers",
  "data": [
    {
      "id": "abc12345",
      "name": "my-app",
      "status": "running"
    }
  ]
}
```

**Status update:**
```json
{
  "type": "container_status",
  "data": {
    "container_id": "abc12345",
    "status": "running",
    "external_ip": "10.0.0.5"
  }
}
```

Status values: `pending`, `initializing`, `running`, `stopped`, `failed`.

---

### GET /compute/containers/:id/terminal

WebSocket terminal session into a running container.

**Auth:** Session / API token
**Token Scope:** `compute.<uid>.containers.<id>` with `update`

**Connection:**
```
wss://compute.cloud.eddisonso.com/compute/containers/:id/terminal
```

The container must be running. The server allocates a PTY (`xterm-256color`, 24x80) and proxies binary data between the WebSocket and the container's SSH session.

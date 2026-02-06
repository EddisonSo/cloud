---
sidebar_position: 1
---

# API Tokens

API tokens provide programmatic access to the compute and storage APIs. Unlike session JWTs (which grant full access to your resources), API tokens carry **scoped permissions** — you choose exactly which actions a token can perform.

## Quick Start

### 1. Create a token

```bash
curl -X POST https://auth.cloud.eddisonso.com/api/tokens \
  -H "Authorization: Bearer <your-session-jwt>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "CI/CD pipeline",
    "scopes": {
      "compute.<your-user-id>.containers": ["read", "create", "update", "delete"]
    },
    "expires_in": "90d"
  }'
```

Response:

```json
{
  "id": "abc123",
  "name": "CI/CD pipeline",
  "scopes": {
    "compute.XyZ123.containers": ["read", "create", "update", "delete"]
  },
  "expires_at": 1744000000,
  "created_at": 1736000000,
  "token": "ecloud_eyJhbGciOiJIUzI1NiIs..."
}
```

:::warning
The `token` field is only returned once at creation time. Copy it immediately — only a SHA-256 hash is stored on the server.
:::

### 2. Use the token

Pass the token as a Bearer token in the `Authorization` header, exactly like a session JWT:

```bash
# List containers
curl https://compute.cloud.eddisonso.com/compute/containers \
  -H "Authorization: Bearer ecloud_eyJhbGciOiJIUzI1NiIs..."

# Upload a file
curl -X POST https://storage.cloud.eddisonso.com/storage/upload?namespace=default \
  -H "Authorization: Bearer ecloud_eyJhbGciOiJIUzI1NiIs..." \
  -F "file=@myfile.txt"
```

### 3. Revoke a token

```bash
curl -X DELETE https://auth.cloud.eddisonso.com/api/tokens/abc123 \
  -H "Authorization: Bearer <your-session-jwt>"
```

Revoked tokens stop working within 5 minutes (due to caching in compute/storage services).

## Token Format

API tokens are JWTs with an `ecloud_` prefix to distinguish them from session JWTs. The prefix is automatically stripped during validation.

```
ecloud_<base64-header>.<base64-payload>.<signature>
```

JWT payload:

```json
{
  "user_id": "XyZ123",
  "token_id": "abc123",
  "type": "api_token",
  "scopes": {
    "compute.XyZ123.containers": ["read", "create"]
  },
  "iat": 1736000000,
  "exp": 1744000000
}
```

## Permission Model

### Scope Format

Scopes follow a dot-separated hierarchy:

```
<service>.<user_id>.<resource>
```

You can only create tokens scoped to your own user ID — cross-user access is not possible.

### Available Scopes

#### Compute

| Scope | Actions | Description |
|-------|---------|-------------|
| `compute.<uid>.containers` | `create`, `read`, `update`, `delete` | Container lifecycle, SSH toggle, ingress rules, mounts, terminal |
| `compute.<uid>.keys` | `create`, `read`, `delete` | SSH key management |
| `compute.<uid>` | `read` | WebSocket real-time updates |

#### Storage

| Scope | Actions | Description |
|-------|---------|-------------|
| `storage.<uid>.namespaces` | `create`, `read`, `update`, `delete` | Namespace management and visibility |
| `storage.<uid>.files` | `create`, `read`, `delete` | File upload, download, and delete |

### Cascading Permissions

Permissions **cascade downward**. Granting `read` on `compute.<uid>` implicitly grants `read` on `compute.<uid>.containers` and `compute.<uid>.keys`.

Example — a read-only monitoring token:

```json
{
  "scopes": {
    "compute.XyZ123": ["read"],
    "storage.XyZ123": ["read"]
  }
}
```

This single token can list containers, view SSH keys, list namespaces, list files, and connect to the WebSocket — but cannot create, update, or delete anything.

### Action Reference

| Action | Meaning |
|--------|---------|
| `create` | Create new resources (containers, keys, namespaces, files) |
| `read` | List and view resources |
| `update` | Modify resources (start/stop containers, toggle SSH, edit ingress/mounts, update namespace visibility) |
| `delete` | Remove resources |

## Token Management API

All management endpoints require **session JWT** authentication (not API tokens).

**Base URL:** `https://auth.cloud.eddisonso.com`

### Create Token

```
POST /api/tokens
```

**Request body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Display name (max 64 chars) |
| `scopes` | object | Yes | Map of scope path to action array |
| `expires_in` | string | No | `30d`, `90d`, `365d`, or `never` (default: `never`) |

**Response:** Token object with the `token` field (one-time only).

### List Tokens

```
GET /api/tokens
```

Returns all tokens for the authenticated user. Does **not** include token strings or hashes.

**Response:**

```json
[
  {
    "id": "abc123",
    "name": "CI/CD pipeline",
    "scopes": { "compute.XyZ123.containers": ["read", "create"] },
    "expires_at": 1744000000,
    "last_used_at": 1740000000,
    "created_at": 1736000000
  }
]
```

### Delete Token

```
DELETE /api/tokens/{id}
```

Deletes the token. Only the token owner can delete it. Returns `404` if the token doesn't exist or belongs to another user.

## Endpoint Scope Map

### Compute Endpoints

| Endpoint | Scope | Action |
|----------|-------|--------|
| `GET /compute/containers` | `compute.<uid>.containers` | `read` |
| `POST /compute/containers` | `compute.<uid>.containers` | `create` |
| `GET /compute/containers/:id` | `compute.<uid>.containers` | `read` |
| `DELETE /compute/containers/:id` | `compute.<uid>.containers` | `delete` |
| `POST /compute/containers/:id/stop` | `compute.<uid>.containers` | `update` |
| `POST /compute/containers/:id/start` | `compute.<uid>.containers` | `update` |
| `PUT /compute/containers/:id/ssh` | `compute.<uid>.containers` | `update` |
| `GET /compute/containers/:id/ingress` | `compute.<uid>.containers` | `read` |
| `POST /compute/containers/:id/ingress` | `compute.<uid>.containers` | `update` |
| `DELETE /compute/containers/:id/ingress/:port` | `compute.<uid>.containers` | `update` |
| `GET /compute/containers/:id/mounts` | `compute.<uid>.containers` | `read` |
| `PUT /compute/containers/:id/mounts` | `compute.<uid>.containers` | `update` |
| `GET /compute/containers/:id/terminal` | `compute.<uid>.containers` | `update` |
| `GET /compute/ssh-keys` | `compute.<uid>.keys` | `read` |
| `POST /compute/ssh-keys` | `compute.<uid>.keys` | `create` |
| `DELETE /compute/ssh-keys/:id` | `compute.<uid>.keys` | `delete` |
| `GET /compute/ws` | `compute.<uid>` | `read` |

### Storage Endpoints

| Endpoint | Scope | Action |
|----------|-------|--------|
| `GET /storage/namespaces` | `storage.<uid>.namespaces` | `read` |
| `POST /storage/namespaces` | `storage.<uid>.namespaces` | `create` |
| `DELETE /storage/namespaces/:name` | `storage.<uid>.namespaces` | `delete` |
| `PUT /storage/namespaces/:name` | `storage.<uid>.namespaces` | `update` |
| `GET /storage/files` | `storage.<uid>.files` | `read` |
| `POST /storage/upload` | `storage.<uid>.files` | `create` |
| `DELETE /storage/delete` | `storage.<uid>.files` | `delete` |
| `GET /storage/download` | `storage.<uid>.files` | `read` |

## Examples

### Deploy Script Token

A token that can manage containers but not SSH keys:

```bash
curl -X POST https://auth.cloud.eddisonso.com/api/tokens \
  -H "Authorization: Bearer $SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "deploy-script",
    "scopes": {
      "compute.'"$USER_ID"'.containers": ["create", "read", "update", "delete"]
    },
    "expires_in": "30d"
  }'
```

### Backup Token

A read-only token for downloading files:

```bash
curl -X POST https://auth.cloud.eddisonso.com/api/tokens \
  -H "Authorization: Bearer $SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "nightly-backup",
    "scopes": {
      "storage.'"$USER_ID"'.files": ["read"],
      "storage.'"$USER_ID"'.namespaces": ["read"]
    },
    "expires_in": "365d"
  }'
```

### Full Access Token

Grants all permissions across compute and storage (use sparingly):

```bash
curl -X POST https://auth.cloud.eddisonso.com/api/tokens \
  -H "Authorization: Bearer $SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "full-access",
    "scopes": {
      "compute.'"$USER_ID"'": ["create", "read", "update", "delete"],
      "storage.'"$USER_ID"'": ["create", "read", "update", "delete"]
    },
    "expires_in": "30d"
  }'
```

## Error Responses

| Status | Meaning |
|--------|---------|
| `401 Unauthorized` | Missing, invalid, or expired token |
| `403 Forbidden` | Token is valid but lacks the required scope |
| `404 Not Found` | Token revoked (on revocation check) |

## UI

Tokens can also be managed from the dashboard at **Settings > API Tokens** (`cloud.eddisonso.com/settings/tokens`), which provides a visual interface for creating tokens with permission checkboxes, viewing active tokens, and revoking them.

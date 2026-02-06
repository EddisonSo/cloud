---
sidebar_position: 1
---

# Auth

**Base URL:** `https://auth.cloud.eddisonso.com`

The auth service handles user authentication, session management, and API token management.

## Common Patterns

All Edd Cloud APIs share these conventions.

### Base URLs

| Service | Base URL |
|---------|----------|
| Auth | `https://auth.cloud.eddisonso.com` |
| Compute | `https://compute.cloud.eddisonso.com` |
| Storage | `https://storage.cloud.eddisonso.com` |

### Authentication

Every authenticated request uses the same header:

```
Authorization: Bearer <token>
```

Where `<token>` is either:
- A **session JWT** from `POST /api/login` — full access to your resources
- An **API token** (`ecloud_...`) from `POST /api/tokens` — scoped access

Both token types work identically on compute and storage endpoints. API tokens are additionally checked against their granted scopes and can be revoked independently.

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

### Pagination

The API does not paginate. All list endpoints return the full result set.

### Rate Limiting

There is no rate limiting at the API level. Abuse is mitigated by resource limits (3 containers per user, 10 SSH keys per user).

---

## Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/api/login` | No | Authenticate and get session JWT |
| `GET` | `/api/session` | Session | Get current user info |
| `POST` | `/api/logout` | No | Acknowledge logout |
| `POST` | `/api/tokens` | Session | Create API token |
| `GET` | `/api/tokens` | Session | List API tokens |
| `DELETE` | `/api/tokens/{id}` | Session | Delete API token |
| `GET` | `/api/tokens/{id}/check` | No | Service-to-service revocation check |

---

## Login

```
POST /api/login
```

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

Store the `token` and pass it as `Authorization: Bearer <token>` on subsequent requests. The `user_id` is needed when creating API tokens with scoped permissions.

**Example:**

```bash
TOKEN=$(curl -s -X POST https://auth.cloud.eddisonso.com/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"eddison","password":"secret"}' | jq -r '.token')
```

## Get Session

```
GET /api/session
```

**Auth:** Session JWT required

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

## Logout

```
POST /api/logout
```

Server acknowledges the logout. Token removal is handled client-side.

**Response:**

```json
{ "status": "ok" }
```

## Session JWT Structure

```json
{
  "username": "eddison",
  "display_name": "Eddison",
  "user_id": "XyZ123",
  "exp": 1736086400,
  "iat": 1736000000,
  "sub": "eddison"
}
```

Session JWTs are signed with HS256 and expire based on the server's configured session TTL (default: 24 hours).

---

## API Tokens

API tokens provide programmatic access to the compute and storage APIs. Unlike session JWTs (which grant full access to your resources), API tokens carry **scoped permissions** — you choose exactly which actions a token can perform.

### Quick Start

**1. Create a token**

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

**2. Use the token**

Pass it as a Bearer token in the `Authorization` header, exactly like a session JWT:

```bash
curl https://compute.cloud.eddisonso.com/compute/containers \
  -H "Authorization: Bearer ecloud_eyJhbGciOiJIUzI1NiIs..."
```

**3. Revoke a token**

```bash
curl -X DELETE https://auth.cloud.eddisonso.com/api/tokens/abc123 \
  -H "Authorization: Bearer <your-session-jwt>"
```

Revoked tokens stop working within 5 minutes (due to caching in compute/storage services).

### Token Format

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

### Create Token

```
POST /api/tokens
```

**Auth:** Session JWT required

Creates a new API token with scoped permissions. The token string is returned **once** — only a SHA-256 hash is stored server-side.

**Request:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Display name (max 64 chars) |
| `scopes` | object | Yes | Map of scope path to action array |
| `expires_in` | string | No | `30d`, `90d`, `365d`, or `never` (default: `never`) |

**Example:**

```bash
curl -X POST https://auth.cloud.eddisonso.com/api/tokens \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "CI pipeline",
    "scopes": {
      "compute.XyZ123.containers": ["read", "create", "update", "delete"]
    },
    "expires_in": "90d"
  }'
```

**Response:**

```json
{
  "id": "abc123",
  "name": "CI pipeline",
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

### List Tokens

```
GET /api/tokens
```

**Auth:** Session JWT required

Returns all tokens for the authenticated user. Does **not** include token strings or hashes.

**Response:**

```json
[
  {
    "id": "abc123",
    "name": "CI pipeline",
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

**Auth:** Session JWT required

Deletes the token. Only the token owner can delete it.

**Response:**

```json
{ "status": "ok" }
```

Returns `404` if the token doesn't exist or belongs to another user.

### Check Token (Service-to-Service)

```
GET /api/tokens/{id}/check
```

**Auth:** None (internal endpoint)

Used by compute and storage services to verify a token has not been revoked. Returns `200` if valid and unexpired, `404` otherwise.

---

### Permission Model

#### Scope Format

Scopes follow a dot-separated hierarchy:

```
<service>.<user_id>.<resource>
```

You can only create tokens scoped to your own user ID — cross-user access is not possible.

#### Available Scopes

**Compute:**

| Scope | Actions | Description |
|-------|---------|-------------|
| `compute.<uid>.containers` | `create`, `read`, `update`, `delete` | Container lifecycle, SSH toggle, ingress rules, mounts, terminal |
| `compute.<uid>.keys` | `create`, `read`, `delete` | SSH key management |
| `compute.<uid>` | `read` | WebSocket real-time updates |

**Storage:**

| Scope | Actions | Description |
|-------|---------|-------------|
| `storage.<uid>.namespaces` | `create`, `read`, `update`, `delete` | Namespace management and visibility |
| `storage.<uid>.files` | `create`, `read`, `delete` | File upload, download, and delete |

#### Cascading Permissions

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

#### Action Reference

| Action | Meaning |
|--------|---------|
| `create` | Create new resources (containers, keys, namespaces, files) |
| `read` | List and view resources |
| `update` | Modify resources (start/stop containers, toggle SSH, edit ingress/mounts, update namespace visibility) |
| `delete` | Remove resources |

### Endpoint Scope Map

#### Compute Endpoints

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

#### Storage Endpoints

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

### Token Examples

**Deploy script** — manage containers but not SSH keys:

```bash
curl -X POST https://auth.cloud.eddisonso.com/api/tokens \
  -H "Authorization: Bearer $SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "deploy-script",
    "scopes": {
      "compute.''"$USER_ID"''.containers": ["create", "read", "update", "delete"]
    },
    "expires_in": "30d"
  }'
```

**Backup** — read-only file access:

```bash
curl -X POST https://auth.cloud.eddisonso.com/api/tokens \
  -H "Authorization: Bearer $SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "nightly-backup",
    "scopes": {
      "storage.''"$USER_ID"''.files": ["read"],
      "storage.''"$USER_ID"''.namespaces": ["read"]
    },
    "expires_in": "365d"
  }'
```

**Full access** — all permissions (use sparingly):

```bash
curl -X POST https://auth.cloud.eddisonso.com/api/tokens \
  -H "Authorization: Bearer $SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "full-access",
    "scopes": {
      "compute.''"$USER_ID"''": ["create", "read", "update", "delete"],
      "storage.''"$USER_ID"''": ["create", "read", "update", "delete"]
    },
    "expires_in": "30d"
  }'
```

### UI

Tokens can also be managed from the dashboard at **Settings > API Tokens** (`cloud.eddisonso.com/settings/tokens`), which provides a visual interface for creating tokens with permission checkboxes, viewing active tokens, and revoking them.

---

## Health Check

```
GET /healthz
```

Returns `200 OK` with body `ok`.

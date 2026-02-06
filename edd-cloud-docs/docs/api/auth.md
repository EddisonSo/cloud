---
sidebar_position: 2
---

# Auth API

**Base URL:** `https://auth.cloud.eddisonso.com`

The auth service handles user authentication, session management, and API token management.

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

Store the `token` and pass it as `Authorization: Bearer <token>` on subsequent requests. The `user_id` is needed when creating [API tokens](./tokens) with scoped permissions.

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

## API Token Endpoints

For a full guide on API tokens (permission model, scoping, examples), see [API Tokens](./tokens).

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

## Health Check

```
GET /healthz
```

Returns `200 OK` with body `ok`.

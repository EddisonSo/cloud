---
sidebar_position: 2
---

# Auth API

Base URL: `https://auth.cloud.eddisonso.com`

## Authentication

### POST /api/login

Authenticate and receive a session JWT.

**Auth:** None

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| username | string | body | Yes | Username |
| password | string | body | Yes | Password |

**Example request:**
```bash
curl -X POST https://auth.cloud.eddisonso.com/api/login \
  -H "Content-Type: application/json" \
  -d '{"username": "alice", "password": "secret"}'
```

**Response:**
```json
{
  "username": "alice",
  "display_name": "Alice",
  "user_id": "abc123",
  "is_admin": false,
  "token": "eyJhbGci..."
}
```

---

### GET /api/session

Get current user info from session token.

**Auth:** Session

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| Authorization | string | header | Yes | `Bearer <token>` |

**Example request:**
```bash
curl https://auth.cloud.eddisonso.com/api/session \
  -H "Authorization: Bearer eyJhbGci..."
```

**Response:**
```json
{
  "username": "alice",
  "display_name": "Alice",
  "user_id": "abc123",
  "is_admin": false
}
```

---

### POST /api/logout

Acknowledge logout (client removes token).

**Auth:** None

**Example request:**
```bash
curl -X POST https://auth.cloud.eddisonso.com/api/logout
```

**Response:**
```json
{
  "status": "ok"
}
```

---

## API Tokens

### POST /api/tokens

Create a new API token.

**Auth:** Session

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| name | string | body | Yes | Token name (max 64 chars) |
| scopes | object | body | Yes | Scope-to-actions map (see [Token Scopes](/api#token-scopes)) |
| expires_in | string | body | No | `"30d"`, `"90d"`, `"365d"`, or `"never"` (default `"never"`) |

Each scope key follows the format `<service>.<user_id>[.<resource>[.<id>]]` and maps to an array of actions: `"create"`, `"read"`, `"update"`, `"delete"`.

**Example request:**
```bash
curl -X POST https://auth.cloud.eddisonso.com/api/tokens \
  -H "Authorization: Bearer eyJhbGci..." \
  -H "Content-Type: application/json" \
  -d '{
    "name": "ci-deploy",
    "scopes": {
      "compute.abc123.containers": ["read", "create", "delete"]
    },
    "expires_in": "90d"
  }'
```

**Response:**
```json
{
  "id": "tok_5f3a",
  "name": "ci-deploy",
  "scopes": {
    "compute.abc123.containers": ["read", "create", "delete"]
  },
  "expires_at": 1720000000,
  "created_at": 1712224000,
  "last_used_at": 0,
  "token": "ecloud_eyJhbGci..."
}
```

The `token` field is only returned on creation. Store it — it cannot be retrieved later.

---

### GET /api/tokens

List all API tokens for the authenticated user.

**Auth:** Session

**Example request:**
```bash
curl https://auth.cloud.eddisonso.com/api/tokens \
  -H "Authorization: Bearer eyJhbGci..."
```

**Response:**
```json
[
  {
    "id": "tok_5f3a",
    "name": "ci-deploy",
    "scopes": {
      "compute.abc123.containers": ["read", "create", "delete"]
    },
    "expires_at": 1720000000,
    "created_at": 1712224000,
    "last_used_at": 1712300000
  }
]
```

---

### DELETE /api/tokens/\{id\}

Delete an API token.

**Auth:** Session

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| id | string | path | Yes | Token ID |

**Example request:**
```bash
curl -X DELETE https://auth.cloud.eddisonso.com/api/tokens/tok_5f3a \
  -H "Authorization: Bearer eyJhbGci..."
```

**Response:**
```json
{
  "status": "ok"
}
```

---

### GET /api/tokens/\{id\}/check

Service-to-service endpoint to check whether an API token is valid and not revoked.

**Auth:** None

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| id | string | path | Yes | Token ID |

**Example request:**
```bash
curl https://auth.cloud.eddisonso.com/api/tokens/tok_5f3a/check
```

**Response:**
```json
{
  "status": "valid"
}
```

Returns `404` if the token is not found, revoked, or expired.

For service account tokens, the response includes the service account's current scopes:

```json
{
  "status": "valid",
  "scopes": {
    "compute.abc123.containers": ["read", "create"]
  }
}
```

---

## Service Accounts

Service accounts separate identity and permissions from credentials. Create a service account with scoped permissions, then generate tokens that inherit those permissions. Updating a service account's scopes takes effect for all its tokens within the 5-minute cache TTL.

### POST /api/service-accounts

Create a new service account.

**Auth:** Session

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| name | string | body | Yes | Service account name (max 64 chars) |
| scopes | object | body | Yes | Scope-to-actions map (same format as token scopes) |

**Example request:**
```bash
curl -X POST https://auth.cloud.eddisonso.com/api/service-accounts \
  -H "Authorization: Bearer eyJhbGci..." \
  -H "Content-Type: application/json" \
  -d '{
    "name": "ci-pipeline",
    "scopes": {
      "compute.abc123.containers": ["read", "create", "delete"]
    }
  }'
```

**Response:**
```json
{
  "id": "xK9mPq",
  "name": "ci-pipeline",
  "scopes": {
    "compute.abc123.containers": ["read", "create", "delete"]
  },
  "token_count": 0,
  "created_at": 1712224000
}
```

---

### GET /api/service-accounts

List all service accounts for the authenticated user.

**Auth:** Session

**Example request:**
```bash
curl https://auth.cloud.eddisonso.com/api/service-accounts \
  -H "Authorization: Bearer eyJhbGci..."
```

**Response:**
```json
[
  {
    "id": "xK9mPq",
    "name": "ci-pipeline",
    "scopes": {
      "compute.abc123.containers": ["read", "create", "delete"]
    },
    "token_count": 2,
    "created_at": 1712224000
  }
]
```

---

### GET /api/service-accounts/\{id\}

Get a service account by ID.

**Auth:** Session

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| id | string | path | Yes | Service account ID |

**Response:** Same format as list item above.

---

### PUT /api/service-accounts/\{id\}/scopes

Update a service account's scopes. Changes take effect for all tokens within the 5-minute cache TTL.

**Auth:** Session

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| id | string | path | Yes | Service account ID |
| scopes | object | body | Yes | New scope-to-actions map |

**Example request:**
```bash
curl -X PUT https://auth.cloud.eddisonso.com/api/service-accounts/xK9mPq/scopes \
  -H "Authorization: Bearer eyJhbGci..." \
  -H "Content-Type: application/json" \
  -d '{
    "scopes": {
      "compute.abc123.containers": ["read"],
      "storage.abc123.files": ["read"]
    }
  }'
```

**Response:**
```json
{
  "status": "ok"
}
```

---

### DELETE /api/service-accounts/\{id\}

Delete a service account. All its tokens are revoked (cascade delete).

**Auth:** Session

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| id | string | path | Yes | Service account ID |

**Example request:**
```bash
curl -X DELETE https://auth.cloud.eddisonso.com/api/service-accounts/xK9mPq \
  -H "Authorization: Bearer eyJhbGci..."
```

**Response:**
```json
{
  "status": "ok"
}
```

---

### POST /api/service-accounts/\{id\}/tokens

Create a token for a service account. The token inherits the service account's scopes (no scopes in the request).

**Auth:** Session

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| id | string | path | Yes | Service account ID |
| name | string | body | Yes | Token name (max 64 chars) |
| expires_in | string | body | No | `"30d"`, `"90d"`, `"365d"`, or `"never"` |

**Example request:**
```bash
curl -X POST https://auth.cloud.eddisonso.com/api/service-accounts/xK9mPq/tokens \
  -H "Authorization: Bearer eyJhbGci..." \
  -H "Content-Type: application/json" \
  -d '{"name": "production", "expires_in": "365d"}'
```

**Response:**
```json
{
  "id": "tK3nAb",
  "name": "production",
  "expires_at": 1743760000,
  "created_at": 1712224000,
  "last_used_at": 0,
  "token": "ecloud_eyJhbGci..."
}
```

The `token` field is only returned on creation. Store it — it cannot be retrieved later.

---

### GET /api/service-accounts/\{id\}/tokens

List all tokens for a service account.

**Auth:** Session

| Param | Type | In | Required | Description |
|-------|------|----|----------|-------------|
| id | string | path | Yes | Service account ID |

**Example request:**
```bash
curl https://auth.cloud.eddisonso.com/api/service-accounts/xK9mPq/tokens \
  -H "Authorization: Bearer eyJhbGci..."
```

**Response:**
```json
[
  {
    "id": "tK3nAb",
    "name": "production",
    "expires_at": 1743760000,
    "created_at": 1712224000,
    "last_used_at": 1712300000
  }
]
```

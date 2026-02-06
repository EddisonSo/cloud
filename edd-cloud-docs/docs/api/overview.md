---
sidebar_position: 1
---

# Overview

Edd Cloud exposes REST APIs across three services. All authenticated endpoints accept a JWT in the `Authorization: Bearer <token>` header — either a session JWT from login or an [API token](./tokens).

## Base URLs

| Service | Base URL | Description |
|---------|----------|-------------|
| Auth | `https://auth.cloud.eddisonso.com` | Authentication, sessions, API tokens |
| Compute | `https://compute.cloud.eddisonso.com` | Containers, SSH keys, ingress |
| Storage | `https://storage.cloud.eddisonso.com` | Files, namespaces |

## Authentication

Every authenticated request uses the same header:

```
Authorization: Bearer <token>
```

Where `<token>` is either:
- A **session JWT** from [`POST /api/login`](./auth#login) — full access to your resources
- An **[API token](./tokens)** (`ecloud_...`) from `POST /api/tokens` — scoped access

Both token types work identically on compute and storage endpoints. API tokens are additionally checked against their granted scopes and can be revoked independently.

## Error Responses

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

## Pagination

The API does not paginate. All list endpoints return the full result set.

## Rate Limiting

There is no rate limiting at the API level. Abuse is mitigated by resource limits (3 containers per user, 10 SSH keys per user).

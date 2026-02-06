---
sidebar_position: 1
title: Overview
---

# API References

All authenticated endpoints accept a JWT in the `Authorization: Bearer <token>` header — either a session JWT or an API token.

| Service | Base URL |
|---------|----------|
| Auth | `https://auth.cloud.eddisonso.com` |
| Compute | `https://compute.cloud.eddisonso.com` |
| Storage | `https://storage.cloud.eddisonso.com` |

### Service Pages

- [**Auth API**](auth) — Login, sessions, and API token management
- [**Compute API**](compute) — Containers, SSH keys, ingress, and terminal access
- [**Storage API**](storage) — Namespaces, file upload/download, and public access

---

## Token Scopes

API tokens support resource-specific scopes with up to 4 segments:

```
<service>.<user_id>                        # all resources in service
<service>.<user_id>.<resource>             # all items of resource type
<service>.<user_id>.<resource>.<id>        # specific item only
```

**Compute examples:**
- `compute.<uid>.containers` — access all containers
- `compute.<uid>.containers.<container_id>` — access one specific container
- `compute.<uid>.keys` — access all SSH keys

**Storage examples:**
- `storage.<uid>.namespaces` — manage all namespaces
- `storage.<uid>.namespaces.<name>` — manage a specific namespace
- `storage.<uid>.files` — access files in all namespaces
- `storage.<uid>.files.<name>` — access files in a specific namespace

Broad scopes cascade: a token with `compute.<uid>.containers` grants access to all individual containers. A token with `compute.<uid>.containers.<id>` only grants access to that one container.

---

## Errors

All errors return JSON:

```json
{ "error": "description of what went wrong" }
```

| Status | Meaning |
|--------|---------|
| `400` | Invalid request |
| `401` | Missing or invalid authentication |
| `403` | Insufficient permissions |
| `404` | Resource not found |
| `409` | Conflict |
| `500` | Internal server error |

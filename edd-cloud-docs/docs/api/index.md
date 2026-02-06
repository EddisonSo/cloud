---
sidebar_position: 1
---

# API References

All authenticated endpoints accept a JWT in the `Authorization: Bearer <token>` header — either a session JWT or an API token.

| Service | Base URL |
|---------|----------|
| Auth | `https://auth.cloud.eddisonso.com` |
| Compute | `https://compute.cloud.eddisonso.com` |
| Storage | `https://storage.cloud.eddisonso.com` |

---

## Auth

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/api/login` | No | Authenticate and get session JWT |
| `GET` | `/api/session` | Session | Get current user info |
| `POST` | `/api/logout` | No | Acknowledge logout |
| `POST` | `/api/tokens` | Session | Create API token |
| `GET` | `/api/tokens` | Session | List API tokens |
| `DELETE` | `/api/tokens/{id}` | Session | Delete API token |
| `GET` | `/api/tokens/{id}/check` | No | Service-to-service revocation check |
| `GET` | `/healthz` | No | Health check |

## Compute

| Method | Path | Auth | Token Scope | Description |
|--------|------|------|-------------|-------------|
| `GET` | `/compute/containers` | Yes | `compute.<uid>.containers` | List containers |
| `POST` | `/compute/containers` | Yes | `compute.<uid>.containers` | Create container |
| `GET` | `/compute/containers/:id` | Yes | `compute.<uid>.containers.<id>` | Get container |
| `DELETE` | `/compute/containers/:id` | Yes | `compute.<uid>.containers.<id>` | Delete container |
| `POST` | `/compute/containers/:id/start` | Yes | `compute.<uid>.containers.<id>` | Start container |
| `POST` | `/compute/containers/:id/stop` | Yes | `compute.<uid>.containers.<id>` | Stop container |
| `GET` | `/compute/ssh-keys` | Yes | `compute.<uid>.keys` | List SSH keys |
| `POST` | `/compute/ssh-keys` | Yes | `compute.<uid>.keys` | Add SSH key |
| `DELETE` | `/compute/ssh-keys/:id` | Yes | `compute.<uid>.keys` | Delete SSH key |
| `GET` | `/compute/containers/:id/ssh` | Yes | `compute.<uid>.containers.<id>` | Get SSH status |
| `PUT` | `/compute/containers/:id/ssh` | Yes | `compute.<uid>.containers.<id>` | Toggle SSH access |
| `GET` | `/compute/containers/:id/ingress` | Yes | `compute.<uid>.containers.<id>` | List ingress rules |
| `POST` | `/compute/containers/:id/ingress` | Yes | `compute.<uid>.containers.<id>` | Add ingress rule |
| `DELETE` | `/compute/containers/:id/ingress/:port` | Yes | `compute.<uid>.containers.<id>` | Remove ingress rule |
| `GET` | `/compute/containers/:id/mounts` | Yes | `compute.<uid>.containers.<id>` | Get mount paths |
| `PUT` | `/compute/containers/:id/mounts` | Yes | `compute.<uid>.containers.<id>` | Update mount paths |
| `GET` | `/compute/ws` | Yes | `compute.<uid>` | WebSocket status updates |
| `GET` | `/compute/containers/:id/terminal` | Yes | `compute.<uid>.containers.<id>` | Terminal WebSocket |
| `GET` | `/healthz` | No | — | Health check |

## Storage

| Method | Path | Auth | Token Scope | Description |
|--------|------|------|-------------|-------------|
| `GET` | `/storage/files?namespace=X` | Private ns | `storage.<uid>.files.<X>` | List files |
| `POST` | `/storage/upload?namespace=X` | Yes | `storage.<uid>.files.<X>` | Upload file |
| `GET` | `/storage/download?namespace=X` | Private ns | `storage.<uid>.files.<X>` | Download file |
| `DELETE` | `/storage/delete?namespace=X` | Yes | `storage.<uid>.files.<X>` | Delete file |
| `GET` | `/storage/:namespace/:filename` | Public ns | — | Direct file access |
| `GET` | `/storage/download/:namespace/:filename` | Public ns | — | Direct download |
| `GET` | `/storage/namespaces` | Yes | `storage.<uid>.namespaces` | List namespaces |
| `POST` | `/storage/namespaces` | Yes | `storage.<uid>.namespaces` | Create namespace |
| `DELETE` | `/storage/namespaces/:name` | Yes | `storage.<uid>.namespaces.<name>` | Delete namespace |
| `PUT` | `/storage/namespaces/:name` | Yes | `storage.<uid>.namespaces.<name>` | Update namespace |
| `GET` | `/storage/status` | No | — | Cluster status |
| `GET` | `/healthz` | No | — | Health check |

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

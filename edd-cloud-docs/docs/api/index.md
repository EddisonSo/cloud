---
sidebar_position: 1
---

# API References

All authenticated endpoints accept a JWT in the `Authorization: Bearer <token>` header — either a session JWT or an [API token](/docs/services/auth#api-tokens).

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

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/compute/containers` | Yes | List containers |
| `POST` | `/compute/containers` | Yes | Create container |
| `GET` | `/compute/containers/:id` | Yes | Get container |
| `DELETE` | `/compute/containers/:id` | Yes | Delete container |
| `POST` | `/compute/containers/:id/start` | Yes | Start container |
| `POST` | `/compute/containers/:id/stop` | Yes | Stop container |
| `GET` | `/compute/ssh-keys` | Yes | List SSH keys |
| `POST` | `/compute/ssh-keys` | Yes | Add SSH key |
| `DELETE` | `/compute/ssh-keys/:id` | Yes | Delete SSH key |
| `GET` | `/compute/containers/:id/ssh` | Yes | Get SSH status |
| `PUT` | `/compute/containers/:id/ssh` | Yes | Toggle SSH access |
| `GET` | `/compute/containers/:id/ingress` | Yes | List ingress rules |
| `POST` | `/compute/containers/:id/ingress` | Yes | Add ingress rule |
| `DELETE` | `/compute/containers/:id/ingress/:port` | Yes | Remove ingress rule |
| `GET` | `/compute/containers/:id/mounts` | Yes | Get mount paths |
| `PUT` | `/compute/containers/:id/mounts` | Yes | Update mount paths |
| `GET` | `/compute/ws` | Yes | WebSocket status updates |
| `GET` | `/compute/containers/:id/terminal` | Yes | Terminal WebSocket |
| `GET` | `/healthz` | No | Health check |

## Storage

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/storage/files` | Private ns | List files |
| `POST` | `/storage/upload` | Yes | Upload file |
| `GET` | `/storage/download` | Private ns | Download file |
| `DELETE` | `/storage/delete` | Yes | Delete file |
| `GET` | `/storage/:namespace/:filename` | Public ns | Direct file access |
| `GET` | `/storage/download/:namespace/:filename` | Public ns | Direct download |
| `GET` | `/storage/namespaces` | Yes | List namespaces |
| `POST` | `/storage/namespaces` | Yes | Create namespace |
| `DELETE` | `/storage/namespaces/:name` | Yes | Delete namespace |
| `PUT` | `/storage/namespaces/:name` | Yes | Update namespace |
| `GET` | `/storage/status` | No | Cluster status |
| `GET` | `/healthz` | No | Health check |

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

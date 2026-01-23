---
sidebar_position: 4
---

# Event-Driven Architecture

Edd-cloud uses an event-driven architecture for service communication, enabling loose coupling and resilience.

## Overview

Services communicate through events published to NATS JetStream:
- **Publishers**: Emit events when state changes
- **Subscribers**: React to events from other services
- **Persistence**: Events are stored for replay and recovery

## Architecture

Auth-service is the single source of truth for user data. Other services maintain a read-only `user_cache` table populated via NATS events.

```
┌─────────────┐
│ Auth Service│ ─── auth.user.* events ───▶ NATS JetStream
│  (auth_db)  │                                    │
└─────────────┘                     ┌──────────────┼──────────────┐
                                    ▼              ▼              ▼
                              ┌──────────┐  ┌──────────┐  ┌─────────┐
                              │   SFS    │  │ Compute  │  │ Gateway │
                              │ (sfs_db) │  │(compute) │  │(gateway)│
                              └──────────┘  └──────────┘  └─────────┘
```

## Service Boundaries

Each service owns its data and publishes events for others to consume:

| Service | Database | Owns | Publishes | Subscribes |
|---------|----------|------|-----------|------------|
| **Auth** | `auth_db` | users, sessions | `auth.user.*` | - |
| **SFS** | `sfs_db` | namespaces, files, user_cache | `sfs.*` | `auth.user.*` |
| **Compute** | `compute_db` | containers, ssh_keys, user_cache | `compute.*` | `auth.user.*` |
| **Gateway** | `gateway_db` | routes, ingress_rules | `gateway.*` | `compute.container.*` |

## Event Flow Examples

### User Registration

```mermaid
sequenceDiagram
    participant User
    participant Auth
    participant NATS
    participant SFS
    participant Compute

    User->>Auth: Register
    Auth->>NATS: auth.user.{id}.created
    NATS-->>SFS: Event
    NATS-->>Compute: Event
    SFS->>SFS: Upsert to user_cache
    Compute->>Compute: Upsert to user_cache
```

### User Deletion (Cascade)

```mermaid
sequenceDiagram
    participant Admin
    participant Auth
    participant NATS
    participant SFS
    participant Compute

    Admin->>Auth: Delete user
    Auth->>NATS: auth.user.{id}.deleted
    NATS-->>SFS: Event
    NATS-->>Compute: Event
    SFS->>SFS: Set namespace owner_id to NULL
    Compute->>Compute: Delete containers & SSH keys
```

### Service Startup Sync

On startup, services fetch all users from auth-service to populate the cache:

```mermaid
sequenceDiagram
    participant SFS
    participant Auth
    participant NATS

    SFS->>Auth: GET /api/users
    Auth-->>SFS: [users]
    SFS->>SFS: Populate user_cache
    SFS->>NATS: Subscribe to auth.user.*
```

## Event Subjects

Events use a hierarchical subject pattern:

```
{service}.{entity}.{id}.{action}
```

### User Events

| Subject | Description |
|---------|-------------|
| `auth.user.{id}.created` | New user registered |
| `auth.user.{id}.deleted` | User was deleted |
| `auth.user.{id}.updated` | User profile updated |

## JSON Message Types

Events are serialized as JSON:

### UserCreated

```json
{
  "metadata": {
    "event_id": "uuid-v4",
    "entity_id": "nanoid-user-id",
    "timestamp": 1705920000,
    "source": "auth-service",
    "version": 1
  },
  "user_id": "V1StGXR8_Z5jdHi6B-myT",
  "username": "johndoe",
  "display_name": "John Doe"
}
```

### UserDeleted

```json
{
  "metadata": {
    "event_id": "uuid-v4",
    "entity_id": "nanoid-user-id",
    "timestamp": 1705920000,
    "source": "auth-service"
  },
  "user_id": "V1StGXR8_Z5jdHi6B-myT",
  "username": "johndoe"
}
```

### UserUpdated

```json
{
  "metadata": {
    "event_id": "uuid-v4",
    "entity_id": "nanoid-user-id",
    "timestamp": 1705920000,
    "source": "auth-service"
  },
  "user_id": "V1StGXR8_Z5jdHi6B-myT",
  "username": "johndoe",
  "display_name": "John D."
}
```

## User Cache Pattern

Services maintain a local `user_cache` table:

```sql
CREATE TABLE user_cache (
    user_id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL DEFAULT '',
    synced_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

### Event Handlers

```go
// OnUserCreated - Upsert to cache
func (h *Handler) OnUserCreated(ctx context.Context, event events.UserCreated) error {
    _, err := h.db.Exec(`
        INSERT INTO user_cache (user_id, username, display_name, synced_at)
        VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
        ON CONFLICT (user_id) DO UPDATE SET
            username = EXCLUDED.username,
            display_name = EXCLUDED.display_name,
            synced_at = CURRENT_TIMESTAMP
    `, event.UserID, event.Username, event.DisplayName)
    return err
}

// OnUserDeleted - Remove from cache (FK cascades to owned resources)
func (h *Handler) OnUserDeleted(ctx context.Context, event events.UserDeleted) error {
    _, err := h.db.Exec(`DELETE FROM user_cache WHERE user_id = $1`, event.UserID)
    return err
}
```

## Cascade Deletion

When a user is deleted:

| Service | Action |
|---------|--------|
| **SFS** | Namespace `owner_id` set to NULL (via event handler) |
| **Compute** | Containers deleted (K8s namespace + DB), SSH keys deleted |

## Configuration

Services require these environment variables:

| Variable | Description |
|----------|-------------|
| `NATS_URL` | NATS JetStream URL (e.g., `nats://nats:4222`) |
| `AUTH_SERVICE_URL` | Auth service URL for initial sync (e.g., `http://auth-service:80`) |

## Reliability

### At-Least-Once Delivery

- JetStream persists messages until acknowledged
- Consumers must explicitly acknowledge messages
- Unacknowledged messages are redelivered

### Idempotency

Handlers use upserts to be idempotent:
- `ON CONFLICT DO UPDATE` for creates/updates
- Delete operations are naturally idempotent

### Graceful Degradation

- Services continue working with cached data if NATS is temporarily unavailable
- Initial sync failure logs a warning but doesn't prevent startup

## Benefits

1. **Single Source of Truth**: Auth-service owns user data
2. **Loose Coupling**: Services don't share databases
3. **Resilience**: Events are persisted if a service is down
4. **Scalability**: Multiple consumers can process events in parallel
5. **Eventual Consistency**: Services sync via events, not RPC

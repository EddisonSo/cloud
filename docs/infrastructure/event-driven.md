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

## Service Boundaries

Each service owns its data and publishes events for others to consume:

| Service | Database | Owns | Publishes | Subscribes |
|---------|----------|------|-----------|------------|
| **Auth** | `auth_db` | users, sessions | `auth.*` | - |
| **SFS** | `sfs_db` | namespaces, files | `sfs.*` | `auth.user.deleted` |
| **Compute** | `compute_db` | containers, ssh_keys | `compute.*` | `auth.user.deleted` |
| **Gateway** | `gateway_db` | routes, ingress_rules | `gateway.*` | `compute.container.*` |

## Event Flow Examples

### User Registration

```
User registers
      │
      ▼
┌─────────┐
│  Auth   │──── auth.user.{id}.created ────▶ NATS
└─────────┘
                                              │
              ┌───────────────────────────────┤
              ▼                               ▼
         ┌─────────┐                    ┌─────────┐
         │   SFS   │                    │ Compute │
         │ (cache) │                    │ (cache) │
         └─────────┘                    └─────────┘
```

### Container Creation

```
User creates container
      │
      ▼
┌─────────┐
│ Compute │──┬── compute.container.{id}.created ──▶ NATS
└─────────┘  │
             └── compute.ingress.{id}.requested ──▶ NATS
                                                     │
                                                     ▼
                                               ┌─────────┐
                                               │ Gateway │
                                               │ (routes)│
                                               └─────────┘
```

### User Deletion (Cascade)

```
Admin deletes user
      │
      ▼
┌─────────┐
│  Auth   │──── auth.user.{id}.deleted ────▶ NATS
└─────────┘
                                              │
              ┌───────────────────────────────┼───────────────┐
              ▼                               ▼               ▼
         ┌─────────┐                    ┌─────────┐     ┌─────────┐
         │   SFS   │                    │ Compute │     │ Gateway │
         │ (delete │                    │ (delete │     │ (remove │
         │  files) │                    │  VMs)   │     │  routes)│
         └─────────┘                    └─────────┘     └─────────┘
```

## Event Subjects

Events use a hierarchical subject pattern:

```
{service}.{entity}.{id}.{action}
```

### Examples

| Subject | Description |
|---------|-------------|
| `auth.user.123.created` | User 123 was created |
| `auth.user.123.deleted` | User 123 was deleted |
| `compute.container.abc.started` | Container abc started |
| `compute.container.abc.stopped` | Container abc stopped |
| `sfs.namespace.456.created` | Namespace 456 created |

## Protobuf Messages

Events are serialized using Protocol Buffers:

```protobuf
// proto/auth/events.proto
message UserCreated {
  EventMetadata metadata = 1;
  int64 user_id = 2;
  string username = 3;
  string display_name = 4;
  string public_id = 5;
}

message UserDeleted {
  EventMetadata metadata = 1;
  int64 user_id = 2;
  string username = 3;
}
```

## Event Metadata

All events include standard metadata:

```protobuf
message EventMetadata {
  string event_id = 1;      // Unique event ID (UUID)
  string entity_id = 2;     // ID of the entity
  Timestamp timestamp = 3;  // When event occurred
  string source = 4;        // Service that emitted
  int64 version = 5;        // For optimistic concurrency
}
```

## Reliability

### At-Least-Once Delivery

- JetStream persists messages until acknowledged
- Consumers must explicitly acknowledge messages
- Unacknowledged messages are redelivered

### Idempotency

Handlers must be idempotent:
- Use `event_id` for deduplication
- Use `version` for optimistic concurrency
- Design for messages being processed multiple times

### Error Handling

Failed messages are retried with backoff:

```go
consumerConfig := &nats.ConsumerConfig{
    MaxDeliver: 5,
    AckWait:    30 * time.Second,
    BackOff:    []time.Duration{
        1 * time.Second,
        5 * time.Second,
        30 * time.Second,
    },
}
```

### Dead Letter Queue

After max retries, messages go to a DLQ:

```
auth.user.created.dlq
compute.container.created.dlq
```

## Benefits

1. **Loose Coupling**: Services don't need to know about each other
2. **Resilience**: Events are persisted if a service is down
3. **Scalability**: Multiple consumers can process events in parallel
4. **Auditability**: Event log provides complete history
5. **Eventual Consistency**: Services sync via events, not shared databases

# Event-Driven Architecture Design

## Overview

Transition edd-cloud from shared database to event-driven microservices using:
- **NATS JetStream** for message queue (ordered, persistent)
- **Protobuf** for message serialization
- **Separate databases** per service
- **edd-cloud-gateway** for routing (NOT Traefik)

---

## Service Architecture

### Services and Ownership

| Service | Database | Tables Owned | Publishes | Subscribes |
|---------|----------|--------------|-----------|------------|
| **Auth** | `auth_db` | `users`, `sessions` | `auth.user.*`, `auth.session.*` | - |
| **SFS** | `sfs_db` | `namespaces`, file metadata | `sfs.namespace.*`, `sfs.file.*` | `auth.user.deleted` |
| **Compute** | `compute_db` | `containers`, `ssh_keys` | `compute.container.*`, `compute.sshkey.*` | `auth.user.deleted` |
| **Gateway** | `gateway_db` | `static_routes`, `ingress_rules` | `gateway.route.*` | `compute.container.*`, `compute.ingress.*` |
| **Log Service** | - (GFS) | Logs | - | - |
| **Cluster Monitor** | - | - | - | - |

### Database Credentials (Already Created)

```bash
# Secrets created:
kubectl get secrets | grep db-credentials
# auth-db-credentials
# sfs-db-credentials
# compute-db-credentials

# Databases created:
# auth_db, sfs_db, compute_db

# Still need:
# gateway_db + gateway-db-credentials
```

---

## NATS JetStream Configuration

### Streams

| Stream | Subjects | Retention | Replicas | Description |
|--------|----------|-----------|----------|-------------|
| `AUTH` | `auth.>` | 7 days | 1 | User and session events |
| `COMPUTE` | `compute.>` | 7 days | 1 | Container and SSH key events |
| `GATEWAY` | `gateway.>` | 7 days | 1 | Routing events |
| `SFS` | `sfs.>` | 7 days | 1 | Namespace and file events |

### Consumers (Durable, Fan-out)

| Stream | Consumer | Service | Filter |
|--------|----------|---------|--------|
| `AUTH` | `sfs-service` | SFS | `auth.user.>` |
| `AUTH` | `compute-service` | Compute | `auth.user.>` |
| `AUTH` | `gateway-service` | Gateway | `auth.user.>` |
| `COMPUTE` | `gateway-service` | Gateway | `compute.container.>`, `compute.ingress.>` |

### Ordering Strategy

- **Subject per entity**: `compute.container.{container_id}.{event}`
- JetStream guarantees order within a subject
- Example: `compute.container.abc123.created` then `compute.container.abc123.deleted`

### NATS Deployment (K8s)

```yaml
# manifests/nats/nats.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nats
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nats
  template:
    metadata:
      labels:
        app: nats
    spec:
      containers:
        - name: nats
          image: nats:2.10-alpine
          args:
            - "--jetstream"
            - "--store_dir=/data"
          ports:
            - containerPort: 4222
              name: client
            - containerPort: 8222
              name: monitor
          volumeMounts:
            - name: data
              mountPath: /data
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: nats-data
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: nats-data
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: local-path
  resources:
    requests:
      storage: 5Gi
---
apiVersion: v1
kind: Service
metadata:
  name: nats
spec:
  selector:
    app: nats
  ports:
    - port: 4222
      targetPort: 4222
      name: client
    - port: 8222
      targetPort: 8222
      name: monitor
```

---

## Protobuf Definitions

### Directory Structure

```
proto/
├── common/
│   └── types.proto       # Shared types (Timestamp, etc.)
├── auth/
│   └── events.proto      # User and session events
├── compute/
│   └── events.proto      # Container, SSH key, ingress events
├── gateway/
│   └── events.proto      # Route events
└── sfs/
    └── events.proto      # Namespace and file events
```

### proto/common/types.proto

```protobuf
syntax = "proto3";

package common;

option go_package = "eddisonso.com/edd-cloud/proto/common";

message Timestamp {
  int64 seconds = 1;
  int32 nanos = 2;
}

message EventMetadata {
  string event_id = 1;        // Unique event ID (UUID)
  string entity_id = 2;       // ID of the entity (user_id, container_id, etc.)
  Timestamp timestamp = 3;    // When event occurred
  string source = 4;          // Service that emitted the event
  int64 version = 5;          // For optimistic concurrency
}
```

### proto/auth/events.proto

```protobuf
syntax = "proto3";

package auth;

import "common/types.proto";

option go_package = "eddisonso.com/edd-cloud/proto/auth";

// Published when a new user registers
// Subject: auth.user.{user_id}.created
message UserCreated {
  common.EventMetadata metadata = 1;
  int64 user_id = 2;
  string username = 3;
  string display_name = 4;
  string public_id = 5;       // 6-char nanoid
}

// Published when a user is deleted
// Subject: auth.user.{user_id}.deleted
message UserDeleted {
  common.EventMetadata metadata = 1;
  int64 user_id = 2;
  string username = 3;
}

// Published when user profile is updated
// Subject: auth.user.{user_id}.updated
message UserUpdated {
  common.EventMetadata metadata = 1;
  int64 user_id = 2;
  string username = 3;
  string display_name = 4;
}

// Published when a session is created (login)
// Subject: auth.session.{session_id}.created
message SessionCreated {
  common.EventMetadata metadata = 1;
  string session_id = 2;
  int64 user_id = 3;
  common.Timestamp expires_at = 4;
}

// Published when a session is invalidated (logout)
// Subject: auth.session.{session_id}.invalidated
message SessionInvalidated {
  common.EventMetadata metadata = 1;
  string session_id = 2;
  int64 user_id = 3;
}
```

### proto/compute/events.proto

```protobuf
syntax = "proto3";

package compute;

import "common/types.proto";

option go_package = "eddisonso.com/edd-cloud/proto/compute";

// Container lifecycle events
// Subject: compute.container.{container_id}.created
message ContainerCreated {
  common.EventMetadata metadata = 1;
  string container_id = 2;
  int64 user_id = 3;
  string owner_username = 4;
  string name = 5;
  string namespace = 6;
  int32 memory_mb = 7;
  int32 storage_gb = 8;
  string image = 9;
}

// Subject: compute.container.{container_id}.started
message ContainerStarted {
  common.EventMetadata metadata = 1;
  string container_id = 2;
  string external_ip = 3;
}

// Subject: compute.container.{container_id}.stopped
message ContainerStopped {
  common.EventMetadata metadata = 1;
  string container_id = 2;
}

// Subject: compute.container.{container_id}.deleted
message ContainerDeleted {
  common.EventMetadata metadata = 1;
  string container_id = 2;
  int64 user_id = 3;
}

// Subject: compute.container.{container_id}.ssh_toggled
message ContainerSSHToggled {
  common.EventMetadata metadata = 1;
  string container_id = 2;
  bool ssh_enabled = 3;
}

// Subject: compute.container.{container_id}.https_toggled
message ContainerHTTPSToggled {
  common.EventMetadata metadata = 1;
  string container_id = 2;
  bool https_enabled = 3;
}

// SSH Key events
// Subject: compute.sshkey.{key_id}.created
message SSHKeyCreated {
  common.EventMetadata metadata = 1;
  int64 key_id = 2;
  int64 user_id = 3;
  string name = 4;
  string fingerprint = 5;
}

// Subject: compute.sshkey.{key_id}.deleted
message SSHKeyDeleted {
  common.EventMetadata metadata = 1;
  int64 key_id = 2;
  int64 user_id = 3;
}

// Ingress rule events (Compute requests Gateway to create/delete)
// Subject: compute.ingress.{container_id}.requested
message IngressRuleRequested {
  common.EventMetadata metadata = 1;
  string container_id = 2;
  int32 port = 3;
  int32 target_port = 4;
}

// Subject: compute.ingress.{container_id}.delete_requested
message IngressRuleDeleteRequested {
  common.EventMetadata metadata = 1;
  string container_id = 2;
  int32 port = 3;
}
```

### proto/gateway/events.proto

```protobuf
syntax = "proto3";

package gateway;

import "common/types.proto";

option go_package = "eddisonso.com/edd-cloud/proto/gateway";

// Published when Gateway creates an ingress rule
// Subject: gateway.ingress.{container_id}.created
message IngressRuleCreated {
  common.EventMetadata metadata = 1;
  int64 rule_id = 2;
  string container_id = 3;
  int32 port = 4;
  int32 target_port = 5;
}

// Subject: gateway.ingress.{container_id}.deleted
message IngressRuleDeleted {
  common.EventMetadata metadata = 1;
  int64 rule_id = 2;
  string container_id = 3;
  int32 port = 4;
}

// Subject: gateway.route.{route_id}.registered
message RouteRegistered {
  common.EventMetadata metadata = 1;
  int64 route_id = 2;
  string host = 3;
  string path_prefix = 4;
  string target = 5;
  bool strip_prefix = 6;
}

// Subject: gateway.route.{route_id}.unregistered
message RouteUnregistered {
  common.EventMetadata metadata = 1;
  int64 route_id = 2;
  string host = 3;
  string path_prefix = 4;
}

// Internal event when router cache is refreshed
// Subject: gateway.router.refreshed
message RouterRefreshed {
  common.EventMetadata metadata = 1;
  int32 container_count = 2;
  int32 route_count = 3;
}
```

### proto/sfs/events.proto

```protobuf
syntax = "proto3";

package sfs;

import "common/types.proto";

option go_package = "eddisonso.com/edd-cloud/proto/sfs";

// Namespace events
// Subject: sfs.namespace.{namespace_id}.created
message NamespaceCreated {
  common.EventMetadata metadata = 1;
  int64 namespace_id = 2;
  int64 owner_id = 3;
  string name = 4;
  string visibility = 5;  // "private", "internal", "public"
}

// Subject: sfs.namespace.{namespace_id}.deleted
message NamespaceDeleted {
  common.EventMetadata metadata = 1;
  int64 namespace_id = 2;
  int64 owner_id = 3;
}

// Subject: sfs.namespace.{namespace_id}.visibility_changed
message NamespaceVisibilityChanged {
  common.EventMetadata metadata = 1;
  int64 namespace_id = 2;
  string old_visibility = 3;
  string new_visibility = 4;
}

// File events (optional, for audit/analytics)
// Subject: sfs.file.{namespace_id}.uploaded
message FileUploaded {
  common.EventMetadata metadata = 1;
  int64 namespace_id = 2;
  string path = 3;
  int64 size_bytes = 4;
  int64 uploader_id = 5;
}

// Subject: sfs.file.{namespace_id}.deleted
message FileDeleted {
  common.EventMetadata metadata = 1;
  int64 namespace_id = 2;
  string path = 3;
  int64 deleter_id = 4;
}
```

---

## Event Flow Examples

### 1. User Registration

```
User registers via Auth API
         │
         ▼
    ┌─────────┐
    │  Auth   │──── auth.user.{id}.created ────▶ NATS AUTH Stream
    └─────────┘
                                                       │
                    ┌──────────────────────────────────┼──────────────────┐
                    ▼                                  ▼                  ▼
              ┌─────────┐                        ┌─────────┐        ┌─────────┐
              │   SFS   │                        │ Compute │        │ Gateway │
              └─────────┘                        └─────────┘        └─────────┘
              (cache user)                       (cache user)       (cache user)
```

### 2. Container Creation with Ingress

```
User creates container via Compute API
         │
         ▼
    ┌─────────┐
    │ Compute │──┬── compute.container.{id}.created ──▶ NATS COMPUTE Stream
    └─────────┘  │
                 └── compute.ingress.{id}.requested ──▶ NATS COMPUTE Stream
                                                              │
                                                              ▼
                                                        ┌─────────┐
                                                        │ Gateway │
                                                        └────┬────┘
                                                             │
                    ┌────────────────────────────────────────┘
                    ▼
         1. Create ingress_rule in gateway_db
         2. Publish gateway.ingress.{id}.created
         3. Reload router cache
```

### 3. User Deletion (Cascade Cleanup)

```
Admin deletes user via Auth API
         │
         ▼
    ┌─────────┐
    │  Auth   │──── auth.user.{id}.deleted ────▶ NATS AUTH Stream
    └─────────┘
                                                       │
                    ┌──────────────────────────────────┼──────────────────┐
                    ▼                                  ▼                  ▼
              ┌─────────┐                        ┌─────────┐        ┌─────────┐
              │   SFS   │                        │ Compute │        │ Gateway │
              └─────────┘                        └─────────┘        └─────────┘
              Delete user's                      Delete user's      Remove user's
              namespaces                         containers &       container routes
                                                 SSH keys
```

---

## Implementation Plan

### Phase 1: Infrastructure Setup
1. [x] Create separate databases (auth_db, sfs_db, compute_db)
2. [x] Create database secrets
3. [ ] Create gateway_db and gateway-db-credentials
4. [ ] Deploy NATS with JetStream
5. [ ] Create NATS streams and consumers

### Phase 2: Proto and Shared Library
1. [ ] Create proto/ directory structure
2. [ ] Write all .proto files
3. [ ] Create shared Go library for NATS client
4. [ ] Generate Go code from protos

### Phase 3: Auth Service Extraction
1. [ ] Create services/auth/ directory
2. [ ] Move user/session logic from SFS
3. [ ] Implement event publishing
4. [ ] Create auth manifest
5. [ ] Update SFS to consume auth events

### Phase 4: Gateway Database Migration
1. [ ] Move ingress_rules table to gateway_db
2. [ ] Gateway subscribes to compute.ingress.* events
3. [ ] Compute publishes ingress events instead of direct DB writes
4. [ ] Remove ingress_rules from compute_db

### Phase 5: Full Event-Driven
1. [ ] All services publish events on state changes
2. [ ] Services subscribe to events they need
3. [ ] Remove direct database cross-references
4. [ ] Implement event replay for recovery

---

## Request/Reply Pattern (Sync when needed)

For cases where sync response is needed (e.g., validate JWT):

```go
// Auth service - responder
nc.Subscribe("auth.validate.request", func(msg *nats.Msg) {
    var req pb.ValidateTokenRequest
    proto.Unmarshal(msg.Data, &req)

    // Validate token
    resp := &pb.ValidateTokenResponse{
        Valid:    true,
        UserId:   123,
        Username: "john",
    }

    data, _ := proto.Marshal(resp)
    msg.Respond(data)
})

// Other service - requester
reply, err := nc.Request("auth.validate.request", tokenData, 2*time.Second)
```

---

## Error Handling

### Dead Letter Queue
Failed messages after N retries go to `*.dlq` subject:
```
auth.user.created.dlq
compute.container.created.dlq
```

### Retry Policy
```go
consumerConfig := &nats.ConsumerConfig{
    MaxDeliver: 5,           // Max retry attempts
    AckWait:    30 * time.Second,
    BackOff:    []time.Duration{1*time.Second, 5*time.Second, 30*time.Second},
}
```

### Idempotency
- All handlers must be idempotent (same event processed twice = same result)
- Use event_id for deduplication
- Use version field for optimistic concurrency

---

## Monitoring

### NATS Monitoring Endpoints
- `http://nats:8222/jsz` - JetStream info
- `http://nats:8222/connz` - Connections
- `http://nats:8222/subsz` - Subscriptions

### Metrics to Track
- Consumer lag (messages pending)
- Message publish rate
- Consumer ack rate
- Dead letter queue size

---

## Files to Create/Modify

### New Files
| File | Description |
|------|-------------|
| `proto/common/types.proto` | Shared types |
| `proto/auth/events.proto` | Auth events |
| `proto/compute/events.proto` | Compute events |
| `proto/gateway/events.proto` | Gateway events |
| `proto/sfs/events.proto` | SFS events |
| `manifests/nats/nats.yaml` | NATS deployment |
| `services/auth/` | New auth service |
| `pkg/events/` | Shared NATS client library |

### Files to Modify
| File | Changes |
|------|---------|
| `services/sfs/main.go` | Remove user management, add event consumer |
| `services/compute/internal/api/` | Publish events, remove ingress DB writes |
| `edd-gateway/internal/router/` | Subscribe to compute events |
| `manifests/sfs/simple-file-share.yaml` | Update DB secret |
| `manifests/edd-compute/edd-compute.yaml` | Add NATS connection |
| `manifests/edd-gateway/` | Add NATS connection, new DB secret |

---

## Rollback Plan

1. Keep old `postgres-credentials` secret
2. Keep shared `eddcloud` database intact during migration
3. Feature flag for event publishing (can disable)
4. Services can fall back to direct DB queries if NATS is down

---

## Gateway Routing

All HTTP/HTTPS traffic is routed through **edd-cloud-gateway** (NOT Traefik).

### Why edd-cloud-gateway?
- Custom routing logic for container access
- SSH proxy support (port 22 → container)
- Dynamic route updates via events
- Better control over streaming/SSE connections
- No need for Kubernetes Ingress resources

### Configuration

Routes are defined in `edd-gateway/routes.yaml`:
```yaml
routes:
  # API routes to cloud-api.eddisonso.com
  - host: cloud-api.eddisonso.com
    path: /compute
    target: edd-compute:80
    strip_prefix: false

  # SSE endpoints for cluster monitoring
  - host: cloud-api.eddisonso.com
    path: /sse/cluster-info
    target: cluster-monitor:80
    strip_prefix: true

  # Frontend SPA
  - host: cloud.eddisonso.com
    path: /
    target: simple-file-share-frontend:80
    strip_prefix: false
```

### Updating Routes
1. Edit `edd-gateway/routes.yaml`
2. Apply configmap: `kubectl create configmap gateway-routes --from-file=routes.yaml --dry-run=client -o yaml | kubectl apply -f -`
3. Restart gateway: `kubectl rollout restart deployment/gateway`

### Service Ports
- HTTP: 80 (→ 8080 internally)
- HTTPS: 443 (→ 8443 internally, TLS termination)
- SSH: 22 (→ 2222 internally, for container SSH)
- Dynamic ports: 8000-8100+ for container ingress

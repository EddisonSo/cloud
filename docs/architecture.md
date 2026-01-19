---
sidebar_position: 2
---

# Architecture

## System Overview

```
┌───────────────────────────────────────────────────────────────────────────────┐
│                                    INTERNET                                   │
└───────────────────────────────────────┬───────────────────────────────────────┘
                                        │
                          ┌─────────────▼─────────────┐
                          │          Gateway          │
                          │   (TLS + SSH + Routing)   │
                          └─────────────┬─────────────┘
                                        │
        ┌───────────────────────────────┼───────────────────────────────┐
        │                               │                               │
        ▼                               ▼                               ▼
┌───────────────┐             ┌─────────────────┐             ┌─────────────────┐
│   Frontend    │             │   Storage API   │             │   Compute API   │
│    (React)    │             │  (SFS Backend)  │             │  (edd-compute)  │
└───────────────┘             └───────┬─────────┘             └────────┬────────┘
                                      │                                │
                         ┌────────────┴────────────┐        ┌──────────┴──────────┐
                         │                         │        │                     │
                         ▼                         │        ▼                     ▼
            ┌─────────────────────┐                │  ┌───────────┐         ┌───────────┐
            │     GFS Master      │                │  │ Container │         │ Container │
            └──────────┬──────────┘                │  │  (user1)  │         │  (user2)  │
                       │                           │  └───────────┘         └───────────┘
      ┌────────────────┼────────────────┐          │
      │                │                │          │
      ▼                ▼                ▼          │
┌──────────┐    ┌──────────┐    ┌──────────┐       │
│ Chunk 1  │    │ Chunk 2  │    │ Chunk 3  │       │
│  (rp1)   │    │  (rp2)   │    │  (rp3)   │       │
└──────────┘    └──────────┘    └──────────┘       │
                                                   │
                       ┌───────────────────────────┘
                       ▼
              ┌──────────────┐
              │  PostgreSQL  │◄── Storage + Compute APIs
              └──────────────┘
```

## Request Flow

### Storage Request Flow

1. Client makes HTTPS request to `storage.cloud.eddisonso.com`
2. Gateway terminates TLS and routes to Storage API
3. Storage API authenticates via JWT
4. For file operations:
   - **Write**: Storage API → GFS Master (allocate chunk) → Chunkservers (2PC write)
   - **Read**: Storage API → GFS Master (get locations) → Chunkserver (read data)
5. Response returned to client

### Compute Request Flow

1. Client makes HTTPS request to `compute.cloud.eddisonso.com`
2. Gateway terminates TLS and routes to Compute API
3. Compute API authenticates via JWT
4. Compute API interacts with Kubernetes API for container operations
5. Container status updates streamed via WebSocket

## Data Persistence

### GFS (Distributed File System)

- **Chunk Size**: 64MB
- **Replication Factor**: 3
- **Consistency**: Two-Phase Commit (2PC)
- **Write Quorum**: 2 of 3 replicas

### PostgreSQL

Stores metadata for:
- User accounts and sessions
- Container definitions
- SSH keys
- Ingress rules
- Gateway routing rules
- Namespace configurations

## Network Architecture

### External Domains

| Domain | Purpose |
|--------|---------|
| `cloud.eddisonso.com` | Main dashboard |
| `storage.cloud.eddisonso.com` | Storage API |
| `compute.cloud.eddisonso.com` | Compute API |
| `health.cloud.eddisonso.com` | Health/Monitoring API |
| `docs.cloud.eddisonso.com` | Documentation |

### Internal Services

| Service | Port | Protocol |
|---------|------|----------|
| gateway | 8080/8443/2222 | HTTP/HTTPS/SSH |
| simple-file-share-backend | 80 | HTTP |
| simple-file-share-frontend | 80 | HTTP |
| edd-compute | 80 | HTTP |
| cluster-monitor | 80 | HTTP |
| log-service | 50051/80 | gRPC/HTTP |
| gfs-master | 9000 | gRPC |
| gfs-chunkserver-N | 8080/8081 | TCP/gRPC |
| postgres | 5432 | PostgreSQL |

## Security

### Authentication

- JWT-based authentication for API requests
- Tokens issued on login, stored in localStorage
- Token passed via `Authorization: Bearer` header or query param for SSE/WebSocket

### TLS

- All external traffic encrypted with TLS 1.2+
- Certificates managed by cert-manager with Let's Encrypt
- Wildcard certificates for `*.eddisonso.com` and `*.cloud.eddisonso.com`

### CORS

- Each service implements CORS middleware
- Origin header reflected for cross-domain requests
- Credentials allowed for authenticated requests

## TODO / Roadmap

### Gateway Improvements

- [ ] **Radix tree routing** - Replace linear route matching with radix tree for O(log n) lookups
- [ ] **L4 load balancer pre-ingress** - Add TCP/UDP load balancer layer for distributed gateway deployment
- [ ] **Connection pooling** - Reuse backend connections to reduce latency

### Distributed Gateway Architecture (Future)

```
                    ┌─────────────┐
                    │   L4 LB     │
                    │  (TCP/UDP)  │
                    └──────┬──────┘
                           │
         ┌─────────────────┼─────────────────┐
         │                 │                 │
         ▼                 ▼                 ▼
   ┌───────────┐     ┌───────────┐     ┌───────────┐
   │ Gateway 1 │     │ Gateway 2 │     │ Gateway 3 │
   └───────────┘     └───────────┘     └───────────┘
```

### Storage Improvements

- [ ] **Chunk garbage collection** - Clean up orphaned chunks
- [ ] **Erasure coding** - Reduce storage overhead vs 3x replication
- [ ] **Tiered storage** - Hot/cold data separation

### Compute Improvements

- [ ] **Container resource limits** - CPU/memory quotas per user
- [ ] **Container networking** - Private networks between user containers
- [ ] **Persistent volumes** - User-attached storage volumes

### Monitoring

- [ ] **Distributed tracing** - Request tracing across services
- [ ] **Alerting** - Automated alerts for service health
- [ ] **Log aggregation** - Searchable log storage

---
sidebar_position: 10
---

# Roadmap

## Gateway Improvements

- [ ] **Radix tree routing** - Replace linear route matching with radix tree for O(log n) lookups
- [ ] **L4 load balancer pre-ingress** - Add TCP/UDP load balancer layer for distributed gateway deployment
- [ ] **Connection pooling** - Reuse backend connections to reduce latency
- [ ] **HTTP/2 support** - Enable gRPC and improved multiplexing

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

## Storage Improvements

- [x] **Chunk garbage collection** - Clean up orphaned chunks (1-hour grace period, via heartbeat)
- [ ] **Chunk corruption recovery** - Detect corrupted chunks via checksums and re-replicate from healthy replicas
- [ ] **Tiered storage** - Hot/cold data separation
- [ ] **Programmatic API** - Upload and download files via REST API for external integrations

## Compute Improvements

- [ ] **Container access control** - Users can only access their own containers (SSH, logs, management)
- [ ] **Container resource limits** - CPU/memory quotas per user
- [ ] **Container networking** - Private networks between user containers
- [ ] **Persistent volumes** - User-attached storage volumes
- [ ] **True VMs** - Full virtual machines via Type 1 hypervisor (KVM) for stronger isolation
- [ ] **Multi-architecture support** - Provision compute on different architectures (amd64, arm64)

## Infrastructure

- [ ] **Migrate control plane to s0** - Move K3s server from rp1 (arm64, 8GB, flash) to s0 (amd64, 16GB, SSD)
- [ ] **Multi-master HA** - Add redundant control plane for high availability

## Monitoring

- [ ] **Distributed tracing** - Request tracing across services
- [ ] **Alerting** - Automated alerts for service health
- [ ] **Log aggregation** - Searchable log storage
- [ ] **Delta updates for SSE** - Send only changes instead of full state to reduce bandwidth

## Future Services

- [ ] **Message Queue** - Pub/sub messaging for async communication between services
- [ ] **Datastore** - NoSQL database for flexible document/key-value storage

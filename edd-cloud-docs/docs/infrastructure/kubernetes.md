---
sidebar_position: 1
---

# Kubernetes Infrastructure

Edd Cloud runs on a K3s Kubernetes cluster with mixed architecture nodes. All workloads run in the `core` namespace.

## Cluster Overview

| Node | Architecture | Labels | Role |
|------|--------------|--------|------|
| s0 | amd64 | `core-services=true`, `gfs-master=true` | GFS master, gateway |
| rp1 | arm64 | `db-role=replica` | Database primary (promoted), HAProxy |
| rp2 | arm64 | `backend=true`, `core-services=true` | Backend services |
| rp3 | arm64 | `backend=true` | Backend services |
| rp4 | arm64 | `backend=true` | Backend services |
| s1 | amd64 | `role=chunkserver`, control-plane, etcd | GFS chunkserver, control plane |
| s2 | amd64 | `role=chunkserver`, control-plane, etcd | GFS chunkserver, control plane |
| s3 | amd64 | `role=chunkserver`, control-plane, etcd | GFS chunkserver, control plane |

## Node Scheduling

Services are scheduled based on node labels:

```yaml
# Core services (s0, rp2)
nodeSelector:
  core-services: "true"

# Backend workloads (rp2, rp3, rp4)
nodeSelector:
  backend: "true"

# Database (rp1)
nodeSelector:
  db-role: replica

# GFS master (s0)
nodeSelector:
  gfs-master: "true"

# GFS chunkservers (s1, s2, s3)
nodeSelector:
  role: chunkserver

# Small services
nodeSelector:
  size: mini
```

### Core Services (s0, rp2)

- gateway
- gfs-master
- notification-service

### Backend Services (rp2, rp3, rp4)

- auth-service
- simple-file-share-backend
- simple-file-share-frontend
- edd-compute
- cluster-monitor
- log-service
- edd-cloud-docs
- alerting-service

### Database (rp1)

- postgres-replica (promoted to primary)
- haproxy (connection pooling)

### NATS

- nats (`size=mini` node)

### GFS Chunkservers (s1, s2, s3)

- gfs-chunkserver (hostNetwork DaemonSet)
- k3s control plane + etcd

## Deployments

### Application Deployments

```bash
NAME                         READY   NODES
gateway                      1/1     s0
gfs-master                   1/1     s0
auth-service                 1/1     rp{2-4}
simple-file-share-backend    2/2     rp{2-4}
simple-file-share-frontend   2/2     rp{2-4}
edd-compute                  2/2     rp{2-4}
cluster-monitor              2/2     rp{2-4}
log-service                  1/1     rp{2-4}
notification-service         1/1     rp2 (core-services)
edd-cloud-docs               2/2     rp{2-4}
alerting-service             1/1     rp{2-4}
nats                         1/1     (size=mini)
postgres-replica             1/1     rp1
haproxy                      1/1     rp1
```

### Database

PostgreSQL runs in a promoted-replica configuration on rp1. The original primary on s0 is **disabled** (0 replicas). The deployment is named `postgres-replica` for PVC/service compatibility, but operates as the primary.

The `postgres` service selector points to `app: postgres-replica`, so all services connect transparently.

```yaml
# postgres-replica (PRIMARY on rp1)
replicas: 1
image: postgres:16-alpine
nodeSelector:
  db-role: replica
```

HAProxy provides connection pooling and health checking on rp1:

```yaml
# HAProxy on rp1
replicas: 1
nodeSelector:
  db-role: replica
```

### GFS Chunkservers

GFS chunkservers run as a DaemonSet with `hostNetwork: true` on s1, s2, and s3:

```bash
# DaemonSet
NAME              DESIRED   CURRENT   NODES
gfs-chunkserver   3         3         s1, s2, s3
```

## Persistent Storage

### PostgreSQL

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: postgres-replica-data
spec:
  accessModes: [ReadWriteOnce]
  storageClassName: local-path
  resources:
    requests:
      storage: 5Gi
```

### GFS Master

```yaml
volumes:
  - name: master-data
    hostPath:
      path: /data/gfs
      type: DirectoryOrCreate
```

### GFS Chunkservers

Each chunkserver uses a hostPath volume on its node:

```yaml
volumes:
  - name: chunk-data
    hostPath:
      path: /data/gfs/chunkserver
      type: DirectoryOrCreate
```

### NATS JetStream

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: nats-data
spec:
  accessModes: [ReadWriteOnce]
  storageClassName: local-path
  resources:
    requests:
      storage: 5Gi
```

## Services

| Service | Type | Ports |
|---------|------|-------|
| gateway | LoadBalancer | 80, 443, 2222, 8000-8999 |
| auth-service | ClusterIP | 80 |
| simple-file-share-backend | ClusterIP | 80 |
| simple-file-share-frontend | ClusterIP | 80 |
| edd-compute | ClusterIP | 80 |
| cluster-monitor | ClusterIP | 80 |
| log-service | ClusterIP | 50051 (gRPC), 80 (HTTP) |
| notification-service | ClusterIP | 80 |
| alerting-service | ClusterIP | 80 |
| edd-cloud-docs | ClusterIP | 80 |
| postgres | ClusterIP | 5432 |
| postgres-replica | ClusterIP | 5432 |
| haproxy | ClusterIP | 5432 |
| gfs-master | ClusterIP + NodePort | 9000, 30900 |
| nats | ClusterIP | 4222 (client), 8222 (monitor) |

## Network Policies

### Core Namespace Isolation

The `core` namespace has a `NetworkPolicy` restricting ingress:

- Allow all traffic within the `core` namespace (pod-to-pod)
- Allow traffic from node network (`192.168.0.0/16`)
- Allow traffic from pod overlay network (`10.42.0.0/16`)
- Allow external traffic on gateway ports: 2222 (SSH), 18080 (HTTP), 8443 (HTTPS)

### GFS Chunkserver Access

A Calico `GlobalNetworkPolicy` (`allow-host-chunkserver`) permits traffic on:

- Port 9080 (GFS client)
- Port 9081 (GFS replication)
- Port 22 (SSH)
- Port 6443 (K3s API)
- Port 10250 (Kubelet)

## MetalLB

MetalLB provides LoadBalancer IP allocation in L2 mode:

```yaml
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: compute-pool
  namespace: metallb-system
spec:
  addresses:
    - 192.168.3.150-192.168.3.200
```

## Secrets

| Secret | Purpose |
|--------|---------|
| `postgres-credentials` | PostgreSQL admin and replication passwords |
| `compute-db-credentials` | Compute service database access |
| `auth-db-credentials` | Auth service database access |
| `sfs-db-credentials` | File sharing service database access |
| `notification-db-credentials` | Notification service database access |
| `edd-cloud-auth` | JWT_SECRET, default credentials |
| `edd-cloud-admin` | Admin username (shared across services) |
| `service-api-key` | Inter-service authentication key |
| `gfs-jwt-secret` | GFS JWT signing secret |
| `discord-webhook-url` | Discord webhook for alerting |
| `eddisonso-wildcard-tls` | Wildcard TLS certificate |
| `regcred` | Docker registry credentials |

## Maintenance

### Image Cleanup CronJob

Unused container images are pruned daily across all nodes:

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: image-cleanup
spec:
  schedule: "0 3 * * *"
  concurrencyPolicy: Forbid
```

Runs `crictl rmi --prune` on each node via a privileged container with host access. Uses `tolerations: [{operator: Exists}]` to schedule on all nodes including control-plane.

## CI/CD

Deployments are automated via GitHub Actions:

1. Push to `main` branch
2. GitHub Actions detects changed services
3. Docker images built and tagged with UTC timestamp (`YYYYMMDD-HHMMSS`)
4. Images pushed to Docker Hub
5. Kubernetes deployment updated via `kubectl set image`

```yaml
# CI/CD generates timestamp tags - never use 'latest'
- name: Deploy to Kubernetes
  run: |
    kubectl --kubeconfig=kubeconfig set image deployment/myapp \
      myapp=eddisonso/myapp:20260215-062053
```

Images must always use timestamp tags (e.g., `eddisonso/ecloud-auth:20260215-062053`). The `latest` tag on Docker Hub may be stale.

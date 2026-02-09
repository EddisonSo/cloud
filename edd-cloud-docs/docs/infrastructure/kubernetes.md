---
sidebar_position: 1
---

# Kubernetes Infrastructure

Edd Cloud runs on a K3s Kubernetes cluster with mixed architecture nodes.

## Cluster Overview

| Node | Architecture | Labels | Role |
|------|--------------|--------|------|
| s0 | amd64 | `core-services=true`, `gfs-master=true` | Database primary, GFS master, gateway |
| rp1 | arm64 | `db-role=replica` | Database replica, HAProxy |
| rp2 | arm64 | `backend=true`, `core-services=true` | Backend services |
| rp3 | arm64 | `backend=true` | Backend services |
| rp4 | arm64 | `backend=true` | Backend services |
| s1 | amd64 | `role=chunkserver`, control-plane, etcd | GFS chunkserver, control plane |
| s2 | amd64 | `role=chunkserver`, control-plane, etcd | GFS chunkserver, control plane |
| s3 | amd64 | `role=chunkserver`, control-plane, etcd | GFS chunkserver, control plane |

## Node Scheduling

Services are scheduled based on node labels:

```yaml
# Schedule on core-services nodes (s0, rp2)
nodeSelector:
  core-services: "true"

# Schedule on backend nodes (rp2, rp3, rp4)
nodeSelector:
  backend: "true"
```

### Core Services (s0)

- gateway
- gfs-master
- notification-service
- postgres-primary
- postgres-replica-s0

### Backend Services (rp2, rp3, rp4)

- auth-service
- simple-file-share-backend
- simple-file-share-frontend
- edd-compute
- cluster-monitor
- log-service
- nats
- edd-cloud-docs
- postgres-replicas

### Database (s0, rp1)

- postgres-primary (s0)
- postgres-replica (rp1)
- haproxy (rp1)

### GFS Chunkservers (s1, s2, s3)

- gfs-chunkserver (hostNetwork DaemonSet)
- k3s control plane + etcd

## Deployments

### Application Deployments

```bash
# Example output
NAME                         READY   NODES
gateway                      1/1     s0
auth-service                 1/1     rp4
simple-file-share-backend    2/2     rp3, rp4
simple-file-share-frontend   2/2     rp2, rp3
edd-compute                  2/2     rp2, rp4
cluster-monitor              2/2     rp2, rp3
log-service                  1/1     rp3
notification-service         1/1     s0
nats                         1/1     rp4
edd-cloud-docs               2/2     rp3, rp4
gfs-master                   1/1     s0
haproxy                      1/1     rp1
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
  name: postgres-data
spec:
  accessModes: [ReadWriteOnce]
  storageClassName: local-path
  resources:
    requests:
      storage: 5Gi
```

### GFS Chunkservers

Each chunkserver has its own PVC for data storage:

```yaml
volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: [ReadWriteOnce]
      storageClassName: local-path
      resources:
        requests:
          storage: 100Gi
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
| log-service | ClusterIP | 50051, 80 |
| notification-service | ClusterIP | 80 |
| edd-cloud-docs | ClusterIP | 80 |
| postgres | ClusterIP | 5432 |
| haproxy | ClusterIP | 5432 |
| gfs-master | ClusterIP | 9000 |
| nats | ClusterIP | 4222, 8222 |

## Secrets

| Secret | Purpose |
|--------|---------|
| `postgres-credentials` | PostgreSQL connection string |
| `edd-cloud-auth` | JWT signing secret |
| `eddisonso-wildcard-tls` | TLS certificates |
| `regcred` | Docker registry credentials |

## CI/CD

Deployments are automated via GitHub Actions:

1. Push to `main` branch
2. GitHub Actions builds Docker image
3. Image pushed to Docker Hub
4. Kubernetes deployment updated via `kubectl`

```yaml
# Example workflow step
- name: Deploy to Kubernetes
  run: |
    echo "${{ secrets.KUBECONFIG }}" | base64 -d > kubeconfig
    kubectl --kubeconfig=kubeconfig set image deployment/myapp \
      myapp=eddisonso/myapp:${{ github.sha }}
```

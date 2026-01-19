---
sidebar_position: 1
---

# Kubernetes Infrastructure

Edd Cloud runs on a K3s Kubernetes cluster with mixed architecture nodes.

## Cluster Overview

| Node | Architecture | Labels | Role |
|------|--------------|--------|------|
| s0 | amd64 | `size=medium` | Core services |
| rp1 | arm64 | `size=mini` | GFS chunkserver, workloads |
| rp2 | arm64 | `size=mini` | GFS chunkserver, workloads |
| rp3 | arm64 | `size=mini` | GFS chunkserver, workloads |
| rp4 | arm64 | `size=mini` | GFS chunkserver, workloads |

## Node Scheduling

Services are scheduled based on node labels:

```yaml
# Schedule on medium nodes (s0)
nodeSelector:
  size: medium

# Schedule on mini nodes (rp1-4)
nodeSelector:
  size: mini
```

### Core Services (s0)

- gateway
- simple-file-share-backend
- simple-file-share-frontend
- edd-compute
- cluster-monitor
- log-service
- postgres
- gfs-master

### Distributed Services (rp1-4)

- gfs-chunkserver-1 (rp1)
- gfs-chunkserver-2 (rp2)
- gfs-chunkserver-3 (rp3)
- User containers

## Deployments

### Core Services

```bash
# List deployments
kubectl get deployments

# Example output
NAME                         READY   UP-TO-DATE   AVAILABLE
gateway                      1/1     1            1
simple-file-share-backend    1/1     1            1
simple-file-share-frontend   1/1     1            1
edd-compute                  1/1     1            1
cluster-monitor              1/1     1            1
log-service                  1/1     1            1
postgres                     1/1     1            1
gfs-master                   1/1     1            1
```

### GFS Chunkservers

```bash
# StatefulSet for each chunkserver
kubectl get statefulsets

# Example
gfs-chunkserver-1   1/1
gfs-chunkserver-2   1/1
gfs-chunkserver-3   1/1
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
| gateway | LoadBalancer | 80, 443, 2222 |
| simple-file-share-backend | ClusterIP | 80 |
| simple-file-share-frontend | ClusterIP | 80 |
| edd-compute | ClusterIP | 80 |
| cluster-monitor | ClusterIP | 80 |
| log-service | ClusterIP | 50051, 80 |
| postgres | ClusterIP | 5432 |
| gfs-master | ClusterIP | 9000 |

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

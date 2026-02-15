---
sidebar_position: 1
slug: /
---

# Edd Cloud Overview

Edd Cloud is a personal cloud infrastructure platform that provides storage, compute, and monitoring capabilities. It runs on a Kubernetes cluster with a custom-built distributed file system (GFS) for storage.

## Core Components

| Component | Description | Domain |
|-----------|-------------|--------|
| **Gateway** | TLS termination, routing, SSH tunneling | `cloud.eddisonso.com` |
| **Frontend** | React-based dashboard UI | `cloud.eddisonso.com` |
| **Auth Service** | Authentication, sessions, and service accounts | `auth.cloud.eddisonso.com` |
| **Storage API** | File storage service backed by GFS | `storage.cloud.eddisonso.com` |
| **Compute API** | Container management service | `compute.cloud.eddisonso.com` |
| **Health API** | Cluster monitoring and metrics | `health.cloud.eddisonso.com` |
| **GFS** | Distributed file system (Google File System clone) | Internal |
| **Notifications** | Real-time push notifications via WebSocket | `notifications.cloud.eddisonso.com` |
| **Log Service** | Centralized logging with SSE streaming | Internal |
| **Docs** | Documentation site | `docs.cloud.eddisonso.com` |

## Technology Stack

- **Backend**: Go
- **Frontend**: React + Vite + Tailwind CSS
- **Storage**: Custom GFS implementation with 64MB chunks, RF=3
- **Database**: PostgreSQL (HA with streaming replication)
- **Messaging**: NATS JetStream
- **Container Runtime**: Kubernetes (K3s)
- **TLS**: cert-manager with Let's Encrypt

## Cluster Nodes

| Node | Architecture | Role |
|------|-------------|------|
| s0 | amd64 | Database primary, GFS master, gateway |
| rp1 | arm64 | Database replica, HAProxy |
| rp2, rp3, rp4 | arm64 | Backend services (auth, compute, storage, notifications, etc.) |
| s1, s2, s3 | amd64 | Control plane, etcd, GFS chunkservers |

## Quick Links

- [Architecture Overview](./architecture)
- [Gateway Service](./services/gateway)
- [Auth Service](./services/auth)
- [Storage Service](./services/storage)
- [Compute Service](./services/compute)
- [GFS Distributed Storage](./services/gfs)
- [Notification Service](./services/notifications)

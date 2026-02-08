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
| **Storage API** | File storage service backed by GFS | `storage.cloud.eddisonso.com` |
| **Compute API** | Container management service | `compute.cloud.eddisonso.com` |
| **Health API** | Cluster monitoring and metrics | `health.cloud.eddisonso.com` |
| **GFS** | Distributed file system (Google File System clone) | Internal |
| **Notifications** | Real-time push notifications via WebSocket | `notifications.cloud.eddisonso.com` |
| **Log Service** | Centralized logging with SSE streaming | Internal |

## Technology Stack

- **Backend**: Go
- **Frontend**: React + Vite + Tailwind CSS
- **Storage**: Custom GFS implementation with 64MB chunks, RF=3
- **Database**: PostgreSQL
- **Container Runtime**: Kubernetes (K3s)
- **TLS**: cert-manager with Let's Encrypt

## Cluster Nodes

| Node | Type | Architecture | Role |
|------|------|--------------|------|
| s0 | Medium | amd64 | Core services (gateway, storage, compute, postgres) |
| rp1-rp4 | Mini | arm64 | GFS chunkservers, workloads |

## Quick Links

- [Architecture Overview](./architecture)
- [Gateway Service](./services/gateway)
- [Storage Service](./services/storage)
- [Compute Service](./services/compute)
- [GFS Distributed Storage](./services/gfs)
- [Notification Service](./services/notifications)

---
sidebar_position: 2
---

# Architecture

## What is Edd-Cloud?

Edd-Cloud is a self-hosted cloud platform running on a Raspberry Pi cluster. It provides:

- **File Storage**: A distributed file system (GFS) for storing and sharing files with configurable visibility
- **Container Compute**: Run Docker containers with SSH access and custom ingress routing
- **User Management**: Multi-user authentication with namespaced resources

The platform is designed as a microservices architecture, with each service owning its data and communicating via events.

## Technology Stack

| Layer | Technology |
|-------|------------|
| **Frontend** | React + TypeScript + Vite |
| **API Gateway** | Custom Go gateway (edd-gateway) |
| **Backend Services** | Go |
| **Container Orchestration** | Kubernetes (K3s) |
| **Distributed Storage** | Custom GFS implementation |
| **Database** | PostgreSQL |
| **Message Broker** | NATS JetStream |
| **TLS Certificates** | cert-manager + Let's Encrypt |
| **Load Balancer** | MetalLB |

## System Overview

```
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                                      INTERNET                                        │
│                        Users access via cloud.eddisonso.com                          │
└─────────────────────────────────────┬───────────────────────────────────────────────┘
                                      │
                                      │ HTTPS (443) / SSH (2222)
                                      ▼
                        ┌─────────────────────────────┐
                        │      edd-cloud-gateway      │
                        │  ┌───────────────────────┐  │
                        │  │ TLS Termination       │  │
                        │  │ Host-based Routing    │  │
                        │  │ SSH Multiplexing      │  │
                        │  │ Dynamic Route Config  │  │
                        │  └───────────────────────┘  │
                        └─────────────┬───────────────┘
                                      │
        ┌─────────────────────────────┼─────────────────────────────┐
        │                             │                             │
        ▼                             ▼                             ▼
┌───────────────────┐     ┌─────────────────────┐     ┌─────────────────────┐
│     Frontend      │     │     Storage API     │     │    Compute API      │
│  ┌─────────────┐  │     │  ┌───────────────┐  │     │  ┌───────────────┐  │
│  │ React SPA   │  │     │  │ File CRUD     │  │     │  │ Container Mgmt│  │
│  │ Dashboard   │  │     │  │ Namespace Mgmt│  │     │  │ SSH Key Mgmt  │  │
│  │ File Browser│  │     │  │ User Auth     │  │     │  │ Ingress Rules │  │
│  └─────────────┘  │     │  └───────────────┘  │     │  └───────────────┘  │
└───────────────────┘     └──────────┬──────────┘     └──────────┬──────────┘
                                     │                           │
                                     ▼                           │
                        ┌─────────────────────┐                  │
                        │      GFS Master     │                  │
                        │  ┌───────────────┐  │                  │
                        │  │ Metadata Mgmt │  │                  │
                        │  │ Chunk Alloc   │  │                  │
                        │  │ Replication   │  │                  │
                        │  │ WAL Logging   │  │                  │
                        │  └───────────────┘  │                  │
                        └──────────┬──────────┘                  │
                                   │                             │
          ┌────────────────────────┼────────────────────────┐    │
          │                        │                        │    │
          ▼                        ▼                        ▼    │
    ┌───────────┐            ┌───────────┐            ┌───────────┐
    │Chunkserver│            │Chunkserver│            │Chunkserver│
    │   (rp1)   │            │   (rp2)   │            │   (rp3)   │
    │  ┌─────┐  │            │  ┌─────┐  │            │  ┌─────┐  │
    │  │64MB │  │            │  │64MB │  │            │  │64MB │  │
    │  │Chunks│  │            │  │Chunks│  │            │  │Chunks│  │
    │  └─────┘  │            │  └─────┘  │            │  └─────┘  │
    └───────────┘            └───────────┘            └───────────┘
                                                                 │
                                   ┌─────────────────────────────┘
                                   ▼
┌───────────────────────────────────────────────────────────────────────────────────┐
│                              Supporting Services                                   │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐   │
│  │ PostgreSQL  │  │    NATS     │  │ Log Service │  │   Cluster Monitor       │   │
│  │ ┌─────────┐ │  │ ┌─────────┐ │  │ ┌─────────┐ │  │ ┌─────────────────────┐ │   │
│  │ │ users   │ │  │ │JetStream│ │  │ │  gRPC   │ │  │ │ Pod Metrics (SSE)   │ │   │
│  │ │sessions │ │  │ │ Streams │ │  │ │ Log Agg │ │  │ │ Node Health (SSE)   │ │   │
│  │ │containers│ │  │ │ Events  │ │  │ │  Query  │ │  │ │ Real-time Dashboard │ │   │
│  │ │namespaces│ │  │ └─────────┘ │  │ └─────────┘ │  │ └─────────────────────┘ │   │
│  │ └─────────┘ │  └─────────────┘  └─────────────┘  └─────────────────────────┘   │
│  └─────────────┘                                                                   │
└───────────────────────────────────────────────────────────────────────────────────┘
```

## Core Components

### Gateway (edd-gateway)

The gateway is the single entry point for all external traffic. Unlike typical setups using Traefik or nginx-ingress, edd-cloud uses a custom Go gateway that provides:

- **TLS Termination**: Handles HTTPS with wildcard certificates from Let's Encrypt
- **Host-based Routing**: Routes requests to backend services based on hostname
- **SSH Multiplexing**: Proxies SSH connections to user containers on port 2222
- **Dynamic Routes**: Routes stored in PostgreSQL, configurable at runtime
- **Path Rewriting**: Optional prefix stripping for backend compatibility

Routes are configured via `routes.yaml` and loaded into the database on startup.

### Storage API (SFS Backend)

The Storage API provides file storage with namespace isolation:

- **Namespaces**: Logical groupings of files with visibility controls
  - `private` (0): Only owner can see/access
  - `visible` (1): Not listed, but accessible via direct URL
  - `public` (2): Listed and accessible to everyone
- **File Operations**: Upload, download, delete with progress tracking via SSE/WebSocket
- **Authentication**: JWT-based auth with sessions tracked in PostgreSQL
- **GFS Integration**: Files stored in the distributed file system

### Compute API (edd-compute)

The Compute API manages user containers:

- **Container Lifecycle**: Create, start, stop, delete containers
- **SSH Access**: Users can SSH into their containers via the gateway
- **Ingress Rules**: Custom subdomain routing to container ports
- **Resource Limits**: CPU and memory constraints per container

### GFS (Distributed File System)

A custom implementation inspired by Google File System:

| Property | Value |
|----------|-------|
| Chunk Size | 64 MB |
| Replication Factor | 3 |
| Consistency Model | Two-Phase Commit (2PC) |
| Write Quorum | 2 of 3 replicas |

**Components:**
- **Master**: Manages metadata, chunk allocation, and replication
- **Chunkservers**: Store actual file data on Raspberry Pi nodes
- **WAL**: Write-ahead log for crash recovery

**Garbage Collection**: Orphaned chunks (on disk but not in master metadata) are automatically cleaned up after a 1-hour grace period.

### Cluster Monitor

Provides real-time cluster observability:

- **Pod Metrics**: CPU, memory usage per pod via SSE
- **Node Health**: Raspberry Pi node status and resource usage
- **Log Streaming**: Aggregated logs from all services

## Request Flows

### File Upload Flow

```
User                Frontend            Storage API         GFS Master        Chunkserver
  │                    │                    │                   │                  │
  │─── Select file ───▶│                    │                   │                  │
  │                    │── POST /upload ───▶│                   │                  │
  │                    │                    │── CreateFile ────▶│                  │
  │                    │                    │◀── ChunkHandle ───│                  │
  │                    │                    │                   │                  │
  │                    │                    │───────── Write data (2PC) ─────────▶│
  │                    │                    │                   │                  │
  │◀─── SSE progress ──│◀── SSE progress ──│                   │                  │
  │                    │                    │                   │                  │
  │                    │◀─── 200 OK ────────│                   │                  │
  │◀─── Upload done ───│                    │                   │                  │
```

### Container SSH Flow

```
User                  Gateway              Compute API         Container
  │                      │                      │                  │
  │── ssh -p 2222 ──────▶│                      │                  │
  │   user@container     │                      │                  │
  │                      │── Lookup container ─▶│                  │
  │                      │◀── Container IP ─────│                  │
  │                      │                      │                  │
  │◀──────────────── SSH tunnel ──────────────────────────────────▶│
```

### Event-Driven User Deletion

```
Admin                Auth Service           NATS              SFS           Compute
  │                      │                   │                 │               │
  │── DELETE user ──────▶│                   │                 │               │
  │                      │── auth.user.X.deleted ─▶            │               │
  │                      │                   │                 │               │
  │                      │                   │── Event ───────▶│               │
  │                      │                   │                 │── Delete      │
  │                      │                   │                 │   namespaces  │
  │                      │                   │                 │               │
  │                      │                   │── Event ────────────────────────▶│
  │                      │                   │                 │               │
  │                      │                   │                 │   Delete      │
  │                      │                   │                 │   containers ◀│
  │◀─── 200 OK ─────────│                   │                 │               │
```

## Data Ownership

Each service owns its data and exposes it via APIs:

| Service | Database | Tables Owned |
|---------|----------|--------------|
| **SFS** | PostgreSQL | `users`, `sessions`, `namespaces` |
| **Compute** | PostgreSQL | `containers`, `ssh_keys`, `ingress_rules` |
| **Gateway** | PostgreSQL | `static_routes` |
| **GFS Master** | WAL + Memory | File metadata, chunk locations |

Services communicate about shared concepts (like users) via NATS events rather than shared database access.

## Hardware Topology

The cluster runs on Raspberry Pi nodes:

```
┌─────────────────────────────────────────────────────────────────┐
│                        Raspberry Pi Cluster                      │
│                                                                  │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐           │
│  │     rp1      │  │     rp2      │  │     rp3      │           │
│  │  (Master)    │  │   (Worker)   │  │   (Worker)   │           │
│  │              │  │              │  │              │           │
│  │ K3s Server   │  │ K3s Agent    │  │ K3s Agent    │           │
│  │ GFS Chunk    │  │ GFS Chunk    │  │ GFS Chunk    │           │
│  │ PostgreSQL   │  │              │  │              │           │
│  │ NATS         │  │              │  │              │           │
│  └──────────────┘  └──────────────┘  └──────────────┘           │
│                                                                  │
│  Node Selector Labels:                                           │
│  - size: mini (resource-constrained workloads)                   │
│  - size: large (resource-intensive workloads)                    │
└─────────────────────────────────────────────────────────────────┘
```

## Network Architecture

### External Domains

| Domain | Service | Purpose |
|--------|---------|---------|
| `cloud.eddisonso.com` | Frontend | Main dashboard and file browser |
| `storage.cloud.eddisonso.com` | SFS Backend | Storage API (alternative route) |
| `compute.cloud.eddisonso.com` | edd-compute | Container management API |
| `health.cloud.eddisonso.com` | cluster-monitor | Real-time metrics and logs |
| `docs.cloud.eddisonso.com` | Docusaurus | This documentation |
| `*.eddisonso.com` | Gateway | User container ingress |

### Internal Services

All services communicate via Kubernetes DNS (`<service>.default.svc.cluster.local`):

| Service | Ports | Protocol | Purpose |
|---------|-------|----------|---------|
| gateway | 8080, 8443, 2222 | HTTP, HTTPS, SSH | External entry point |
| simple-file-share-backend | 80 | HTTP | Storage API |
| simple-file-share-frontend | 80 | HTTP | React frontend |
| edd-compute | 80 | HTTP | Compute API |
| cluster-monitor | 80 | HTTP | Metrics and logs |
| log-service | 50051, 80 | gRPC, HTTP | Log aggregation |
| gfs-master | 9000 | gRPC | GFS coordination |
| gfs-chunkserver-{1,2,3} | 8080, 8081 | TCP, gRPC | Chunk storage |
| postgres | 5432 | PostgreSQL | Metadata storage |
| nats | 4222, 8222 | NATS, HTTP | Event messaging |

## Security Model

### Authentication Flow

```
┌────────┐         ┌─────────┐         ┌──────────┐
│ Client │         │ Storage │         │PostgreSQL│
│        │         │   API   │         │          │
└───┬────┘         └────┬────┘         └────┬─────┘
    │                   │                   │
    │── POST /login ───▶│                   │
    │   {user, pass}    │── Verify hash ───▶│
    │                   │◀── User record ───│
    │                   │                   │
    │◀── JWT token ─────│                   │
    │                   │                   │
    │── GET /files ────▶│                   │
    │   Authorization:  │                   │
    │   Bearer <jwt>    │── Validate JWT    │
    │                   │   (local, no DB)  │
    │◀── File list ─────│                   │
```

- **JWT Tokens**: Signed with HMAC-SHA256, contain user ID and username
- **Token Lifetime**: 24 hours (configurable)
- **Session Tracking**: Sessions stored in PostgreSQL for audit/revocation

### TLS Configuration

- **Certificates**: Wildcard certs from Let's Encrypt via cert-manager
- **DNS Challenge**: Cloudflare DNS-01 for wildcard validation
- **Minimum Version**: TLS 1.2

### Authorization

- **Namespace Visibility**: Private/Visible/Public controls per namespace
- **Ownership**: Namespaces and containers have owner_id linking to users
- **Admin Role**: Special admin user (configured via env var) can access all resources

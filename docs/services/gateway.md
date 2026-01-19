---
sidebar_position: 1
---

# Gateway

The Gateway service is the main entry point for all external traffic. It handles TLS termination, HTTP/HTTPS routing, and SSH tunneling.

## Features

- **TLS Termination**: Handles HTTPS with automatic certificate management
- **Dynamic Routing**: Routes based on host and path prefix from PostgreSQL
- **Route Caching**: LRU cache (1000 entries) for O(1) route lookups
- **SSH Tunneling**: Provides SSH access to containers via port 2222
- **WebSocket Support**: Proxies WebSocket connections for real-time features
- **HTTP→HTTPS Redirect**: Automatic upgrade for core services

## Architecture

```
Internet → Gateway (TLS) → Internal Services
              │
              ├── HTTPS (8443) → Route to backend services
              ├── HTTP (8080) → Redirect to HTTPS
              └── SSH (2222) → Container SSH tunnels
```

## Routing

Routes are stored in the `static_routes` PostgreSQL table:

```sql
CREATE TABLE static_routes (
    id SERIAL PRIMARY KEY,
    host TEXT NOT NULL,
    path_prefix TEXT NOT NULL,
    target TEXT NOT NULL,
    strip_prefix BOOLEAN DEFAULT false,
    priority INTEGER DEFAULT 0
);
```

### Route Matching

1. Routes are sorted by priority (descending)
2. Host must match exactly
3. Path must start with `path_prefix`
4. First matching route wins

### Route Cache

The gateway uses an LRU (Least Recently Used) cache to optimize route lookups:

- **Cache Size**: 1000 entries
- **Key**: `host + path`
- **Hit**: O(1) lookup, bypasses route matching
- **Miss**: Linear scan through routes, result cached
- **Invalidation**: Cache cleared on route table reload

This means repeated requests to the same endpoints are served with minimal overhead, regardless of the total number of routes.

### Example Routes

| Host | Path Prefix | Target |
|------|-------------|--------|
| `storage.cloud.eddisonso.com` | `/` | `simple-file-share-backend:80` |
| `compute.cloud.eddisonso.com` | `/` | `edd-compute:80` |
| `health.cloud.eddisonso.com` | `/` | `cluster-monitor:80` |

## SSH Tunneling

The gateway provides SSH access to containers:

```bash
ssh <container-id>@compute.cloud.eddisonso.com
```

SSH keys are managed through the Compute API and stored in PostgreSQL.

## Configuration

| Flag | Description | Default |
|------|-------------|---------|
| `-ssh-port` | SSH listen port | 2222 |
| `-http-port` | HTTP listen port | 8080 |
| `-https-port` | HTTPS listen port | 8443 |
| `-tls-cert` | TLS certificate path | - |
| `-tls-key` | TLS private key path | - |
| `-fallback` | Fallback IP for unmatched routes | - |

## Health Check

```
GET /health → 200 OK
```

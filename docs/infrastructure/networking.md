---
sidebar_position: 2
---

# Networking

## Domain Structure

```
eddisonso.com
├── cloud.eddisonso.com          # Main dashboard
├── storage.cloud.eddisonso.com  # Storage API
├── compute.cloud.eddisonso.com  # Compute API
├── health.cloud.eddisonso.com   # Health/Monitoring API
└── docs.cloud.eddisonso.com     # Documentation
```

## DNS Configuration

| Record | Type | Value |
|--------|------|-------|
| `*.eddisonso.com` | A | Gateway IP |
| `*.cloud.eddisonso.com` | A | Gateway IP |

## TLS Certificates

Managed by cert-manager with Let's Encrypt:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: eddisonso-wildcard
spec:
  secretName: eddisonso-wildcard-tls
  issuerRef:
    name: letsencrypt-cloudflare
    kind: ClusterIssuer
  dnsNames:
    - eddisonso.com
    - "*.eddisonso.com"
    - "*.cloud.eddisonso.com"
```

## Gateway Routing

The gateway routes requests based on host and path:

```
┌─────────────────────────────────────────────────────────────┐
│                         Gateway                              │
├─────────────────────────────────────────────────────────────┤
│  Host                          │  Path     │  Target        │
├────────────────────────────────┼───────────┼────────────────┤
│  cloud.eddisonso.com           │  /        │  frontend      │
│  cloud-api.eddisonso.com       │  /api     │  sfs-backend   │
│  storage.cloud.eddisonso.com   │  /        │  sfs-backend   │
│  compute.cloud.eddisonso.com   │  /        │  edd-compute   │
│  health.cloud.eddisonso.com    │  /        │  cluster-mon   │
│  docs.cloud.eddisonso.com      │  /        │  docs          │
└────────────────────────────────┴───────────┴────────────────┘
```

### Route Priority

Routes are matched by priority (highest first):

1. Exact path matches (e.g., `/sse/health`)
2. Prefix matches (e.g., `/compute`)
3. Root path (`/`)

## CORS Configuration

Each backend service implements CORS:

```go
func corsMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        origin := r.Header.Get("Origin")
        if origin != "" {
            w.Header().Set("Access-Control-Allow-Origin", origin)
            w.Header().Set("Access-Control-Allow-Credentials", "true")
            w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
            w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
        }
        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusOK)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

## Connection Pooling

To avoid browser connection limits (6 per domain in HTTP/1.1), services are split across subdomains:

| Domain | Connections Used |
|--------|------------------|
| `cloud.eddisonso.com` | Dashboard, Auth |
| `storage.cloud.eddisonso.com` | File operations, SSE progress |
| `compute.cloud.eddisonso.com` | Container operations, WebSocket |
| `health.cloud.eddisonso.com` | Metrics SSE, Logs SSE |

## Internal Network

Services communicate via Kubernetes DNS:

```
<service>.<namespace>.svc.cluster.local
```

Examples:
- `postgres.default.svc.cluster.local:5432`
- `gfs-master.default.svc.cluster.local:9000`
- `log-service.default.svc.cluster.local:50051`

## Load Balancing

The gateway service uses MetalLB for external IP allocation:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: gateway
spec:
  type: LoadBalancer
  ports:
    - name: http
      port: 80
      targetPort: 8080
    - name: https
      port: 443
      targetPort: 8443
    - name: ssh
      port: 2222
      targetPort: 2222
```

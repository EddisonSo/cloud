---
sidebar_position: 5
---

# Log Service

The Log Service provides centralized logging for all Edd Cloud services with real-time streaming capabilities.

## Features

- **Centralized Collection**: All services send logs via gRPC
- **Real-time Streaming**: SSE and WebSocket log streaming to the dashboard
- **Log Levels**: DEBUG, INFO, WARN, ERROR
- **Source Filtering**: Filter logs by service/pod name
- **GFS Persistence**: Async, best-effort log persistence to GFS

## Architecture

```mermaid
flowchart TB
    Auth[Auth Service] -->|gRPC :50051| LS[Log Service]
    Storage[Storage Service] -->|gRPC :50051| LS
    Compute[Compute Service] -->|gRPC :50051| LS
    Gateway[Gateway] -->|gRPC :50051| LS
    CM[Cluster Monitor] -->|gRPC :50051| LS
    GFSMaster[GFS Master] -->|gRPC :50051| LS
    GFSChunk[GFS Chunkserver] -->|gRPC :50051| LS

    LS --> RB["Ring Buffers<br/>(1000 entries per source×level)"]
    LS --> Sub[Subscribers<br/>SSE / WebSocket]
    LS --> PW[Persistence Worker]

    PW -->|"batch append<br/>(500 entries or 1 min)"| GFS[GFS Storage<br/>/YYYY-MM-DD/source.jsonl]
```

## Storage Model

Logs are stored in two layers:

### In-Memory Ring Buffers (primary)

Each unique source + level combination gets its own circular buffer holding up to **1000 entries**. These are the primary serving layer — subscribers receive recent entries from ring buffers on connect, then live updates via pub/sub broadcast.

- **Ephemeral**: Lost on pod restart
- **Per-replica**: Each log-service pod has independent buffers

### GFS Persistence (async, best-effort)

Log entries are queued to an internal channel (capacity: 1000). A background worker batches entries and appends them to GFS as JSONL files organized by date and source:

```
/core-logs/2026-02-08/gateway.jsonl
/core-logs/2026-02-08/cluster-monitor.jsonl
/core-logs/2026-02-08/auth-service.jsonl
```

| Setting | Value |
|---------|-------|
| Batch size | 500 entries |
| Flush interval | 1 minute |
| GFS namespace | `core-logs` |
| Queue capacity | 1000 entries |
| Overflow behavior | Silent drop |

If GFS is unavailable at startup, persistence is disabled and logs are kept in memory only.

### Limitations

- No log replay from GFS on startup — ring buffers start empty after a pod restart
- Each log-service replica has independent ring buffers (no cross-replica sharing)
- Persistence queue drops entries silently when full

## Client Library

Services use the `gfslog` package to send logs:

```go
import "eddisonso.com/go-gfs/pkg/gfslog"

logger := gfslog.NewLogger(gfslog.Config{
    Source:         "my-service",
    LogServiceAddr: "log-service:50051",
    MinLevel:       slog.LevelInfo, // Info is the recommended default; set LevelDebug only temporarily for deep debugging so per-operation debug logs are not shipped to the centralized stream
})
slog.SetDefault(logger.Logger)
defer logger.Close()

// Now use standard slog
slog.Info("Service started", "port", 8080)
slog.Error("Connection failed", "error", err)
```

## API Endpoints

### gRPC (Internal)

| Method | Description |
|--------|-------------|
| `PushLog(PushLogRequest)` | Send a log entry |
| `StreamLogs(StreamLogsRequest)` | Stream logs (server-side streaming) |
| `GetLogs(GetLogsRequest)` | Query recent log entries |

### HTTP/SSE and WebSocket (External)

| Endpoint | Auth | Description |
|----------|------|-------------|
| `GET /sse/logs` | JWT required (admin only) | Stream logs via SSE |
| `GET /sse/logs?source=<name>` | JWT required (admin only) | Filter by source name |
| `GET /sse/logs?level=<level>` | JWT required (admin only) | Filter by minimum level |
| `WS /ws/logs` | JWT required (admin only) | Stream logs via WebSocket |
| `GET /logs/download?date=YYYY-MM-DD` | JWT required (admin only) | Download a day's logs (all sources, merged) as a .zip containing .log + .jsonl |

The download endpoint returns a `.zip` archive containing two files: a human-readable `edd-cloud-logs-<date>.log` and a raw `edd-cloud-logs-<date>.jsonl`, both containing entries from all sources merged and sorted chronologically for the requested UTC date. The token may be passed via the `Authorization` header or the `?token=` query parameter (same as `/ws/logs`). Returns `400` for a malformed date and `404` if no logs exist for that date.

:::warning Phase 1 — Admin-only log access
Log streaming currently requires a valid JWT with admin privileges. Because log entries do not yet carry per-user or per-namespace ownership data, non-admin users cannot be scoped to their own container logs at this time.

Per-user log streaming is planned for Phase 2, which will add a `namespace` field to log entries so that non-admin users can stream logs from their own `compute-{userID}-*` containers. This requires coordinated changes across all log-producing services.
:::

## Log Entry Format

```json
{
  "timestamp": 1707350096,
  "level": 1,
  "source": "edd-storage",
  "message": "Request processed",
  "attributes": {
    "method": "GET",
    "path": "/storage/files",
    "duration": "15ms"
  }
}
```

## Log Levels

| Level | Value | Description |
|-------|-------|-------------|
| DEBUG | 0 | Detailed debugging information |
| INFO | 1 | General operational messages |
| WARN | 2 | Warning conditions |
| ERROR | 3 | Error conditions |

## Frontend Integration

The dashboard streams logs via SSE:

```javascript
const params = new URLSearchParams();
params.set('source', 'edd-storage');
params.set('level', 'INFO');

const eventSource = new EventSource(`/sse/logs?${params}`);

eventSource.onmessage = (event) => {
  const entry = JSON.parse(event.data);
  console.log(`[${entry.source}] ${entry.message}`);
};
```

## Configuration

| Flag | Description | Default |
|------|-------------|---------|
| `-grpc` | gRPC listen address | `:50051` |
| `-http` | HTTP listen address | `:8080` |
| `-master` | GFS master address | `gfs-master:9000` |

# Alerting Service Design

## Overview

Extract alerting from cluster-monitor into a standalone single-replica service. Cluster-monitor returns to being a pure metrics producer. The alerting service consumes metrics and logs via NATS (protobuf-serialized), evaluates rules, and delivers alerts to Discord.

## Architecture

```
cluster-monitor  ──publish──→  NATS (cluster.>)  ──subscribe──→  alerting-service  →  Discord webhook
log-service      ──publish──→  NATS (log.error.>)  ──subscribe──→       ↑
```

Services are fully decoupled — cluster-monitor and log-service publish events without knowing about alerting-service. Alerting-service subscribes to NATS subjects and evaluates rules.

## Data Sources

1. **Cluster metrics** (`cluster.metrics`) — node CPU, memory, disk percentages, node conditions, published by cluster-monitor every 5s
2. **Pod status** (`cluster.pods`) — pod restart counts, OOMKilled termination reasons, published by cluster-monitor every 5s
3. **Error logs** (`log.error.>`) — error-level log entries, published by log-service on ingestion

## NATS Subjects & Streams

| Stream | Subjects | Publisher | Consumer |
|--------|----------|-----------|----------|
| `CLUSTER` | `cluster.>` | cluster-monitor | alerting-service |
| `LOGS` | `log.error.>` | log-service | alerting-service |

Subject naming:
- `cluster.metrics` — full cluster snapshot (all nodes)
- `cluster.pods` — pod status snapshot (restarts, OOM)
- `log.error.{source}` — error log entries per source

## Protobuf Messages

New proto file: `proto/cluster/events.proto`

```protobuf
message ClusterMetrics {
  common.EventMetadata metadata = 1;
  repeated NodeMetrics nodes = 2;
}

message NodeMetrics {
  string name = 1;
  double cpu_percent = 2;
  double memory_percent = 3;
  double disk_percent = 4;
  repeated NodeCondition conditions = 5;
}

message NodeCondition {
  string type = 1;
  string status = 2;
}

message PodStatusSnapshot {
  common.EventMetadata metadata = 1;
  repeated PodStatus pods = 2;
}

message PodStatus {
  string name = 1;
  string namespace = 2;
  int32 restart_count = 3;
  bool oom_killed = 4;
}
```

New proto file: `proto/log/events.proto`

```protobuf
message LogError {
  common.EventMetadata metadata = 1;
  string source = 2;
  string message = 3;
  string level = 4;
}
```

## What Moves

- `cluster-monitor/internal/alerting/` package → `alerting-service/internal/alerting/`
- Remove evaluator wiring from cluster-monitor `main.go`
- Remove `-discord-webhook`, `-alert-cooldown`, `-log-service-http` flags from cluster-monitor
- Revert cluster-monitor manifest (remove Discord webhook secret mount and log-service-http arg)

## What Changes in Existing Services

### cluster-monitor
- Add NATS publisher: publish `ClusterMetrics` to `cluster.metrics` after each metrics fetch
- Add NATS publisher: publish `PodStatusSnapshot` to `cluster.pods` after each pod metrics fetch
- Remove all alerting code

### log-service
- Add NATS publisher: publish `LogError` to `log.error.{source}` when ERROR+ level logs are ingested

## New Components (alerting-service)

- **NATS subscriber** — subscribes to `cluster.metrics`, `cluster.pods`, `log.error.>` using JetStream durable consumers
- **main.go** — wires NATS subscribers, evaluator, and Discord sender
- No SSE client, no WebSocket client, no K8s API polling needed

## Deployment

- Single replica on backend nodes
- No ServiceAccount needed (no K8s API access — pod status comes via NATS)
- Discord webhook URL from existing `discord-webhook-url` secret
- NATS URL from standard `nats://nats:4222`
- Healthcheck via `/healthz`

## Flags

- `-nats` — NATS server URL (default: `nats://nats:4222`)
- `-discord-webhook` — Discord webhook URL
- `-alert-cooldown` — default cooldown duration (default: 5m)
- `-log-source` — log source name for structured logging
- `-log-service-grpc` — log-service gRPC address for structured logging

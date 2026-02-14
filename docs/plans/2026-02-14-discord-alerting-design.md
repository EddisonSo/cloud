# Discord Alerting Design

## Overview

Add Discord webhook alerting to cluster-monitor for infrastructure health alerts and log error detection. No new services — all logic lives inside the existing cluster-monitor binary.

## Architecture

Two alert sources feed into a shared Discord webhook sender:

```
cluster-monitor metrics loop (every 5s)
  → threshold evaluator → cooldown check → Discord webhook POST

log-service /ws/logs?level=ERROR subscription
  → error burst detector → cooldown check → Discord webhook POST
```

## Alert Rules

### Cluster Health

| Alert | Condition | Cooldown |
|-------|-----------|----------|
| Node high CPU | > 90% for 2 consecutive checks | 5 min |
| Node high memory | > 85% | 5 min |
| Node high disk | > 90% | 15 min |
| Node condition | MemoryPressure, DiskPressure, PIDPressure | 5 min |
| Pod crash loop | RestartCount increases | 5 min per pod |
| Pod OOMKilled | OOMKilled termination reason | 5 min per pod |

### Log Errors

| Alert | Condition | Cooldown |
|-------|-----------|----------|
| Error burst | > 5 ERROR logs from same source within 30s | 5 min per source |
| Critical error | Single ERROR log with keywords (panic, fatal, crash) | 5 min per source |

## Discord Integration

- Webhook URL stored as Kubernetes Secret (`discord-webhook-url`)
- Mounted into cluster-monitor pod, passed via `-discord-webhook` flag
- Rich embeds with color coding: red (critical), orange (warning)

## Cooldown Mechanism

- In-memory map: `alertKey → lastFiredAt`
- Alert key format: `"{type}:{subject}"` (e.g., `"cpu:s0"`, `"error-burst:edd-gateway"`)
- Skip firing if `time.Since(lastFired) < cooldownDuration`
- Configurable default cooldown via `-alert-cooldown` flag

## What Changes

- **cluster-monitor**: Add `internal/alerting/` package with:
  - `evaluator.go` — threshold checks against metrics
  - `discord.go` — webhook sender with embed formatting
  - `cooldown.go` — cooldown tracker
  - `logsub.go` — WebSocket subscriber to log-service for error detection
- **manifests/cluster-monitor.yaml**: Mount Discord webhook secret, add log-service address flag
- **No changes** to log-service or notification-service

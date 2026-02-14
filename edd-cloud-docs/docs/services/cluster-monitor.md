---
sidebar_position: 6
---

# Cluster Monitor

The Cluster Monitor service provides real-time metrics and health information for the Kubernetes cluster.

## Features

- **Node Metrics**: CPU, memory, disk usage per node
- **Pod Metrics**: Resource usage per pod
- **Health Status**: Node conditions and pressure indicators
- **Real-time Streaming**: SSE-based metrics updates
- **Automated Alerting**: Discord webhook notifications for cluster health issues

## Architecture

```mermaid
flowchart TB
    subgraph K8s[Kubernetes API]
        Nodes[Nodes API]
        Metrics[Metrics API]
        Kubelet[Kubelet API]
    end

    Nodes --> CM[Cluster Monitor]
    Metrics --> CM
    Kubelet --> CM

    CM --> Cache[Cache]
    CM --> Workers[Workers]
    CM --> SSE[SSE]

    SSE --> Clients[Browser Clients]
```

## API Endpoints

### REST

| Endpoint | Description |
|----------|-------------|
| `GET /cluster-info` | Current node metrics (JSON) |
| `GET /pod-metrics` | Current pod metrics (JSON) |
| `GET /healthz` | Health check |

### SSE (Real-time)

| Endpoint | Description |
|----------|-------------|
| `GET /sse/health` | Combined cluster + pod metrics stream |
| `GET /sse/cluster-info` | Node metrics stream |
| `GET /sse/pod-metrics` | Pod metrics stream |

### WebSocket (Legacy)

| Endpoint | Description |
|----------|-------------|
| `WS /ws/cluster-info` | Node metrics WebSocket |
| `WS /ws/pod-metrics` | Pod metrics WebSocket |

## Metrics

### Node Metrics

```json
{
  "timestamp": "2024-01-19T12:34:56Z",
  "nodes": [
    {
      "name": "s0",
      "cpu_usage": "500m",
      "cpu_capacity": "4",
      "cpu_percent": 12.5,
      "memory_usage": "2Gi",
      "memory_capacity": "8Gi",
      "memory_percent": 25.0,
      "disk_usage": 10737418240,
      "disk_capacity": 107374182400,
      "disk_percent": 10.0,
      "conditions": [
        {"type": "MemoryPressure", "status": "False"},
        {"type": "DiskPressure", "status": "False"}
      ]
    }
  ]
}
```

### Pod Metrics

```json
{
  "timestamp": "2024-01-19T12:34:56Z",
  "pods": [
    {
      "name": "gateway-abc123",
      "namespace": "default",
      "node": "s0",
      "cpu_usage": 50000000,
      "cpu_capacity": 4000000000,
      "memory_usage": 67108864,
      "memory_capacity": 8589934592,
      "disk_usage": 1048576,
      "disk_capacity": 107374182400
    }
  ]
}
```

## Combined Health Stream

The `/sse/health` endpoint combines both metrics types to reduce connections:

```javascript
const eventSource = new EventSource('/sse/health');

eventSource.onmessage = (event) => {
  const { type, payload } = JSON.parse(event.data);

  if (type === 'cluster') {
    // Node metrics
    updateNodes(payload.nodes);
  } else if (type === 'pods') {
    // Pod metrics
    updatePods(payload.pods);
  }
};
```

## Refresh Interval

Metrics are fetched from the Kubernetes API every 5 seconds (configurable).

## Alerting

Cluster Monitor includes automated alerting via Discord webhooks for the following conditions:

### Alert Rules

| Alert Type | Trigger Condition | Severity | Cooldown |
|-----------|-------------------|----------|----------|
| High CPU | Node CPU > 90% for 2 consecutive checks | Critical | 5 minutes |
| High Memory | Node memory > 85% | Warning | 5 minutes |
| High Disk | Node disk > 90% | Warning | 15 minutes |
| Node Condition | Node has MemoryPressure/DiskPressure | Critical | 5 minutes |
| OOMKilled | Container terminated with OOMKilled | Critical | 5 minutes |
| Pod Restart | Pod restart count increased | Warning | 5 minutes |
| Critical Log | Logs contain "panic", "fatal", "crash" | Critical | 5 minutes |
| Error Burst | 5+ errors in 30s window | Warning | 5 minutes |

### Discord Webhook Setup

Alerts are sent to Discord via webhook. Configure the webhook URL as a Kubernetes secret:

```bash
kubectl create secret generic discord-webhook-url \
  --from-literal=WEBHOOK_URL='https://discord.com/api/webhooks/...'
```

The webhook URL is mounted via `secretKeyRef` in the cluster-monitor deployment.

### Log-based Alerts

Cluster Monitor subscribes to the log-service WebSocket (`/ws/logs?level=ERROR`) to detect critical errors and error bursts in real-time across all services.

## Configuration

| Flag | Description | Default |
|------|-------------|---------|
| `-addr` | Listen address | `:8080` |
| `-refresh` | Metrics refresh interval | `5s` |
| `-log-service` | Log service address | - |
| `-log-service-http` | Log service HTTP address for WebSocket subscription | - |
| `-discord-webhook` | Discord webhook URL for alerts | - |
| `-alert-cooldown` | Default alert cooldown duration | `5m` |

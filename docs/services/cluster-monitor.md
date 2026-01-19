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

## Architecture

```
┌──────────────────────────────────────────────────────┐
│                  Kubernetes API                       │
│  ┌─────────────┐  ┌─────────────┐  ┌──────────────┐  │
│  │ Nodes API   │  │ Metrics API │  │ Kubelet API  │  │
│  └──────┬──────┘  └──────┬──────┘  └──────┬───────┘  │
└─────────┼────────────────┼────────────────┼──────────┘
          │                │                │
          └────────────────┼────────────────┘
                           │
                    ┌──────▼──────┐
                    │   Cluster   │
                    │   Monitor   │
                    │             │
                    │  - Cache    │
                    │  - Workers  │
                    │  - SSE      │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │   Clients   │
                    │  (Browser)  │
                    └─────────────┘
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

## Configuration

| Flag | Description | Default |
|------|-------------|---------|
| `-addr` | Listen address | `:8080` |
| `-refresh` | Metrics refresh interval | `5s` |
| `-log-service` | Log service address | - |

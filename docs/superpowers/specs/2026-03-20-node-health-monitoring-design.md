# Node Health Monitoring: NotReady Detection & Stale Cache Handling

**Date:** 2026-03-20
**Status:** Approved

## Problem

Two gaps in node health monitoring:

1. **Stale health page**: When a node goes down, `cluster-monitor` fails to fetch metrics from the K8s API, returns early, and leaves the cache with the last known good state. The frontend keeps showing the downed node as healthy.

2. **Missing Discord alerts**: `cluster-monitor` only tracks `MemoryPressure`, `DiskPressure`, and `PIDPressure` conditions. It ignores the `Ready` condition, so when K8s marks a node as `NotReady` (the standard signal for a downed/unreachable node), no alert fires.

## Design

### 1. Add `NotReady` condition tracking (cluster-monitor)

Add `corev1.NodeReady` to the conditions filter in `fetchClusterInfo()` (main.go, line 587). Unlike pressure conditions (where `True` = bad), `Ready: False` means unhealthy. To keep the conditions array as "list of active problems," only append when the node is not ready:

```go
case corev1.NodeReady:
    if cond.Status != corev1.ConditionTrue {
        conditions = append(conditions, NodeCondition{
            Type:   "NotReady",
            Status: "True", // normalized: "the NotReady problem is active"
        })
    }
```

The `Status` is normalized to `"True"` rather than using the raw K8s value (`"False"` or `"Unknown"`) because:
- The alerting service filters conditions by `c.Status == "True"` (main.go line 188) — a `"False"` status would be silently dropped and no Discord alert would fire.
- The frontend's `isHealthy` check (`conditions.every(c => c.status === "False")`) would incorrectly treat `NotReady: False` as healthy.
- Since the type is already renamed to `NotReady`, `True` correctly reads as "this problem is active."

### 2. Stale cache handling (cluster-monitor)

Add a `Stale` field to `ClusterInfo`:

```go
type ClusterInfo struct {
    Timestamp time.Time     `json:"timestamp"`
    Nodes     []NodeMetrics `json:"nodes"`
    Stale     bool          `json:"stale,omitempty"`
}
```

There are three early-return error paths in `fetchClusterInfo` that all need stale-marking:

1. Line 495-498: Failed to get node metrics (metrics-server)
2. Line 501-504: Failed to list nodes (K8s API)
3. Line 507-510: Failed to parse metrics JSON

Use a helper to avoid duplication:

```go
func markCacheStale(cache *MetricsCache) {
    existing := cache.GetClusterInfo()
    if existing == nil {
        existing = &ClusterInfo{}
    }
    existing.Stale = true
    cache.SetClusterInfo(existing)
}
```

Each error path calls `markCacheStale(cache)` before returning. On successful fetch, `Stale` is implicitly `false` (new struct, zero value).

The nil check prevents a panic on the first fetch failure before any successful fetch has populated the cache. In practice `NewMetricsCache()` initializes with an empty `ClusterInfo`, but the guard is defensive.

### 3. Stale data warning (frontend)

When `ClusterInfo.stale` is `true`, show a warning banner above the nodes table on the health page:

> ⚠ Health data may be stale — unable to reach cluster API

Dismisses automatically when fresh data arrives (next successful SSE update where `stale` is false). No user interaction needed.

### 4. Alerting: No changes needed

The alerting service evaluator already handles conditions generically. The condition filter in `main.go` (line 188) passes conditions with `Status == "True"` to the evaluator. With the normalized `NotReady: True` status, this works correctly. A `NotReady` condition will fire:

- Title: `Node Condition: {nodeName}`
- Message: `Node {nodeName} has condition: NotReady`
- Severity: Critical
- Cooldown key: `condition:{nodeName}:NotReady` (default 5-minute cooldown)

## Files Changed

| File | Change |
|------|--------|
| `cluster-monitor/main.go` | Add `NodeReady` condition tracking, stale cache handling |
| `edd-cloud-interface/frontend/src/pages/HealthPage.tsx` | Stale data warning banner |

## Future Considerations

- **Node disappearance detection**: If a node is removed from the K8s node list entirely (not just NotReady), no alert fires. This is rare — K8s almost always marks nodes NotReady first — but could be added later by comparing previous vs current node lists.
- **Connectivity probes**: ICMP/TCP probes from cluster-monitor to each node would distinguish "node is down" from "kubelet crashed." Deferred — NotReady provides sufficient signal for now.

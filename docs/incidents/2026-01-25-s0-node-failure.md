---
sidebar_position: 1
title: "2026-01-25: s0 Node Failure"
---

# Incident Report: s0 Node Failure

**Date:** January 25, 2026
**Duration:** ~2 hours
**Severity:** Critical
**Status:** Resolved

## Summary

Node s0 (192.168.3.100) became unresponsive, causing a complete site outage. s0 hosted critical services including the PostgreSQL primary database, gateway, and GFS master. The outage required manual intervention to promote the database replica and reschedule services to other nodes.

## Timeline

*All times in EST (UTC-5)*

### Day 1 - January 23, 2026

| Time (EST) | Event |
|------------|-------|
| 21:07 | Last recorded SSH activity on s1 (K3s control plane) |
| 23:03 | s0 kubelet's last heartbeat to K3s API |
| 23:08 | s0 marked as NotReady; pods begin terminating |
| 23:08 | Gateway pod on s0 starts terminating; new pod stuck Pending |
| 23:08 | PostgreSQL primary on s0 becomes unavailable |
| 23:08 | gfs-master on s0 becomes unavailable |
| 23:08 | **Site goes down** - gateway cannot route traffic |

### Day 2 - January 24, 2026

| Time (EST) | Event |
|------------|-------|
| 12:12 | Issue discovered while debugging unrelated auth problem |
| 12:12 | `kubectl get nodes` shows s0 as NotReady |
| 12:15 | s1 (K3s control plane) also found unresponsive |
| 12:16 | s1 restarted manually |
| 12:16 | Cluster access restored via s1 |
| 12:17 | Investigation into s0 begins |
| 16:40 | GitHub Actions workflow triggered to deploy fixes |
| 16:43 | Deploy step fails - cannot reach K3s API (s0 down again) |
| 17:19 | Manual workflow re-trigger attempted |
| 17:20 | s0 confirmed down again (second failure) |
| 17:22 | Gateway found in CrashLoopBackOff (read-only DB) |
| 17:22 | Postgres primary pod Pending (nodeSelector: s0) |
| 17:23 | Decision made to failover database to rp1 |
| 17:25 | `core-services=true` label added to rp2 |
| 17:28 | PostgreSQL replica on rp1 promoted via `SELECT pg_promote()` |
| 17:29 | `postgres` service patched to point to postgres-replica |
| 17:29 | Gateway pod deleted and rescheduled |
| 17:30 | `role=backend` label added to rp3 |
| 17:30 | gfs-master rescheduled to rp3 |
| 17:31 | Gateway comes up successfully |
| 17:32 | **Site restored** |
| 17:33 | Frontend and backend deployments updated |
| 17:35 | Manifests updated to reflect new architecture |
| 17:40 | Read replicas created on rp2, rp3, rp4 |

### Resolution Time

- **Time to detection:** ~13 hours (overnight, no alerting)
- **Time to resolution:** ~25 minutes (from decision to failover)
- **Total outage duration:** ~18 hours

## Impact

- **Complete site outage** for approximately 15+ hours (undetected overnight)
- Users unable to access cloud.eddisonso.com
- No data loss (replica was in sync with primary)

## Root Cause

The exact cause of s0's failure is unknown. The node became completely unresponsive with no logs captured before the failure. Possible causes:
- Hardware failure (power supply, memory, disk)
- Kernel hang/panic
- Power outage

The node had previously been up for ~3 days before failing.

## What Went Wrong

### 1. Single Point of Failure
Critical services were concentrated on s0 with no automatic failover:
- PostgreSQL primary (only writable database)
- Gateway (only ingress point)
- GFS master (chunk metadata)

### 2. GFS Chunk Metadata Lost
The GFS master stored all chunk metadata (file-to-chunk mappings) in memory/local storage on s0. When s0 went down:
- All file metadata was lost
- Chunk data still exists on chunkservers (s1, s2, s3) but is now orphaned
- Files cannot be reconstructed without metadata
- **This is a critical data loss scenario**

### 3. No Alerting
There was no monitoring or alerting configured to detect:
- Node failures
- Pod scheduling failures
- Service unavailability

The outage went unnoticed for ~15 hours overnight.

### 4. Rigid Node Selectors
Services used strict `nodeSelector` constraints that prevented automatic rescheduling:
- `db-role: primary` - only s0
- `core-services: true` - only s0
- `role: backend` - only s0

### 5. GitHub Actions Deployment Failures Hidden
The CI/CD pipeline's deploy step used `|| true` which silently ignored connection failures to the Kubernetes API, masking deployment issues.

## What Went Well

### 1. Database Replica Available
The PostgreSQL streaming replica on rp1 was fully synchronized with the primary. This enabled:
- Zero data loss
- Quick promotion to primary (`SELECT pg_promote()`)

### 2. Manual Failover Was Straightforward
Once the decision was made to failover:
- Promoting the replica took seconds
- Updating the service selector was a single kubectl patch
- Gateway recovered immediately after DB became writable

### 3. Documentation Existed
The CLAUDE.md file documented the node layout and architecture, which helped in understanding the cluster topology during the incident.

## Action Items

### Immediate (Completed)
- [x] Promote rp1 PostgreSQL replica to primary
- [x] Add `core-services=true` label to rp2
- [x] Add `role=backend` label to rp3
- [x] Update manifests to reflect new architecture
- [x] Reschedule gateway and gfs-master

### Short-term
- [x] Create read replicas on rp2, rp3, rp4 for redundancy
- [x] Remove `|| true` from CI/CD deploy steps or add proper error handling

### Long-term
- [ ] Implement automatic database failover (Patroni or similar)
- [ ] Distribute critical services across multiple nodes
- [ ] Add pod anti-affinity rules to spread replicas
- [ ] Implement health checks for external monitoring
- [ ] Consider running multiple gateway replicas
- [ ] **Implement GFS master WAL replication for metadata redundancy**
- [ ] Consider storing GFS metadata in distributed key-value store (etcd/FoundationDB)

## Architecture Changes

### Before (s0-dependent)
```
s0 (down)
├── postgres (primary) - SPOF
├── gateway - SPOF
└── gfs-master - SPOF

rp1
└── postgres-replica (read-only)
```

### After (distributed)
```
rp1
└── postgres-replica (promoted to primary)

rp2
├── gateway
└── postgres-replica-2 (planned)

rp3
├── gfs-master
└── postgres-replica-3 (planned)

rp4
└── postgres-replica-4 (planned)
```

## Lessons Learned

1. **Never rely on a single node for critical services** - Even with replicas, the inability to automatically failover created extended downtime.

2. **Alerting is not optional** - A 15-hour outage went unnoticed. Basic uptime monitoring would have detected this in minutes.

3. **Test failover procedures** - The manual failover was successful but had never been tested. Regular DR drills should be scheduled.

4. **CI/CD should fail loudly** - Silent failures in deployment pipelines mask real issues and create false confidence.

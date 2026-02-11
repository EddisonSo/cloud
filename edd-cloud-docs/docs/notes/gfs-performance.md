---
sidebar_position: 1
---

# GFS Performance Characteristics

Performance findings from load testing file retrieval through the storage API (`storage.cloud.eddisonso.com`).

## Test Setup

- **Tool**: Custom Go perftest framework (`~/perftest`)
- **Endpoint**: `GET /storage/public/cat.jpg` (155KB file)
- **Duration**: 60 seconds per test
- **Network capacity**: 200 Mb/s

## Results

### Throughput Scaling

**Before fixes** (gateway 500m CPU, HAProxy 100m CPU, PostgreSQL 500m CPU):

| Concurrency | Ops/sec | Throughput | Notes |
|-------------|---------|------------|-------|
| 10 (keep-alive) | 44 | ~54 Mb/s | Single clean run |
| 50 (keep-alive) | 36 | ~44 Mb/s | Gateway CPU-bound at 500m limit |
| 50 (keep-alive, 2000m gateway) | 43 | ~53 Mb/s | After raising gateway CPU limit |
| 10 (no keep-alive) | 41 | ~50 Mb/s | New TLS connection per request |
| 1000 (keep-alive) | 121 | ~147 Mb/s | Approaching network cap |
| 1000 (no keep-alive) | 108 | ~133 Mb/s | ~9% overhead from TLS handshakes |

**After fixes** (gateway 2000m, HAProxy 500m, PostgreSQL 1000m):

| Concurrency | Ops/sec | Throughput | Notes |
|-------------|---------|------------|-------|
| 5 (no keep-alive) | 96 | ~118 Mb/s | +140% vs before |
| 10 (no keep-alive) | 91 | ~112 Mb/s | +117% vs before |
| 25 (no keep-alive) | 63 | ~77 Mb/s | Bandwidth/TLS limited |
| 50 (no keep-alive) | 61 | ~75 Mb/s | +69% vs before |

### Per-Request Latency Breakdown

From a single external request with `curl`:

| Phase | Time | % of Total |
|-------|------|------------|
| DNS lookup | 12ms | 14% |
| TCP handshake | 3ms | 3% |
| TLS handshake | 17ms | 20% |
| Server processing (TTFB) | 50ms | 57% |
| Data transfer (155KB) | 5ms | 6% |
| **Total** | **87ms** | |

Only **6%** of the request time is spent transferring data. The rest is overhead.

### Internal GFS Read Path Breakdown

Measured by running benchmarks directly inside the SFS pod (eliminating all external network overhead):

| Operation | p50 | Description |
|-----------|-----|-------------|
| `GetFile` gRPC | 0.67ms | Master metadata lookup |
| `GetChunkLocations` gRPC | 0.71ms | Master chunk location lookup |
| Chunkserver TCP read (155KB) | ~5.5ms | Connect + transfer from chunkserver |
| **Full GFS read** (all combined) | **6.4ms** | Two gRPC calls + chunkserver read |
| **SFS handler simulation** | **6.9ms** | GetFile + Full Read (what the HTTP handler does) |
| SFS HTTP overhead (no GFS) | 3.7ms | Handler routing, response writing |
| **Total in-cluster HTTP request** | **~11ms** | SFS HTTP + GFS read |

The GFS master is fast (~8000 ops/sec for metadata lookups). The chunkserver TCP read dominates the internal path.

**Internal capacity: 550 ops/sec** (10 concurrent) = 83 MB/s through GFS — far exceeding the 200 Mb/s external link.

### External Overhead Stack

For a single external request (~46ms):

```
GFS read (master + chunkserver):    ~7ms
SFS HTTP handler:                   ~4ms
Gateway TCP proxy:                  ~1ms
TLS handshake:                     ~17ms
TCP + DNS:                         ~17ms
Total:                             ~46ms
```

## Request Path

Every file read traverses four network hops:

```
client -> gateway (TLS termination + reverse proxy)
  -> SFS backend (HTTP handler)
    -> GFS master (gRPC metadata lookup)
    -> GFS chunkserver (TCP data read)
  <- back through all layers
```

## Latency Under Concurrency

Latency distribution measured with per-request timing (15s runs, no keep-alive). **Before CPU limit fixes:**

| Workers | Ops/sec | p50 | p90 | p99 | max |
|---------|---------|-----|-----|-----|-----|
| 1 | 22 | 46ms | 52ms | 70ms | 95ms |
| 2 | 47 | 42ms | 50ms | 60ms | 90ms |
| 3 | 72 | 40ms | 50ms | 98ms | 262ms |
| 5 | 99 | 39ms | 90ms | 204ms | 26s |
| 10 | 40 | 109ms | 612ms | 977ms | 1.3s |

Throughput scaled linearly from 1-5 workers (22→99 ops/sec) then collapsed at 10. Root cause was CFS CPU throttling on HAProxy and PostgreSQL (see below).

### Control: Health Endpoint (no GFS, 19 bytes)

| Workers | Ops/sec | p50 | p99 |
|---------|---------|-----|-----|
| 1 | 33 | 30ms | 46ms |
| 5 | 231 | 21ms | 37ms |
| 10 | 333 | 30ms | 50ms |

Linear scaling, no tail latency. All internal components (gateway, TLS, backend services) have ample headroom.

## Primary Bottleneck: CFS CPU Throttling (resolved)

The throughput cliff at 5-6 workers was caused by **Linux CFS CPU throttling** on the database path. Every file download queries PostgreSQL for namespace visibility (`canAccessNamespace`), and both HAProxy and PostgreSQL had tight CPU limits.

### Root Cause: CPU Cgroup Throttling

When a pod exceeds its CPU limit, the CFS scheduler pauses all its processes for the remainder of the 100ms period. This caused ~90ms latency spikes on every throttled query.

| Component | Old CPU Limit | Throttle Events | Effect |
|-----------|---------------|-----------------|--------|
| HAProxy | **100m** | 8,456 | 53.6% of queries >10ms at c=10 |
| PostgreSQL | **500m** | 3,784 | 27.1% of queries >10ms at c=10 |

### Evidence

**DB query benchmarks (inside SFS pod, namespace lookup query):**

| Target | c=1 ops/sec | c=10 ops/sec | Spike rate at c=10 |
|--------|-------------|--------------|-------------------|
| Via HAProxy (100m CPU) | 165 | 168 (flat!) | 53.6% |
| Direct to PostgreSQL (500m CPU) | 746 | 381 | 27.1% |

HAProxy at 100m was the primary bottleneck — only 10% of a CPU core for proxying all database traffic.

### Fix Applied

| Component | Before | After |
|-----------|--------|-------|
| Gateway | 500m | **2000m** |
| HAProxy | 100m | **500m** |
| PostgreSQL | 500m | **1000m** |

**After fix — DB query throughput through HAProxy:**

| Workers | Before (ops/sec) | After (ops/sec) | Improvement |
|---------|------------------|-----------------|-------------|
| c=1 | 165 | 631 | 3.8x |
| c=3 | 134 | 892 | 6.7x |
| c=5 | 152 | 828 | 5.4x |
| c=10 | 168 | 874 | 5.2x |

Spike rate at c=10 dropped from 53.6% to 13.6%.

### Layer-by-Layer Isolation

| Test | Ops/sec (10 workers) | p50 | Conclusion |
|------|---------------------|-----|------------|
| Full stack (gateway + TLS) | 91 | — | After fix |
| Direct to SFS (in-cluster, no TLS) | 43 | 108ms | Before fix; gateway/TLS add less than 5% overhead |
| Health endpoint (no GFS) | 333 | 30ms | GFS path adds ~7ms per request internally |
| GFS read (inside SFS pod, 10 concurrent) | 550 | 17ms | Internal GFS capacity far exceeds external link |

## Remaining Bottlenecks

### 1. External Bandwidth (200 Mb/s)

At peak throughput (96 ops/sec × 155KB = ~118 Mb/s), the external link is ~59% utilized. TLS handshake overhead (~17ms per connection with no keep-alive) limits per-worker bandwidth utilization.

### 2. ~~Duplicate GFS Master Calls~~ (RESOLVED)

**Before:** Each file read made **two** gRPC calls to the GFS master:
1. `GetFile()` — to get file size for `Content-Length` header
2. `GetChunkLocations()` — to find which chunkserver holds the data

**After:** Both calls eliminated by:
- Using `getCachedChunks()` instead of `GetChunkLocations()` in `ReadToWithNamespace()`
- Adding `FileSizeWithNamespace()` that computes size from cached chunk locations

Now each download makes zero fresh master calls if chunks are cached (5-minute TTL).

### 4. Gateway Forces Connection: close

The gateway injects `Connection: close` on every proxied request and opens a new TCP connection to the backend per request. This prevents HTTP keep-alive between gateway and backend.

### 5. No Caching Layer

There is no caching at the SFS level. Every request reads from GFS end-to-end, even for the same file requested thousands of times.

## GFS as Object Storage

GFS was designed for **large sequential reads/writes** (following Google's GFS paper). This makes it a poor fit for small-file object storage workloads:

| Characteristic | GFS Design | Object Storage Need |
|---------------|-----------|-------------------|
| File size | Large (GB+) | Any (KB to GB) |
| Access pattern | Sequential append/read | Random read/write |
| Metadata cost | gRPC round-trip per read | Colocated/cached |
| Chunk granularity | 64MB | Packed small objects |
| Write model | Append-only | Overwrite support |

### Where GFS Excels

GFS is well-suited for workloads like **log aggregation** (used by `log-service`):

- Append-only writes (logs are write-once)
- Large sequential writes (continuous log streams)
- Large sequential reads (scanning log ranges)
- Low metadata pressure (appending to few open files, chunk allocations amortized over 64MB)

### Where GFS Struggles

For the **storage service** serving many small files to users:

- Every read pays the metadata round-trip (~1.4ms for two gRPC calls) regardless of file size
- For a 1KB file, useful data transfer is less than 1% of total request time
- No built-in caching or HTTP-aware optimizations (range requests, ETags, etc.)

### 3. ~~Per-Request Database Queries~~ (RESOLVED)

**Before:** Every file download performed a database query to check namespace visibility (`canAccessNamespace`), which under concurrent load caused ~90ms latency spikes when HAProxy was throttled.

**After:** Namespace visibility information is now cached in-memory with a 30-second TTL:
- Cache hit: no database round-trip
- Cache miss: single query, result cached for future requests
- Invalidation: automatic on namespace visibility updates

This eliminates the database as a bottleneck on the download path.

## Potential Improvements

1. ~~**Use cached chunk locations on read path**~~ ✅ **DONE** — `ReadToWithNamespace` now uses `getCachedChunks()`
2. **Add an in-memory/on-disk cache at the SFS layer** — skip GFS entirely for hot files
3. **HTTP caching headers** — allow browsers and proxies to cache responses (ETag, Cache-Control)

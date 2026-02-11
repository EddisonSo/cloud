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

| Concurrency | Ops/sec | Throughput | Notes |
|-------------|---------|------------|-------|
| 10 (keep-alive) | 44 | ~54 Mb/s | Single clean run |
| 50 (keep-alive) | 36 | ~44 Mb/s | Gateway CPU-bound at 500m limit |
| 50 (keep-alive, 2000m gateway) | 43 | ~53 Mb/s | After raising gateway CPU limit |
| 10 (no keep-alive) | 41 | ~50 Mb/s | New TLS connection per request |
| 1000 (keep-alive) | 121 | ~147 Mb/s | Approaching network cap |
| 1000 (no keep-alive) | 108 | ~133 Mb/s | ~9% overhead from TLS handshakes |

### Per-Request Latency Breakdown

From a single request with `curl`:

| Phase | Time | % of Total |
|-------|------|------------|
| DNS lookup | 12ms | 14% |
| TCP handshake | 3ms | 3% |
| TLS handshake | 17ms | 20% |
| Server processing (TTFB) | 50ms | 57% |
| Data transfer (155KB) | 5ms | 6% |
| **Total** | **87ms** | |

Only **6%** of the request time is spent transferring data. The rest is overhead.

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

Latency distribution measured with per-request timing (15s runs, no keep-alive):

| Workers | Ops/sec | p50 | p90 | p99 | max |
|---------|---------|-----|-----|-----|-----|
| 1 | 22 | 46ms | 52ms | 70ms | 95ms |
| 2 | 47 | 42ms | 50ms | 60ms | 90ms |
| 3 | 72 | 40ms | 50ms | 98ms | 262ms |
| 5 | 99 | 39ms | 90ms | 204ms | 26s |
| 10 | 40 | 109ms | 612ms | 977ms | 1.3s |

Throughput scales linearly from 1-5 workers (22→99 ops/sec) then collapses at 10. The health endpoint (19 bytes) scales to 333 ops/sec at 10 workers with p99 under 50ms — proving the issue is bandwidth, not server capacity.

### Control: Health Endpoint (no GFS, 19 bytes)

| Workers | Ops/sec | p50 | p99 |
|---------|---------|-----|-----|
| 1 | 33 | 30ms | 46ms |
| 5 | 231 | 21ms | 37ms |
| 10 | 333 | 30ms | 50ms |

Linear scaling, no tail latency. All internal components (gateway, TLS, backend services) have ample headroom.

## Primary Bottleneck: External Bandwidth (200 Mb/s)

At 5 workers × 99 ops/sec × 155KB = ~120 Mb/s — the external link is 60% full. Adding more workers causes TCP queuing and tail latency explodes.

**Layer-by-layer isolation** confirmed this:

| Test | Ops/sec (10 workers) | p50 | Conclusion |
|------|---------------------|-----|------------|
| Full stack (gateway + TLS) | 40 | 109ms | - |
| Direct to SFS (in-cluster, no TLS) | 43 | 108ms | Gateway/TLS add <5% overhead |
| Health endpoint (no GFS) | 333 | 30ms | GFS path adds ~80ms per request |

The gateway and TLS are not significant contributors. The GFS read path adds ~80ms per request (two gRPC master calls + chunkserver TCP), but the dominant bottleneck under concurrent load is the external bandwidth cap.

## Secondary Bottlenecks

### 1. Gateway CPU Limit (resolved)

The gateway was configured with a 500m CPU limit. Under load it hit 492m/500m — fully saturated. Raising the limit to 2000m resolved this.

### 2. Duplicate GFS Master Calls

Each file read makes **two** gRPC calls to the GFS master:
1. `GetFile()` — to get file size for `Content-Length` header
2. `GetChunkLocations()` — to find which chunkserver holds the data

The SDK has a `getCachedChunks` method for caching chunk locations, but `ReadToWithNamespace` calls the master directly instead of using it.

### 3. Gateway Forces Connection: close

The gateway injects `Connection: close` on every proxied request and opens a new TCP connection to the backend per request. This prevents HTTP keep-alive between gateway and backend.

### 4. No Caching Layer

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

- Every read pays the full metadata round-trip (~20-30ms) regardless of file size
- For a 1KB file, useful data transfer is <1% of total request time
- No built-in caching or HTTP-aware optimizations (range requests, ETags, etc.)

## Potential Improvements

1. **Use cached chunk locations on read path** — the SDK already has `getCachedChunks`, just needs to be wired into `ReadToWithNamespace`
2. **Add an in-memory/on-disk cache at the SFS layer** — skip GFS entirely for hot files
3. **HTTP caching headers** — allow browsers and proxies to cache responses (ETag, Cache-Control)

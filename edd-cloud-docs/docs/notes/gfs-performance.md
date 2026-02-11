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

## Bottlenecks Found

### 1. Gateway CPU Limit (resolved)

The gateway was configured with a 500m CPU limit. Under load it hit 492m/500m — fully saturated. Raising the limit to 2000m resolved this.

### 2. GFS Master Metadata Lookup

Every read calls `GetChunkLocationsWithNamespace` via gRPC to the GFS master, even for repeated reads of the same file. The SDK has a `getCachedChunks` method, but the `ReadToWithNamespace` code path does not use it. This adds ~20-30ms of overhead per request regardless of file size.

### 3. No Caching Layer

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

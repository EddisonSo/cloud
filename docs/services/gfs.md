---
sidebar_position: 4
---

# GFS (Distributed File System)

GFS is a custom implementation of Google File System, providing distributed, replicated file storage for Edd Cloud.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      CLIENT APPLICATION                     │
└────────────────────────────┬────────────────────────────────┘
                             │
          ┌──────────────────┴──────────────────┐
          │ gRPC (port 9000)                    │ Custom TCP (port 8080)
          ▼                                     ▼
┌───────────────┐              ┌─────────────────────────────────┐
│    MASTER     │              │       PRIMARY CHUNKSERVER       │
│  - Namespace  │              │  - Receives client data         │
│  - Chunk map  │◄────────────▶│  - Coordinates 2PC              │
│  - CS registry│   Heartbeat  │  - Reports commits to master    │
└───────────────┘              └───────────┬───────────┬─────────┘
                                           │           │
                                           ▼           ▼
                                  ┌──────────────┐  ┌──────────────┐
                                  │  REPLICA CS2 │  │  REPLICA CS3 │
                                  └──────────────┘  └──────────────┘
```

## Key Specifications

| Property | Value |
|----------|-------|
| Chunk Size | 64 MB |
| Replication Factor | 3 |
| Write Quorum | 2 of 3 |
| Consistency | Strong (2PC) |
| Chunk Handle | UUID v4 |

## Components

### Master Server

- Manages file namespace and metadata
- Tracks chunk locations across chunkservers
- Handles chunk allocation for writes
- Persists metadata via Write-Ahead Log (WAL)

### Chunkserver

- Stores actual file data in 64MB chunks
- Participates in Two-Phase Commit for writes
- Reports chunk inventory via heartbeats
- Handles data replication to peers

## Write Flow (Two-Phase Commit)

```
Phase 1: READY (Data Replication)
─────────────────────────────────
1. Client → Primary: Send data via TCP
2. Primary: Allocate offset + sequence number
3. Primary → Replicas: Stream data via gRPC
4. Replicas: Stage data (status=READY)
5. Forwarders → Primary: Ready acknowledgment
6. Primary: Wait for quorum (2/3 replicas)

Phase 2: COMMIT (Data Persistence)
──────────────────────────────────
7. Primary → All Replicas: SendCommit(opID)
8. Replicas: Write to disk, respond success
9. Primary: Write to disk locally
10. Primary → Master: ReportCommit(handle, size)
11. Primary → Client: Success response
```

## Read Flow

```
1. Client → Master: GetChunkLocations(path)
2. Master → Client: [locations with primary]
3. Client → Chunkserver: Read data via TCP
4. (On failure: retry with replica)
```

## Sequence Numbers

Each write operation receives a monotonic sequence number to ensure ordering:

- Primary assigns sequence on write
- Replicas buffer out-of-order commits
- Commits applied in sequence order

## SDK Usage

```go
import gfs "eddisonso.com/go-gfs/pkg/go-gfs-sdk"

// Create client
client, err := gfs.New(ctx, "gfs-master:9000")
defer client.Close()

// Write file
err = client.WriteFile(ctx, "/myfile.txt", []byte("hello"))

// Read file
data, err := client.ReadFile(ctx, "/myfile.txt")

// Append data
err = client.AppendFile(ctx, "/myfile.txt", []byte(" world"))

// Delete file
err = client.DeleteFile(ctx, "/myfile.txt")
```

## Deployment

GFS runs as separate Kubernetes deployments:

- **gfs-master**: Single instance (metadata server)
- **gfs-chunkserver-1/2/3**: One per node (data storage)

Each chunkserver has a PersistentVolumeClaim for data storage.

## Monitoring

Resource metrics available via Master API:

```protobuf
rpc GetClusterPressure(GetClusterPressureRequest)
    returns (GetClusterPressureResponse);
```

Returns CPU, memory, and disk usage for each chunkserver.

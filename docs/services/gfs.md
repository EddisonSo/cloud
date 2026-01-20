---
sidebar_position: 4
---

# GFS (Distributed File System)

GFS is a custom implementation of Google File System, providing distributed, replicated file storage for Edd Cloud.

## Architecture

```mermaid
flowchart TB
    Client[Client Application]

    Client -->|gRPC :9000| Master[Master<br/>Namespace + Chunk Map]
    Client -->|TCP :8080| Primary[Primary Chunkserver<br/>Receives data + 2PC]

    Master <-->|Heartbeat| Primary

    Primary --> Replica1[Replica CS2]
    Primary --> Replica2[Replica CS3]
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

```mermaid
sequenceDiagram
    participant C as Client
    participant P as Primary
    participant R1 as Replica 1
    participant R2 as Replica 2
    participant M as Master

    Note over C,R2: Phase 1: READY (Data Replication)
    C->>P: Send data via TCP
    P->>P: Allocate offset + seq
    P->>R1: Stream data (gRPC)
    P->>R2: Stream data (gRPC)
    R1-->>P: Ready ACK
    R2-->>P: Ready ACK

    Note over C,R2: Phase 2: COMMIT
    P->>R1: SendCommit(opID)
    P->>R2: SendCommit(opID)
    R1-->>P: Success
    R2-->>P: Success
    P->>P: Write to disk
    P->>M: ReportCommit(handle, size)
    P->>C: Success
```

## Read Flow

```mermaid
sequenceDiagram
    participant C as Client
    participant M as Master
    participant CS as Chunkserver

    C->>M: GetChunkLocations(path)
    M->>C: [locations with primary]
    C->>CS: Read data via TCP
    Note over C,CS: On failure: retry with replica
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

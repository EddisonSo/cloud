# Alerting Service Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Extract alerting from cluster-monitor into a standalone service that subscribes to NATS for cluster metrics, pod status, and error logs (all protobuf-serialized), evaluates alert rules, and sends alerts to Discord.

**Architecture:** Cluster-monitor and log-service publish protobuf messages to NATS. Alerting-service subscribes to NATS subjects using JetStream durable consumers. Services are fully decoupled — no direct connections between them.

**Tech Stack:** Go 1.24, NATS JetStream, protobuf, gfslog (structured logging)

---

### Task 1: Define protobuf messages for cluster metrics and log errors

Create proto definitions for the messages that will flow through NATS.

**Files:**
- Create: `proto/cluster/events.proto`
- Create: `proto/log/events.proto`

**Step 1: Create cluster events proto**

```protobuf
syntax = "proto3";

package cluster;

option go_package = "eddisonso.com/edd-cloud/proto/cluster";

import "common/types.proto";

message ClusterMetrics {
  common.EventMetadata metadata = 1;
  repeated NodeMetrics nodes = 2;
}

message NodeMetrics {
  string name = 1;
  double cpu_percent = 2;
  double memory_percent = 3;
  double disk_percent = 4;
  repeated NodeCondition conditions = 5;
}

message NodeCondition {
  string type = 1;
  string status = 2;
}

message PodStatusSnapshot {
  common.EventMetadata metadata = 1;
  repeated PodStatus pods = 2;
}

message PodStatus {
  string name = 1;
  string namespace = 2;
  int32 restart_count = 3;
  bool oom_killed = 4;
}
```

**Step 2: Create log events proto**

```protobuf
syntax = "proto3";

package log;

option go_package = "eddisonso.com/edd-cloud/proto/log";

import "common/types.proto";

message LogError {
  common.EventMetadata metadata = 1;
  string source = 2;
  string message = 3;
  string level = 4;
}
```

**Step 3: Commit**

Commit message: `feat(proto): add cluster metrics and log error event definitions`

---

### Task 2: Create alerting-service scaffold and move alerting package

Move the `internal/alerting/` package from cluster-monitor to the new service. Remove `logclient.go` and `logsub.go` WebSocket code (replaced by NATS).

**Files:**
- Create: `alerting-service/go.mod`
- Create: `alerting-service/main.go` (minimal, just healthz for now)
- Create: `alerting-service/Dockerfile`
- Create: `alerting-service/Makefile`
- Move: `cluster-monitor/internal/alerting/cooldown.go`, `cooldown_test.go`, `discord.go`, `discord_test.go`, `evaluator.go`, `evaluator_test.go` → `alerting-service/internal/alerting/`
- Do NOT move: `logclient.go`, `logsub.go`, `logsub_test.go` (WebSocket-based, replaced by NATS subscriber)

**Step 1: Create directory structure**

```bash
mkdir -p alerting-service/internal/alerting
mkdir -p alerting-service/pkg/pb
```

**Step 2: Create go.mod**

```go
module eddisonso.com/alerting-service

go 1.24.0

toolchain go1.24.11

require (
    eddisonso.com/go-gfs v0.0.0
    github.com/nats-io/nats.go v1.41.2
    google.golang.org/protobuf v1.36.6
)

replace eddisonso.com/go-gfs => ../go-gfs
```

Then run `go mod tidy`.

**Step 3: Create Makefile**

Follow notification-service pattern. Generate Go code from `proto/common/types.proto`, `proto/cluster/events.proto`, `proto/log/events.proto` into `pkg/pb/`.

```makefile
.PHONY: proto build clean

PROTO_DIR = ../proto
OUT_DIR = pkg/pb

proto:
	protoc \
		--proto_path=$(PROTO_DIR) \
		--go_out=$(OUT_DIR) \
		--go_opt=Mcommon/types.proto=eddisonso.com/alerting-service/pkg/pb/common \
		--go_opt=Mcluster/events.proto=eddisonso.com/alerting-service/pkg/pb/cluster \
		--go_opt=Mlog/events.proto=eddisonso.com/alerting-service/pkg/pb/log \
		--go_opt=module=eddisonso.com/alerting-service/pkg/pb \
		$(PROTO_DIR)/common/types.proto \
		$(PROTO_DIR)/cluster/events.proto \
		$(PROTO_DIR)/log/events.proto

build: proto
	go build -o bin/alerting-service .

clean:
	rm -rf bin/
```

**Step 4: Run `make proto`** to generate Go code from proto files.

**Step 5: Copy alerting package files** (only the ones we keep)

Copy from `cluster-monitor/internal/alerting/` to `alerting-service/internal/alerting/`:
- `cooldown.go`, `cooldown_test.go`
- `discord.go`, `discord_test.go`
- `evaluator.go`, `evaluator_test.go`

Update import paths: change `eddisonso.com/cluster-monitor/` to `eddisonso.com/alerting-service/` in all copied files.

**Step 6: Create minimal main.go**

```go
package main

import (
    "flag"
    "log/slog"
    "net/http"
    "os"

    "eddisonso.com/go-gfs/pkg/gfslog"
)

func main() {
    logServiceAddr := flag.String("log-service-grpc", "", "Log service gRPC address for structured logging")
    logSource := flag.String("log-source", "alerting-service", "Log source name")
    flag.Parse()

    if *logServiceAddr != "" {
        logger := gfslog.NewLogger(gfslog.Config{
            Source:         *logSource,
            LogServiceAddr: *logServiceAddr,
            MinLevel:       slog.LevelDebug,
        })
        slog.SetDefault(logger.Logger)
        defer logger.Close()
    }

    http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("ok"))
    })

    slog.Info("alerting-service listening", "addr", ":8080")
    if err := http.ListenAndServe(":8080", nil); err != nil {
        slog.Error("HTTP server failed", "error", err)
        os.Exit(1)
    }
}
```

**Step 7: Create Dockerfile**

```dockerfile
FROM golang:1.24-alpine AS builder

ENV GOTOOLCHAIN=auto
WORKDIR /src

COPY go-gfs /go-gfs

COPY alerting-service/go.mod alerting-service/go.sum /src/
COPY alerting-service /src/

RUN sed -i 's|replace eddisonso.com/go-gfs => ../go-gfs|replace eddisonso.com/go-gfs => /go-gfs|' go.mod

RUN CGO_ENABLED=0 go build -o /out/alerting-service .

FROM alpine:3.20
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /out/alerting-service .
ENTRYPOINT ["./alerting-service"]
```

**Step 8: Run tests**

Run: `cd alerting-service && go mod tidy && go test ./internal/alerting/ -v`
Expected: All cooldown, discord, and evaluator tests pass.

**Step 9: Commit**

Commit message: `feat(alerting-service): scaffold service with alerting package from cluster-monitor`

---

### Task 3: Add NATS publisher to cluster-monitor

Cluster-monitor publishes protobuf-serialized `ClusterMetrics` and `PodStatusSnapshot` to NATS after each metrics fetch.

**Files:**
- Create: `cluster-monitor/Makefile` (proto generation)
- Create: `cluster-monitor/pkg/pb/` (generated proto code)
- Modify: `cluster-monitor/go.mod` (add nats dependency)
- Modify: `cluster-monitor/main.go` (add NATS publishing)

**Step 1: Create Makefile for proto generation**

```makefile
.PHONY: proto

PROTO_DIR = ../proto
OUT_DIR = pkg/pb

proto:
	protoc \
		--proto_path=$(PROTO_DIR) \
		--go_out=$(OUT_DIR) \
		--go_opt=Mcommon/types.proto=eddisonso.com/cluster-monitor/pkg/pb/common \
		--go_opt=Mcluster/events.proto=eddisonso.com/cluster-monitor/pkg/pb/cluster \
		--go_opt=module=eddisonso.com/cluster-monitor/pkg/pb \
		$(PROTO_DIR)/common/types.proto \
		$(PROTO_DIR)/cluster/events.proto
```

**Step 2: Run `make proto`** to generate Go code.

**Step 3: Add NATS dependency**

```bash
cd cluster-monitor && go get github.com/nats-io/nats.go github.com/nats-io/nats.go/jetstream google.golang.org/protobuf
```

**Step 4: Add NATS publisher to main.go**

Add `-nats` flag (default: `nats://nats:4222`).

Create a NATS connection and JetStream context in `main()`. Ensure the `CLUSTER` stream exists:

```go
nc, err := nats.Connect(*natsURL,
    nats.RetryOnFailedConnect(true),
    nats.MaxReconnects(-1),
    nats.ReconnectWait(2*time.Second),
)
js, err := jetstream.New(nc)
js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
    Name:     "CLUSTER",
    Subjects: []string{"cluster.>"},
    Retention: jetstream.LimitsPolicy,
    MaxMsgs:  1000000,
    MaxBytes: 1024 * 1024 * 1024,
    MaxAge:   7 * 24 * time.Hour,
    Storage:  jetstream.FileStorage,
})
```

**Step 5: Publish ClusterMetrics at end of fetchClusterInfo**

After building the `ClusterInfo` struct and broadcasting to SSE/WS subscribers, convert node metrics to protobuf `ClusterMetrics` and publish to `cluster.metrics`:

```go
func publishClusterMetrics(js jetstream.JetStream, info ClusterInfo) {
    msg := &pbcluster.ClusterMetrics{
        Metadata: &pbcommon.EventMetadata{
            EventId:   generateUUID(),
            Timestamp: &pbcommon.Timestamp{Seconds: time.Now().Unix()},
            Source:    "cluster-monitor",
        },
    }
    for _, n := range info.Nodes {
        node := &pbcluster.NodeMetrics{
            Name:          n.Name,
            CpuPercent:    n.CPUPercent,
            MemoryPercent: n.MemoryPercent,
            DiskPercent:   n.DiskPercent,
        }
        for _, c := range n.Conditions {
            node.Conditions = append(node.Conditions, &pbcluster.NodeCondition{
                Type:   c.Type,
                Status: c.Status,
            })
        }
        msg.Nodes = append(msg.Nodes, node)
    }
    data, _ := proto.Marshal(msg)
    js.Publish(context.Background(), "cluster.metrics", data)
}
```

**Step 6: Publish PodStatusSnapshot at end of fetchPodMetrics**

After collecting pod statuses (the existing OOMKilled detection block), convert to protobuf and publish to `cluster.pods`:

```go
func publishPodStatus(js jetstream.JetStream, pods []PodMetrics, podStatuses []podStatusInfo) {
    msg := &pbcluster.PodStatusSnapshot{
        Metadata: &pbcommon.EventMetadata{
            EventId:   generateUUID(),
            Timestamp: &pbcommon.Timestamp{Seconds: time.Now().Unix()},
            Source:    "cluster-monitor",
        },
    }
    for _, ps := range podStatuses {
        msg.Pods = append(msg.Pods, &pbcluster.PodStatus{
            Name:         ps.Name,
            Namespace:    ps.Namespace,
            RestartCount: ps.RestartCount,
            OomKilled:    ps.OOMKilled,
        })
    }
    data, _ := proto.Marshal(msg)
    js.Publish(context.Background(), "cluster.pods", data)
}
```

Note: The `podStatusInfo` struct and OOMKilled detection logic already exist in `fetchPodMetrics` from the alerting implementation. Extract the pod status data before removing the evaluator call.

**Step 7: Update fetchClusterInfo and fetchPodMetrics signatures**

Add `js jetstream.JetStream` parameter. Pass from worker goroutines.

**Step 8: Verify build**

Run: `cd cluster-monitor && go mod tidy && go build .`
Expected: Compiles

**Step 9: Commit**

Commit message: `feat(cluster-monitor): publish cluster metrics and pod status to NATS`

---

### Task 4: Add NATS publisher to log-service for error logs

Log-service publishes protobuf `LogError` to NATS when ERROR+ level logs are ingested.

**Files:**
- Create: `log-service/Makefile` (update for new proto)
- Create: `log-service/pkg/pb/` (generated proto code for cluster/log events)
- Modify: `log-service/go.mod` (add nats dependency)
- Modify: `log-service/internal/server/server.go` (publish errors to NATS)
- Modify: `log-service/main.go` (pass NATS connection)

**Step 1: Update Makefile** to also generate `log/events.proto` and `common/types.proto` Go code.

Alternatively create a separate Makefile target. Follow the same pattern as notification-service.

**Step 2: Run proto generation.**

**Step 3: Add NATS dependency**

```bash
cd log-service && go get github.com/nats-io/nats.go github.com/nats-io/nats.go/jetstream
```

**Step 4: Add NATS connection to main.go**

Add `-nats` flag. Connect to NATS, create JetStream, ensure `LOGS` stream exists:

```go
js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
    Name:     "LOGS",
    Subjects: []string{"log.error.>"},
    Retention: jetstream.LimitsPolicy,
    MaxMsgs:  1000000,
    MaxBytes: 1024 * 1024 * 1024,
    MaxAge:   7 * 24 * time.Hour,
    Storage:  jetstream.FileStorage,
})
```

Pass `js` to `LogServer`.

**Step 5: Publish LogError in PushLog handler**

In `server.go` `PushLog()`, after the broadcast line, add NATS publishing for ERROR+ level:

```go
// After s.broadcast(entry)
if entry.Level >= pb.LogLevel_ERROR && s.js != nil {
    logErr := &pblog.LogError{
        Metadata: &pbcommon.EventMetadata{
            EventId:   generateUUID(),
            Timestamp: &pbcommon.Timestamp{Seconds: entry.Timestamp},
            Source:    "log-service",
        },
        Source:  entry.Source,
        Message: entry.Message,
        Level:   entry.Level.String(),
    }
    data, _ := proto.Marshal(logErr)
    s.js.Publish(context.Background(), fmt.Sprintf("log.error.%s", entry.Source), data)
}
```

**Step 6: Add `js jetstream.JetStream` field to LogServer struct.**

**Step 7: Verify build**

Run: `cd log-service && go mod tidy && go build .`
Expected: Compiles

**Step 8: Commit**

Commit message: `feat(log-service): publish error logs to NATS for downstream consumers`

---

### Task 5: Wire NATS subscribers in alerting-service main.go

Connect NATS subscriptions to the evaluator and Discord sender.

**Files:**
- Modify: `alerting-service/main.go`

**Step 1: Update main.go**

```go
package main

import (
    "context"
    "flag"
    "log/slog"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/nats-io/nats.go"
    "github.com/nats-io/nats.go/jetstream"
    "google.golang.org/protobuf/proto"

    "eddisonso.com/alerting-service/internal/alerting"
    pbcluster "eddisonso.com/alerting-service/pkg/pb/cluster"
    pbcommon "eddisonso.com/alerting-service/pkg/pb/common"
    pblog "eddisonso.com/alerting-service/pkg/pb/log"
    "eddisonso.com/go-gfs/pkg/gfslog"
)

func main() {
    natsURL := flag.String("nats", "nats://nats:4222", "NATS server URL")
    discordWebhook := flag.String("discord-webhook", "", "Discord webhook URL for alerts")
    alertCooldown := flag.Duration("alert-cooldown", 5*time.Minute, "Default alert cooldown duration")
    logServiceGRPC := flag.String("log-service-grpc", "", "Log service gRPC address for structured logging")
    logSource := flag.String("log-source", "alerting-service", "Log source name")
    flag.Parse()

    // Structured logging
    if *logServiceGRPC != "" {
        logger := gfslog.NewLogger(gfslog.Config{
            Source:         *logSource,
            LogServiceAddr: *logServiceGRPC,
            MinLevel:       slog.LevelDebug,
        })
        slog.SetDefault(logger.Logger)
        defer logger.Close()
    }

    // NATS connection
    nc, err := nats.Connect(*natsURL,
        nats.RetryOnFailedConnect(true),
        nats.MaxReconnects(-1),
        nats.ReconnectWait(2*time.Second),
        nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
            slog.Warn("NATS disconnected", "error", err)
        }),
        nats.ReconnectHandler(func(_ *nats.Conn) {
            slog.Info("NATS reconnected")
        }),
    )
    if err != nil {
        slog.Error("failed to connect to NATS", "error", err)
        os.Exit(1)
    }
    defer nc.Close()

    js, err := jetstream.New(nc)
    if err != nil {
        slog.Error("failed to create JetStream", "error", err)
        os.Exit(1)
    }

    // Alert delivery
    discord := alerting.NewDiscordSender(*discordWebhook)
    fireAlert := func(a alerting.Alert) {
        slog.Warn("alert fired", "title", a.Title, "message", a.Message)
        if err := discord.Send(a); err != nil {
            slog.Error("failed to send Discord alert", "error", err)
        }
    }

    // Evaluator
    evaluator := alerting.NewEvaluator(alerting.EvaluatorConfig{
        CPUThreshold:    90,
        MemThreshold:    85,
        DiskThreshold:   90,
        DefaultCooldown: *alertCooldown,
    }, fireAlert)

    // Log error detector
    logDetector := alerting.NewLogDetector(alerting.LogDetectorConfig{
        BurstThreshold: 5,
        BurstWindow:    30 * time.Second,
        Cooldown:       *alertCooldown,
    }, fireAlert)

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Subscribe to cluster.metrics
    go subscribeClusterMetrics(ctx, js, evaluator)

    // Subscribe to cluster.pods
    go subscribeClusterPods(ctx, js, evaluator)

    // Subscribe to log.error.>
    go subscribeLogErrors(ctx, js, logDetector)

    // Health endpoint
    go func() {
        http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
            w.WriteHeader(http.StatusOK)
            w.Write([]byte("ok"))
        })
        slog.Info("alerting-service listening", "addr", ":8080")
        if err := http.ListenAndServe(":8080", nil); err != nil {
            slog.Error("HTTP server failed", "error", err)
        }
    }()

    sig := make(chan os.Signal, 1)
    signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
    <-sig
    slog.Info("shutting down")
}

func subscribeClusterMetrics(ctx context.Context, js jetstream.JetStream, evaluator *alerting.Evaluator) {
    consumer, err := js.CreateOrUpdateConsumer(ctx, "CLUSTER", jetstream.ConsumerConfig{
        Durable:       "alerting-metrics",
        FilterSubject: "cluster.metrics",
        AckPolicy:     jetstream.AckExplicitPolicy,
        DeliverPolicy: jetstream.DeliverLastPolicy,
        AckWait:       30 * time.Second,
        MaxDeliver:    5,
    })
    if err != nil {
        slog.Error("failed to create cluster metrics consumer", "error", err)
        return
    }

    for {
        select {
        case <-ctx.Done():
            return
        default:
            msgs, err := consumer.Fetch(1, jetstream.FetchMaxWait(5*time.Second))
            if err != nil {
                continue
            }
            for msg := range msgs.Messages() {
                var metrics pbcluster.ClusterMetrics
                if err := proto.Unmarshal(msg.Data(), &metrics); err != nil {
                    slog.Error("failed to unmarshal cluster metrics", "error", err)
                    msg.Nak()
                    continue
                }
                var nodes []alerting.NodeSnapshot
                for _, n := range metrics.Nodes {
                    var conditions []string
                    for _, c := range n.Conditions {
                        if c.Status == "True" {
                            conditions = append(conditions, c.Type)
                        }
                    }
                    nodes = append(nodes, alerting.NodeSnapshot{
                        Name:        n.Name,
                        CPUPercent:  n.CpuPercent,
                        MemPercent:  n.MemoryPercent,
                        DiskPercent: n.DiskPercent,
                        Conditions:  conditions,
                    })
                }
                evaluator.EvaluateCluster(alerting.ClusterSnapshot{Nodes: nodes})
                msg.Ack()
            }
        }
    }
}

func subscribeClusterPods(ctx context.Context, js jetstream.JetStream, evaluator *alerting.Evaluator) {
    consumer, err := js.CreateOrUpdateConsumer(ctx, "CLUSTER", jetstream.ConsumerConfig{
        Durable:       "alerting-pods",
        FilterSubject: "cluster.pods",
        AckPolicy:     jetstream.AckExplicitPolicy,
        DeliverPolicy: jetstream.DeliverLastPolicy,
        AckWait:       30 * time.Second,
        MaxDeliver:    5,
    })
    if err != nil {
        slog.Error("failed to create pod status consumer", "error", err)
        return
    }

    for {
        select {
        case <-ctx.Done():
            return
        default:
            msgs, err := consumer.Fetch(1, jetstream.FetchMaxWait(5*time.Second))
            if err != nil {
                continue
            }
            for msg := range msgs.Messages() {
                var snapshot pbcluster.PodStatusSnapshot
                if err := proto.Unmarshal(msg.Data(), &snapshot); err != nil {
                    slog.Error("failed to unmarshal pod status", "error", err)
                    msg.Nak()
                    continue
                }
                var podStatuses []alerting.PodStatus
                for _, p := range snapshot.Pods {
                    podStatuses = append(podStatuses, alerting.PodStatus{
                        Name:         p.Name,
                        Namespace:    p.Namespace,
                        RestartCount: p.RestartCount,
                        OOMKilled:    p.OomKilled,
                    })
                }
                evaluator.EvaluatePods(alerting.PodSnapshot{Pods: podStatuses})
                msg.Ack()
            }
        }
    }
}

func subscribeLogErrors(ctx context.Context, js jetstream.JetStream, logDetector *alerting.LogDetector) {
    consumer, err := js.CreateOrUpdateConsumer(ctx, "LOGS", jetstream.ConsumerConfig{
        Durable:       "alerting-logs",
        FilterSubject: "log.error.>",
        AckPolicy:     jetstream.AckExplicitPolicy,
        DeliverPolicy: jetstream.DeliverNewPolicy,
        AckWait:       30 * time.Second,
        MaxDeliver:    5,
    })
    if err != nil {
        slog.Error("failed to create log error consumer", "error", err)
        return
    }

    for {
        select {
        case <-ctx.Done():
            return
        default:
            msgs, err := consumer.Fetch(10, jetstream.FetchMaxWait(5*time.Second))
            if err != nil {
                continue
            }
            for msg := range msgs.Messages() {
                var logErr pblog.LogError
                if err := proto.Unmarshal(msg.Data(), &logErr); err != nil {
                    slog.Error("failed to unmarshal log error", "error", err)
                    msg.Nak()
                    continue
                }
                logDetector.HandleLogEntry(logErr.Source, logErr.Message)
                msg.Ack()
            }
        }
    }
}
```

**Step 2: Run tests**

Run: `cd alerting-service && go mod tidy && go test ./... -v`
Expected: All tests pass

**Step 3: Verify build**

Run: `cd alerting-service && go build .`
Expected: Compiles

**Step 4: Commit**

Commit message: `feat(alerting-service): wire NATS subscribers for cluster metrics, pods, and log errors`

---

### Task 6: Remove alerting from cluster-monitor

Strip all alerting code from cluster-monitor — it returns to being a pure metrics producer.

**Files:**
- Delete: `cluster-monitor/internal/alerting/` (entire directory)
- Modify: `cluster-monitor/main.go`
- Modify: `manifests/cluster-monitor/cluster-monitor.yaml`

**Step 1: Remove alerting directory**

```bash
rm -rf cluster-monitor/internal/alerting/
```

**Step 2: Edit main.go**

Remove from imports:
```go
"eddisonso.com/cluster-monitor/internal/alerting"
```

Remove these flags:
```go
discordWebhook := flag.String("discord-webhook", "", "Discord webhook URL for alerts")
alertCooldown := flag.Duration("alert-cooldown", 5*time.Minute, "Default alert cooldown duration")
logServiceHTTP := flag.String("log-service-http", "", "Log service HTTP address for WebSocket subscription")
```

Remove the alerting initialization block (discord sender, evaluator, logDetector, SubscribeLogService).

Remove `evaluator *alerting.Evaluator` parameter from `fetchClusterInfo` and `fetchPodMetrics` signatures. Update call sites.

Remove the `if evaluator != nil` alert evaluation blocks from both functions.

Keep the pod status collection logic in `fetchPodMetrics` — it's now used for NATS publishing.

**Step 3: Revert manifest**

Remove from `manifests/cluster-monitor/cluster-monitor.yaml`:

Args to remove:
```yaml
- "-discord-webhook"
- "$(DISCORD_WEBHOOK_URL)"
- "-log-service-http"
- "log-service:80"
```

Args to add:
```yaml
- "-nats"
- "nats://nats:4222"
```

Env var to remove:
```yaml
- name: DISCORD_WEBHOOK_URL
  valueFrom:
    secretKeyRef:
      name: discord-webhook-url
      key: WEBHOOK_URL
```

**Step 4: Verify build**

Run: `cd cluster-monitor && go build .`
Expected: Compiles

**Step 5: Commit**

Commit message: `refactor(cluster-monitor): remove alerting code, keep NATS publisher for metrics`

---

### Task 7: Create Kubernetes manifest for alerting-service

**Files:**
- Create: `manifests/alerting-service/alerting-service.yaml`

**Step 1: Write manifest**

No ServiceAccount needed — alerting-service doesn't access K8s API (pod status comes via NATS).

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: alerting-service
spec:
  replicas: 1
  selector:
    matchLabels:
      app: alerting-service
  template:
    metadata:
      labels:
        app: alerting-service
    spec:
      nodeSelector:
        backend: "true"
      imagePullSecrets:
        - name: regcred
      containers:
        - name: alerting-service
          image: eddisonso/alerting-service:latest
          args:
            - "-nats"
            - "nats://nats:4222"
            - "-discord-webhook"
            - "$(DISCORD_WEBHOOK_URL)"
            - "-log-service-grpc"
            - "log-service:50051"
            - "-log-source"
            - "$(POD_NAME)"
          env:
            - name: POD_NAME
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
            - name: DISCORD_WEBHOOK_URL
              valueFrom:
                secretKeyRef:
                  name: discord-webhook-url
                  key: WEBHOOK_URL
          ports:
            - containerPort: 8080
              name: http
          livenessProbe:
            httpGet:
              path: /healthz
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10
          readinessProbe:
            httpGet:
              path: /healthz
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10
---
apiVersion: v1
kind: Service
metadata:
  name: alerting-service
spec:
  selector:
    app: alerting-service
  ports:
    - name: http
      port: 80
      targetPort: 8080
```

**Step 2: Update log-service manifest** to add `-nats` flag.

**Step 3: Commit**

Commit message: `feat(manifests): add alerting-service deployment and update log-service for NATS`

---

### Task 8: Add alerting-service to CI/CD pipeline

**Files:**
- Modify: `.github/workflows/build-deploy.yml`

**Step 1: Add to CI/CD** following existing service patterns:

1. Add `ALERTING_IMAGE: docker.io/eddisonso/alerting-service` to env vars
2. Add `alerting` output to detect-changes job
3. Add `alerting` filter path: `alerting-service/**` and `go-gfs/**`
4. Add `alerting` to `any_service` filter
5. Add `build-alerting` job (same pattern as `build-cluster-monitor` — multi-arch build with go-gfs dependency)
6. Add `build-alerting` to `create-manifests` needs
7. Add alerting manifest creation step
8. Add alerting deploy step: `kubectl set image deployment/alerting-service alerting-service=${ALERTING_IMAGE}:${GITHUB_SHA}`

Also update `build-cluster-monitor` and `build-log-service` triggers to include `proto/**` path changes.

**Step 2: Commit**

Commit message: `ci: add alerting-service to build and deploy pipeline`

---

### Task 9: Update roadmap and docs

**Files:**
- Modify: `edd-cloud-docs/docs/roadmap.md`

**Step 1: Update roadmap**

Change:
```markdown
- [x] **Alerting** - Automated alerts for service health (Discord webhook via cluster-monitor)
```
To:
```markdown
- [x] **Alerting** - Automated alerts for service health (standalone alerting-service, NATS + protobuf, Discord webhooks)
```

**Step 2: Commit**

Commit message: `docs: update roadmap to reflect NATS-based alerting service`

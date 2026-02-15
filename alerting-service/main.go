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
	pblog "eddisonso.com/alerting-service/pkg/pb/log"
	"eddisonso.com/go-gfs/pkg/gfslog"
)

func main() {
	natsURL := flag.String("nats", "nats://nats:4222", "NATS server URL")
	discordWebhook := flag.String("discord-webhook", "", "Discord webhook URL for alerts")
	alertCooldown := flag.Duration("alert-cooldown", 5*time.Minute, "Cooldown between repeated alerts")
	logServiceAddr := flag.String("log-service-grpc", "", "Log service gRPC address for structured logging")
	logSource := flag.String("log-source", "alerting-service", "Log source name")
	flag.Parse()

	// Structured logging
	if *logServiceAddr != "" {
		logger := gfslog.NewLogger(gfslog.Config{
			Source:         *logSource,
			LogServiceAddr: *logServiceAddr,
			MinLevel:       slog.LevelDebug,
		})
		slog.SetDefault(logger.Logger)
		defer logger.Close()
	}

	// Connect to NATS
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
	slog.Info("connected to NATS", "url", *natsURL)

	// JetStream
	js, err := jetstream.New(nc)
	if err != nil {
		slog.Error("failed to create JetStream context", "error", err)
		os.Exit(1)
	}

	// Discord sender and alert callback
	discord := alerting.NewDiscordSender(*discordWebhook)
	fireAlert := func(alert alerting.Alert) {
		slog.Info("alert fired", "title", alert.Title, "severity", alert.Severity)
		if err := discord.Send(alert); err != nil {
			slog.Error("failed to send discord alert", "error", err, "title", alert.Title)
		}
	}

	// Evaluator
	eval := alerting.NewEvaluator(alerting.EvaluatorConfig{
		CPUThreshold:    90,
		MemThreshold:    85,
		DiskThreshold:   90,
		DefaultCooldown: *alertCooldown,
	}, fireAlert)

	// Log detector
	logDetector := alerting.NewLogDetector(alerting.LogDetectorConfig{
		BurstThreshold:  5,
		BurstWindow:     30 * time.Second,
		DefaultCooldown: *alertCooldown,
	}, fireAlert)

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start subscribers
	go subscribeClusterMetrics(ctx, js, eval)
	go subscribeClusterPods(ctx, js, eval)
	go subscribeLogErrors(ctx, js, logDetector)

	// Health endpoint
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	go func() {
		slog.Info("alerting-service listening", "addr", ":8080")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			slog.Error("HTTP server failed", "error", err)
		}
	}()

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	slog.Info("shutting down", "signal", sig)
	cancel()
}

// createConsumerWithRetry retries consumer creation with exponential backoff
// until it succeeds or ctx is cancelled. This handles the startup race where
// the stream may not exist yet (cluster-monitor creates it).
func createConsumerWithRetry(ctx context.Context, js jetstream.JetStream, stream string, cfg jetstream.ConsumerConfig) (jetstream.Consumer, error) {
	backoff := 2 * time.Second
	maxBackoff := 30 * time.Second
	for {
		cons, err := js.CreateOrUpdateConsumer(ctx, stream, cfg)
		if err == nil {
			return cons, nil
		}
		slog.Warn("consumer creation failed, retrying", "stream", stream, "subject", cfg.FilterSubject, "error", err, "retry_in", backoff)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func subscribeClusterMetrics(ctx context.Context, js jetstream.JetStream, eval *alerting.Evaluator) {
	cons, err := createConsumerWithRetry(ctx, js, "CLUSTER", jetstream.ConsumerConfig{
		Durable:       "alerting-metrics",
		FilterSubject: "cluster.metrics",
		DeliverPolicy: jetstream.DeliverLastPolicy,
		MaxDeliver:    5,
	})
	if err != nil {
		slog.Error("giving up on cluster.metrics consumer", "error", err)
		return
	}
	slog.Info("subscribed to cluster.metrics")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msgs, err := cons.Fetch(1, jetstream.FetchMaxWait(5*time.Second))
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Debug("cluster.metrics fetch", "error", err)
			continue
		}

		for msg := range msgs.Messages() {
			var pb pbcluster.ClusterMetrics
			if err := proto.Unmarshal(msg.Data(), &pb); err != nil {
				slog.Error("unmarshal ClusterMetrics", "error", err)
				msg.Nak()
				continue
			}

			snapshot := alerting.ClusterSnapshot{
				Nodes: make([]alerting.NodeSnapshot, 0, len(pb.Nodes)),
			}
			for _, n := range pb.Nodes {
				conditions := make([]string, 0, len(n.Conditions))
				for _, c := range n.Conditions {
					if c.Status == "True" {
						conditions = append(conditions, c.Type)
					}
				}
				snapshot.Nodes = append(snapshot.Nodes, alerting.NodeSnapshot{
					Name:        n.Name,
					CPUPercent:  n.CpuPercent,
					MemPercent:  n.MemoryPercent,
					DiskPercent: n.DiskPercent,
					Conditions:  conditions,
				})
			}
			eval.EvaluateCluster(snapshot)
			msg.Ack()
		}

		if err := msgs.Error(); err != nil {
			slog.Debug("cluster.metrics fetch stream", "error", err)
		}
	}
}

func subscribeClusterPods(ctx context.Context, js jetstream.JetStream, eval *alerting.Evaluator) {
	cons, err := createConsumerWithRetry(ctx, js, "CLUSTER", jetstream.ConsumerConfig{
		Durable:       "alerting-pods",
		FilterSubject: "cluster.pods",
		DeliverPolicy: jetstream.DeliverLastPolicy,
		MaxDeliver:    5,
	})
	if err != nil {
		slog.Error("giving up on cluster.pods consumer", "error", err)
		return
	}
	slog.Info("subscribed to cluster.pods")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msgs, err := cons.Fetch(1, jetstream.FetchMaxWait(5*time.Second))
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Debug("cluster.pods fetch", "error", err)
			continue
		}

		for msg := range msgs.Messages() {
			var pb pbcluster.PodStatusSnapshot
			if err := proto.Unmarshal(msg.Data(), &pb); err != nil {
				slog.Error("unmarshal PodStatusSnapshot", "error", err)
				msg.Nak()
				continue
			}

			snapshot := alerting.PodSnapshot{
				Pods: make([]alerting.PodStatus, 0, len(pb.Pods)),
			}
			for _, p := range pb.Pods {
				snapshot.Pods = append(snapshot.Pods, alerting.PodStatus{
					Name:         p.Name,
					Namespace:    p.Namespace,
					RestartCount: p.RestartCount,
					OOMKilled:    p.OomKilled,
				})
			}
			eval.EvaluatePods(snapshot)
			msg.Ack()
		}

		if err := msgs.Error(); err != nil {
			slog.Debug("cluster.pods fetch stream", "error", err)
		}
	}
}

func subscribeLogErrors(ctx context.Context, js jetstream.JetStream, detector *alerting.LogDetector) {
	cons, err := createConsumerWithRetry(ctx, js, "LOGS", jetstream.ConsumerConfig{
		Durable:       "alerting-logs",
		FilterSubject: "log.error.>",
		DeliverPolicy: jetstream.DeliverNewPolicy,
		MaxDeliver:    5,
	})
	if err != nil {
		slog.Error("giving up on log.error consumer", "error", err)
		return
	}
	slog.Info("subscribed to log.error.>")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msgs, err := cons.Fetch(10, jetstream.FetchMaxWait(5*time.Second))
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Debug("log.error fetch", "error", err)
			continue
		}

		for msg := range msgs.Messages() {
			var pb pblog.LogError
			if err := proto.Unmarshal(msg.Data(), &pb); err != nil {
				slog.Error("unmarshal LogError", "error", err)
				msg.Nak()
				continue
			}

			detector.HandleLogEntry(alerting.LogEntry{
				Source:  pb.Source,
				Message: pb.Message,
				Level:   pb.Level,
			})
			msg.Ack()
		}

		if err := msgs.Error(); err != nil {
			slog.Debug("log.error fetch stream", "error", err)
		}
	}
}

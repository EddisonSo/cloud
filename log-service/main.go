// Package main implements the centralized logging service for the GFS cluster.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	gfs "eddisonso.com/go-gfs/pkg/go-gfs-sdk"
	"github.com/eddisonso/log-service/internal/server"
	pb "github.com/eddisonso/log-service/proto/logging"
	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/grpc"
)

func isAllowedOrigin(origin string) bool {
	return origin == "https://cloud.eddisonso.com" ||
		(len(origin) > len("https://.cloud.eddisonso.com") &&
			strings.HasSuffix(origin, ".cloud.eddisonso.com") &&
			strings.HasPrefix(origin, "https://"))
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		return origin == "" || isAllowedOrigin(origin)
	},
}

func main() {
	grpcAddr := flag.String("grpc", ":50051", "gRPC listen address")
	httpAddr := flag.String("http", ":8080", "HTTP listen address")
	masterAddr := flag.String("master", "gfs-master:9000", "GFS master address")
	natsAddr := flag.String("nats", "nats://nats:4222", "NATS server address")
	flag.Parse()

	// Connect to GFS for log persistence
	ctx := context.Background()
	var gfsClient *gfs.Client

	gfsClient, err := gfs.New(ctx, *masterAddr)
	if err != nil {
		slog.Warn("failed to connect to GFS master, logs will not be persisted", "error", err)
		gfsClient = nil
	} else {
		slog.Info("connected to GFS master", "addr", *masterAddr)
		defer gfsClient.Close()
	}

	// Connect to NATS for error log publishing
	var js jetstream.JetStream
	nc, err := nats.Connect(*natsAddr,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		slog.Warn("failed to connect to NATS, error logs will not be published", "error", err)
	} else {
		slog.Info("connected to NATS", "addr", *natsAddr)
		defer nc.Close()

		js, err = jetstream.New(nc)
		if err != nil {
			slog.Warn("failed to create JetStream context", "error", err)
			js = nil
		} else {
			_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
				Name:      "LOGS",
				Subjects:  []string{"log.error.>"},
				Retention: jetstream.LimitsPolicy,
				MaxMsgs:   1000000,
				MaxBytes:  1024 * 1024 * 1024,
				MaxAge:    7 * 24 * time.Hour,
				Storage:   jetstream.FileStorage,
			})
			if err != nil {
				slog.Warn("failed to create/update LOGS stream", "error", err)
				js = nil
			} else {
				slog.Info("NATS JetStream LOGS stream ready")
			}
		}
	}

	// Create log server
	logServer := server.NewLogServer(gfsClient, js)
	defer logServer.Close()

	// Start gRPC server
	grpcLis, err := net.Listen("tcp", *grpcAddr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", *grpcAddr, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterLogServiceServer(grpcServer, logServer)

	go func() {
		slog.Info("gRPC server listening", "addr", *grpcAddr)
		if err := grpcServer.Serve(grpcLis); err != nil {
			log.Fatalf("gRPC server failed: %v", err)
		}
	}()

	// Start HTTP server for WebSocket
	http.HandleFunc("/ws/logs", func(w http.ResponseWriter, r *http.Request) {
		handleWebSocket(w, r, logServer)
	})

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	httpServer := &http.Server{Addr: *httpAddr}
	go func() {
		slog.Info("HTTP server listening", "addr", *httpAddr)
		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	slog.Info("shutting down...")

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	httpServer.Shutdown(shutdownCtx)
	grpcServer.GracefulStop()
}

func handleWebSocket(w http.ResponseWriter, r *http.Request, logServer *server.LogServer) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("WebSocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	// Parse query parameters
	source := r.URL.Query().Get("source")
	levelStr := r.URL.Query().Get("level")
	minLevel := pb.LogLevel_DEBUG
	if levelStr != "" {
		switch levelStr {
		case "INFO":
			minLevel = pb.LogLevel_INFO
		case "WARN":
			minLevel = pb.LogLevel_WARN
		case "ERROR":
			minLevel = pb.LogLevel_ERROR
		}
	}

	slog.Info("WebSocket client connected", "source", source, "level", levelStr, "minLevel", minLevel)

	// Subscribe to logs
	ch, unsubscribe := logServer.Subscribe(source, minLevel)
	defer unsubscribe()

	var mu sync.Mutex
	done := make(chan struct{})

	// Read pump (handle close)
	go func() {
		defer close(done)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	// Write pump
	for {
		select {
		case entry, ok := <-ch:
			if !ok {
				return
			}
			if matchesFilter(entry, source, minLevel) {
				data, err := json.Marshal(entry)
				if err != nil {
					continue
				}
				mu.Lock()
				err = conn.WriteMessage(websocket.TextMessage, data)
				mu.Unlock()
				if err != nil {
					return
				}
			}
		case <-done:
			return
		}
	}
}

func matchesFilter(entry *pb.LogEntry, source string, minLevel pb.LogLevel) bool {
	if entry == nil {
		return false
	}
	if source != "" && entry.Source != source {
		return false
	}
	if entry.Level < minLevel {
		return false
	}
	return true
}

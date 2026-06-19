// Package main implements the centralized logging service for the edd-cloud cluster.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
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
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/grpc"
)

// ---------------------------------------------------------------------------
// JWT auth — Phase 1
// ---------------------------------------------------------------------------

// JWTClaims mirrors the claims issued by edd-cloud-auth.
// IsAdmin is set at token issuance time from the ADMIN_USERNAME check in auth
// and carried in the JWT so consumers do not need a separate ADMIN_USERNAME env var.
type JWTClaims struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	UserID      string `json:"user_id"`
	IsAdmin     bool   `json:"is_admin"`
	Type        string `json:"type"`
	jwt.RegisteredClaims
}

var jwtSecret []byte

// initJWTSecret reads the shared JWT secret from the JWT_SECRET environment variable.
// NOTE: the log-service Deployment manifest must be updated to mount JWT_SECRET
// from the jwt-secret Kubernetes Secret (same secret used by cluster-monitor and auth).
// That is a manifest-layer change flagged in cross_service_flags.
func initJWTSecret() {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		slog.Warn("JWT_SECRET not set; /ws/logs will reject all requests with 401")
		return
	}
	jwtSecret = []byte(secret)
}

// validateToken parses and validates a JWT, returning the claims on success or nil.
func validateToken(tokenString string) *JWTClaims {
	if len(jwtSecret) == 0 || tokenString == "" {
		return nil
	}
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return jwtSecret, nil
	})
	if err != nil {
		return nil
	}
	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil
	}
	// Reject intermediate challenge tokens (e.g. 2fa_challenge) that share the
	// same signing secret but are not fully-authenticated session tokens.
	if claims.Type == "2fa_challenge" {
		return nil
	}
	return claims
}

// getTokenFromRequest extracts a Bearer token from the Authorization header,
// the ?token= query parameter (used by browser WebSocket connections which
// cannot set custom headers), or the "token" cookie issued by edd-cloud-auth.
func getTokenFromRequest(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	if token := r.URL.Query().Get("token"); token != "" {
		return token
	}
	if cookie, err := r.Cookie("token"); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	return ""
}

// ---------------------------------------------------------------------------
// CORS middleware
// ---------------------------------------------------------------------------

func isAllowedOrigin(origin string) bool {
	return origin == "https://cloud.eddisonso.com" ||
		(len(origin) > len("https://.cloud.eddisonso.com") &&
			strings.HasSuffix(origin, ".cloud.eddisonso.com") &&
			strings.HasPrefix(origin, "https://"))
}

// corsMiddleware adds Access-Control-* headers and handles OPTIONS preflights
// with 200 BEFORE any auth check runs, satisfying the dashboard cross-origin rule.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if isAllowedOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		}
		if r.Method == http.MethodOptions {
			// Preflight — return 200 before auth so tokenless OPTIONS succeeds.
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---------------------------------------------------------------------------
// Auth middleware for /ws/logs
// ---------------------------------------------------------------------------

// requireAdminForLogs enforces Phase-1 access policy on /ws/logs:
//   - Missing or invalid JWT → 401 Unauthorized
//   - Valid JWT, non-admin → 403 Forbidden
//
// Phase 1 rationale: LogEntry carries only a `source` (pod name) with no
// user/namespace ownership field, so we cannot prove which logs belong to
// which caller. Until Phase 2 adds a `namespace` field to the LogEntry proto
// and producers tag entries with their compute-{userID}-* namespace, the only
// safe access policy for non-admins is denial.
//
// Phase 2: once LogEntry.namespace is available, replace the 403 block below
// with: extract the caller's userID from claims, inject it into the handler
// context, and filter Subscribe() results to entries whose namespace matches
// "compute-{userID}-" prefix. Admins continue to see all sources.
func requireAdminForLogs(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenStr := getTokenFromRequest(r)
		claims := validateToken(tokenStr)
		if claims == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if !claims.IsAdmin {
			// Phase 2: replace this 403 with per-user namespace filtering.
			// Non-admins will be able to stream logs for their own containers
			// once LogEntry carries a namespace field populated by producers.
			http.Error(w, "forbidden: log streaming is admin-only until Phase 2 namespace tagging", http.StatusForbidden)
			return
		}

		next(w, r)
	})
}

// ---------------------------------------------------------------------------
// WebSocket upgrader
// ---------------------------------------------------------------------------

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		return origin == "" || isAllowedOrigin(origin)
	},
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	grpcAddr := flag.String("grpc", ":50051", "gRPC listen address")
	httpAddr := flag.String("http", ":8080", "HTTP listen address")
	masterAddr := flag.String("master", "gfs-master:9000", "GFS master address")
	natsAddr := flag.String("nats", "nats://nats:4222", "NATS server address")
	flag.Parse()

	initJWTSecret()

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

	// Start gRPC server.
	// NOTE: gRPC (:50051) is a ClusterIP service — not exposed through the gateway.
	// All callers are internal cluster services (compute, cluster-monitor, etc.) that
	// push logs via PushLog. GetLogs and StreamLogs are internal-only; they are not
	// reachable from outside the cluster, so gateway-level auth is not required here.
	// Phase 2: if gRPC is ever exposed externally, add a UnaryServerInterceptor and
	// StreamServerInterceptor that validate a JWT passed in gRPC metadata.
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

	// Start HTTP server for WebSocket.
	// Route order: corsMiddleware runs first (OPTIONS → 200 before auth), then
	// requireAdminForLogs validates the JWT and admin claim before upgrading.
	mux := http.NewServeMux()
	mux.Handle("/ws/logs", corsMiddleware(requireAdminForLogs(func(w http.ResponseWriter, r *http.Request) {
		handleWebSocket(w, r, logServer)
	})))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	httpServer := &http.Server{
		Addr:    *httpAddr,
		Handler: mux,
	}
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

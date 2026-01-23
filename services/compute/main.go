package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"eddisonso.com/edd-cloud/pkg/events"
	"eddisonso.com/edd-cloud/services/compute/internal/api"
	"eddisonso.com/edd-cloud/services/compute/internal/db"
	eventshandler "eddisonso.com/edd-cloud/services/compute/internal/events"
	"eddisonso.com/edd-cloud/services/compute/internal/k8s"
	"eddisonso.com/go-gfs/pkg/gfslog"
)

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	logService := flag.String("log-service", "", "Log service address")
	flag.Parse()

	// Logger setup
	logger := gfslog.NewLogger(gfslog.Config{
		Source:         "edd-compute",
		LogServiceAddr: *logService,
		MinLevel:       slog.LevelDebug,
	})
	slog.SetDefault(logger.Logger)
	defer logger.Close()

	// Database connection string from environment
	dbConnStr := os.Getenv("DATABASE_URL")
	if dbConnStr == "" {
		dbConnStr = "postgres://localhost:5432/eddcloud?sslmode=disable"
	}

	database, err := db.Open(dbConnStr)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	// K8s client (in-cluster config)
	k8sClient, err := k8s.NewClient()
	if err != nil {
		slog.Error("failed to create k8s client", "error", err)
		os.Exit(1)
	}

	// Initialize user sync from auth-service
	authServiceURL := os.Getenv("AUTH_SERVICE_URL")
	if authServiceURL != "" {
		if err := eventshandler.SyncUsersFromAuthService(database, authServiceURL); err != nil {
			slog.Warn("initial user sync failed", "error", err)
		}
	}

	// Initialize NATS event consumer
	natsURL := os.Getenv("NATS_URL")
	var eventConsumer *events.Consumer
	if natsURL != "" {
		eventHandler := eventshandler.NewHandler(database, k8sClient)
		consumer, err := events.NewConsumer(events.ConsumerConfig{
			NatsURL:      natsURL,
			ConsumerName: "edd-compute",
			Handler:      eventHandler,
		})
		if err != nil {
			slog.Warn("failed to create NATS consumer, events will not be processed", "error", err)
		} else {
			eventConsumer = consumer
			if err := consumer.Start(context.Background()); err != nil {
				slog.Error("failed to start event consumer", "error", err)
			} else {
				slog.Info("NATS event consumer started")
			}
		}
	}

	// HTTP server with CORS
	apiHandler := api.NewHandler(database, k8sClient)
	server := &http.Server{Addr: *addr, Handler: corsMiddleware(apiHandler)}

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		slog.Info("shutting down")
		if eventConsumer != nil {
			eventConsumer.Stop()
		}
		server.Close()
	}()

	slog.Info("edd-compute listening", "addr", *addr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

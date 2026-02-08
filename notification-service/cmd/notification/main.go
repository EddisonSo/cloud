package main

import (
	"flag"
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"eddisonso.com/go-gfs/pkg/gfslog"
	"eddisonso.com/notification-service/internal/api"
	"eddisonso.com/notification-service/internal/consumer"
	"eddisonso.com/notification-service/internal/db"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	logServiceAddr := flag.String("log-service", "", "Log service address (e.g., log-service:50051)")
	flag.Parse()

	// Initialize logger
	if *logServiceAddr != "" {
		logger := gfslog.NewLogger(gfslog.Config{
			Source:         "edd-notifications",
			LogServiceAddr: *logServiceAddr,
			MinLevel:       slog.LevelInfo,
		})
		slog.SetDefault(logger.Logger)
		defer logger.Close()
	}

	// Get configuration from environment
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable required")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET environment variable required")
	}

	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://nats:4222"
	}

	// Connect to database
	database, err := db.Open(dbURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer database.Close()

	// Run migrations
	if err := database.Migrate(); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	// Create API handler
	handler := api.NewHandler(database, []byte(jwtSecret))

	// Create NATS consumer
	natsConsumer, err := consumer.New(natsURL, database, handler.BroadcastNotification)
	if err != nil {
		slog.Warn("failed to connect to NATS, notifications will not be consumed", "error", err)
	} else {
		if err := natsConsumer.Start(context.Background()); err != nil {
			slog.Error("failed to start NATS consumer", "error", err)
		} else {
			slog.Info("NATS consumer started")
		}
	}

	// Setup routes
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	server := &http.Server{
		Addr:    *addr,
		Handler: api.CORSMiddleware(api.LogRequests(mux)),
	}

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		slog.Info("shutting down")
		if natsConsumer != nil {
			natsConsumer.Stop()
		}
		server.Close()
	}()

	slog.Info("notification service listening", "addr", *addr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("server stopped: %v", err)
	}
}

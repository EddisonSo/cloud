package main

import (
	"flag"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"eddisonso.com/edd-cloud-auth/internal/api"
	"eddisonso.com/edd-cloud-auth/internal/db"
	"eddisonso.com/edd-cloud-auth/internal/events"
	"eddisonso.com/go-gfs/pkg/gfslog"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	sessionTTL := flag.Duration("session-ttl", 24*time.Hour, "Session lifetime")
	logServiceAddr := flag.String("log-service", "", "Log service address (e.g., log-service:50051)")
	flag.Parse()

	// Initialize logger
	if *logServiceAddr != "" {
		logger := gfslog.NewLogger(gfslog.Config{
			Source:         "edd-auth",
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

	adminUsername := os.Getenv("ADMIN_USERNAME")
	if adminUsername == "" {
		log.Fatal("ADMIN_USERNAME environment variable required")
	}

	defaultUsername := os.Getenv("DEFAULT_USERNAME")
	defaultPassword := os.Getenv("DEFAULT_PASSWORD")
	if defaultUsername == "" || defaultPassword == "" {
		log.Fatal("DEFAULT_USERNAME and DEFAULT_PASSWORD environment variables required")
	}

	serviceAPIKey := os.Getenv("SERVICE_API_KEY")
	if serviceAPIKey == "" {
		log.Fatal("SERVICE_API_KEY environment variable required")
	}

	natsURL := os.Getenv("NATS_URL")

	// Connect to database
	database, err := db.Open(dbURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer database.Close()

	// Create default user if not exists
	hash, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("failed to hash default password: %v", err)
	}
	if err := database.InitDefaultUser(defaultUsername, string(hash)); err != nil {
		log.Fatalf("failed to initialize default user: %v", err)
	}

	// Connect to NATS (optional)
	var publisher *events.Publisher
	if natsURL != "" {
		publisher, err = events.NewPublisher(natsURL, "auth-service")
		if err != nil {
			slog.Warn("failed to connect to NATS, events will not be published", "error", err)
		} else {
			defer publisher.Close()
			slog.Info("connected to NATS", "url", natsURL)
		}
	} else {
		slog.Info("NATS_URL not set, events will not be published")
	}

	// Create handler
	handler := api.NewHandler(api.Config{
		DB:            database,
		Publisher:     publisher,
		JWTSecret:     []byte(jwtSecret),
		SessionTTL:    *sessionTTL,
		AdminUser:     adminUsername,
		ServiceAPIKey: serviceAPIKey,
	})

	// Setup routes
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Add middleware
	finalHandler := corsMiddleware(logRequests(mux))

	slog.Info("starting auth service", "addr", *addr)
	if err := http.ListenAndServe(*addr, finalHandler); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Debug("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}

func isAllowedOrigin(origin string) bool {
	return origin == "https://cloud.eddisonso.com" ||
		(len(origin) > len("https://.cloud.eddisonso.com") &&
			strings.HasSuffix(origin, ".cloud.eddisonso.com") &&
			strings.HasPrefix(origin, "https://"))
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && isAllowedOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

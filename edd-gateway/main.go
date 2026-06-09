package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"eddisonso.com/edd-gateway/internal/api"
	"eddisonso.com/edd-gateway/internal/auth"
	"eddisonso.com/edd-gateway/internal/k8s"
	"eddisonso.com/edd-gateway/internal/proxy"
	"eddisonso.com/edd-gateway/internal/router"
	"eddisonso.com/edd-gateway/internal/tlsmgr"
	"eddisonso.com/go-gfs/pkg/gfslog"
	"gopkg.in/yaml.v3"
)

// ready indicates whether the gateway is fully initialized
var ready atomic.Bool

type routeConfig struct {
	Routes []struct {
		Host        string `yaml:"host"`
		Path        string `yaml:"path"`
		Target      string `yaml:"target"`
		StripPrefix bool   `yaml:"strip_prefix"`
	} `yaml:"routes"`
}

func main() {
	sshPort := flag.Int("ssh-port", 22, "SSH proxy port")
	httpPort := flag.Int("http-port", 80, "HTTP proxy port")
	httpsPort := flag.Int("https-port", 443, "HTTPS/TLS proxy port")
	fallbackAddr := flag.String("fallback", "", "Fallback upstream for non-container traffic (e.g., 192.168.3.150)")
	logService := flag.String("log-service", "", "Log service address")
	tlsCert := flag.String("tls-cert", "", "TLS certificate file for TLS termination")
	tlsKey := flag.String("tls-key", "", "TLS private key file for TLS termination")
	flag.Parse()

	// Logger setup
	logger := gfslog.NewLogger(gfslog.Config{
		Source:         "edd-gateway",
		LogServiceAddr: *logService,
		MinLevel:       slog.LevelInfo,
	})
	slog.SetDefault(logger.Logger)
	defer logger.Close()

	// Initialize SSH client key from K8s Secret
	if err := k8s.InitClientKey(); err != nil {
		slog.Error("failed to initialize SSH client key", "error", err)
		os.Exit(1)
	}

	// Database connection string from environment
	dbConnStr := os.Getenv("DATABASE_URL")
	if dbConnStr == "" {
		dbConnStr = "postgres://localhost:5432/eddcloud?sslmode=require"
	}

	// Router for container lookups
	r, err := router.New(dbConnStr)
	if err != nil {
		slog.Error("failed to create router", "error", err)
		os.Exit(1)
	}
	defer r.Close()

	// Load routes from routes.yaml
	routesFile := os.Getenv("ROUTES_FILE")
	if routesFile == "" {
		routesFile = "routes.yaml"
	}
	if data, err := os.ReadFile(routesFile); err == nil {
		var cfg routeConfig
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			slog.Error("failed to parse routes.yaml", "error", err)
		} else {
			for _, rt := range cfg.Routes {
				if err := r.RegisterRoute(rt.Host, rt.Path, rt.Target, rt.StripPrefix); err != nil {
					slog.Warn("failed to register route", "host", rt.Host, "path", rt.Path, "error", err)
				} else {
					slog.Info("registered route", "host", rt.Host, "path", rt.Path, "target", rt.Target)
				}
			}
		}
	} else {
		slog.Debug("no routes.yaml found, skipping static routes", "path", routesFile)
	}

	// Route the management API host to the loopback API server.
	if err := r.RegisterRoute("net.cloud.eddisonso.com", "/", "127.0.0.1:9092", false); err != nil {
		slog.Error("failed to register management API route", "error", err)
	}

	// Create proxy server
	srv := proxy.NewServer(r, *fallbackAddr)

	var tlsMgr *tlsmgr.Manager
	// Load TLS certificate for termination if provided
	if *tlsCert != "" && *tlsKey != "" {
		if err := srv.LoadTLSCert(*tlsCert, *tlsKey); err != nil {
			slog.Error("failed to load TLS certificate", "error", err)
			os.Exit(1)
		}
		slog.Info("TLS termination enabled")

		tlsMgr, err = tlsmgr.New(dbConnStr, r)
		if err != nil {
			slog.Error("failed to init TLS manager", "error", err)
			os.Exit(1)
		}
		srv.EnableOnDemandTLS(tlsMgr.GetCertificate)
		slog.Info("on-demand TLS enabled")
	}

	// Start SSH listener
	go func() {
		if err := srv.ListenSSH(*sshPort); err != nil {
			slog.Error("SSH listener failed", "error", err)
		}
	}()

	// Start standard HTTP/TLS listeners
	go func() {
		if err := srv.ListenHTTP(*httpPort); err != nil {
			slog.Error("HTTP listener failed", "error", err)
		}
	}()

	go func() {
		if err := srv.ListenTLS(*httpsPort); err != nil {
			slog.Error("TLS listener failed", "error", err)
		}
	}()

	// Start multi-protocol listeners on all allowed ingress ports (8000-8999)
	for port := 8000; port <= 8999; port++ {
		p := port
		go func() {
			if err := srv.ListenMulti(p); err != nil {
				slog.Error("multi listener failed", "port", p, "error", err)
			}
		}()
	}

	// Start health check server
	go startHealthServer(9091)

	// Management API on loopback; exposed via the net.cloud.eddisonso.com static route.
	validator := auth.NewSessionValidator()
	var preIssue func(string)
	if tlsMgr != nil {
		preIssue = tlsMgr.PreIssue
	}
	apiSrv := api.New(r, validator, newDomainID, preIssue)
	go func() {
		slog.Info("management API listening", "addr", "127.0.0.1:9092")
		if err := http.ListenAndServe("127.0.0.1:9092", apiSrv.Handler()); err != nil {
			// A bind failure means net.cloud.eddisonso.com would 502 while the pod
			// still reports ready — fail fast so the deployment surfaces it.
			slog.Error("management API failed", "error", err)
			os.Exit(1)
		}
	}()

	// DNS-TXT verification worker.
	worker := api.NewVerifyWorker(r, preIssue, 0)
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	defer cancelWorker()
	go worker.Run(workerCtx)

	// Mark as ready
	ready.Store(true)

	slog.Info("gateway started", "ssh", *sshPort, "http", *httpPort, "https", *httpsPort, "extra_ports", "8000-8999", "health_port", 9091)

	// Wait for shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	slog.Info("gateway shutting down")
	srv.Close()
}

// newDomainID returns a unique id for a custom_domains row.
func newDomainID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "cd_" + hex.EncodeToString(b)
}

// startHealthServer starts a simple HTTP server for health checks
func startHealthServer(port int) {
	mux := http.NewServeMux()

	// /healthz - always returns 200 if the process is alive
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// /readyz - returns 200 only when fully initialized
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if ready.Load() {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ready"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("not ready"))
		}
	})

	addr := fmt.Sprintf(":%d", port)
	slog.Info("health server listening", "port", port)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("health server failed", "error", err)
	}
}

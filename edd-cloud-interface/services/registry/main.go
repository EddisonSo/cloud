package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	gfs "eddisonso.com/go-gfs/pkg/go-gfs-sdk"
	notifypub "eddisonso.com/notification-service/pkg/publisher"
	_ "github.com/lib/pq"
)

const gfsNamespace = "core-registry"

type server struct {
	gfs       *gfs.Client
	db        *sql.DB
	jwtSecret []byte
	notifier  *notifypub.Publisher
}

func main() {
	addr := flag.String("addr", "0.0.0.0:8080", "listen address")
	master := flag.String("master", "gfs-master:9000", "GFS master address")
	flag.Parse()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("failed to ping database: %v", err)
	}
	defer db.Close()

	if err := initDB(db); err != nil {
		log.Fatalf("failed to init database: %v", err)
	}

	ctx := context.Background()
	gfsClient, err := gfs.New(ctx, *master,
		gfs.WithConnectionPool(8, 60*time.Second),
		gfs.WithUploadBufferSize(64*1024),
	)
	if err != nil {
		log.Fatalf("failed to connect to gfs master: %v", err)
	}
	defer gfsClient.Close()

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET is required")
	}

	srv := &server{
		gfs:       gfsClient,
		db:        db,
		jwtSecret: []byte(jwtSecret),
	}

	natsURL := os.Getenv("NATS_URL")
	if natsURL != "" {
		np, err := notifypub.New(natsURL, "edd-registry")
		if err != nil {
			slog.Warn("failed to create notification publisher", "error", err)
		} else {
			srv.notifier = np
			defer np.Close()
		}
	}

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	gcCtx, gcCancel := context.WithCancel(context.Background())
	defer gcCancel()
	srv.startGC(gcCtx, 24*time.Hour)

	httpServer := &http.Server{
		Addr:    *addr,
		Handler: mux,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("shutting down")
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		httpServer.Shutdown(shutCtx)
	}()

	slog.Info("starting registry", "addr", *addr)
	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func (s *server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v2/", s.routeV2)
}

func (s *server) routeV2(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// GET /v2/ — API version check
	if path == "/v2/" && r.Method == http.MethodGet {
		w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
		w.WriteHeader(http.StatusOK)
		return
	}

	// GET /v2/_catalog — repository catalog
	if path == "/v2/_catalog" {
		if r.Method == http.MethodGet {
			s.handleCatalog(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	// Upload session routes (must be checked before generic blobs route)
	// PATCH or PUT /v2/{name}/blobs/uploads/{uuid}
	if strings.Contains(path, "/blobs/uploads/") {
		// Determine if there is a UUID segment after /blobs/uploads/
		idx := strings.LastIndex(path, "/blobs/uploads/")
		rest := path[idx+len("/blobs/uploads/"):]
		if rest != "" {
			// Has a UUID
			switch r.Method {
			case http.MethodPatch:
				s.handleUploadPatch(w, r)
			case http.MethodPut:
				s.handleUploadComplete(w, r)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
	}

	// POST /v2/{name}/blobs/uploads/ (or /blobs/uploads without trailing slash)
	if strings.HasSuffix(path, "/blobs/uploads/") || strings.HasSuffix(path, "/blobs/uploads") {
		if r.Method == http.MethodPost {
			s.handleUploadStart(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	// HEAD/GET/DELETE /v2/{name}/blobs/{digest}
	if strings.Contains(path, "/blobs/") {
		switch r.Method {
		case http.MethodHead:
			s.handleBlobHead(w, r)
		case http.MethodGet:
			s.handleBlobGet(w, r)
		case http.MethodDelete:
			s.handleBlobDelete(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	// Tags list: GET /v2/{name}/tags/list
	if strings.HasSuffix(path, "/tags/list") {
		s.handleTagsList(w, r)
		return
	}

	// Manifests: /v2/{name}/manifests/{reference}
	if strings.Contains(path, "/manifests/") {
		switch r.Method {
		case http.MethodHead:
			s.handleManifestHead(w, r)
		case http.MethodGet:
			s.handleManifestGet(w, r)
		case http.MethodPut:
			s.handleManifestPut(w, r)
		case http.MethodDelete:
			s.handleManifestDelete(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	http.NotFound(w, r)
}


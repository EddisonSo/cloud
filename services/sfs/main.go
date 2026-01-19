package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	gfs "eddisonso.com/go-gfs/pkg/go-gfs-sdk"
	"eddisonso.com/go-gfs/pkg/gfslog"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/net/websocket"
	_ "github.com/lib/pq"
)

type fileInfo struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Namespace  string `json:"namespace"`
	Size       uint64 `json:"size"`
	CreatedAt  int64  `json:"created_at"`
	ModifiedAt int64  `json:"modified_at"`
}

type server struct {
	client     *gfs.Client
	prefix     string
	staticDir  string
	maxUpload  int64
	listPrefix string
	uploadTTL  time.Duration
	db         *sql.DB
	jwtSecret  []byte
	sessionTTL time.Duration
	wsMu       sync.Mutex
	wsConns    map[string]*websocket.Conn
	sseMu      sync.Mutex
	sseConns   map[string]chan progressMessage
}

// JWTClaims represents the claims in a JWT token
type JWTClaims struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	UserID      int64  `json:"user_id"`
	jwt.RegisteredClaims
}

const (
	defaultNamespace = "default"
	hiddenNamespace  = "hidden"
)

type namespaceInfo struct {
	Name    string `json:"name"`
	Count   int    `json:"count"`
	Hidden  bool   `json:"hidden"`
	OwnerID *int   `json:"owner_id,omitempty"`
}

// getJWTSecret returns the JWT signing secret from environment variable
func getJWTSecret() []byte {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		// Generate a random secret if not provided (not recommended for production)
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			log.Fatal("failed to generate JWT secret:", err)
		}
		log.Println("WARNING: JWT_SECRET not set, using random secret (sessions won't persist across restarts)")
		return b
	}
	return []byte(secret)
}

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	master := flag.String("master", "127.0.0.1:50051", "GFS master gRPC address")
	prefix := flag.String("prefix", "/sfs", "GFS namespace prefix for simple file store")
	staticDir := flag.String("static", "frontend", "path to frontend assets")
	maxUploadMB := flag.Int64("max-upload-mb", 0, "max upload size in MB (0 = unlimited)")
	uploadTTL := flag.Duration("upload-timeout", 10*time.Minute, "max time allowed for a single upload")
	// authDB flag kept for backwards compatibility but DATABASE_URL takes precedence
	sessionTTL := flag.Duration("session-ttl", 24*time.Hour, "session lifetime")
	logServiceAddr := flag.String("log-service", "", "Log service address (e.g., log-service:50051)")
	logSource := flag.String("log-source", "edd-cloud-interface", "Log source name (e.g., pod name)")
	flag.Parse()

	// Initialize logger
	if *logServiceAddr != "" {
		logger := gfslog.NewLogger(gfslog.Config{
			Source:         *logSource,
			LogServiceAddr: *logServiceAddr,
			MinLevel:       slog.LevelInfo,
		})
		slog.SetDefault(logger.Logger)
		defer logger.Close()
	}

	cleanPrefix := normalizePrefix(*prefix)
	if cleanPrefix == "/" {
		log.Fatal("prefix cannot be root")
	}

	ctx := context.Background()
	client, err := gfs.New(ctx, *master)
	if err != nil {
		log.Fatalf("failed to connect to gfs master: %v", err)
	}
	defer client.Close()

	absStatic, err := filepath.Abs(*staticDir)
	if err != nil {
		log.Fatalf("failed to resolve static path: %v", err)
	}

	defaultUsername := strings.TrimSpace(os.Getenv("DEFAULT_USERNAME"))
	defaultPassword := os.Getenv("DEFAULT_PASSWORD")
	if defaultUsername == "" || defaultPassword == "" {
		log.Fatal("missing DEFAULT_USERNAME or DEFAULT_PASSWORD")
	}

	dbConnStr := os.Getenv("DATABASE_URL")
	if dbConnStr == "" {
		dbConnStr = "postgres://localhost:5432/eddcloud?sslmode=disable"
	}
	db, err := sql.Open("postgres", dbConnStr)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("failed to ping database: %v", err)
	}
	defer db.Close()
	if err := initAuthDB(db, defaultUsername, defaultPassword); err != nil {
		log.Fatalf("failed to init auth db: %v", err)
	}

	srv := &server{
		client:     client,
		prefix:     cleanPrefix,
		staticDir:  absStatic,
		maxUpload:  maxUploadBytes(*maxUploadMB),
		listPrefix: "",
		uploadTTL:  *uploadTTL,
		db:         db,
		jwtSecret:  getJWTSecret(),
		sessionTTL: *sessionTTL,
		wsConns:    make(map[string]*websocket.Conn),
		sseConns:   make(map[string]chan progressMessage),
	}

	mux := http.NewServeMux()
	// Auth endpoints
	mux.HandleFunc("/api/login", srv.handleLogin)
	mux.HandleFunc("/api/logout", srv.handleLogout)
	mux.HandleFunc("/api/session", srv.handleSession)
	// Storage endpoints
	mux.HandleFunc("/storage/namespaces", srv.handleNamespaces)
	mux.HandleFunc("DELETE /storage/namespaces/{name}", srv.handleNamespaceDeleteByPath)
	mux.HandleFunc("PUT /storage/namespaces/{name}", srv.handleNamespaceUpdateByPath)
	mux.HandleFunc("/storage/files", srv.handleList)
	mux.HandleFunc("/storage/upload", srv.handleUpload)
	mux.HandleFunc("/storage/download", srv.handleDownload)
	mux.HandleFunc("/storage/delete", srv.handleDelete)
	mux.HandleFunc("GET /storage/download/{namespace}/{file...}", srv.handleFileDownload)
	mux.HandleFunc("GET /storage/{namespace}/{file...}", srv.handleFileGet)
	// Admin endpoints
	mux.HandleFunc("/admin/files", srv.handleAdminFiles)
	mux.HandleFunc("/admin/namespaces", srv.handleAdminNamespaces)
	mux.HandleFunc("/admin/users", srv.handleAdminUsers)
	mux.HandleFunc("/admin/sessions", srv.handleAdminSessions)
	mux.Handle("/ws", websocket.Handler(srv.handleWS))
	mux.HandleFunc("/sse/progress", srv.handleSSE)
	mux.Handle("/", srv.staticHandler())

	log.Printf("listening on %s", *addr)
	log.Printf("serving frontend from %s", srv.staticDir)
		log.Printf("sharing files under namespace prefix %s", srv.prefix)
	if err := http.ListenAndServe(*addr, corsMiddleware(logRequests(mux))); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

func normalizePrefix(prefix string) string {
	trimmed := strings.TrimSpace(prefix)
	if trimmed == "" {
		return "/shared"
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	return strings.TrimSuffix(trimmed, "/")
}

func initAuthDB(db *sql.DB, username string, password string) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			display_name TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id SERIAL PRIMARY KEY,
			user_id INTEGER NOT NULL,
			token TEXT NOT NULL UNIQUE,
			expires_at BIGINT NOT NULL,
			CONSTRAINT fk_user FOREIGN KEY(user_id) REFERENCES users(id)
		)`,
		`CREATE TABLE IF NOT EXISTS namespaces (
			name TEXT PRIMARY KEY,
			hidden INTEGER NOT NULL DEFAULT 0,
			owner_id INTEGER REFERENCES users(id)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	// Migration: add display_name column if it doesn't exist
	_, _ = db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS display_name TEXT NOT NULL DEFAULT ''`)

	// Migration: add owner_id column to namespaces if it doesn't exist
	_, _ = db.Exec(`ALTER TABLE namespaces ADD COLUMN IF NOT EXISTS owner_id INTEGER REFERENCES users(id)`)

	// Migration: add created_at column to sessions if it doesn't exist
	_, _ = db.Exec(`ALTER TABLE sessions ADD COLUMN IF NOT EXISTS created_at BIGINT NOT NULL DEFAULT 0`)

	// Migration: add ip_address column to sessions if it doesn't exist
	_, _ = db.Exec(`ALTER TABLE sessions ADD COLUMN IF NOT EXISTS ip_address TEXT NOT NULL DEFAULT ''`)

	var count int
	if err := db.QueryRow(`SELECT COUNT(1) FROM users`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if _, err := db.Exec(
			`INSERT INTO users (username, password_hash, display_name) VALUES ($1, $2, $3)`,
			username,
			string(hash),
			username, // Default display_name to username
		); err != nil {
			return err
		}
	}

	if err := ensureNamespaceRow(db, defaultNamespace, false); err != nil {
		return err
	}
	if err := ensureNamespaceRow(db, hiddenNamespace, true); err != nil {
		return err
	}
	return nil
}

func ensureNamespaceRow(db *sql.DB, name string, hidden bool) error {
	hiddenValue := 0
	if hidden {
		hiddenValue = 1
	}
	_, err := db.Exec(
		`INSERT INTO namespaces (name, hidden) VALUES ($1, $2)
		 ON CONFLICT(name) DO NOTHING`,
		name,
		hiddenValue,
	)
	return err
}

func (s *server) handleList(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	namespaceParam := strings.TrimSpace(r.URL.Query().Get("namespace"))
	namespace := ""
	if namespaceParam != "" {
		var err error
		namespace, err = sanitizeNamespace(namespaceParam)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !s.canAccessNamespace(r, namespace) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	} else {
		namespace = defaultNamespace
	}

	files, err := s.client.ListFilesWithNamespace(ctx, s.gfsNamespace(namespace), s.listPrefix)
	if err != nil {
		http.Error(w, fmt.Sprintf("list files failed: %v", err), http.StatusBadGateway)
		return
	}

	resp := make([]fileInfo, 0, len(files))
	for _, file := range files {
		relative := relativeNameWithPrefix(file.Path, s.listPrefix)
		if relative == "" {
			continue
		}
		name := relative
		resp = append(resp, fileInfo{
			Name:       name,
			Path:       file.Path,
			Namespace:  namespace,
			Size:       file.Size,
			CreatedAt:  file.CreatedAt,
			ModifiedAt: file.ModifiedAt,
		})
	}

	writeJSON(w, resp)
}

func (s *server) handleNamespaces(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleNamespaceList(w, r)
	case http.MethodPost:
		s.handleNamespaceCreate(w, r)
	case http.MethodDelete:
		s.handleNamespaceDelete(w, r)
	case http.MethodPatch:
		s.handleNamespaceUpdate(w, r)
	default:
		w.Header().Set("Allow", "GET, POST, DELETE, PATCH")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *server) handleNamespaceList(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	currentUserID, _ := s.currentUserID(r)
	namespaceRows, err := s.loadAllNamespaces()
	if err != nil {
		http.Error(w, "failed to load namespaces", http.StatusInternalServerError)
		return
	}

	// Build map for quick lookup
	nsMap := make(map[string]namespaceInfo)
	for _, entry := range namespaceRows {
		// Hidden namespaces: only show to owner
		if entry.Hidden {
			if entry.OwnerID == nil || *entry.OwnerID != currentUserID {
				continue
			}
		}
		count, err := s.countNamespaceFiles(ctx, entry.Name)
		if err != nil {
			http.Error(w, "failed to list namespace files", http.StatusBadGateway)
			return
		}
		entry.Count = count
		nsMap[entry.Name] = entry
	}

	// Add default namespace if not present
	if _, ok := nsMap[defaultNamespace]; !ok {
		count, err := s.countNamespaceFiles(ctx, defaultNamespace)
		if err != nil {
			http.Error(w, "failed to list namespace files", http.StatusBadGateway)
			return
		}
		nsMap[defaultNamespace] = namespaceInfo{
			Name:   defaultNamespace,
			Count:  count,
			Hidden: false,
		}
	}

	resp := make([]namespaceInfo, 0, len(nsMap))
	for _, ns := range nsMap {
		resp = append(resp, ns)
	}

	writeJSON(w, resp)
}

type namespaceCreateRequest struct {
	Name   string `json:"name"`
	Hidden bool   `json:"hidden"`
}

func (s *server) handleNamespaceCreate(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}

	var payload namespaceCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	name, err := sanitizeNamespace(payload.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if name == hiddenNamespace && !payload.Hidden {
		http.Error(w, "hidden namespace must be marked hidden", http.StatusBadRequest)
		return
	}

	if exists, err := s.namespaceExists(name); err != nil {
		http.Error(w, "failed to check namespace", http.StatusInternalServerError)
		return
	} else if exists {
		http.Error(w, "namespace already exists", http.StatusConflict)
		return
	}

	// Set owner for namespace
	var ownerID *int
	if uid, ok := s.currentUserID(r); ok {
		ownerID = &uid
	}

	if err := s.upsertNamespace(name, payload.Hidden, ownerID); err != nil {
		http.Error(w, "failed to save namespace", http.StatusInternalServerError)
		return
	}

	writeJSON(w, namespaceInfo{
		Name:    name,
		Count:   0,
		Hidden:  payload.Hidden,
		OwnerID: ownerID,
	})
}

func (s *server) handleNamespaceDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	name, err := sanitizeNamespace(r.URL.Query().Get("name"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check ownership for hidden namespaces
	if !s.canAccessNamespace(r, name) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	files, err := s.client.ListFilesWithNamespace(ctx, s.gfsNamespace(name), s.listPrefix)
	if err != nil {
		http.Error(w, fmt.Sprintf("list files failed: %v", err), http.StatusBadGateway)
		return
	}

	for _, file := range files {
		if err := s.client.DeleteFileWithNamespace(ctx, file.Path, s.gfsNamespace(name)); err != nil {
			http.Error(w, fmt.Sprintf("delete failed: %v", err), http.StatusBadGateway)
			return
		}
	}

	if err := s.deleteNamespace(name); err != nil {
		http.Error(w, "failed to delete namespace", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"status": "ok"})
}

type namespaceUpdateRequest struct {
	Name   string `json:"name"`
	Hidden bool   `json:"hidden"`
}

func (s *server) handleNamespaceUpdate(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}

	var payload namespaceUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	name, err := sanitizeNamespace(payload.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check ownership for hidden namespaces
	if !s.canAccessNamespace(r, name) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if name == hiddenNamespace && !payload.Hidden {
		http.Error(w, "hidden namespace must be marked hidden", http.StatusBadRequest)
		return
	}

	if err := s.updateNamespaceHidden(name, payload.Hidden); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, namespaceInfo{
		Name:   name,
		Hidden: payload.Hidden,
	})
}

// handleNamespaceDeleteByPath handles DELETE /storage/namespaces/{name}
func (s *server) handleNamespaceDeleteByPath(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}

	name, err := sanitizeNamespace(r.PathValue("name"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check ownership for hidden namespaces
	if !s.canAccessNamespace(r, name) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	files, err := s.client.ListFilesWithNamespace(ctx, s.gfsNamespace(name), s.listPrefix)
	if err != nil {
		http.Error(w, fmt.Sprintf("list files failed: %v", err), http.StatusBadGateway)
		return
	}

	for _, file := range files {
		if err := s.client.DeleteFileWithNamespace(ctx, file.Path, s.gfsNamespace(name)); err != nil {
			http.Error(w, fmt.Sprintf("delete failed: %v", err), http.StatusBadGateway)
			return
		}
	}

	if err := s.deleteNamespace(name); err != nil {
		http.Error(w, "failed to delete namespace", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"status": "ok"})
}

// handleNamespaceUpdateByPath handles PUT /storage/namespaces/{name}
func (s *server) handleNamespaceUpdateByPath(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}

	name, err := sanitizeNamespace(r.PathValue("name"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check ownership for hidden namespaces
	if !s.canAccessNamespace(r, name) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	var payload struct {
		Hidden bool `json:"hidden"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	if name == hiddenNamespace && !payload.Hidden {
		http.Error(w, "hidden namespace must be marked hidden", http.StatusBadRequest)
		return
	}

	if err := s.updateNamespaceHidden(name, payload.Hidden); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, namespaceInfo{
		Name:   name,
		Hidden: payload.Hidden,
	})
}

func (s *server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	start := time.Now()
	status := "ok"
	errDetail := ""
	name := ""
	namespace := defaultNamespace
	transferID := s.transferID(r)
	var total int64
	defer func() {
		duration := time.Since(start).Truncate(time.Millisecond)
		if errDetail == "" {
			log.Printf(
				"upload %s namespace=%s name=%s size=%d transfer=%s duration=%s",
				status,
				namespace,
				name,
				total,
				transferID,
				duration,
			)
		} else {
			log.Printf(
				"upload %s namespace=%s name=%s size=%d transfer=%s duration=%s err=%s",
				status,
				namespace,
				name,
				total,
				transferID,
				duration,
				errDetail,
			)
		}
	}()
	fail := func(message string, code int) {
		status = "error"
		errDetail = message
		http.Error(w, message, code)
	}

	if s.maxUpload > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, s.maxUpload)
	}
	mr, err := r.MultipartReader()
	if err != nil {
		fail("invalid multipart upload", http.StatusBadRequest)
		return
	}

	var file io.Reader
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			fail("invalid multipart upload", http.StatusBadRequest)
			return
		}
		if part.FormName() != "file" {
			part.Close()
			continue
		}
		filename, err := sanitizeName(part.FileName())
		if err != nil {
			part.Close()
			fail(err.Error(), http.StatusBadRequest)
			return
		}
		name = filename
		file = part
		break
	}

	if file == nil {
		fail("missing file", http.StatusBadRequest)
		return
	}

	if rawNamespace := strings.TrimSpace(r.URL.Query().Get("namespace")); rawNamespace != "" {
		var err error
		namespace, err = sanitizeNamespace(rawNamespace)
		if err != nil {
			fail(err.Error(), http.StatusBadRequest)
			return
		}
	}

	// Check that namespace exists before allowing upload
	exists, err := s.namespaceExists(namespace)
	if err != nil {
		fail("failed to verify namespace", http.StatusInternalServerError)
		return
	}
	if !exists {
		fail("namespace does not exist", http.StatusNotFound)
		return
	}

	fullPath := name
	overwrite := r.URL.Query().Get("overwrite") == "true"
	ctx, cancel := context.WithTimeout(r.Context(), s.uploadTTL)
	defer cancel()
	defer func() {
		if ctx.Err() != nil {
			log.Printf(
				"upload context done namespace=%s name=%s transfer=%s err=%v",
				namespace,
				name,
				transferID,
				ctx.Err(),
			)
		}
	}()

	// Check if file exists
	existingFile, err := s.client.GetFileWithNamespace(ctx, fullPath, s.gfsNamespace(namespace))
	fileExists := err == nil && existingFile != nil

	if fileExists {
		if !overwrite {
			fail(fmt.Sprintf("file already exists: %s", fullPath), http.StatusConflict)
			return
		}
		// Delete existing file before overwriting
		if err := s.client.DeleteFileWithNamespace(ctx, fullPath, s.gfsNamespace(namespace)); err != nil {
			fail(fmt.Sprintf("failed to delete existing file: %v", err), http.StatusInternalServerError)
			return
		}
		log.Printf("upload overwrite namespace=%s name=%s transfer=%s", namespace, name, transferID)
	}

	// Create new file
	if _, err := s.client.CreateFileWithNamespace(ctx, fullPath, s.gfsNamespace(namespace)); err != nil {
		fail(fmt.Sprintf("prepare file failed: %v", err), http.StatusBadGateway)
		return
	}

	total = s.parseSizeHeader(r.Header.Get("X-File-Size"))
	reporter := s.newReporter(transferID, "upload", total)
	log.Printf(
		"upload start namespace=%s name=%s size=%d transfer=%s gfs_namespace=%s",
		namespace,
		name,
		total,
		transferID,
		s.gfsNamespace(namespace),
	)

	// Send immediate "started" progress so UI shows activity right away
	reporter.Update(0)

	// Wrap file reader to track progress as HTTP data is received
	counting := &countingReader{reader: file, reporter: reporter}

	// Use AppendFrom directly - allocates chunks on-demand for faster start
	if _, err := s.client.AppendFromWithNamespace(ctx, fullPath, s.gfsNamespace(namespace), counting); err != nil {
		reporter.Error(err)
		log.Printf(
			"upload append failed namespace=%s name=%s size=%d transfer=%s err=%v",
			namespace,
			name,
			total,
			transferID,
			err,
		)
		fail(fmt.Sprintf("upload failed: %v", err), http.StatusBadGateway)
		return
	}
	reporter.Done()
	log.Printf(
		"upload complete namespace=%s name=%s size=%d transfer=%s",
		namespace,
		name,
		total,
		transferID,
	)

	writeJSON(w, map[string]string{"status": "ok", "name": name})
}

func (s *server) handleDownload(w http.ResponseWriter, r *http.Request) {
	name, err := sanitizeName(r.URL.Query().Get("name"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	namespace := defaultNamespace
	if rawNamespace := strings.TrimSpace(r.URL.Query().Get("namespace")); rawNamespace != "" {
		namespace, err = sanitizeNamespace(rawNamespace)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !s.canAccessNamespace(r, namespace) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	fullPath := name
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	transferID := s.transferID(r)
	var total int64
	if transferID != "" {
		if info, err := s.client.GetFileWithNamespace(ctx, fullPath, s.gfsNamespace(namespace)); err == nil {
			total = int64(info.Size)
		}
	}

	reporter := s.newReporter(transferID, "download", total)
	counting := &countingWriter{writer: w, reporter: reporter}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))

	if _, err := s.client.ReadToWithNamespace(ctx, fullPath, s.gfsNamespace(namespace), counting); err != nil {
		reporter.Error(err)
		http.Error(w, fmt.Sprintf("download failed: %v", err), http.StatusBadGateway)
		return
	}
	reporter.Done()
}

// handleFileGet serves files via path: GET /storage/{namespace}/{file...}
func (s *server) handleFileGet(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	file := r.PathValue("file")

	// If no file specified, serve the SPA for frontend routing
	if namespace == "" || file == "" {
		indexPath := filepath.Join(s.staticDir, "index.html")
		http.ServeFile(w, r, indexPath)
		return
	}

	// URL-decode the file path to handle special characters
	file, err := url.PathUnescape(file)
	if err != nil {
		serveErrorPage(w, http.StatusBadRequest, "Bad Request",
			"The file path contains invalid characters.")
		return
	}

	namespace, err = sanitizeNamespace(namespace)
	if err != nil {
		serveErrorPage(w, http.StatusBadRequest, "Bad Request",
			"The namespace name is invalid. Namespaces can only contain letters, numbers, hyphens, underscores, and dots.")
		return
	}

	if !s.canAccessNamespace(r, namespace) {
		serveErrorPage(w, http.StatusUnauthorized, "Unauthorized",
			"You don't have permission to access this namespace. Please log in if this is a private namespace.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	// Detect content type from extension
	ext := filepath.Ext(file)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)

	if _, err := s.client.ReadToWithNamespace(ctx, file, s.gfsNamespace(namespace), w); err != nil {
		serveErrorPage(w, http.StatusNotFound, "File Not Found",
			fmt.Sprintf("The file \"%s\" was not found in namespace \"%s\".", file, namespace))
		return
	}
}

// handleFileDownload forces file download: GET /storage/download/{namespace}/{file...}
func (s *server) handleFileDownload(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	file := r.PathValue("file")

	if namespace == "" || file == "" {
		serveErrorPage(w, http.StatusBadRequest, "Bad Request",
			"The requested URL is incomplete. Please provide both a namespace and filename.")
		return
	}

	// URL-decode the file path to handle special characters
	file, err := url.PathUnescape(file)
	if err != nil {
		serveErrorPage(w, http.StatusBadRequest, "Bad Request",
			"The file path contains invalid characters.")
		return
	}

	namespace, err = sanitizeNamespace(namespace)
	if err != nil {
		serveErrorPage(w, http.StatusBadRequest, "Bad Request",
			"The namespace name is invalid. Namespaces can only contain letters, numbers, hyphens, underscores, and dots.")
		return
	}

	if !s.canAccessNamespace(r, namespace) {
		serveErrorPage(w, http.StatusUnauthorized, "Unauthorized",
			"You don't have permission to access this namespace. Please log in if this is a private namespace.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	ext := filepath.Ext(file)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(file)))

	if _, err := s.client.ReadToWithNamespace(ctx, file, s.gfsNamespace(namespace), w); err != nil {
		serveErrorPage(w, http.StatusNotFound, "File Not Found",
			fmt.Sprintf("The file \"%s\" was not found in namespace \"%s\".", file, namespace))
		return
	}
}

func (s *server) handleDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodDelete)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name, err := sanitizeName(r.URL.Query().Get("name"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	namespace := defaultNamespace
	if rawNamespace := strings.TrimSpace(r.URL.Query().Get("namespace")); rawNamespace != "" {
		namespace, err = sanitizeNamespace(rawNamespace)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	fullPath := name
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := s.client.DeleteFileWithNamespace(ctx, fullPath, s.gfsNamespace(namespace)); err != nil {
		http.Error(w, fmt.Sprintf("delete failed: %v", err), http.StatusBadGateway)
		return
	}

	writeJSON(w, map[string]string{"status": "ok", "name": name})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type sessionResponse struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	IsAdmin     bool   `json:"is_admin"`
	Token       string `json:"token,omitempty"`
}

var adminUsername = os.Getenv("ADMIN_USERNAME")

func isAdmin(username string) bool {
	return adminUsername != "" && username == adminUsername
}

func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload loginRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid login payload", http.StatusBadRequest)
		return
	}
	payload.Username = strings.TrimSpace(payload.Username)
	if payload.Username == "" || payload.Password == "" {
		http.Error(w, "username and password required", http.StatusBadRequest)
		return
	}

	var (
		userID      int64
		hash        string
		displayName string
	)
	err := s.db.QueryRow(`SELECT id, password_hash, COALESCE(display_name, username) FROM users WHERE username = $1`, payload.Username).
		Scan(&userID, &hash, &displayName)
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(payload.Password)); err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	// Fall back to username if display_name is empty
	if displayName == "" {
		displayName = payload.Username
	}

	// Generate JWT token
	expires := time.Now().Add(s.sessionTTL)
	claims := JWTClaims{
		Username:    payload.Username,
		DisplayName: displayName,
		UserID:      userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expires),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   payload.Username,
		},
	}
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := jwtToken.SignedString(s.jwtSecret)
	if err != nil {
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	// Store session in database for tracking (keep only latest per user)
	now := time.Now().Unix()
	clientIP := r.Header.Get("X-Forwarded-For")
	if clientIP == "" {
		clientIP = r.Header.Get("X-Real-IP")
	}
	if clientIP == "" {
		clientIP = r.RemoteAddr
	}
	// Take first IP if comma-separated
	if idx := strings.Index(clientIP, ","); idx != -1 {
		clientIP = strings.TrimSpace(clientIP[:idx])
	}
	// Remove port if present
	if idx := strings.LastIndex(clientIP, ":"); idx != -1 {
		if !strings.Contains(clientIP[idx:], "]") { // not IPv6
			clientIP = clientIP[:idx]
		}
	}
	_, _ = s.db.Exec(`DELETE FROM sessions WHERE user_id = $1`, userID)
	_, _ = s.db.Exec(`INSERT INTO sessions (user_id, token, expires_at, created_at, ip_address) VALUES ($1, $2, $3, $4, $5)`,
		userID, tokenString, expires.Unix(), now, clientIP)

	writeJSON(w, sessionResponse{
		Username:    payload.Username,
		DisplayName: displayName,
		IsAdmin:     isAdmin(payload.Username),
		Token:       tokenString,
	})
}

func (s *server) handleSession(w http.ResponseWriter, r *http.Request) {
	username, displayName, ok := s.currentUserWithDisplay(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, sessionResponse{Username: username, DisplayName: displayName, IsAdmin: isAdmin(username)})
}

func (s *server) handleLogout(w http.ResponseWriter, r *http.Request) {
	// With JWT, logout is handled client-side by removing the token
	// Server just acknowledges the request
	writeJSON(w, map[string]string{"status": "ok"})
}

// isSecureRequest checks if the request is over HTTPS (directly or via proxy)
func isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	// Check for proxy headers
	if r.Header.Get("X-Forwarded-Proto") == "https" {
		return true
	}
	// Check for Cloudflare header
	if r.Header.Get("CF-Visitor") != "" {
		return strings.Contains(r.Header.Get("CF-Visitor"), `"scheme":"https"`)
	}
	return false
}

// getCookieDomain extracts the parent domain for cookie sharing across subdomains
func getCookieDomain(r *http.Request) string {
	host := r.Host
	// Remove port if present
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	// For eddisonso.com subdomains, use .eddisonso.com
	if strings.HasSuffix(host, ".eddisonso.com") {
		return ".eddisonso.com"
	}
	// For localhost or other domains, don't set domain (use default)
	return ""
}

func (s *server) requireAuth(w http.ResponseWriter, r *http.Request) (string, bool) {
	username, ok := s.currentUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", false
	}
	return username, true
}

// parseJWT extracts and validates claims from a JWT token
func (s *server) parseJWT(tokenString string) (*JWTClaims, bool) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, false
	}
	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, false
	}
	return claims, true
}

func (s *server) currentUser(r *http.Request) (string, bool) {
	token := s.sessionToken(r)
	if token == "" {
		return "", false
	}
	claims, ok := s.parseJWT(token)
	if !ok {
		return "", false
	}
	return claims.Username, true
}

func (s *server) currentUserID(r *http.Request) (int, bool) {
	token := s.sessionToken(r)
	if token == "" {
		return 0, false
	}
	claims, ok := s.parseJWT(token)
	if !ok {
		return 0, false
	}
	return int(claims.UserID), true
}

func (s *server) currentUserWithDisplay(r *http.Request) (string, string, bool) {
	token := s.sessionToken(r)
	if token == "" {
		return "", "", false
	}
	claims, ok := s.parseJWT(token)
	if !ok {
		return "", "", false
	}
	displayName := claims.DisplayName
	// Fall back to username if display_name is empty
	if displayName == "" {
		displayName = claims.Username
	}
	return claims.Username, displayName, true
}

func (s *server) sessionToken(r *http.Request) string {
	// Check Authorization header first (Bearer token)
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	// Check query string token (for shareable download links)
	if token := r.URL.Query().Get("token"); token != "" {
		return token
	}
	return ""
}

func generateToken(length int) (string, error) {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (s *server) ensureEmptyFile(ctx context.Context, namespace, fullPath string) error {
	// Check if file already exists - reject if so
	if _, err := s.client.GetFileWithNamespace(ctx, fullPath, s.gfsNamespace(namespace)); err == nil {
		return fmt.Errorf("file already exists: %s", fullPath)
	}
	_, err := s.client.CreateFileWithNamespace(ctx, fullPath, s.gfsNamespace(namespace))
	return err
}

func (s *server) loadHiddenNamespaces() (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT name, hidden FROM namespaces`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hidden := make(map[string]bool)
	for rows.Next() {
		var name string
		var hiddenFlag int
		if err := rows.Scan(&name, &hiddenFlag); err != nil {
			return nil, err
		}
		if hiddenFlag != 0 {
			hidden[name] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return hidden, nil
}

func (s *server) loadAllNamespaces() ([]namespaceInfo, error) {
	rows, err := s.db.Query(`SELECT name, hidden, owner_id FROM namespaces`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var namespaces []namespaceInfo
	for rows.Next() {
		var name string
		var hiddenFlag int
		var ownerID *int
		if err := rows.Scan(&name, &hiddenFlag, &ownerID); err != nil {
			return nil, err
		}
		namespaces = append(namespaces, namespaceInfo{
			Name:    name,
			Hidden:  hiddenFlag != 0,
			OwnerID: ownerID,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return namespaces, nil
}

func (s *server) upsertNamespace(name string, hidden bool, ownerID *int) error {
	hiddenValue := 0
	if hidden {
		hiddenValue = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO namespaces (name, hidden, owner_id) VALUES ($1, $2, $3)
		 ON CONFLICT(name) DO UPDATE SET hidden = excluded.hidden`,
		name,
		hiddenValue,
		ownerID,
	)
	return err
}

func (s *server) deleteNamespace(name string) error {
	_, err := s.db.Exec(`DELETE FROM namespaces WHERE name = $1`, name)
	return err
}

func (s *server) gfsNamespace(namespace string) string {
	base := strings.TrimPrefix(s.prefix, "/")
	if base == "" {
		return namespace
	}
	return path.Join(base, namespace)
}

func (s *server) updateNamespaceHidden(name string, hidden bool) error {
	hiddenValue := 0
	if hidden {
		hiddenValue = 1
	}
	result, err := s.db.Exec(`UPDATE namespaces SET hidden = $1 WHERE name = $2`, hiddenValue, name)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated == 0 {
		return fmt.Errorf("namespace not found")
	}
	return nil
}

func (s *server) namespaceExists(name string) (bool, error) {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM namespaces WHERE name = $1`, name).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// canAccessNamespace checks if a user can access a namespace.
// Hidden namespaces are only accessible by their owner.
func (s *server) canAccessNamespace(r *http.Request, namespace string) bool {
	// Get namespace info
	var hidden int
	var ownerID *int
	err := s.db.QueryRow(
		`SELECT hidden, owner_id FROM namespaces WHERE name = $1`,
		namespace,
	).Scan(&hidden, &ownerID)
	if err != nil {
		// Namespace doesn't exist in DB - allow access (e.g., default namespace)
		return true
	}

	// Non-hidden namespaces are accessible to everyone
	if hidden == 0 {
		return true
	}

	// Hidden namespace: must be owner
	userID, ok := s.currentUserID(r)
	if !ok {
		return false
	}
	if ownerID == nil {
		return false
	}
	return *ownerID == userID
}

func (s *server) countNamespaceFiles(ctx context.Context, namespace string) (int, error) {
	files, err := s.client.ListFilesWithNamespace(ctx, s.gfsNamespace(namespace), s.listPrefix)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, file := range files {
		if relativeNameWithPrefix(file.Path, s.listPrefix) == "" {
			continue
		}
		count++
	}
	return count, nil
}

func relativeNameWithPrefix(fullPath, prefix string) string {
	if prefix == "" {
		return strings.TrimPrefix(fullPath, "/")
	}
	trimmed := strings.TrimPrefix(fullPath, prefix)
	trimmed = strings.TrimPrefix(trimmed, "/")
	if trimmed == "" || trimmed == fullPath {
		return ""
	}
	return trimmed
}

func splitNamespaceAndName(relative string) (string, string) {
	parts := strings.SplitN(relative, "/", 2)
	if len(parts) == 1 {
		return defaultNamespace, parts[0]
	}
	return parts[0], parts[1]
}

func maxUploadBytes(mb int64) int64 {
	if mb <= 0 {
		return 0
	}
	return mb * 1024 * 1024
}

type progressMessage struct {
	ID        string `json:"id"`
	Direction string `json:"direction"`
	Bytes     int64  `json:"bytes"`
	Total     int64  `json:"total"`
	Done      bool   `json:"done"`
	Error     string `json:"error,omitempty"`
}

type progressReporter struct {
	server      *server
	id          string
	direction   string
	total       int64
	lastBytes   int64
	lastSent    time.Time
	minBytes    int64
	minInterval time.Duration
}

func (s *server) newReporter(id, direction string, total int64) *progressReporter {
	// Scale minBytes with file size: target ~100 updates per file
	// Minimum 64KB, maximum 10MB between updates
	minBytes := total / 100
	if minBytes < 64*1024 {
		minBytes = 64 * 1024
	}
	if minBytes > 10*1024*1024 {
		minBytes = 10 * 1024 * 1024
	}
	return &progressReporter{
		server:      s,
		id:          id,
		direction:   direction,
		total:       total,
		lastSent:    time.Time{}, // Zero time so first update sends immediately
		minBytes:    minBytes,
		minInterval: 500 * time.Millisecond,
	}
}

func (p *progressReporter) Update(bytes int64) {
	if p == nil || p.id == "" {
		return
	}
	now := time.Now()
	// Always send first update immediately (even if 0 bytes)
	isFirst := p.lastSent.IsZero()
	if !isFirst && bytes-p.lastBytes < p.minBytes && now.Sub(p.lastSent) < p.minInterval {
		return
	}
	p.lastBytes = bytes
	p.lastSent = now
	p.server.sendProgress(progressMessage{
		ID:        p.id,
		Direction: p.direction,
		Bytes:     bytes,
		Total:     p.total,
	})
}

func (p *progressReporter) Done() {
	if p == nil || p.id == "" {
		return
	}
	p.server.sendProgress(progressMessage{
		ID:        p.id,
		Direction: p.direction,
		Bytes:     p.lastBytes,
		Total:     p.total,
		Done:      true,
	})
}

func (p *progressReporter) Error(err error) {
	if p == nil || p.id == "" {
		return
	}
	p.server.sendProgress(progressMessage{
		ID:        p.id,
		Direction: p.direction,
		Bytes:     p.lastBytes,
		Total:     p.total,
		Done:      true,
		Error:     err.Error(),
	})
}

type countingReader struct {
	reader   io.Reader
	reporter *progressReporter
	read     int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	if n > 0 {
		c.read += int64(n)
		c.reporter.Update(c.read)
	}
	if err == io.EOF {
		c.reporter.Update(c.read)
	}
	return n, err
}

type countingWriter struct {
	writer   io.Writer
	reporter *progressReporter
	wrote    int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.writer.Write(p)
	if n > 0 {
		c.wrote += int64(n)
		c.reporter.Update(c.wrote)
	}
	return n, err
}

func (s *server) handleWS(ws *websocket.Conn) {
	r := ws.Request()
	if _, ok := s.currentUser(r); !ok {
		log.Printf("ws auth failed origin=%s cookies=%d", r.Header.Get("Origin"), len(r.Cookies()))
		_ = ws.Close()
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		_ = ws.Close()
		return
	}
	log.Printf("ws connected id=%s", id)
	s.registerWS(id, ws)
	defer s.unregisterWS(id, ws)
	_, _ = io.Copy(io.Discard, ws)
}

func (s *server) registerWS(id string, conn *websocket.Conn) {
	s.wsMu.Lock()
	if prev := s.wsConns[id]; prev != nil && prev != conn {
		_ = prev.Close()
	}
	s.wsConns[id] = conn
	s.wsMu.Unlock()
}

func (s *server) unregisterWS(id string, conn *websocket.Conn) {
	s.wsMu.Lock()
	if current, ok := s.wsConns[id]; ok && current == conn {
		delete(s.wsConns, id)
	}
	s.wsMu.Unlock()
}

func (s *server) sendProgress(msg progressMessage) {
	if msg.ID == "" {
		return
	}
	// Send to websocket if connected
	s.wsMu.Lock()
	wsConn := s.wsConns[msg.ID]
	s.wsMu.Unlock()
	if wsConn != nil {
		_ = websocket.JSON.Send(wsConn, msg)
	}
	// Send to SSE if connected
	s.sseMu.Lock()
	sseChan := s.sseConns[msg.ID]
	s.sseMu.Unlock()
	if sseChan != nil {
		select {
		case sseChan <- msg:
		default:
			// Channel full, skip this update
		}
	}
}

func (s *server) handleSSE(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.currentUser(r); !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id parameter", http.StatusBadRequest)
		return
	}

	// Set SSE headers (CORS handled by middleware)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Create channel for this connection
	ch := make(chan progressMessage, 10)
	s.sseMu.Lock()
	s.sseConns[id] = ch
	s.sseMu.Unlock()

	log.Printf("sse connected id=%s", id)

	defer func() {
		s.sseMu.Lock()
		if current := s.sseConns[id]; current == ch {
			delete(s.sseConns, id)
		}
		s.sseMu.Unlock()
		close(ch)
		log.Printf("sse disconnected id=%s", id)
	}()

	// Send initial keepalive
	fmt.Fprintf(w, ": keepalive\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(msg)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			if msg.Done {
				return
			}
		}
	}
}

func (s *server) transferID(r *http.Request) string {
	if id := r.URL.Query().Get("id"); id != "" {
		return id
	}
	return r.Header.Get("X-Transfer-Id")
}

func (s *server) parseSizeHeader(raw string) int64 {
	if raw == "" {
		return 0
	}
	size, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || size < 0 {
		return 0
	}
	return size
}

func (s *server) staticHandler() http.Handler {
	fileServer := http.FileServer(http.Dir(s.staticDir))
	indexPath := filepath.Join(s.staticDir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// API routes should 404 if not handled by other handlers
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		// Check if file exists on disk
		filePath := filepath.Join(s.staticDir, filepath.Clean(r.URL.Path))
		if _, err := os.Stat(filePath); err == nil {
			// File exists, serve it
			fileServer.ServeHTTP(w, r)
			return
		}
		// File doesn't exist - serve index.html for SPA routing
		http.ServeFile(w, r, indexPath)
	})
}

func sanitizeName(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("filename required")
	}
	base := path.Base(trimmed)
	if base == "." || base == "/" || base == "" {
		return "", fmt.Errorf("invalid filename")
	}
	if base != trimmed || strings.Contains(base, "\\") {
		return "", fmt.Errorf("invalid filename")
	}
	return base, nil
}

func sanitizeNamespace(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("namespace required")
	}
	if strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\") {
		return "", fmt.Errorf("invalid namespace")
	}
	for _, r := range trimmed {
		if r > 127 {
			return "", fmt.Errorf("invalid namespace")
		}
		if !(r >= 'a' && r <= 'z' ||
			r >= 'A' && r <= 'Z' ||
			r >= '0' && r <= '9' ||
			r == '-' || r == '_' || r == '.') {
			return "", fmt.Errorf("invalid namespace")
		}
	}
	return trimmed, nil
}

// serveErrorPage renders a styled HTML error page
func serveErrorPage(w http.ResponseWriter, statusCode int, title, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%d %s - Edd Cloud</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #0d1117;
            color: #e6edf3;
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .container {
            text-align: center;
            padding: 2rem;
            max-width: 500px;
        }
        .status-code {
            font-size: 6rem;
            font-weight: 700;
            color: #58a6ff;
            line-height: 1;
            margin-bottom: 0.5rem;
        }
        .title {
            font-size: 1.5rem;
            font-weight: 600;
            margin-bottom: 1rem;
            color: #e6edf3;
        }
        .message {
            color: #8b949e;
            margin-bottom: 2rem;
            line-height: 1.5;
        }
        .home-link {
            display: inline-block;
            padding: 0.75rem 1.5rem;
            background: #21262d;
            color: #58a6ff;
            text-decoration: none;
            border-radius: 6px;
            border: 1px solid #30363d;
            transition: background 0.2s;
        }
        .home-link:hover {
            background: #30363d;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="status-code">%d</div>
        <h1 class="title">%s</h1>
        <p class="message">%s</p>
        <a href="/" class="home-link">Go to Homepage</a>
    </div>
</body>
</html>`, statusCode, title, statusCode, title, message)
	w.Write([]byte(html))
}

// Admin handlers

func (s *server) handleAdminFiles(w http.ResponseWriter, r *http.Request) {
	username, ok := s.currentUser(r)
	if !ok || !isAdmin(username) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Get all namespaces
	namespaces, err := s.loadAllNamespaces()
	if err != nil {
		http.Error(w, "failed to load namespaces", http.StatusInternalServerError)
		return
	}

	var allFiles []fileInfo
	for _, ns := range namespaces {
		files, err := s.client.ListFilesWithNamespace(ctx, s.gfsNamespace(ns.Name), s.listPrefix)
		if err != nil {
			log.Printf("failed to list files for namespace %s: %v", ns.Name, err)
			continue
		}
		for _, file := range files {
			relative := relativeNameWithPrefix(file.Path, s.listPrefix)
			if relative == "" {
				continue
			}
			allFiles = append(allFiles, fileInfo{
				Name:       relative,
				Path:       file.Path,
				Namespace:  ns.Name,
				Size:       file.Size,
				CreatedAt:  file.CreatedAt,
				ModifiedAt: file.ModifiedAt,
			})
		}
	}

	writeJSON(w, allFiles)
}

func (s *server) handleAdminNamespaces(w http.ResponseWriter, r *http.Request) {
	username, ok := s.currentUser(r)
	if !ok || !isAdmin(username) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	namespaces, err := s.loadAllNamespaces()
	if err != nil {
		http.Error(w, "failed to load namespaces", http.StatusInternalServerError)
		return
	}

	type adminNamespace struct {
		Name    string `json:"name"`
		Count   int    `json:"count"`
		Hidden  bool   `json:"hidden"`
		OwnerID *int   `json:"owner_id"`
	}

	result := make([]adminNamespace, 0, len(namespaces))
	for _, ns := range namespaces {
		count, _ := s.countNamespaceFiles(ctx, ns.Name)
		result = append(result, adminNamespace{
			Name:    ns.Name,
			Count:   count,
			Hidden:  ns.Hidden,
			OwnerID: ns.OwnerID,
		})
	}

	writeJSON(w, result)
}

func (s *server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	username, ok := s.currentUser(r)
	if !ok || !isAdmin(username) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleAdminUsersList(w, r)
	case http.MethodPost:
		s.handleAdminUsersCreate(w, r)
	case http.MethodDelete:
		s.handleAdminUsersDelete(w, r)
	default:
		w.Header().Set("Allow", "GET, POST, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type adminUser struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

func (s *server) handleAdminUsersList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`SELECT id, username, COALESCE(display_name, username) FROM users ORDER BY id`)
	if err != nil {
		http.Error(w, "failed to list users", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	users := make([]adminUser, 0)
	for rows.Next() {
		var u adminUser
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName); err != nil {
			http.Error(w, "failed to scan user", http.StatusInternalServerError)
			return
		}
		if u.DisplayName == "" {
			u.DisplayName = u.Username
		}
		users = append(users, u)
	}

	writeJSON(w, users)
}

type createUserRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

func (s *server) handleAdminUsersCreate(w http.ResponseWriter, r *http.Request) {
	var payload createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	payload.Username = strings.TrimSpace(payload.Username)
	payload.DisplayName = strings.TrimSpace(payload.DisplayName)
	if payload.Username == "" || payload.Password == "" {
		http.Error(w, "username and password required", http.StatusBadRequest)
		return
	}
	// Default display_name to username if not provided
	if payload.DisplayName == "" {
		payload.DisplayName = payload.Username
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(payload.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "failed to hash password", http.StatusInternalServerError)
		return
	}

	var id int64
	err = s.db.QueryRow(
		`INSERT INTO users (username, password_hash, display_name) VALUES ($1, $2, $3) RETURNING id`,
		payload.Username,
		string(hash),
		payload.DisplayName,
	).Scan(&id)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "UNIQUE") {
			http.Error(w, "username already exists", http.StatusConflict)
			return
		}
		http.Error(w, "failed to create user", http.StatusInternalServerError)
		return
	}

	writeJSON(w, adminUser{ID: id, Username: payload.Username, DisplayName: payload.DisplayName})
}

func (s *server) handleAdminUsersDelete(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	// Check if user exists and get username
	var targetUsername string
	err = s.db.QueryRow(`SELECT username FROM users WHERE id = $1`, id).Scan(&targetUsername)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	// Prevent deleting self
	currentUsername, _ := s.currentUser(r)
	if targetUsername == currentUsername {
		http.Error(w, "cannot delete yourself", http.StatusBadRequest)
		return
	}

	// Delete user's sessions first
	_, _ = s.db.Exec(`DELETE FROM sessions WHERE user_id = $1`, id)

	// Clear ownership of user's namespaces (they become inaccessible)
	_, _ = s.db.Exec(`UPDATE namespaces SET owner_id = NULL WHERE owner_id = $1`, id)

	// Delete user
	result, err := s.db.Exec(`DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		http.Error(w, "failed to delete user", http.StatusInternalServerError)
		return
	}

	deleted, _ := result.RowsAffected()
	if deleted == 0 {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	writeJSON(w, map[string]string{"status": "ok"})
}

type recentSession struct {
	UserID      int64  `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	CreatedAt   int64  `json:"created_at"`
	IPAddress   string `json:"ip_address"`
}

func (s *server) handleAdminSessions(w http.ResponseWriter, r *http.Request) {
	username, ok := s.currentUser(r)
	if !ok || !isAdmin(username) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get active sessions (not expired), most recent per user
	now := time.Now().Unix()
	rows, err := s.db.Query(`
		SELECT DISTINCT ON (s.user_id) s.user_id, u.username, COALESCE(u.display_name, u.username), s.created_at, COALESCE(s.ip_address, '')
		FROM sessions s
		JOIN users u ON s.user_id = u.id
		WHERE s.expires_at > $1
		ORDER BY s.user_id, s.created_at DESC
	`, now)
	if err != nil {
		http.Error(w, "failed to list sessions", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	sessions := make([]recentSession, 0)
	for rows.Next() {
		var sess recentSession
		if err := rows.Scan(&sess.UserID, &sess.Username, &sess.DisplayName, &sess.CreatedAt, &sess.IPAddress); err != nil {
			http.Error(w, "failed to scan session", http.StatusInternalServerError)
			return
		}
		sessions = append(sessions, sess)
	}

	writeJSON(w, sessions)
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	if err := enc.Encode(payload); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		duration := time.Since(start)
		log.Printf("%s %s %s", r.Method, r.URL.Path, duration.Round(time.Millisecond))
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		// Allow requests from cloud.eddisonso.com and localhost for dev
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-File-Size")
		}
		// Handle preflight
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

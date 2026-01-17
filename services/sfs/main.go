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
	cookieName string
	sessionTTL time.Duration
	wsMu       sync.Mutex
	wsConns    map[string]*websocket.Conn
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
		cookieName: "sfs_session",
		sessionTTL: *sessionTTL,
		wsConns:    make(map[string]*websocket.Conn),
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
	mux.Handle("/ws", websocket.Handler(srv.handleWS))
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

	fullPath := name
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

	if err := s.ensureEmptyFile(ctx, namespace, fullPath); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			fail(err.Error(), http.StatusConflict)
		} else {
			fail(fmt.Sprintf("prepare file failed: %v", err), http.StatusBadGateway)
		}
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

	// Use PrepareUpload when file size is known to pre-allocate chunks
	if total > 0 {
		prepared, err := s.client.PrepareUploadWithNamespace(ctx, fullPath, s.gfsNamespace(namespace), total)
		if err != nil {
			reporter.Error(err)
			log.Printf(
				"upload prepare failed namespace=%s name=%s size=%d transfer=%s err=%v",
				namespace,
				name,
				total,
				transferID,
				err,
			)
			fail(fmt.Sprintf("prepare upload failed: %v", err), http.StatusBadGateway)
			return
		}
		// Track progress based on bytes actually written to GFS (not bytes read from HTTP)
		prepared.OnProgress(func(bytesWritten int64) {
			reporter.Update(bytesWritten)
		})
		if _, err := prepared.AppendFrom(ctx, file); err != nil {
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
	} else {
		// Fallback to regular append when size is unknown (track read progress)
		counting := &countingReader{reader: file, reporter: reporter}
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

	if namespace == "" || file == "" {
		http.Error(w, "namespace and file required", http.StatusBadRequest)
		return
	}

	// URL-decode the file path to handle special characters
	file, err := url.PathUnescape(file)
	if err != nil {
		http.Error(w, "invalid file path", http.StatusBadRequest)
		return
	}

	namespace, err = sanitizeNamespace(namespace)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !s.canAccessNamespace(r, namespace) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
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
		http.Error(w, fmt.Sprintf("file not found: %v", err), http.StatusNotFound)
		return
	}
}

// handleFileDownload forces file download: GET /storage/download/{namespace}/{file...}
func (s *server) handleFileDownload(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	file := r.PathValue("file")

	if namespace == "" || file == "" {
		http.Error(w, "namespace and file required", http.StatusBadRequest)
		return
	}

	// URL-decode the file path to handle special characters
	file, err := url.PathUnescape(file)
	if err != nil {
		http.Error(w, "invalid file path", http.StatusBadRequest)
		return
	}

	namespace, err = sanitizeNamespace(namespace)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !s.canAccessNamespace(r, namespace) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
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
		http.Error(w, fmt.Sprintf("file not found: %v", err), http.StatusNotFound)
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

	token, err := generateToken(32)
	if err != nil {
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}
	expires := time.Now().Add(s.sessionTTL)
	if _, err := s.db.Exec(
		`INSERT INTO sessions (user_id, token, expires_at) VALUES ($1, $2, $3)`,
		userID,
		token,
		expires.Unix(),
	); err != nil {
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	})
	writeJSON(w, sessionResponse{Username: payload.Username, DisplayName: displayName, IsAdmin: isAdmin(payload.Username)})
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
	token := s.sessionToken(r)
	if token != "" {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE token = $1`, token)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	})
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *server) requireAuth(w http.ResponseWriter, r *http.Request) (string, bool) {
	username, ok := s.currentUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", false
	}
	return username, true
}

func (s *server) currentUser(r *http.Request) (string, bool) {
	token := s.sessionToken(r)
	if token == "" {
		return "", false
	}

	var (
		username  string
		expiresAt int64
	)
	err := s.db.QueryRow(
		`SELECT users.username, sessions.expires_at
		 FROM sessions
		 JOIN users ON sessions.user_id = users.id
		 WHERE sessions.token = $1`,
		token,
	).Scan(&username, &expiresAt)
	if err != nil {
		return "", false
	}
	if time.Now().Unix() > expiresAt {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE token = $1`, token)
		return "", false
	}
	return username, true
}

func (s *server) currentUserID(r *http.Request) (int, bool) {
	token := s.sessionToken(r)
	if token == "" {
		return 0, false
	}

	var (
		userID    int
		expiresAt int64
	)
	err := s.db.QueryRow(
		`SELECT users.id, sessions.expires_at
		 FROM sessions
		 JOIN users ON sessions.user_id = users.id
		 WHERE sessions.token = $1`,
		token,
	).Scan(&userID, &expiresAt)
	if err != nil {
		return 0, false
	}
	if time.Now().Unix() > expiresAt {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE token = $1`, token)
		return 0, false
	}
	return userID, true
}

func (s *server) currentUserWithDisplay(r *http.Request) (string, string, bool) {
	token := s.sessionToken(r)
	if token == "" {
		return "", "", false
	}

	var (
		username    string
		displayName string
		expiresAt   int64
	)
	err := s.db.QueryRow(
		`SELECT users.username, COALESCE(users.display_name, users.username), sessions.expires_at
		 FROM sessions
		 JOIN users ON sessions.user_id = users.id
		 WHERE sessions.token = $1`,
		token,
	).Scan(&username, &displayName, &expiresAt)
	if err != nil {
		return "", "", false
	}
	if time.Now().Unix() > expiresAt {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE token = $1`, token)
		return "", "", false
	}
	// Fall back to username if display_name is empty
	if displayName == "" {
		displayName = username
	}
	return username, displayName, true
}

func (s *server) sessionToken(r *http.Request) string {
	cookie, err := r.Cookie(s.cookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
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
	return &progressReporter{
		server:      s,
		id:          id,
		direction:   direction,
		total:       total,
		lastSent:    time.Now(),
		minBytes:    256 * 1024,
		minInterval: 350 * time.Millisecond,
	}
}

func (p *progressReporter) Update(bytes int64) {
	if p == nil || p.id == "" {
		return
	}
	now := time.Now()
	if bytes-p.lastBytes < p.minBytes && now.Sub(p.lastSent) < p.minInterval {
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
	if _, ok := s.currentUser(ws.Request()); !ok {
		_ = ws.Close()
		return
	}
	id := ws.Request().URL.Query().Get("id")
	if id == "" {
		_ = ws.Close()
		return
	}
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
	s.wsMu.Lock()
	conn := s.wsConns[msg.ID]
	s.wsMu.Unlock()
	if conn == nil {
		return
	}
	_ = websocket.JSON.Send(conn, msg)
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
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/storage/") {
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
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}
		// Handle preflight
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

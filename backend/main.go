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
	"net/http"
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
	_ "modernc.org/sqlite"
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
	Name   string `json:"name"`
	Count  int    `json:"count"`
	Hidden bool   `json:"hidden"`
}

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	master := flag.String("master", "127.0.0.1:50051", "GFS master gRPC address")
	prefix := flag.String("prefix", "/sfs", "GFS namespace prefix for simple file store")
	staticDir := flag.String("static", "frontend", "path to frontend assets")
	maxUploadMB := flag.Int64("max-upload-mb", 0, "max upload size in MB (0 = unlimited)")
	uploadTTL := flag.Duration("upload-timeout", 10*time.Minute, "max time allowed for a single upload")
	authDB := flag.String("auth-db", "auth.db", "path to sqlite database for auth")
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

	defaultUsername := strings.TrimSpace(os.Getenv("SFS_DEFAULT_USERNAME"))
	defaultPassword := os.Getenv("SFS_DEFAULT_PASSWORD")
	if defaultUsername == "" || defaultPassword == "" {
		log.Fatal("missing SFS_DEFAULT_USERNAME or SFS_DEFAULT_PASSWORD")
	}

	db, err := sql.Open("sqlite", *authDB)
	if err != nil {
		log.Fatalf("failed to open auth db: %v", err)
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
	mux.HandleFunc("/storage/files", srv.handleList)
	mux.HandleFunc("/storage/upload", srv.handleUpload)
	mux.HandleFunc("/storage/download", srv.handleDownload)
	mux.HandleFunc("/storage/delete", srv.handleDelete)
	mux.Handle("/ws", websocket.Handler(srv.handleWS))
	mux.Handle("/", srv.staticHandler())

	log.Printf("listening on %s", *addr)
	log.Printf("serving frontend from %s", srv.staticDir)
		log.Printf("sharing files under namespace prefix %s", srv.prefix)
	if err := http.ListenAndServe(*addr, logRequests(mux)); err != nil {
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
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			token TEXT NOT NULL UNIQUE,
			expires_at INTEGER NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS namespaces (
			name TEXT PRIMARY KEY,
			hidden INTEGER NOT NULL DEFAULT 0
		);`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

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
			`INSERT INTO users (username, password_hash) VALUES (?, ?)`,
			username,
			string(hash),
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
		`INSERT INTO namespaces (name, hidden) VALUES (?, ?)
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
		if namespace == hiddenNamespace {
			if _, ok := s.currentUser(r); !ok {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
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

	hiddenSet, err := s.loadHiddenNamespaces()
	if err != nil {
		http.Error(w, "failed to load namespaces", http.StatusInternalServerError)
		return
	}

	_, authed := s.currentUser(r)
	namespaceRows, err := s.loadAllNamespaces()
	if err != nil {
		http.Error(w, "failed to load namespaces", http.StatusInternalServerError)
		return
	}

	counts := make(map[string]int)
	for _, entry := range namespaceRows {
		hiddenSet[entry.Name] = entry.Hidden
		if entry.Hidden && !authed {
			continue
		}
		count, err := s.countNamespaceFiles(ctx, entry.Name)
		if err != nil {
			http.Error(w, "failed to list namespace files", http.StatusBadGateway)
			return
		}
		counts[entry.Name] = count
	}

	if _, ok := counts[defaultNamespace]; !ok {
		if !hiddenSet[defaultNamespace] || authed {
			count, err := s.countNamespaceFiles(ctx, defaultNamespace)
			if err != nil {
				http.Error(w, "failed to list namespace files", http.StatusBadGateway)
				return
			}
			counts[defaultNamespace] = count
		}
	}

	resp := make([]namespaceInfo, 0, len(counts))
	for name, count := range counts {
		if hiddenSet[name] && !authed {
			continue
		}
		resp = append(resp, namespaceInfo{
			Name:   name,
			Count:  count,
			Hidden: hiddenSet[name],
		})
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

	if err := s.upsertNamespace(name, payload.Hidden); err != nil {
		http.Error(w, "failed to save namespace", http.StatusInternalServerError)
		return
	}

	writeJSON(w, namespaceInfo{
		Name:   name,
		Count:  0,
		Hidden: payload.Hidden,
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
		if namespace == hiddenNamespace {
			if _, ok := s.currentUser(r); !ok {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
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
	Username string `json:"username"`
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
		userID int64
		hash   string
	)
	err := s.db.QueryRow(`SELECT id, password_hash FROM users WHERE username = ?`, payload.Username).
		Scan(&userID, &hash)
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(payload.Password)); err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	token, err := generateToken(32)
	if err != nil {
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}
	expires := time.Now().Add(s.sessionTTL)
	if _, err := s.db.Exec(
		`INSERT INTO sessions (user_id, token, expires_at) VALUES (?, ?, ?)`,
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
	writeJSON(w, sessionResponse{Username: payload.Username})
}

func (s *server) handleSession(w http.ResponseWriter, r *http.Request) {
	username, ok := s.currentUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, sessionResponse{Username: username})
}

func (s *server) handleLogout(w http.ResponseWriter, r *http.Request) {
	token := s.sessionToken(r)
	if token != "" {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
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
		 WHERE sessions.token = ?`,
		token,
	).Scan(&username, &expiresAt)
	if err != nil {
		return "", false
	}
	if time.Now().Unix() > expiresAt {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
		return "", false
	}
	return username, true
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
	rows, err := s.db.Query(`SELECT name, hidden FROM namespaces`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var namespaces []namespaceInfo
	for rows.Next() {
		var name string
		var hiddenFlag int
		if err := rows.Scan(&name, &hiddenFlag); err != nil {
			return nil, err
		}
		namespaces = append(namespaces, namespaceInfo{
			Name:   name,
			Hidden: hiddenFlag != 0,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return namespaces, nil
}

func (s *server) upsertNamespace(name string, hidden bool) error {
	hiddenValue := 0
	if hidden {
		hiddenValue = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO namespaces (name, hidden) VALUES (?, ?)
		 ON CONFLICT(name) DO UPDATE SET hidden = excluded.hidden`,
		name,
		hiddenValue,
	)
	return err
}

func (s *server) deleteNamespace(name string) error {
	_, err := s.db.Exec(`DELETE FROM namespaces WHERE name = ?`, name)
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
	result, err := s.db.Exec(`UPDATE namespaces SET hidden = ? WHERE name = ?`, hiddenValue, name)
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
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM namespaces WHERE name = ?`, name).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
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
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/storage/") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/" {
			http.ServeFile(w, r, filepath.Join(s.staticDir, "index.html"))
			return
		}
		fileServer.ServeHTTP(w, r)
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

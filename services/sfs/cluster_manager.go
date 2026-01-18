package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/net/websocket"
)

// privilegedTokenTTL is the lifetime of privileged tokens (5 minutes)
const privilegedTokenTTL = 5 * time.Minute

// clusterManagerPort is the port where cluster-manager agents listen
const clusterManagerPort = 9090

var clusterManagerSecret = os.Getenv("CLUSTER_MANAGER_SECRET")

// ClusterNode represents a node with cluster-manager agent
type ClusterNode struct {
	Name      string `json:"name"`
	IP        string `json:"ip"`
	Hostname  string `json:"hostname,omitempty"`
	Uptime    string `json:"uptime,omitempty"`
	CronCount int    `json:"cron_count,omitempty"`
}

// initPrivilegedTokensTable creates the privileged_tokens table
func initPrivilegedTokensTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS privileged_tokens (
			id SERIAL PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id),
			token TEXT NOT NULL UNIQUE,
			expires_at BIGINT NOT NULL
		)
	`)
	return err
}

// handleVerifyPassword verifies password and issues a privileged token
func (s *server) handleVerifyPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username, ok := s.currentUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Require admin
	if !isAdmin(username) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	var payload struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	// Verify password
	var userID int64
	var hash string
	err := s.db.QueryRow(`SELECT id, password_hash FROM users WHERE username = $1`, username).Scan(&userID, &hash)
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(payload.Password)); err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	// Generate privileged token
	token, err := generateToken(32)
	if err != nil {
		http.Error(w, "failed to create token", http.StatusInternalServerError)
		return
	}

	expires := time.Now().Add(privilegedTokenTTL)
	if _, err := s.db.Exec(
		`INSERT INTO privileged_tokens (user_id, token, expires_at) VALUES ($1, $2, $3)`,
		userID,
		token,
		expires.Unix(),
	); err != nil {
		http.Error(w, "failed to create token", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"token":      token,
		"expires_at": expires.Unix(),
	})
}

// validatePrivilegedToken checks if a privileged token is valid
func (s *server) validatePrivilegedToken(r *http.Request) bool {
	token := r.Header.Get("X-Privileged-Token")
	if token == "" {
		return false
	}

	var expiresAt int64
	err := s.db.QueryRow(
		`SELECT expires_at FROM privileged_tokens WHERE token = $1`,
		token,
	).Scan(&expiresAt)
	if err != nil {
		return false
	}

	if time.Now().Unix() > expiresAt {
		// Clean up expired token
		s.db.Exec(`DELETE FROM privileged_tokens WHERE token = $1`, token)
		return false
	}

	return true
}

// requirePrivileged requires admin + privileged token
func (s *server) requirePrivileged(w http.ResponseWriter, r *http.Request) bool {
	username, ok := s.currentUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	if !isAdmin(username) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	if !s.validatePrivilegedToken(r) {
		http.Error(w, "privileged token required", http.StatusForbidden)
		return false
	}
	return true
}

// handleClusterNodes lists nodes with cluster-manager agents
func (s *server) handleClusterNodes(w http.ResponseWriter, r *http.Request) {
	if !s.requirePrivileged(w, r) {
		return
	}

	nodes, err := discoverClusterNodes()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Fetch additional info from each node
	for i := range nodes {
		info, err := fetchNodeInfo(nodes[i].IP)
		if err == nil {
			nodes[i].Hostname = info.Hostname
			nodes[i].Uptime = info.Uptime
			nodes[i].CronCount = info.CronCount
		}
	}

	writeJSON(w, nodes)
}

// handleNodeCronList proxies cron list request to a node
func (s *server) handleNodeCronList(w http.ResponseWriter, r *http.Request) {
	if !s.requirePrivileged(w, r) {
		return
	}

	nodeName := r.PathValue("name")
	if nodeName == "" {
		http.Error(w, "node name required", http.StatusBadRequest)
		return
	}

	nodeIP, err := getNodeIP(nodeName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	proxyRequest(w, r, nodeIP, "/cron", http.MethodGet, nil)
}

// handleNodeCronCreate proxies cron create request to a node
func (s *server) handleNodeCronCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePrivileged(w, r) {
		return
	}

	nodeName := r.PathValue("name")
	if nodeName == "" {
		http.Error(w, "node name required", http.StatusBadRequest)
		return
	}

	nodeIP, err := getNodeIP(nodeName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	proxyRequest(w, r, nodeIP, "/cron", http.MethodPost, body)
}

// handleNodeCronUpdate proxies cron update request to a node
func (s *server) handleNodeCronUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePrivileged(w, r) {
		return
	}

	nodeName := r.PathValue("name")
	cronID := r.PathValue("id")
	if nodeName == "" || cronID == "" {
		http.Error(w, "node name and cron id required", http.StatusBadRequest)
		return
	}

	nodeIP, err := getNodeIP(nodeName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	proxyRequest(w, r, nodeIP, "/cron/"+cronID, http.MethodPut, body)
}

// handleNodeCronDelete proxies cron delete request to a node
func (s *server) handleNodeCronDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePrivileged(w, r) {
		return
	}

	nodeName := r.PathValue("name")
	cronID := r.PathValue("id")
	if nodeName == "" || cronID == "" {
		http.Error(w, "node name and cron id required", http.StatusBadRequest)
		return
	}

	nodeIP, err := getNodeIP(nodeName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	proxyRequest(w, r, nodeIP, "/cron/"+cronID, http.MethodDelete, nil)
}

// handleNodeCronRun proxies cron run request to a node
func (s *server) handleNodeCronRun(w http.ResponseWriter, r *http.Request) {
	if !s.requirePrivileged(w, r) {
		return
	}

	nodeName := r.PathValue("name")
	cronID := r.PathValue("id")
	if nodeName == "" || cronID == "" {
		http.Error(w, "node name and cron id required", http.StatusBadRequest)
		return
	}

	nodeIP, err := getNodeIP(nodeName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	proxyRequest(w, r, nodeIP, "/cron/"+cronID+"/run", http.MethodPost, nil)
}

// handleNodeTerminal proxies WebSocket terminal to a node
func (s *server) handleNodeTerminal(ws *websocket.Conn) {
	r := ws.Request()

	username, ok := s.currentUser(r)
	if !ok {
		ws.Close()
		return
	}
	if !isAdmin(username) {
		ws.Close()
		return
	}
	if !s.validatePrivilegedToken(r) {
		ws.Close()
		return
	}

	nodeName := r.PathValue("name")
	if nodeName == "" {
		ws.Close()
		return
	}

	nodeIP, err := getNodeIP(nodeName)
	if err != nil {
		ws.Close()
		return
	}

	// Connect to node's terminal WebSocket
	nodeWSURL := fmt.Sprintf("ws://%s:%d/terminal", nodeIP, clusterManagerPort)
	config, err := websocket.NewConfig(nodeWSURL, "http://localhost/")
	if err != nil {
		log.Printf("failed to create ws config: %v", err)
		ws.Close()
		return
	}
	if clusterManagerSecret != "" {
		config.Header.Set("X-Cluster-Manager-Secret", clusterManagerSecret)
	}

	nodeWS, err := websocket.DialConfig(config)
	if err != nil {
		log.Printf("failed to connect to node terminal: %v", err)
		ws.Close()
		return
	}
	defer nodeWS.Close()

	// Proxy bidirectionally
	done := make(chan struct{})

	go func() {
		io.Copy(ws, nodeWS)
		close(done)
	}()

	go func() {
		io.Copy(nodeWS, ws)
	}()

	<-done
}

// nodeInfo from cluster-manager /info endpoint
type nodeInfo struct {
	Hostname  string `json:"hostname"`
	Uptime    string `json:"uptime"`
	CronCount int    `json:"cron_count"`
}

func fetchNodeInfo(nodeIP string) (*nodeInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	url := fmt.Sprintf("http://%s:%d/info", nodeIP, clusterManagerPort)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if clusterManagerSecret != "" {
		req.Header.Set("X-Cluster-Manager-Secret", clusterManagerSecret)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("node returned status %d", resp.StatusCode)
	}

	var info nodeInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}
	return &info, nil
}

func proxyRequest(w http.ResponseWriter, r *http.Request, nodeIP, path, method string, body []byte) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	url := fmt.Sprintf("http://%s:%d%s", nodeIP, clusterManagerPort, path)
	var bodyReader io.Reader
	if body != nil {
		bodyReader = strings.NewReader(string(body))
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		http.Error(w, "failed to create request", http.StatusInternalServerError)
		return
	}
	if clusterManagerSecret != "" {
		req.Header.Set("X-Cluster-Manager-Secret", clusterManagerSecret)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "failed to reach node: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// discoverClusterNodes discovers nodes by querying Kubernetes API
func discoverClusterNodes() ([]ClusterNode, error) {
	// Try to read Kubernetes service account token
	tokenPath := "/var/run/secrets/kubernetes.io/serviceaccount/token"
	token, err := os.ReadFile(tokenPath)
	if err != nil {
		// Not running in Kubernetes, return empty list
		log.Printf("not running in kubernetes (no service account token)")
		return []ClusterNode{}, nil
	}

	caPath := "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	caCert, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA cert: %w", err)
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	// Create HTTP client with service account CA
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caCertPool,
			},
		},
	}

	// Query Kubernetes API for nodes
	apiServer := os.Getenv("KUBERNETES_SERVICE_HOST")
	apiPort := os.Getenv("KUBERNETES_SERVICE_PORT")
	if apiServer == "" || apiPort == "" {
		return []ClusterNode{}, nil
	}

	url := fmt.Sprintf("https://%s:%s/api/v1/nodes", apiServer, apiPort)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+string(token))

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query k8s api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("k8s api returned %d: %s", resp.StatusCode, string(body))
	}

	var nodeList struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Status struct {
				Addresses []struct {
					Type    string `json:"type"`
					Address string `json:"address"`
				} `json:"addresses"`
			} `json:"status"`
		} `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&nodeList); err != nil {
		return nil, err
	}

	var nodes []ClusterNode
	for _, item := range nodeList.Items {
		node := ClusterNode{
			Name: item.Metadata.Name,
		}
		for _, addr := range item.Status.Addresses {
			if addr.Type == "InternalIP" {
				node.IP = addr.Address
				break
			}
		}
		if node.IP != "" {
			nodes = append(nodes, node)
		}
	}

	return nodes, nil
}

func getNodeIP(nodeName string) (string, error) {
	nodes, err := discoverClusterNodes()
	if err != nil {
		return "", err
	}
	for _, node := range nodes {
		if node.Name == nodeName {
			return node.IP, nil
		}
	}
	return "", fmt.Errorf("node not found: %s", nodeName)
}

// cleanupExpiredPrivilegedTokens removes expired tokens
func cleanupExpiredPrivilegedTokens(db *sql.DB) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		db.Exec(`DELETE FROM privileged_tokens WHERE expires_at < $1`, time.Now().Unix())
	}
}

// generatePrivilegedToken creates a secure random token
func generatePrivilegedToken(length int) (string, error) {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

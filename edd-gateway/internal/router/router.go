package router

import (
	"container/list"
	"context"
	"crypto/md5"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
	"golang.org/x/crypto/ssh"
)

// LRU cache for route lookups
const routeCacheSize = 100

// routeCacheEntry holds a cached route lookup result.
type routeCacheEntry struct {
	key        string
	route      *StaticRoute
	targetPath string
	err        error
}

// routeCache is a thread-safe LRU cache for route lookups.
type routeCache struct {
	mu       sync.Mutex
	capacity int
	items    map[string]*list.Element
	order    *list.List // front = most recently used
}

// newRouteCache creates a new LRU cache with the given capacity.
func newRouteCache(capacity int) *routeCache {
	return &routeCache{
		capacity: capacity,
		items:    make(map[string]*list.Element),
		order:    list.New(),
	}
}

// get retrieves an entry from the cache, moving it to front (most recently used).
func (c *routeCache) get(key string) (*routeCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.order.MoveToFront(elem)
		return elem.Value.(*routeCacheEntry), true
	}
	return nil, false
}

// put adds an entry to the cache, evicting the least recently used if at capacity.
func (c *routeCache) put(key string, route *StaticRoute, targetPath string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If key exists, update and move to front
	if elem, ok := c.items[key]; ok {
		c.order.MoveToFront(elem)
		entry := elem.Value.(*routeCacheEntry)
		entry.route = route
		entry.targetPath = targetPath
		entry.err = err
		return
	}

	// Evict LRU if at capacity
	if c.order.Len() >= c.capacity {
		oldest := c.order.Back()
		if oldest != nil {
			c.order.Remove(oldest)
			delete(c.items, oldest.Value.(*routeCacheEntry).key)
		}
	}

	// Add new entry at front
	entry := &routeCacheEntry{
		key:        key,
		route:      route,
		targetPath: targetPath,
		err:        err,
	}
	elem := c.order.PushFront(entry)
	c.items[key] = elem
}

// clear removes all entries from the cache.
func (c *routeCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element)
	c.order.Init()
}

// size returns the current number of entries in the cache.
func (c *routeCache) size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

var (
	ErrNotFound        = errors.New("container not found")
	ErrNoIP            = errors.New("container has no external IP")
	ErrProtocolBlocked = errors.New("protocol access not enabled")
	ErrNoRoute         = errors.New("no matching route")
)

// StaticRoute holds routing info for a static path-based route.
type StaticRoute struct {
	ID          int
	Host        string // e.g., "cloud-api.eddisonso.com"
	PathPrefix  string // e.g., "/compute" or "/"
	Target      string // e.g., "edd-compute:80"
	StripPrefix bool   // Whether to strip the path prefix when proxying
	Priority    int    // Higher priority = matched first (longer paths get higher priority)
}

// Container holds routing information for a container.
type Container struct {
	ID           string
	Namespace    string
	ExternalIP   string
	Status       string
	SSHEnabled   bool
	HTTPSEnabled bool
	PortMap      map[int]int // ingress port -> target port
}

// Router resolves container IDs and static routes.
// Uses LRU caching for route lookups.
type Router struct {
	db         *sql.DB
	containers map[string]*Container // containerID -> Container
	routes     []StaticRoute         // sorted by path length (longest first)
	cache      *routeCache           // LRU cache for route lookups
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// New creates a router backed by PostgreSQL.
func New(connStr string) (*Router, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// Ensure static_routes table exists
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS static_routes (
			id SERIAL PRIMARY KEY,
			host TEXT NOT NULL,
			path_prefix TEXT NOT NULL,
			target TEXT NOT NULL,
			strip_prefix BOOLEAN NOT NULL DEFAULT false,
			priority INT NOT NULL DEFAULT 0,
			UNIQUE(host, path_prefix)
		)
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create static_routes table: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	r := &Router{
		db:         db,
		containers: make(map[string]*Container),
		cache:      newRouteCache(routeCacheSize),
		ctx:        ctx,
		cancel:     cancel,
	}

	// Initial load
	if err := r.reload(); err != nil {
		db.Close()
		cancel()
		return nil, fmt.Errorf("initial load: %w", err)
	}

	// Background sync enabled - reload routes periodically
	r.wg.Add(1)
	go r.syncLoop()

	return r, nil
}

// reload fetches all data from the database.
func (r *Router) reload() error {
	// Load containers
	containers := make(map[string]*Container)
	rows, err := r.db.Query(`
		SELECT id, namespace, external_ip, status,
		       COALESCE(ssh_enabled, false), COALESCE(https_enabled, false)
		FROM containers
		WHERE status = 'running' AND external_ip IS NOT NULL AND external_ip != ''
	`)
	if err != nil {
		return fmt.Errorf("query containers: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var c Container
		var externalIP sql.NullString
		if err := rows.Scan(&c.ID, &c.Namespace, &externalIP, &c.Status, &c.SSHEnabled, &c.HTTPSEnabled); err != nil {
			return fmt.Errorf("scan container: %w", err)
		}
		if externalIP.Valid && externalIP.String != "" {
			c.ExternalIP = externalIP.String
			c.PortMap = make(map[int]int)
			containers[c.ID] = &c
		}
	}

	// Load ingress rules
	ruleRows, err := r.db.Query(`SELECT container_id, port, target_port FROM ingress_rules`)
	if err != nil {
		return fmt.Errorf("query ingress rules: %w", err)
	}
	defer ruleRows.Close()

	for ruleRows.Next() {
		var containerID string
		var port, targetPort int
		if err := ruleRows.Scan(&containerID, &port, &targetPort); err != nil {
			return fmt.Errorf("scan ingress rule: %w", err)
		}
		if c, ok := containers[containerID]; ok {
			c.PortMap[port] = targetPort
		}
	}

	// Load static routes
	routeRows, err := r.db.Query(`
		SELECT id, host, path_prefix, target, strip_prefix, priority
		FROM static_routes
	`)
	if err != nil {
		return fmt.Errorf("query static routes: %w", err)
	}
	defer routeRows.Close()

	var routes []StaticRoute
	for routeRows.Next() {
		var route StaticRoute
		if err := routeRows.Scan(&route.ID, &route.Host, &route.PathPrefix, &route.Target, &route.StripPrefix, &route.Priority); err != nil {
			return fmt.Errorf("scan static route: %w", err)
		}
		routes = append(routes, route)
	}

	// Sort routes by path length (longest first) for proper matching
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Host != routes[j].Host {
			return routes[i].Host < routes[j].Host
		}
		return len(routes[i].PathPrefix) > len(routes[j].PathPrefix)
	})

	// Atomic swap
	r.mu.Lock()
	r.containers = containers
	r.routes = routes
	r.mu.Unlock()

	// Clear cache since routes changed
	if r.cache != nil {
		r.cache.clear()
	}

	slog.Debug("reloaded router data", "containers", len(containers), "routes", len(routes))
	for _, route := range routes {
		slog.Debug("loaded route", "host", route.Host, "path", route.PathPrefix, "target", route.Target)
	}

	return nil
}

// syncLoop periodically reloads data from the database.
func (r *Router) syncLoop() {
	defer r.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			if err := r.reload(); err != nil {
				slog.Error("failed to reload router data", "error", err)
			}
		}
	}
}

// Close stops the router.
func (r *Router) Close() error {
	r.cancel()
	r.wg.Wait()
	return r.db.Close()
}

// Resolve looks up a container by ID.
func (r *Router) Resolve(containerID string) (*Container, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if c, ok := r.containers[containerID]; ok {
		return c, nil
	}
	return nil, ErrNotFound
}

// ResolveByHostname extracts container ID from hostname and resolves it.
func (r *Router) ResolveByHostname(hostname string) (*Container, error) {
	containerID := extractContainerID(hostname)
	if containerID == "" {
		return nil, ErrNotFound
	}
	return r.Resolve(containerID)
}

// extractContainerID extracts the container ID from a hostname.
// "abc123.cloud.eddisonso.com" -> "abc123"
func extractContainerID(hostname string) string {
	parts := strings.Split(hostname, ".")
	if len(parts) < 3 {
		return ""
	}
	return parts[0]
}

// ResolveSSH resolves a container and checks SSH access.
func (r *Router) ResolveSSH(containerID string) (*Container, error) {
	c, err := r.Resolve(containerID)
	if err != nil {
		return nil, err
	}
	if !c.SSHEnabled {
		return nil, ErrProtocolBlocked
	}
	return c, nil
}

// ResolveHTTPS resolves a container by hostname and checks HTTPS access.
func (r *Router) ResolveHTTPS(hostname string) (*Container, error) {
	c, err := r.ResolveByHostname(hostname)
	if err != nil {
		return nil, err
	}
	if !c.HTTPSEnabled {
		return nil, ErrProtocolBlocked
	}
	return c, nil
}

// ResolveHTTP resolves a container by hostname for a given ingress port.
func (r *Router) ResolveHTTP(hostname string, ingressPort int) (*Container, int, error) {
	c, err := r.ResolveByHostname(hostname)
	if err != nil {
		return nil, 0, err
	}
	targetPort, ok := c.PortMap[ingressPort]
	if !ok {
		return nil, 0, ErrProtocolBlocked
	}
	return c, targetPort, nil
}

// ResolveStaticRoute finds a matching static route using simple linear scan.
// Returns the route and the path to forward (with prefix stripped if configured).
func (r *Router) ResolveStaticRoute(host, path string) (*StaticRoute, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Linear scan through routes (sorted by path length, longest first)
	for i := range r.routes {
		route := &r.routes[i]
		if route.Host != host {
			continue
		}
		if strings.HasPrefix(path, route.PathPrefix) {
			// Found a match
			targetPath := path
			if route.StripPrefix && route.PathPrefix != "/" {
				targetPath = strings.TrimPrefix(path, route.PathPrefix)
				if targetPath == "" {
					targetPath = "/"
				}
			}
			slog.Debug("route matched", "host", host, "path", path, "prefix", route.PathPrefix, "target", route.Target)
			return route, targetPath, nil
		}
	}

	slog.Debug("no route matched", "host", host, "path", path)
	return nil, "", ErrNoRoute
}

// GetAllIngressPorts returns all unique ingress ports.
func (r *Router) GetAllIngressPorts() []int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	portSet := make(map[int]bool)
	for _, c := range r.containers {
		for port := range c.PortMap {
			portSet[port] = true
		}
	}

	ports := make([]int, 0, len(portSet))
	for port := range portSet {
		ports = append(ports, port)
	}
	return ports
}

// RegisterRoute adds or updates a static route.
func (r *Router) RegisterRoute(host, pathPrefix, target string, stripPrefix bool) error {
	priority := len(pathPrefix) * 10
	if pathPrefix == "/" {
		priority = 0
	}

	_, err := r.db.Exec(`
		INSERT INTO static_routes (host, path_prefix, target, strip_prefix, priority)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (host, path_prefix) DO UPDATE SET
			target = EXCLUDED.target,
			strip_prefix = EXCLUDED.strip_prefix,
			priority = EXCLUDED.priority
	`, host, pathPrefix, target, stripPrefix, priority)
	if err != nil {
		return fmt.Errorf("insert static route: %w", err)
	}

	return r.reload()
}

// UnregisterRoute removes a static route.
func (r *Router) UnregisterRoute(host, pathPrefix string) error {
	result, err := r.db.Exec(`DELETE FROM static_routes WHERE host = $1 AND path_prefix = $2`, host, pathPrefix)
	if err != nil {
		return fmt.Errorf("delete static route: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNoRoute
	}

	return r.reload()
}

// ListRoutes returns all static routes.
func (r *Router) ListRoutes() []StaticRoute {
	r.mu.RLock()
	defer r.mu.RUnlock()

	routes := make([]StaticRoute, len(r.routes))
	copy(routes, r.routes)
	return routes
}

// ValidateSSHKey checks if the given public key belongs to the owner of the specified container.
// Computes an MD5 fingerprint to match the format stored by the compute service.
func (r *Router) ValidateSSHKey(containerID string, pubKey ssh.PublicKey) (bool, error) {
	hash := md5.Sum(pubKey.Marshal())
	parts := make([]string, len(hash))
	for i, b := range hash {
		parts[i] = fmt.Sprintf("%02x", b)
	}
	fp := strings.Join(parts, ":")

	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM ssh_keys sk
		JOIN containers c ON sk.user_id = c.user_id
		WHERE c.id = $1 AND sk.fingerprint = $2
	`, containerID, fp).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("validate ssh key: %w", err)
	}
	return count > 0, nil
}

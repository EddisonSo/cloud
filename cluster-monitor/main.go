package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"eddisonso.com/cluster-monitor/internal/graph"
	"eddisonso.com/cluster-monitor/internal/timeseries"
	"eddisonso.com/go-gfs/pkg/gfslog"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// JWTClaims represents the claims in a JWT token
type JWTClaims struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	UserID      string `json:"user_id"`
	jwt.RegisteredClaims
}

var jwtSecret []byte

func initJWTSecret() {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		slog.Warn("JWT_SECRET not set, pod filtering will be disabled")
		return
	}
	jwtSecret = []byte(secret)
}

func validateToken(tokenString string) *JWTClaims {
	if jwtSecret == nil || tokenString == "" {
		return nil
	}
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return jwtSecret, nil
	})
	if err != nil {
		return nil
	}
	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil
	}
	return claims
}

func getTokenFromRequest(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	if token := r.URL.Query().Get("token"); token != "" {
		return token
	}
	return ""
}

func filterPodsForUser(pods []PodMetrics, userID string) []PodMetrics {
	if userID == "" {
		// No auth - only show default namespace pods
		var filtered []PodMetrics
		for _, pod := range pods {
			if pod.Namespace == "default" {
				filtered = append(filtered, pod)
			}
		}
		return filtered
	}
	// Authenticated - show default + user's compute containers
	var filtered []PodMetrics
	prefix := "compute-" + userID + "-"
	for _, pod := range pods {
		if pod.Namespace == "default" || strings.HasPrefix(pod.Namespace, prefix) {
			filtered = append(filtered, pod)
		}
	}
	return filtered
}

type NodeMetrics struct {
	Name           string          `json:"name"`
	CPUUsage       string          `json:"cpu_usage"`
	MemoryUsage    string          `json:"memory_usage"`
	CPUCapacity    string          `json:"cpu_capacity"`
	MemoryCapacity string          `json:"memory_capacity"`
	CPUPercent     float64         `json:"cpu_percent"`
	MemoryPercent  float64         `json:"memory_percent"`
	DiskCapacity   int64           `json:"disk_capacity"`
	DiskUsage      int64           `json:"disk_usage"`
	DiskPercent    float64         `json:"disk_percent"`
	Conditions     []NodeCondition `json:"conditions,omitempty"`
}

type NodeCondition struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}

type ClusterInfo struct {
	Timestamp time.Time     `json:"timestamp"`
	Nodes     []NodeMetrics `json:"nodes"`
}

type PodMetrics struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	Node           string `json:"node"`
	CPUUsage       int64  `json:"cpu_usage"`        // millicores
	MemoryUsage    int64  `json:"memory_usage"`     // bytes
	DiskUsage      int64  `json:"disk_usage"`       // bytes
	CPUCapacity    int64  `json:"cpu_capacity"`     // millicores (node capacity)
	MemoryCapacity int64  `json:"memory_capacity"`  // bytes (node capacity)
	DiskCapacity   int64  `json:"disk_capacity"`    // bytes (node capacity)
}

type PodMetricsInfo struct {
	Timestamp time.Time    `json:"timestamp"`
	Pods      []PodMetrics `json:"pods"`
}

// MetricsCache holds the latest metrics data updated by background workers
type MetricsCache struct {
	mu             sync.RWMutex
	clusterInfo    *ClusterInfo
	podMetrics     *PodMetricsInfo
	subscribers    []chan *ClusterInfo
	podSubscribers []chan *PodMetricsInfo
	subMu          sync.Mutex
}

func NewMetricsCache() *MetricsCache {
	return &MetricsCache{
		clusterInfo: &ClusterInfo{Timestamp: time.Now()},
		podMetrics:  &PodMetricsInfo{Timestamp: time.Now()},
	}
}

func (c *MetricsCache) GetClusterInfo() *ClusterInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.clusterInfo
}

func (c *MetricsCache) SetClusterInfo(info *ClusterInfo) {
	c.mu.Lock()
	c.clusterInfo = info
	c.mu.Unlock()

	// Notify subscribers
	c.subMu.Lock()
	for _, ch := range c.subscribers {
		select {
		case ch <- info:
		default: // Don't block if subscriber is slow
		}
	}
	c.subMu.Unlock()
}

func (c *MetricsCache) GetPodMetrics() *PodMetricsInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.podMetrics
}

func (c *MetricsCache) SetPodMetrics(info *PodMetricsInfo) {
	c.mu.Lock()
	c.podMetrics = info
	c.mu.Unlock()

	// Notify pod subscribers
	c.subMu.Lock()
	for _, ch := range c.podSubscribers {
		select {
		case ch <- info:
		default: // Don't block if subscriber is slow
		}
	}
	c.subMu.Unlock()
}

func (c *MetricsCache) Subscribe() chan *ClusterInfo {
	ch := make(chan *ClusterInfo, 1)
	c.subMu.Lock()
	c.subscribers = append(c.subscribers, ch)
	c.subMu.Unlock()
	return ch
}

func (c *MetricsCache) Unsubscribe(ch chan *ClusterInfo) {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	for i, sub := range c.subscribers {
		if sub == ch {
			c.subscribers = append(c.subscribers[:i], c.subscribers[i+1:]...)
			close(ch)
			return
		}
	}
}

func (c *MetricsCache) SubscribePods() chan *PodMetricsInfo {
	ch := make(chan *PodMetricsInfo, 1)
	c.subMu.Lock()
	c.podSubscribers = append(c.podSubscribers, ch)
	c.subMu.Unlock()
	return ch
}

func (c *MetricsCache) UnsubscribePods(ch chan *PodMetricsInfo) {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	for i, sub := range c.podSubscribers {
		if sub == ch {
			c.podSubscribers = append(c.podSubscribers[:i], c.podSubscribers[i+1:]...)
			close(ch)
			return
		}
	}
}

type metricsNodeList struct {
	Items []metricsNode `json:"items"`
}

type metricsNode struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Usage struct {
		CPU    string `json:"cpu"`
		Memory string `json:"memory"`
	} `json:"usage"`
}

type kubeletStats struct {
	Node struct {
		Fs struct {
			CapacityBytes  int64 `json:"capacityBytes"`
			UsedBytes      int64 `json:"usedBytes"`
			AvailableBytes int64 `json:"availableBytes"`
		} `json:"fs"`
	} `json:"node"`
	Pods []kubeletPodStats `json:"pods"`
}

type kubeletPodStats struct {
	PodRef struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"podRef"`
	EphemeralStorage *struct {
		UsedBytes int64 `json:"usedBytes"`
	} `json:"ephemeral-storage,omitempty"`
}

type metricsPodList struct {
	Items []metricsPod `json:"items"`
}

type metricsPod struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Containers []struct {
		Name  string `json:"name"`
		Usage struct {
			CPU    string `json:"cpu"`
			Memory string `json:"memory"`
		} `json:"usage"`
	} `json:"containers"`
}

const coreServicesNamespace = "default"

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	refreshInterval := flag.Duration("refresh", 5*time.Second, "Metrics refresh interval")
	logServiceAddr := flag.String("log-service", "", "Log service address (e.g., log-service:50051)")
	logSource := flag.String("log-source", "cluster-monitor", "Log source name (e.g., pod name)")
	apiServer := flag.String("api-server", "", "Kubernetes API server address (e.g., https://k3s.eddisonso.com:6443)")
	flag.Parse()

	initJWTSecret()

	if *logServiceAddr != "" {
		logger := gfslog.NewLogger(gfslog.Config{
			Source:         *logSource,
			LogServiceAddr: *logServiceAddr,
			MinLevel:       slog.LevelDebug,
		})
		slog.SetDefault(logger.Logger)
		defer logger.Close()
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		slog.Error("Failed to get in-cluster config", "error", err)
		os.Exit(1)
	}

	// Override API server address if provided
	if *apiServer != "" {
		config.Host = *apiServer
		slog.Info("Using custom API server", "host", *apiServer)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		slog.Error("Failed to create clientset", "error", err)
		os.Exit(1)
	}

	// Initialize cache and metrics store, start background workers
	cache := NewMetricsCache()
	metricsStore := timeseries.NewMetricsStore(0) // Default capacity (24h at 5s intervals)
	go clusterInfoWorker(clientset, cache, metricsStore, *refreshInterval)
	go podMetricsWorker(clientset, cache, metricsStore, *refreshInterval)

	mux := http.NewServeMux()
	mux.HandleFunc("/cluster-info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cache.GetClusterInfo())
	})

	mux.HandleFunc("/ws/cluster-info", func(w http.ResponseWriter, r *http.Request) {
		handleClusterInfoWS(w, r, cache)
	})

	// SSE endpoint for cluster-info (HTTP/2 compatible)
	mux.HandleFunc("/sse/cluster-info", func(w http.ResponseWriter, r *http.Request) {
		handleClusterInfoSSE(w, r, cache)
	})

	mux.HandleFunc("/pod-metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cache.GetPodMetrics())
	})

	mux.HandleFunc("/ws/pod-metrics", func(w http.ResponseWriter, r *http.Request) {
		handlePodMetricsWS(w, r, cache)
	})

	// SSE endpoint for pod-metrics (HTTP/2 compatible)
	mux.HandleFunc("/sse/pod-metrics", func(w http.ResponseWriter, r *http.Request) {
		handlePodMetricsSSE(w, r, cache)
	})

	// Combined SSE endpoint for both cluster-info and pod-metrics
	mux.HandleFunc("/sse/health", func(w http.ResponseWriter, r *http.Request) {
		handleHealthSSE(w, r, cache)
	})

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Historical metrics API endpoints
	mux.HandleFunc("/api/metrics/nodes", func(w http.ResponseWriter, r *http.Request) {
		handleMetricsNodes(w, r, metricsStore)
	})
	mux.HandleFunc("/api/metrics/nodes/", func(w http.ResponseWriter, r *http.Request) {
		handleMetricsNode(w, r, metricsStore)
	})
	mux.HandleFunc("/api/metrics/pods", func(w http.ResponseWriter, r *http.Request) {
		handleMetricsPods(w, r, metricsStore)
	})

	// Service dependency graph endpoint
	mux.HandleFunc("/api/graph/dependencies", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(graph.GetDependencies())
	})

	corsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		mux.ServeHTTP(w, r)
	})

	slog.Info("Cluster monitor listening", "addr", *addr)
	if err := http.ListenAndServe(*addr, corsHandler); err != nil {
		slog.Error("HTTP server failed", "error", err)
		os.Exit(1)
	}
}

// clusterInfoWorker fetches cluster metrics at the configured interval
func clusterInfoWorker(clientset *kubernetes.Clientset, cache *MetricsCache, store *timeseries.MetricsStore, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Fetch immediately on start
	fetchClusterInfo(clientset, cache, store)

	for range ticker.C {
		fetchClusterInfo(clientset, cache, store)
	}
}

func fetchClusterInfo(clientset *kubernetes.Clientset, cache *MetricsCache, store *timeseries.MetricsStore) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get node metrics from metrics-server
	metricsData, err := clientset.RESTClient().
		Get().
		AbsPath("/apis/metrics.k8s.io/v1beta1/nodes").
		DoRaw(ctx)
	if err != nil {
		slog.Error("Failed to get node metrics", "error", err)
		return
	}

	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		slog.Error("Failed to list nodes", "error", err)
		return
	}

	var metricsResponse metricsNodeList
	if err := json.Unmarshal(metricsData, &metricsResponse); err != nil {
		slog.Error("Failed to parse metrics", "error", err)
		return
	}

	nodeInfo := make(map[string]*corev1.Node)
	for i := range nodes.Items {
		nodeInfo[nodes.Items[i].Name] = &nodes.Items[i]
	}

	// Fetch disk stats from all nodes in parallel
	type diskStats struct {
		capacity int64
		usage    int64
	}
	diskStatsMap := make(map[string]diskStats)
	var diskMu sync.Mutex
	var wg sync.WaitGroup

	for _, item := range metricsResponse.Items {
		wg.Add(1)
		go func(nodeName string) {
			defer wg.Done()
			data, err := clientset.RESTClient().
				Get().
				AbsPath("/api/v1/nodes/" + nodeName + "/proxy/stats/summary").
				DoRaw(ctx)
			if err != nil {
				return
			}
			var stats kubeletStats
			if err := json.Unmarshal(data, &stats); err != nil {
				return
			}
			diskMu.Lock()
			diskStatsMap[nodeName] = diskStats{
				capacity: stats.Node.Fs.CapacityBytes,
				usage:    stats.Node.Fs.UsedBytes,
			}
			diskMu.Unlock()
		}(item.Metadata.Name)
	}
	wg.Wait()

	// Build response
	var nodeMetrics []NodeMetrics
	for _, item := range metricsResponse.Items {
		node := nodeInfo[item.Metadata.Name]
		if node == nil {
			continue
		}

		cpuCapacity := node.Status.Capacity.Cpu()
		memCapacity := node.Status.Capacity.Memory()
		cpuUsage := resource.MustParse(item.Usage.CPU)
		memUsage := resource.MustParse(item.Usage.Memory)

		cpuPercent := 0.0
		if cpuCapacity.MilliValue() > 0 {
			cpuPercent = float64(cpuUsage.MilliValue()) / float64(cpuCapacity.MilliValue()) * 100
		}

		memPercent := 0.0
		if memCapacity.Value() > 0 {
			memPercent = float64(memUsage.Value()) / float64(memCapacity.Value()) * 100
		}

		var diskCapacity, diskUsage int64
		var diskPercent float64
		if ds, ok := diskStatsMap[item.Metadata.Name]; ok {
			diskCapacity = ds.capacity
			diskUsage = ds.usage
			if diskCapacity > 0 {
				diskPercent = float64(diskUsage) / float64(diskCapacity) * 100
			}
		}

		var conditions []NodeCondition
		for _, cond := range node.Status.Conditions {
			switch cond.Type {
			case corev1.NodeMemoryPressure, corev1.NodeDiskPressure, corev1.NodePIDPressure:
				conditions = append(conditions, NodeCondition{
					Type:   string(cond.Type),
					Status: string(cond.Status),
				})
			}
		}

		nodeMetrics = append(nodeMetrics, NodeMetrics{
			Name:           item.Metadata.Name,
			CPUUsage:       item.Usage.CPU,
			MemoryUsage:    item.Usage.Memory,
			CPUCapacity:    cpuCapacity.String(),
			MemoryCapacity: memCapacity.String(),
			CPUPercent:     cpuPercent,
			MemoryPercent:  memPercent,
			DiskCapacity:   diskCapacity,
			DiskUsage:      diskUsage,
			DiskPercent:    diskPercent,
			Conditions:     conditions,
		})
	}

	now := time.Now()
	cache.SetClusterInfo(&ClusterInfo{
		Timestamp: now,
		Nodes:     nodeMetrics,
	})

	// Record to time-series store
	for _, nm := range nodeMetrics {
		store.RecordNode(nm.Name, timeseries.DataPoint{
			Timestamp:   now,
			CPUPercent:  nm.CPUPercent,
			MemPercent:  nm.MemoryPercent,
			DiskPercent: nm.DiskPercent,
		})
	}
}

// podMetricsWorker fetches pod metrics at the configured interval
func podMetricsWorker(clientset *kubernetes.Clientset, cache *MetricsCache, store *timeseries.MetricsStore, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Fetch immediately on start
	fetchPodMetrics(clientset, cache, store)

	for range ticker.C {
		fetchPodMetrics(clientset, cache, store)
	}
}

func fetchPodMetrics(clientset *kubernetes.Clientset, cache *MetricsCache, store *timeseries.MetricsStore) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get list of namespaces to monitor (default + compute-*)
	namespaces, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		slog.Error("Failed to list namespaces", "error", err)
		return
	}

	targetNamespaces := []string{coreServicesNamespace}
	for _, ns := range namespaces.Items {
		if strings.HasPrefix(ns.Name, "compute-") {
			targetNamespaces = append(targetNamespaces, ns.Name)
		}
	}

	// Fetch metrics from all target namespaces
	var metricsResponse metricsPodList
	for _, ns := range targetNamespaces {
		metricsData, err := clientset.RESTClient().
			Get().
			AbsPath("/apis/metrics.k8s.io/v1beta1/namespaces/" + ns + "/pods").
			DoRaw(ctx)
		if err != nil {
			continue // Skip namespaces with no metrics
		}

		var nsMetrics metricsPodList
		if err := json.Unmarshal(metricsData, &nsMetrics); err != nil {
			continue
		}
		metricsResponse.Items = append(metricsResponse.Items, nsMetrics.Items...)
	}

	// List pods from all target namespaces
	var allPods []corev1.Pod
	for _, ns := range targetNamespaces {
		pods, err := clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}
		allPods = append(allPods, pods.Items...)
	}
	pods := &corev1.PodList{Items: allPods}
	if err != nil {
		slog.Error("Failed to list pods", "error", err)
		return
	}

	// Fetch nodes for capacity information
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		slog.Error("Failed to list nodes", "error", err)
		return
	}

	type nodeCapacity struct {
		cpuMillis  int64
		memBytes   int64
		diskBytes  int64
	}
	nodeCapacities := make(map[string]nodeCapacity)
	for _, node := range nodes.Items {
		nodeCapacities[node.Name] = nodeCapacity{
			cpuMillis: node.Status.Capacity.Cpu().MilliValue(),
			memBytes:  node.Status.Capacity.Memory().Value(),
		}
	}

	podToNode := make(map[string]string)
	for _, pod := range pods.Items {
		podToNode[pod.Name] = pod.Spec.NodeName
	}

	nodeSet := make(map[string]bool)
	for _, nodeName := range podToNode {
		if nodeName != "" {
			nodeSet[nodeName] = true
		}
	}

	// Fetch kubelet stats in parallel for disk usage and disk capacity
	podDiskUsage := make(map[string]int64)
	nodeDiskCapacity := make(map[string]int64)
	var podDiskMu sync.Mutex
	var wg sync.WaitGroup

	for nodeName := range nodeSet {
		wg.Add(1)
		go func(nodeName string) {
			defer wg.Done()
			data, err := clientset.RESTClient().
				Get().
				AbsPath("/api/v1/nodes/" + nodeName + "/proxy/stats/summary").
				DoRaw(ctx)
			if err != nil {
				return
			}

			var stats kubeletStats
			if err := json.Unmarshal(data, &stats); err != nil {
				return
			}

			podDiskMu.Lock()
			nodeDiskCapacity[nodeName] = stats.Node.Fs.CapacityBytes
			for _, podStat := range stats.Pods {
				// Include default namespace and compute-* namespaces
				if podStat.PodRef.Namespace != coreServicesNamespace && !strings.HasPrefix(podStat.PodRef.Namespace, "compute-") {
					continue
				}
				key := podStat.PodRef.Namespace + "/" + podStat.PodRef.Name
				if podStat.EphemeralStorage != nil {
					podDiskUsage[key] = podStat.EphemeralStorage.UsedBytes
				}
			}
			podDiskMu.Unlock()
		}(nodeName)
	}
	wg.Wait()

	var podMetrics []PodMetrics
	for _, item := range metricsResponse.Items {
		var totalCPU, totalMemory int64
		for _, container := range item.Containers {
			cpu := resource.MustParse(container.Usage.CPU)
			mem := resource.MustParse(container.Usage.Memory)
			totalCPU += cpu.MilliValue() * 1000000
			totalMemory += mem.Value()
		}

		nodeName := podToNode[item.Metadata.Name]
		key := item.Metadata.Namespace + "/" + item.Metadata.Name

		// Get node capacity for this pod
		var cpuCap, memCap, diskCap int64
		if cap, ok := nodeCapacities[nodeName]; ok {
			cpuCap = cap.cpuMillis * 1000000 // convert to nanocores like usage
			memCap = cap.memBytes
		}
		if dc, ok := nodeDiskCapacity[nodeName]; ok {
			diskCap = dc
		}

		podMetrics = append(podMetrics, PodMetrics{
			Name:           item.Metadata.Name,
			Namespace:      item.Metadata.Namespace,
			Node:           nodeName,
			CPUUsage:       totalCPU,
			MemoryUsage:    totalMemory,
			DiskUsage:      podDiskUsage[key],
			CPUCapacity:    cpuCap,
			MemoryCapacity: memCap,
			DiskCapacity:   diskCap,
		})
	}

	now := time.Now()
	cache.SetPodMetrics(&PodMetricsInfo{
		Timestamp: now,
		Pods:      podMetrics,
	})

	// Record to time-series store
	for _, pm := range podMetrics {
		store.RecordPod(pm.Namespace, pm.Name, timeseries.PodDataPoint{
			Timestamp: now,
			CPUNanos:  pm.CPUUsage,
			MemBytes:  pm.MemoryUsage,
			DiskBytes: pm.DiskUsage,
		})
	}
}

func handleClusterInfoWS(w http.ResponseWriter, r *http.Request, cache *MetricsCache) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("WebSocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	// Subscribe to updates
	updates := cache.Subscribe()
	defer cache.Unsubscribe(updates)

	done := make(chan struct{})

	// Read pump - handle close
	go func() {
		defer close(done)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	// Send current data immediately
	if err := conn.WriteJSON(cache.GetClusterInfo()); err != nil {
		return
	}

	// Send updates as they come in
	for {
		select {
		case <-done:
			return
		case info := <-updates:
			if err := conn.WriteJSON(info); err != nil {
				slog.Error("WebSocket send failed", "error", err)
				return
			}
		}
	}
}

func handlePodMetricsWS(w http.ResponseWriter, r *http.Request, cache *MetricsCache) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("WebSocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	// Subscribe to updates
	updates := cache.SubscribePods()
	defer cache.UnsubscribePods(updates)

	done := make(chan struct{})

	// Read pump - handle close
	go func() {
		defer close(done)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	// Send current data immediately
	if err := conn.WriteJSON(cache.GetPodMetrics()); err != nil {
		return
	}

	// Send updates as they come in
	for {
		select {
		case <-done:
			return
		case info := <-updates:
			if err := conn.WriteJSON(info); err != nil {
				slog.Error("WebSocket send failed", "error", err)
				return
			}
		}
	}
}

// SSE handler for cluster-info (HTTP/2 compatible)
func handleClusterInfoSSE(w http.ResponseWriter, r *http.Request, cache *MetricsCache) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// CORS handled by middleware

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Subscribe to updates
	updates := cache.Subscribe()
	defer cache.Unsubscribe(updates)

	// Send current data immediately
	data, _ := json.Marshal(cache.GetClusterInfo())
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()

	// Send updates as they come in
	for {
		select {
		case <-r.Context().Done():
			return
		case info := <-updates:
			data, _ := json.Marshal(info)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// SSE handler for pod-metrics (HTTP/2 compatible)
func handlePodMetricsSSE(w http.ResponseWriter, r *http.Request, cache *MetricsCache) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// CORS handled by middleware

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Subscribe to updates
	updates := cache.SubscribePods()
	defer cache.UnsubscribePods(updates)

	// Send current data immediately
	data, _ := json.Marshal(cache.GetPodMetrics())
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()

	// Send updates as they come in
	for {
		select {
		case <-r.Context().Done():
			return
		case info := <-updates:
			data, _ := json.Marshal(info)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// HealthData combines cluster info and pod metrics for the combined SSE endpoint
type HealthData struct {
	Type    string      `json:"type"` // "cluster" or "pods"
	Payload interface{} `json:"payload"`
}

// SSE handler for combined health data (cluster-info + pod-metrics)
func handleHealthSSE(w http.ResponseWriter, r *http.Request, cache *MetricsCache) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// CORS handled by middleware

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Extract user ID from token for pod filtering
	var userID string
	token := getTokenFromRequest(r)
	if claims := validateToken(token); claims != nil {
		userID = claims.UserID
	}

	// Subscribe to both updates
	clusterUpdates := cache.Subscribe()
	defer cache.Unsubscribe(clusterUpdates)
	podUpdates := cache.SubscribePods()
	defer cache.UnsubscribePods(podUpdates)

	// Send current data immediately
	clusterData, _ := json.Marshal(HealthData{Type: "cluster", Payload: cache.GetClusterInfo()})
	fmt.Fprintf(w, "data: %s\n\n", clusterData)

	// Filter pods for this user
	podMetrics := cache.GetPodMetrics()
	filteredPodMetrics := &PodMetricsInfo{
		Timestamp: podMetrics.Timestamp,
		Pods:      filterPodsForUser(podMetrics.Pods, userID),
	}
	podData, _ := json.Marshal(HealthData{Type: "pods", Payload: filteredPodMetrics})
	fmt.Fprintf(w, "data: %s\n\n", podData)
	flusher.Flush()

	// Send updates as they come in
	for {
		select {
		case <-r.Context().Done():
			return
		case info := <-clusterUpdates:
			data, _ := json.Marshal(HealthData{Type: "cluster", Payload: info})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case info := <-podUpdates:
			// Filter pods for this user
			filteredInfo := &PodMetricsInfo{
				Timestamp: info.Timestamp,
				Pods:      filterPodsForUser(info.Pods, userID),
			}
			data, _ := json.Marshal(HealthData{Type: "pods", Payload: filteredInfo})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// MetricsResponse is the response format for historical metrics
type MetricsResponse struct {
	Start      time.Time                       `json:"start"`
	End        time.Time                       `json:"end"`
	Resolution string                          `json:"resolution"`
	Series     map[string][]timeseries.DataPoint `json:"series"`
}

// PodMetricsResponse is the response format for historical pod metrics
type PodMetricsResponse struct {
	Start      time.Time                          `json:"start"`
	End        time.Time                          `json:"end"`
	Resolution string                             `json:"resolution"`
	Series     map[string][]timeseries.PodDataPoint `json:"series"`
}

func handleMetricsNodes(w http.ResponseWriter, r *http.Request, store *timeseries.MetricsStore) {
	start, end, resolution := parseTimeRange(r)

	series := store.QueryNodes(start, end, resolution)

	resolutionStr := "raw"
	if resolution >= 15*time.Minute {
		resolutionStr = "15m"
	} else if resolution >= 5*time.Minute {
		resolutionStr = "5m"
	} else if resolution >= time.Minute {
		resolutionStr = "1m"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(MetricsResponse{
		Start:      start,
		End:        end,
		Resolution: resolutionStr,
		Series:     series,
	})
}

func handleMetricsNode(w http.ResponseWriter, r *http.Request, store *timeseries.MetricsStore) {
	// Extract node name from URL path: /api/metrics/nodes/{name}
	path := strings.TrimPrefix(r.URL.Path, "/api/metrics/nodes/")
	if path == "" {
		http.Error(w, "node name required", http.StatusBadRequest)
		return
	}

	start, end, resolution := parseTimeRange(r)

	data := store.QueryNode(path, start, end, resolution)
	if data == nil {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	}

	resolutionStr := "raw"
	if resolution >= 15*time.Minute {
		resolutionStr = "15m"
	} else if resolution >= 5*time.Minute {
		resolutionStr = "5m"
	} else if resolution >= time.Minute {
		resolutionStr = "1m"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(MetricsResponse{
		Start:      start,
		End:        end,
		Resolution: resolutionStr,
		Series:     map[string][]timeseries.DataPoint{path: data},
	})
}

func handleMetricsPods(w http.ResponseWriter, r *http.Request, store *timeseries.MetricsStore) {
	start, end, resolution := parseTimeRange(r)
	namespace := r.URL.Query().Get("namespace")

	series := store.QueryPods(namespace, start, end, resolution)

	resolutionStr := "raw"
	if resolution >= 15*time.Minute {
		resolutionStr = "15m"
	} else if resolution >= 5*time.Minute {
		resolutionStr = "5m"
	} else if resolution >= time.Minute {
		resolutionStr = "1m"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(PodMetricsResponse{
		Start:      start,
		End:        end,
		Resolution: resolutionStr,
		Series:     series,
	})
}

func parseTimeRange(r *http.Request) (start, end time.Time, resolution time.Duration) {
	now := time.Now()

	// Parse start time (default: 1 hour ago)
	if s := r.URL.Query().Get("start"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			start = t
		}
	}
	if start.IsZero() {
		start = now.Add(-time.Hour)
	}

	// Parse end time (default: now)
	if e := r.URL.Query().Get("end"); e != "" {
		if t, err := time.Parse(time.RFC3339, e); err == nil {
			end = t
		}
	}
	if end.IsZero() {
		end = now
	}

	// Parse resolution or auto-select based on time range
	res := r.URL.Query().Get("resolution")
	if res != "" {
		resolution = timeseries.ParseResolution(res)
	} else {
		resolution = timeseries.AutoResolution(start, end)
	}

	return
}

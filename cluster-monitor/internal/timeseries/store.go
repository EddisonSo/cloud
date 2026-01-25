package timeseries

import (
	"sync"
	"time"
)

const (
	// DefaultRetention is 24 hours at 5 second intervals = 17,280 points
	DefaultRetention = 17280
)

// PodDataPoint represents a single time-series data point for a pod
type PodDataPoint struct {
	Timestamp  time.Time `json:"t"`
	CPUNanos   int64     `json:"cpu"`   // nanocores
	MemBytes   int64     `json:"mem"`   // bytes
	DiskBytes  int64     `json:"disk"`  // bytes
}

// PodRingBuffer is a thread-safe circular buffer for pod time-series data
type PodRingBuffer struct {
	mu       sync.RWMutex
	data     []PodDataPoint
	capacity int
	head     int
	full     bool
}

// NewPodRingBuffer creates a new ring buffer for pod metrics
func NewPodRingBuffer(capacity int) *PodRingBuffer {
	return &PodRingBuffer{
		data:     make([]PodDataPoint, capacity),
		capacity: capacity,
	}
}

// Add adds a data point to the buffer
func (r *PodRingBuffer) Add(point PodDataPoint) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.data[r.head] = point
	r.head = (r.head + 1) % r.capacity
	if r.head == 0 {
		r.full = true
	}
}

// Query returns data points within the given time range
func (r *PodRingBuffer) Query(start, end time.Time, resolution time.Duration) []PodDataPoint {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []PodDataPoint
	var count int

	if r.full {
		count = r.capacity
	} else {
		count = r.head
	}

	if count == 0 {
		return result
	}

	startIdx := 0
	if r.full {
		startIdx = r.head
	}

	for i := 0; i < count; i++ {
		idx := (startIdx + i) % r.capacity
		point := r.data[idx]
		if (point.Timestamp.Equal(start) || point.Timestamp.After(start)) &&
			(point.Timestamp.Equal(end) || point.Timestamp.Before(end)) {
			result = append(result, point)
		}
	}

	if resolution > 0 && len(result) > 0 {
		result = downsamplePod(result, resolution)
	}

	return result
}

func downsamplePod(points []PodDataPoint, resolution time.Duration) []PodDataPoint {
	if len(points) == 0 {
		return points
	}

	var result []PodDataPoint
	var bucket []PodDataPoint
	bucketStart := points[0].Timestamp.Truncate(resolution)

	for _, p := range points {
		pBucket := p.Timestamp.Truncate(resolution)
		if pBucket.Equal(bucketStart) {
			bucket = append(bucket, p)
		} else {
			if len(bucket) > 0 {
				result = append(result, aggregatePodBucket(bucket, bucketStart))
			}
			bucket = []PodDataPoint{p}
			bucketStart = pBucket
		}
	}

	if len(bucket) > 0 {
		result = append(result, aggregatePodBucket(bucket, bucketStart))
	}

	return result
}

func aggregatePodBucket(points []PodDataPoint, timestamp time.Time) PodDataPoint {
	if len(points) == 0 {
		return PodDataPoint{Timestamp: timestamp}
	}

	var sumCPU, sumMem, sumDisk int64
	for _, p := range points {
		sumCPU += p.CPUNanos
		sumMem += p.MemBytes
		sumDisk += p.DiskBytes
	}

	n := int64(len(points))
	return PodDataPoint{
		Timestamp: timestamp,
		CPUNanos:  sumCPU / n,
		MemBytes:  sumMem / n,
		DiskBytes: sumDisk / n,
	}
}

// MetricsStore holds historical metrics for nodes and pods
type MetricsStore struct {
	mu         sync.RWMutex
	nodeBuffers map[string]*RingBuffer
	podBuffers  map[string]*PodRingBuffer // key: "namespace/podname"
	capacity    int
}

// NewMetricsStore creates a new metrics store with the given capacity per entity
func NewMetricsStore(capacity int) *MetricsStore {
	if capacity <= 0 {
		capacity = DefaultRetention
	}
	return &MetricsStore{
		nodeBuffers: make(map[string]*RingBuffer),
		podBuffers:  make(map[string]*PodRingBuffer),
		capacity:    capacity,
	}
}

// RecordNode records a node metrics data point
func (s *MetricsStore) RecordNode(name string, point DataPoint) {
	s.mu.Lock()
	buf, ok := s.nodeBuffers[name]
	if !ok {
		buf = NewRingBuffer(s.capacity)
		s.nodeBuffers[name] = buf
	}
	s.mu.Unlock()

	buf.Add(point)
}

// RecordPod records a pod metrics data point
func (s *MetricsStore) RecordPod(namespace, name string, point PodDataPoint) {
	key := namespace + "/" + name
	s.mu.Lock()
	buf, ok := s.podBuffers[key]
	if !ok {
		buf = NewPodRingBuffer(s.capacity)
		s.podBuffers[key] = buf
	}
	s.mu.Unlock()

	buf.Add(point)
}

// QueryNodes returns historical data for all nodes
func (s *MetricsStore) QueryNodes(start, end time.Time, resolution time.Duration) map[string][]DataPoint {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string][]DataPoint)
	for name, buf := range s.nodeBuffers {
		data := buf.Query(start, end, resolution)
		if len(data) > 0 {
			result[name] = data
		}
	}
	return result
}

// QueryNode returns historical data for a single node
func (s *MetricsStore) QueryNode(name string, start, end time.Time, resolution time.Duration) []DataPoint {
	s.mu.RLock()
	buf, ok := s.nodeBuffers[name]
	s.mu.RUnlock()

	if !ok {
		return nil
	}
	return buf.Query(start, end, resolution)
}

// QueryPods returns historical data for pods in a namespace (empty namespace = all)
func (s *MetricsStore) QueryPods(namespace string, start, end time.Time, resolution time.Duration) map[string][]PodDataPoint {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string][]PodDataPoint)
	for key, buf := range s.podBuffers {
		// Filter by namespace if provided
		if namespace != "" && (len(key) <= len(namespace)+1 || key[:len(namespace)+1] != namespace+"/") {
			continue
		}
		data := buf.Query(start, end, resolution)
		if len(data) > 0 {
			result[key] = data
		}
	}
	return result
}

// GetNodeNames returns all node names being tracked
func (s *MetricsStore) GetNodeNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.nodeBuffers))
	for name := range s.nodeBuffers {
		names = append(names, name)
	}
	return names
}

// ParseResolution converts a resolution string to a duration
func ParseResolution(res string) time.Duration {
	switch res {
	case "1m":
		return time.Minute
	case "5m":
		return 5 * time.Minute
	case "15m":
		return 15 * time.Minute
	case "raw", "":
		return 0
	default:
		return 0
	}
}

// AutoResolution picks an appropriate resolution based on time range
func AutoResolution(start, end time.Time) time.Duration {
	duration := end.Sub(start)
	switch {
	case duration <= time.Hour:
		return 0 // raw
	case duration <= 6*time.Hour:
		return time.Minute
	case duration <= 24*time.Hour:
		return 5 * time.Minute
	default:
		return 15 * time.Minute
	}
}

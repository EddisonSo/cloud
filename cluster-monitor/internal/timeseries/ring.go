package timeseries

import (
	"sync"
	"time"
)

// DataPoint represents a single time-series data point
type DataPoint struct {
	Timestamp  time.Time `json:"t"`
	CPUPercent float64   `json:"cpu"`
	MemPercent float64   `json:"mem"`
	DiskPercent float64  `json:"disk"`
}

// RingBuffer is a thread-safe circular buffer for time-series data
type RingBuffer struct {
	mu       sync.RWMutex
	data     []DataPoint
	capacity int
	head     int  // next write position
	full     bool // whether buffer has wrapped
}

// NewRingBuffer creates a new ring buffer with the given capacity
func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		data:     make([]DataPoint, capacity),
		capacity: capacity,
	}
}

// Add adds a data point to the buffer
func (r *RingBuffer) Add(point DataPoint) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.data[r.head] = point
	r.head = (r.head + 1) % r.capacity
	if r.head == 0 {
		r.full = true
	}
}

// Len returns the number of data points in the buffer
func (r *RingBuffer) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.full {
		return r.capacity
	}
	return r.head
}

// Query returns data points within the given time range
// If resolution is provided and > 0, data will be downsampled to that interval
func (r *RingBuffer) Query(start, end time.Time, resolution time.Duration) []DataPoint {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []DataPoint
	var count int

	if r.full {
		count = r.capacity
	} else {
		count = r.head
	}

	if count == 0 {
		return result
	}

	// Calculate starting index
	startIdx := 0
	if r.full {
		startIdx = r.head
	}

	// Collect points in time range
	for i := 0; i < count; i++ {
		idx := (startIdx + i) % r.capacity
		point := r.data[idx]
		if (point.Timestamp.Equal(start) || point.Timestamp.After(start)) &&
			(point.Timestamp.Equal(end) || point.Timestamp.Before(end)) {
			result = append(result, point)
		}
	}

	// Downsample if resolution is specified
	if resolution > 0 && len(result) > 0 {
		result = downsample(result, resolution)
	}

	return result
}

// GetAll returns all data points in chronological order
func (r *RingBuffer) GetAll() []DataPoint {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var count int
	if r.full {
		count = r.capacity
	} else {
		count = r.head
	}

	if count == 0 {
		return nil
	}

	result := make([]DataPoint, count)
	startIdx := 0
	if r.full {
		startIdx = r.head
	}

	for i := 0; i < count; i++ {
		idx := (startIdx + i) % r.capacity
		result[i] = r.data[idx]
	}

	return result
}

// downsample aggregates data points into buckets of the given duration
func downsample(points []DataPoint, resolution time.Duration) []DataPoint {
	if len(points) == 0 {
		return points
	}

	var result []DataPoint
	var bucket []DataPoint
	bucketStart := points[0].Timestamp.Truncate(resolution)

	for _, p := range points {
		pBucket := p.Timestamp.Truncate(resolution)
		if pBucket.Equal(bucketStart) {
			bucket = append(bucket, p)
		} else {
			if len(bucket) > 0 {
				result = append(result, aggregateBucket(bucket, bucketStart))
			}
			bucket = []DataPoint{p}
			bucketStart = pBucket
		}
	}

	if len(bucket) > 0 {
		result = append(result, aggregateBucket(bucket, bucketStart))
	}

	return result
}

// aggregateBucket computes the average of all points in a bucket
func aggregateBucket(points []DataPoint, timestamp time.Time) DataPoint {
	if len(points) == 0 {
		return DataPoint{Timestamp: timestamp}
	}

	var sumCPU, sumMem, sumDisk float64
	for _, p := range points {
		sumCPU += p.CPUPercent
		sumMem += p.MemPercent
		sumDisk += p.DiskPercent
	}

	n := float64(len(points))
	return DataPoint{
		Timestamp:   timestamp,
		CPUPercent:  sumCPU / n,
		MemPercent:  sumMem / n,
		DiskPercent: sumDisk / n,
	}
}

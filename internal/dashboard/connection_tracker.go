package dashboard

import (
	"fmt"
	"sync"
	"time"
)

// TrackedConnection represents a monitored connection
type TrackedConnection struct {
	ID            string    `json:"id"`
	ClientAddr    string    `json:"client"`
	Destination   string    `json:"destination"`
	StartTime     time.Time `json:"start_time"`
	BytesIn       int64     `json:"bytes_in"`
	BytesOut      int64     `json:"bytes_out"`
	LastActivity  time.Time `json:"last_activity"`
	Latency       float64   `json:"latency_ms"`
	State         string    `json:"state"` // active, closing, error
}

// ConnectionTracker manages active connections for dashboard monitoring
type ConnectionTracker struct {
	mu          sync.RWMutex
	connections map[string]*TrackedConnection
	// Historical data for graphs (ring buffer)
	history     *MetricHistory
}

// MetricHistory stores time-series data in a ring buffer
type MetricHistory struct {
	mu          sync.RWMutex
	timestamps  []time.Time
	connCounts  []int
	byteRates   []float64
	latencies   []float64
	maxPoints   int
	writeIndex  int
}

// NewConnectionTracker creates a new connection tracker
func NewConnectionTracker() *ConnectionTracker {
	return &ConnectionTracker{
		connections: make(map[string]*TrackedConnection),
		history:     NewMetricHistory(300), // 5 minutes at 1 second intervals
	}
}

// NewMetricHistory creates a new metric history with specified capacity
func NewMetricHistory(maxPoints int) *MetricHistory {
	return &MetricHistory{
		timestamps: make([]time.Time, maxPoints),
		connCounts: make([]int, maxPoints),
		byteRates:  make([]float64, maxPoints),
		latencies:  make([]float64, maxPoints),
		maxPoints:  maxPoints,
	}
}

// AddConnection registers a new connection
func (ct *ConnectionTracker) AddConnection(id, clientAddr, destination string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	
	ct.connections[id] = &TrackedConnection{
		ID:           id,
		ClientAddr:   clientAddr,
		Destination:  destination,
		StartTime:    time.Now(),
		LastActivity: time.Now(),
		State:        "active",
	}
	
	// Debug logging
	fmt.Printf("🔗 Dashboard: Added connection %s: %s -> %s (total: %d)\n", id, clientAddr, destination, len(ct.connections))
}

// UpdateConnection updates connection metrics
func (ct *ConnectionTracker) UpdateConnection(id string, bytesIn, bytesOut int64, latency float64) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	
	if conn, exists := ct.connections[id]; exists {
		conn.BytesIn += bytesIn
		conn.BytesOut += bytesOut
		conn.LastActivity = time.Now()
		if latency > 0 {
			conn.Latency = latency
		}
	}
}

// RemoveConnection removes a connection
func (ct *ConnectionTracker) RemoveConnection(id string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	
	if conn, exists := ct.connections[id]; exists {
		conn.State = "closing"
		fmt.Printf("🔚 Dashboard: Closing connection %s: %s -> %s\n", id, conn.ClientAddr, conn.Destination)
		// Keep it for a short time for UI transitions
		go func() {
			time.Sleep(2 * time.Second)
			ct.mu.Lock()
			delete(ct.connections, id)
			fmt.Printf("🗑️  Dashboard: Removed connection %s (remaining: %d)\n", id, len(ct.connections))
			ct.mu.Unlock()
		}()
	}
}

// SetConnectionError marks a connection as having an error
func (ct *ConnectionTracker) SetConnectionError(id string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	
	if conn, exists := ct.connections[id]; exists {
		conn.State = "error"
	}
}

// GetActiveConnections returns all currently tracked connections
func (ct *ConnectionTracker) GetActiveConnections() []*TrackedConnection {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	
	connections := make([]*TrackedConnection, 0, len(ct.connections))
	for _, conn := range ct.connections {
		// Create a copy to avoid data races
		connCopy := *conn
		connections = append(connections, &connCopy)
	}
	
	return connections
}

// GetConnectionCount returns the current number of active connections
func (ct *ConnectionTracker) GetConnectionCount() int {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	
	activeCount := 0
	for _, conn := range ct.connections {
		if conn.State == "active" {
			activeCount++
		}
	}
	return activeCount
}

// GetTotalBytes returns total bytes transferred across all connections
func (ct *ConnectionTracker) GetTotalBytes() (int64, int64) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	
	var totalIn, totalOut int64
	for _, conn := range ct.connections {
		totalIn += conn.BytesIn
		totalOut += conn.BytesOut
	}
	
	return totalIn, totalOut
}

// GetAverageLatency returns the average latency across active connections
func (ct *ConnectionTracker) GetAverageLatency() float64 {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	
	var totalLatency float64
	var count int
	
	for _, conn := range ct.connections {
		if conn.State == "active" && conn.Latency > 0 {
			totalLatency += conn.Latency
			count++
		}
	}
	
	if count == 0 {
		return 0
	}
	
	return totalLatency / float64(count)
}

// RecordMetrics adds a data point to the historical metrics
func (ct *ConnectionTracker) RecordMetrics(byteRate float64) {
	ct.history.mu.Lock()
	defer ct.history.mu.Unlock()
	
	now := time.Now()
	connCount := ct.GetConnectionCount()
	avgLatency := ct.GetAverageLatency()
	
	// Add to ring buffer
	ct.history.timestamps[ct.history.writeIndex] = now
	ct.history.connCounts[ct.history.writeIndex] = connCount
	ct.history.byteRates[ct.history.writeIndex] = byteRate
	ct.history.latencies[ct.history.writeIndex] = avgLatency
	
	ct.history.writeIndex = (ct.history.writeIndex + 1) % ct.history.maxPoints
}

// GetHistory returns historical metrics data
func (ct *ConnectionTracker) GetHistory() ([]time.Time, []int, []float64, []float64) {
	ct.history.mu.RLock()
	defer ct.history.mu.RUnlock()
	
	// Return copies to avoid data races
	timestamps := make([]time.Time, ct.history.maxPoints)
	connCounts := make([]int, ct.history.maxPoints)
	byteRates := make([]float64, ct.history.maxPoints)
	latencies := make([]float64, ct.history.maxPoints)
	
	copy(timestamps, ct.history.timestamps)
	copy(connCounts, ct.history.connCounts)
	copy(byteRates, ct.history.byteRates)
	copy(latencies, ct.history.latencies)
	
	return timestamps, connCounts, byteRates, latencies
}

// Global instance
var GlobalConnectionTracker = NewConnectionTracker()

// Metrics collection control
var (
	metricsStopCh = make(chan struct{})
	metricsRunning = false
)

// StartMetricsCollection begins collecting metrics at regular intervals
func StartMetricsCollection() {
	if metricsRunning {
		return
	}
	metricsRunning = true
	
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		
		var lastTotalBytes int64
		var lastTime time.Time = time.Now()
		
		for {
			select {
			case <-metricsStopCh:
				return
			case <-ticker.C:
				totalIn, totalOut := GlobalConnectionTracker.GetTotalBytes()
				currentTotalBytes := totalIn + totalOut
				
				now := time.Now()
				duration := now.Sub(lastTime).Seconds()
				
				var byteRate float64
				if duration > 0 && currentTotalBytes >= lastTotalBytes {
					byteRate = float64(currentTotalBytes-lastTotalBytes) / duration
				}
				
				GlobalConnectionTracker.RecordMetrics(byteRate)
				
				lastTotalBytes = currentTotalBytes
				lastTime = now
			}
		}
	}()
}

// StopMetricsCollection stops the metrics collection goroutine
func StopMetricsCollection() {
	if !metricsRunning {
		return
	}
	metricsRunning = false
	
	select {
	case metricsStopCh <- struct{}{}:
	default: // Channel might be full, that's ok
	}
}
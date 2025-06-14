package metrics

import (
	"expvar"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// Session Metrics
	SessionRTTMs         = expvar.NewFloat("session_rtt_ms")
	sessionHealthy       = expvar.NewInt("session_healthy")
	sessionPingsSent     = expvar.NewInt("session_pings_sent")
	sessionPongsReceived = expvar.NewInt("session_pongs_received")
	sessionMissedPings   = expvar.NewInt("session_missed_pings")
	sessionRotations     = expvar.NewInt("session_rotations")
	sessionLaunches      = expvar.NewInt("session_launches")
	sessionFailures      = expvar.NewInt("session_failures")
	activeSessions       = expvar.NewInt("active_sessions")
	
	// SOCKS5 Proxy Metrics
	socks5Connections    = expvar.NewInt("socks5_connections_total")
	socks5ActiveConns    = expvar.NewInt("socks5_active_connections")
	socks5BytesTransferred = expvar.NewInt("socks5_bytes_transferred")
	socks5FailedConns    = expvar.NewInt("socks5_failed_connections")
	socks5AvgLatencyMs   = expvar.NewFloat("socks5_avg_latency_ms")
	
	// QUIC Metrics
	quicStreamsActive    = expvar.NewInt("quic_streams_active")
	quicStreamsTotal     = expvar.NewInt("quic_streams_total")
	quicBytesTransferred = expvar.NewInt("quic_bytes_transferred")
	quicConnErrors       = expvar.NewInt("quic_connection_errors")
	quicHandshakeTime    = expvar.NewFloat("quic_handshake_time_ms")
	
	// AWS Service Metrics
	s3Operations         = expvar.NewInt("s3_operations_total")
	s3Errors            = expvar.NewInt("s3_errors_total")
	lambdaInvocations   = expvar.NewInt("lambda_invocations_total")
	lambdaErrors        = expvar.NewInt("lambda_errors_total")
	awsAPILatency       = expvar.NewFloat("aws_api_latency_ms")
	
	// System Metrics
	systemGoroutines     = expvar.NewInt("system_goroutines")
	systemMemoryAlloc    = expvar.NewInt("system_memory_alloc_bytes")
	systemMemoryTotal    = expvar.NewInt("system_memory_total_bytes")
	systemMemorySys      = expvar.NewInt("system_memory_sys_bytes")
	systemGCPauses       = expvar.NewFloat("system_gc_pause_ns")
	
	// Performance Metrics
	networkLatencyMs     = expvar.NewFloat("network_latency_ms")
	stunLatencyMs        = expvar.NewFloat("stun_latency_ms")
	natTraversalTime     = expvar.NewFloat("nat_traversal_time_ms")
	
	// Internal tracking
	rttMutex            sync.RWMutex
	lastRTT             time.Duration
	latencyMutex        sync.RWMutex
	latencySum          float64
	latencyCount        int64
	
	// Atomic counters for high-frequency updates
	bytesTransferredAtomic int64
	connectionsAtomic      int64
	
	// Start time for uptime calculation
	startTime = time.Now()
)

// Session Metrics Functions
func RecordRTT(rtt time.Duration) {
	rttMutex.Lock()
	defer rttMutex.Unlock()
	
	lastRTT = rtt
	SessionRTTMs.Set(float64(rtt.Milliseconds()))
	sessionPongsReceived.Add(1)
}

func RecordPingSent() {
	sessionPingsSent.Add(1)
}

func RecordMissedPing() {
	sessionMissedPings.Add(1)
}

func SetSessionHealthy(healthy bool) {
	if healthy {
		sessionHealthy.Set(1)
	} else {
		sessionHealthy.Set(0)
	}
}

func RecordSessionRotation() {
	sessionRotations.Add(1)
}

func RecordSessionLaunch() {
	sessionLaunches.Add(1)
}

func RecordSessionFailure() {
	sessionFailures.Add(1)
}

func SetActiveSessions(count int) {
	activeSessions.Set(int64(count))
}

func GetLastRTT() time.Duration {
	rttMutex.RLock()
	defer rttMutex.RUnlock()
	return lastRTT
}

// SOCKS5 Proxy Metrics Functions
func RecordSOCKS5Connection() {
	socks5Connections.Add(1)
	atomic.AddInt64(&connectionsAtomic, 1)
}

func IncrementActiveSOCKS5Connections() {
	socks5ActiveConns.Add(1)
}

func DecrementActiveSOCKS5Connections() {
	socks5ActiveConns.Add(-1)
}

func RecordSOCKS5BytesTransferred(bytes int64) {
	socks5BytesTransferred.Add(bytes)
	atomic.AddInt64(&bytesTransferredAtomic, bytes)
}

func RecordSOCKS5FailedConnection() {
	socks5FailedConns.Add(1)
}

func RecordSOCKS5Latency(latency time.Duration) {
	latencyMutex.Lock()
	defer latencyMutex.Unlock()
	
	latencySum += float64(latency.Milliseconds())
	latencyCount++
	avgLatency := latencySum / float64(latencyCount)
	socks5AvgLatencyMs.Set(avgLatency)
}

// QUIC Metrics Functions
func IncrementActiveQUICStreams() {
	quicStreamsActive.Add(1)
	quicStreamsTotal.Add(1)
}

func DecrementActiveQUICStreams() {
	quicStreamsActive.Add(-1)
}

func RecordQUICBytesTransferred(bytes int64) {
	quicBytesTransferred.Add(bytes)
}

func RecordQUICConnectionError() {
	quicConnErrors.Add(1)
}

func RecordQUICHandshakeTime(duration time.Duration) {
	quicHandshakeTime.Set(float64(duration.Milliseconds()))
}

// AWS Service Metrics Functions
func RecordS3Operation() {
	s3Operations.Add(1)
}

func RecordS3Error() {
	s3Errors.Add(1)
}

func RecordLambdaInvocation() {
	lambdaInvocations.Add(1)
}

func RecordLambdaError() {
	lambdaErrors.Add(1)
}

func RecordAWSAPILatency(latency time.Duration) {
	awsAPILatency.Set(float64(latency.Milliseconds()))
}

// Performance Metrics Functions
func RecordNetworkLatency(latency time.Duration) {
	networkLatencyMs.Set(float64(latency.Milliseconds()))
}

func RecordSTUNLatency(latency time.Duration) {
	stunLatencyMs.Set(float64(latency.Milliseconds()))
}

func RecordNATTraversalTime(duration time.Duration) {
	natTraversalTime.Set(float64(duration.Milliseconds()))
}

// System Metrics Functions
func UpdateSystemMetrics() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	systemGoroutines.Set(int64(runtime.NumGoroutine()))
	systemMemoryAlloc.Set(int64(m.Alloc))
	systemMemoryTotal.Set(int64(m.TotalAlloc))
	systemMemorySys.Set(int64(m.Sys))
	
	if len(m.PauseNs) > 0 {
		systemGCPauses.Set(float64(m.PauseNs[(m.NumGC+255)%256]))
	}
}

// Metrics Server Functions
func StartMetricsServer(addr string) error {
	// Add custom metrics to expvar
	expvar.Publish("uptime_seconds", expvar.Func(func() interface{} {
		return time.Since(startTime).Seconds()
	}))
	
	expvar.Publish("total_connections", expvar.Func(func() interface{} {
		return atomic.LoadInt64(&connectionsAtomic)
	}))
	
	expvar.Publish("total_bytes_transferred", expvar.Func(func() interface{} {
		return atomic.LoadInt64(&bytesTransferredAtomic)
	}))
	
	// Start system metrics update routine
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				UpdateSystemMetrics()
			}
		}
	}()
	
	// Create HTTP server for metrics
	mux := http.NewServeMux()
	mux.Handle("/metrics", http.HandlerFunc(metricsHandler))
	mux.Handle("/debug/vars", expvar.Handler())
	
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	
	return server.ListenAndServe()
}

// Custom metrics handler that provides Prometheus-compatible output
func metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	
	// Write Prometheus-style metrics
	fmt.Fprintf(w, "# HELP session_rtt_ms Current session RTT in milliseconds\n")
	fmt.Fprintf(w, "# TYPE session_rtt_ms gauge\n")
	fmt.Fprintf(w, "session_rtt_ms %v\n", SessionRTTMs.Value())
	
	fmt.Fprintf(w, "# HELP session_healthy Whether the current session is healthy (1) or not (0)\n")
	fmt.Fprintf(w, "# TYPE session_healthy gauge\n")
	fmt.Fprintf(w, "session_healthy %v\n", sessionHealthy.Value())
	
	fmt.Fprintf(w, "# HELP session_pings_sent_total Total number of pings sent\n")
	fmt.Fprintf(w, "# TYPE session_pings_sent_total counter\n")
	fmt.Fprintf(w, "session_pings_sent_total %v\n", sessionPingsSent.Value())
	
	fmt.Fprintf(w, "# HELP session_pongs_received_total Total number of pongs received\n")
	fmt.Fprintf(w, "# TYPE session_pongs_received_total counter\n")
	fmt.Fprintf(w, "session_pongs_received_total %v\n", sessionPongsReceived.Value())
	
	fmt.Fprintf(w, "# HELP session_missed_pings_total Total number of missed pings\n")
	fmt.Fprintf(w, "# TYPE session_missed_pings_total counter\n")
	fmt.Fprintf(w, "session_missed_pings_total %v\n", sessionMissedPings.Value())
	
	fmt.Fprintf(w, "# HELP session_rotations_total Total number of session rotations\n")
	fmt.Fprintf(w, "# TYPE session_rotations_total counter\n")
	fmt.Fprintf(w, "session_rotations_total %v\n", sessionRotations.Value())
	
	fmt.Fprintf(w, "# HELP active_sessions Number of currently active sessions\n")
	fmt.Fprintf(w, "# TYPE active_sessions gauge\n")
	fmt.Fprintf(w, "active_sessions %v\n", activeSessions.Value())
	
	fmt.Fprintf(w, "# HELP socks5_connections_total Total number of SOCKS5 connections\n")
	fmt.Fprintf(w, "# TYPE socks5_connections_total counter\n")
	fmt.Fprintf(w, "socks5_connections_total %v\n", socks5Connections.Value())
	
	fmt.Fprintf(w, "# HELP socks5_active_connections Number of currently active SOCKS5 connections\n")
	fmt.Fprintf(w, "# TYPE socks5_active_connections gauge\n")
	fmt.Fprintf(w, "socks5_active_connections %v\n", socks5ActiveConns.Value())
	
	fmt.Fprintf(w, "# HELP socks5_bytes_transferred_total Total bytes transferred through SOCKS5 proxy\n")
	fmt.Fprintf(w, "# TYPE socks5_bytes_transferred_total counter\n")
	fmt.Fprintf(w, "socks5_bytes_transferred_total %v\n", socks5BytesTransferred.Value())
	
	fmt.Fprintf(w, "# HELP quic_streams_active Number of currently active QUIC streams\n")
	fmt.Fprintf(w, "# TYPE quic_streams_active gauge\n")
	fmt.Fprintf(w, "quic_streams_active %v\n", quicStreamsActive.Value())
	
	fmt.Fprintf(w, "# HELP quic_streams_total Total number of QUIC streams created\n")
	fmt.Fprintf(w, "# TYPE quic_streams_total counter\n")
	fmt.Fprintf(w, "quic_streams_total %v\n", quicStreamsTotal.Value())
	
	fmt.Fprintf(w, "# HELP s3_operations_total Total number of S3 operations\n")
	fmt.Fprintf(w, "# TYPE s3_operations_total counter\n")
	fmt.Fprintf(w, "s3_operations_total %v\n", s3Operations.Value())
	
	fmt.Fprintf(w, "# HELP lambda_invocations_total Total number of Lambda invocations\n")
	fmt.Fprintf(w, "# TYPE lambda_invocations_total counter\n")
	fmt.Fprintf(w, "lambda_invocations_total %v\n", lambdaInvocations.Value())
	
	fmt.Fprintf(w, "# HELP system_goroutines Number of active goroutines\n")
	fmt.Fprintf(w, "# TYPE system_goroutines gauge\n")
	fmt.Fprintf(w, "system_goroutines %v\n", systemGoroutines.Value())
	
	fmt.Fprintf(w, "# HELP system_memory_alloc_bytes Currently allocated memory in bytes\n")
	fmt.Fprintf(w, "# TYPE system_memory_alloc_bytes gauge\n")
	fmt.Fprintf(w, "system_memory_alloc_bytes %v\n", systemMemoryAlloc.Value())
	
	uptime := time.Since(startTime).Seconds()
	fmt.Fprintf(w, "# HELP uptime_seconds Process uptime in seconds\n")
	fmt.Fprintf(w, "# TYPE uptime_seconds gauge\n")
	fmt.Fprintf(w, "uptime_seconds %v\n", uptime)
}

// Getter functions for dashboard
func GetSystemMemoryAlloc() int64 {
	return systemMemoryAlloc.Value()
}

func GetSystemGoroutines() int {
	return int(systemGoroutines.Value())
}
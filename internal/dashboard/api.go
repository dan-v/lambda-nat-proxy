package dashboard

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/dan-v/lambda-nat-punch-proxy/internal/manager"
	"github.com/dan-v/lambda-nat-punch-proxy/pkg/shared"
)

// DashboardServer serves the dashboard API and static files
type DashboardServer struct {
	collector *DashboardCollector
	mux       *http.ServeMux
	upgrader  websocket.Upgrader
	clients   map[*websocket.Conn]bool
	clientsMu sync.RWMutex
	broadcast chan []byte
	shutdown  chan struct{}
}

// NewDashboardServer creates a new dashboard server
func NewDashboardServer(cm *manager.ConnManager) *DashboardServer {
	server := &DashboardServer{
		collector: NewDashboardCollector(cm),
		mux:       http.NewServeMux(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for development
			},
		},
		clients:   make(map[*websocket.Conn]bool),
		broadcast: make(chan []byte),
		shutdown:  make(chan struct{}),
	}
	
	server.setupRoutes()
	server.startBroadcaster()
	return server
}

// setupRoutes configures all API routes
func (ds *DashboardServer) setupRoutes() {
	// API endpoints
	ds.mux.HandleFunc("/api/dashboard", ds.handleDashboardData)
	ds.mux.HandleFunc("/api/connections", ds.handleConnections)
	ds.mux.HandleFunc("/api/sessions", ds.handleSessions)
	ds.mux.HandleFunc("/api/destinations", ds.handleDestinations)
	ds.mux.HandleFunc("/ws", ds.handleWebSocket)
	
	// Static files - we'll serve our React app here
	ds.mux.HandleFunc("/", ds.handleStaticFiles)
}

// ServeHTTP implements the http.Handler interface
func (ds *DashboardServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Add CORS headers for development
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	
	ds.mux.ServeHTTP(w, r)
}

// handleDashboardData serves the complete dashboard data
func (ds *DashboardServer) handleDashboardData(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	data := ds.collector.CollectDashboardData()
	
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		shared.LogErrorf("Failed to encode dashboard data: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// handleConnections serves just the connections data (lighter endpoint)
func (ds *DashboardServer) handleConnections(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	connections := GlobalConnectionTracker.GetActiveConnections()
	
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(connections); err != nil {
		shared.LogErrorf("Failed to encode connections data: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// handleSessions serves session information
func (ds *DashboardServer) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	sessions := ds.collector.collectSessionInfo()
	
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(sessions); err != nil {
		shared.LogErrorf("Failed to encode sessions data: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// handleDestinations serves destination statistics
func (ds *DashboardServer) handleDestinations(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	connections := GlobalConnectionTracker.GetActiveConnections()
	destinations := ds.collector.calculateDestinationStats(connections)
	
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(destinations); err != nil {
		shared.LogErrorf("Failed to encode destinations data: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// handleWebSocket handles WebSocket connections for real-time updates
func (ds *DashboardServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := ds.upgrader.Upgrade(w, r, nil)
	if err != nil {
		shared.LogErrorf("Failed to upgrade WebSocket connection: %v", err)
		return
	}
	
	ds.clientsMu.Lock()
	ds.clients[conn] = true
	ds.clientsMu.Unlock()
	
	shared.LogInfof("New WebSocket client connected, total clients: %d", len(ds.clients))
	
	// Handle client disconnection
	defer func() {
		ds.clientsMu.Lock()
		delete(ds.clients, conn)
		ds.clientsMu.Unlock()
		conn.Close()
		shared.LogInfof("WebSocket client disconnected, remaining clients: %d", len(ds.clients))
	}()
	
	// Send initial dashboard data
	data := ds.collector.CollectDashboardData()
	if jsonData, err := json.Marshal(data); err == nil {
		conn.WriteMessage(websocket.TextMessage, jsonData)
	}
	
	// Keep connection alive and handle pings
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				shared.LogErrorf("WebSocket error: %v", err)
			}
			break
		}
	}
}

// startBroadcaster starts the background broadcaster for WebSocket updates
func (ds *DashboardServer) startBroadcaster() {
	// Start broadcaster goroutine
	go func() {
		for {
			select {
			case message := <-ds.broadcast:
				ds.clientsMu.RLock()
				for client := range ds.clients {
					err := client.WriteMessage(websocket.TextMessage, message)
					if err != nil {
						shared.LogErrorf("Failed to send WebSocket message: %v", err)
						client.Close()
						delete(ds.clients, client)
					}
				}
				ds.clientsMu.RUnlock()
			case <-ds.shutdown:
				shared.LogInfof("Dashboard broadcaster shutting down")
				return
			}
		}
	}()
	
	// Start periodic updates
	go func() {
		ticker := time.NewTicker(1 * time.Second) // Update every second
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				if len(ds.clients) > 0 {
					data := ds.collector.CollectDashboardData()
					if jsonData, err := json.Marshal(data); err == nil {
						select {
						case ds.broadcast <- jsonData:
						case <-ds.shutdown:
							return
						}
					}
				}
			case <-ds.shutdown:
				shared.LogInfof("Dashboard periodic updater shutting down")
				return
			}
		}
	}()
}

// Shutdown gracefully shuts down the dashboard server
func (ds *DashboardServer) Shutdown() {
	close(ds.shutdown)
	
	// Close all WebSocket connections
	ds.clientsMu.Lock()
	for client := range ds.clients {
		client.Close()
	}
	ds.clients = make(map[*websocket.Conn]bool)
	ds.clientsMu.Unlock()
	
	shared.LogInfof("Dashboard server shutdown complete")
}

// StartDashboardServer starts the dashboard HTTP server (legacy function for compatibility)
func StartDashboardServer(addr string, cm *manager.ConnManager) error {
	server := NewDashboardServer(cm)
	
	shared.LogInfof("Starting dashboard server on %s", addr)
	shared.LogInfof("Dashboard available at: http://localhost%s", addr)
	
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      server,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}
	
	return httpServer.ListenAndServe()
}
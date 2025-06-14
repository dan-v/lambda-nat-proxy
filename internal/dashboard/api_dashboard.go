package dashboard

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
)

//go:embed web/dist/*
var webFiles embed.FS

const DashboardEnabled = true

// handleStaticFiles serves the React frontend
func (ds *DashboardServer) handleStaticFiles(w http.ResponseWriter, r *http.Request) {
	// Get the requested path, default to index.html for root
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	
	// For SPA routing, serve index.html for non-asset requests
	if !strings.Contains(path, ".") && !strings.HasPrefix(path, "api/") {
		path = "index.html"
	}
	
	// Construct file path for embedded filesystem
	filePath := filepath.Join("web/dist", path)
	
	// Set appropriate content type based on file extension
	ext := filepath.Ext(path)
	switch ext {
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript")
	case ".css":
		w.Header().Set("Content-Type", "text/css")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	}
	
	// Set cache headers for static assets
	if ext != ".html" {
		w.Header().Set("Cache-Control", "public, max-age=31536000") // 1 year
	}
	
	// Read and serve the file content
	content, err := fs.ReadFile(webFiles, filePath)
	if err != nil {
		content, err = fs.ReadFile(webFiles, "web/dist/index.html")
		if err != nil {
			http.Error(w, "File read error", http.StatusInternalServerError)
			return
		}
	}
	
	// Set content length
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
	
	// Write the content
	w.Write(content)
}
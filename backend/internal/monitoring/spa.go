package monitoring

import (
	"net/http"
	"os"
	"path"
	"strings"
)

// spaHandler serves static files from dir and falls back to index.html
// for any path that doesn't match a real file (SPA client-side routing).
type spaHandler struct {
	dir string
	fs  http.Handler
}

// NewSPAHandler creates a handler that serves the SPA from dir.
// If dir does not exist or is empty, it returns nil.
func NewSPAHandler(dir string) http.Handler {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}
	// Check index.html exists
	if _, err := os.Stat(path.Join(dir, "index.html")); err != nil {
		return nil
	}
	return &spaHandler{
		dir: dir,
		fs:  http.FileServer(http.Dir(dir)),
	}
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Don't serve SPA for API/health/metrics paths
	p := r.URL.Path
	if strings.HasPrefix(p, "/api/") || p == "/healthz" || p == "/readyz" || p == "/metrics" {
		http.NotFound(w, r)
		return
	}

	// Try to serve the actual file
	filePath := path.Join(h.dir, p)
	_, err := os.Stat(filePath)
	if err == nil {
		h.fs.ServeHTTP(w, r)
		return
	}

	// If file doesn't exist and it's not a file with extension (likely a route),
	// serve index.html for client-side routing
	if !os.IsNotExist(err) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Check if it looks like a static asset (has a file extension)
	if ext := path.Ext(p); ext != "" {
		http.NotFound(w, r)
		return
	}

	// Serve index.html for SPA routes
	indexPath := path.Join(h.dir, "index.html")
	http.ServeFile(w, r, indexPath)
}

// Ensure spaHandler satisfies the interface (compile-time check)
var _ http.Handler = (*spaHandler)(nil)

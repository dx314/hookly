package ui

import (
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Handler serves the embedded frontend assets.
type Handler struct {
	fsys    fs.FS
	index   []byte
	devMode bool
	devPath string
}

// NewHandler creates a new UI handler.
// If devPath is set and DEV=true, serves from filesystem for hot reload.
func NewHandler(devPath string) (*Handler, error) {
	h := &Handler{
		devMode: os.Getenv("DEV") == "true",
		devPath: devPath,
	}

	if h.devMode && devPath != "" {
		// Development mode: serve from filesystem
		h.fsys = os.DirFS(devPath)
		indexPath := filepath.Join(devPath, "index.html")
		index, err := os.ReadFile(indexPath)
		if err != nil {
			return nil, err
		}
		h.index = index
	} else {
		// Production mode: serve from embedded FS
		subFS, err := fs.Sub(Assets, "dist")
		if err != nil {
			return nil, err
		}
		h.fsys = subFS
		index, err := fs.ReadFile(subFS, "index.html")
		if err != nil {
			return nil, err
		}
		h.index = index
	}

	return h, nil
}

// ServeHTTP serves the frontend assets.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Clean the path and remove leading slash
	urlPath := path.Clean(r.URL.Path)
	if urlPath == "/" {
		urlPath = "index.html"
	} else {
		urlPath = strings.TrimPrefix(urlPath, "/")
	}

	// Try to open the file
	f, err := h.fsys.Open(urlPath)
	if err != nil {
		// File not found - serve index.html for SPA routing
		h.serveIndex(w)
		return
	}
	defer f.Close()

	// Check if it's a directory
	stat, err := f.Stat()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if stat.IsDir() {
		// Try to serve index.html from the directory
		indexPath := path.Join(urlPath, "index.html")
		indexFile, err := h.fsys.Open(indexPath)
		if err != nil {
			// No index.html in directory - serve main index for SPA
			h.serveIndex(w)
			return
		}
		indexFile.Close()
		f.Close()

		// Reopen the index file
		f, err = h.fsys.Open(indexPath)
		if err != nil {
			h.serveIndex(w)
			return
		}
		defer f.Close()
		stat, err = f.Stat()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		urlPath = indexPath
	}

	// Set content type
	contentType := getContentType(urlPath)
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}

	// Set cache headers
	setCacheHeaders(w, urlPath)

	// Serve the file content
	w.WriteHeader(http.StatusOK)
	io.Copy(w, f)
}

// serveIndex serves the index.html file for SPA routing.
func (h *Handler) serveIndex(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	w.Write(h.index)
}

// getContentType returns the content type for a file based on its extension.
func getContentType(filePath string) string {
	ext := strings.ToLower(path.Ext(filePath))
	switch ext {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js":
		return "application/javascript"
	case ".css":
		return "text/css"
	case ".svg":
		return "image/svg+xml"
	case ".json":
		return "application/json"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".ico":
		return "image/x-icon"
	default:
		return "application/octet-stream"
	}
}

// setCacheHeaders sets appropriate cache headers based on the file type.
func setCacheHeaders(w http.ResponseWriter, filePath string) {
	ext := path.Ext(filePath)

	// Immutable assets in _app/immutable/ directory
	if strings.Contains(filePath, "_app/immutable/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}

	// HTML files should not be cached
	if ext == ".html" {
		w.Header().Set("Cache-Control", "no-cache")
		return
	}

	// _app directory assets
	if strings.Contains(filePath, "_app/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}

	// Default: short cache for other files
	w.Header().Set("Cache-Control", "public, max-age=3600")
}

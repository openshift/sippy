package partman

import (
	"embed"
	"encoding/json"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
)

//go:embed web/dist
var uiFS embed.FS

type apiHandler struct {
	manager *Manager
}

func (h *apiHandler) handleGetTables(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tables, err := h.manager.GetManagedTables(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(map[string]interface{}{
		"tables": tables,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
}

func (h *apiHandler) handleGetPartitions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tableName := r.URL.Query().Get("table")
	if tableName == "" {
		http.Error(w, "table parameter is required", http.StatusBadRequest)
		return
	}

	schema := r.URL.Query().Get("schema")
	if schema == "" {
		// Default to the first table's schema if not provided
		if len(h.manager.config.Tables) > 0 {
			schema = h.manager.config.Tables[0].Schema
		} else {
			http.Error(w, "schema parameter is required", http.StatusBadRequest)
			return
		}
	}

	// Parse pagination parameters
	limit := 10 // Default limit
	offset := 0 // Default offset

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil {
			http.Error(w, "invalid limit parameter", http.StatusBadRequest)
			return
		}
		limit = parsedLimit
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		parsedOffset, err := strconv.Atoi(offsetStr)
		if err != nil {
			http.Error(w, "invalid offset parameter", http.StatusBadRequest)
			return
		}
		offset = parsedOffset
	}

	partitions, err := h.manager.GetPartitions(r.Context(), schema, tableName, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get parent table info
	parentInfo, err := h.manager.GetParentTableInfo(r.Context(), schema, tableName)
	if err != nil {
		// Log error but don't fail the request
		h.manager.logger.Error("failed to get parent table info", "error", err)
	}

	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"partitions": partitions,
	}
	if parentInfo != nil {
		response["parent_table"] = parentInfo
	}

	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
}

// UIHandler returns an http.Handler that serves the partition manager UI and API
func UIHandler(manager *Manager) http.Handler {
	fsys, err := fs.Sub(uiFS, "web/dist")
	if err != nil {
		panic(err)
	}

	api := &apiHandler{manager: manager}
	mux := http.NewServeMux()

	// API routes
	mux.Handle("/api/tables", enforceJSONHandler(setupCORS(http.HandlerFunc(api.handleGetTables))))
	mux.Handle("/api/partitions", enforceJSONHandler(setupCORS(http.HandlerFunc(api.handleGetPartitions))))

	// UI routes - serve static files with proper MIME types
	mux.Handle("/", setupCORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle API routes
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		// Serve static files
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		// Remove leading slash for embedded files
		if strings.HasPrefix(path, "/") {
			path = path[1:]
		}

		// Get file from embedded filesystem
		file, err := fsys.Open(path)
		if err != nil {
			// If file not found, serve index.html for SPA routing
			if path != "index.html" {
				indexFile, err := fsys.Open("index.html")
				if err != nil {
					http.NotFound(w, r)
					return
				}
				defer indexFile.Close()
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				http.ServeFile(w, r, "index.html")
				return
			}
			http.NotFound(w, r)
			return
		}
		defer file.Close()

		// Set proper MIME type
		ext := filepath.Ext(path)
		mimeType := mime.TypeByExtension(ext)
		if mimeType == "" {
			switch ext {
			case ".js":
				mimeType = "application/javascript"
			case ".css":
				mimeType = "text/css"
			case ".html":
				mimeType = "text/html; charset=utf-8"
			case ".svg":
				mimeType = "image/svg+xml"
			case ".ico":
				mimeType = "image/x-icon"
			default:
				mimeType = "application/octet-stream"
			}
		}
		w.Header().Set("Content-Type", mimeType)

		// Serve the file using http.FileServer
		fileServer := http.FileServer(http.FS(fsys))
		fileServer.ServeHTTP(w, r)
	})))

	return mux
}

// APIHandler returns an http.Handler that serves only the API endpoints
// This allows users to mount the API on their own router
func APIHandler(manager *Manager) http.Handler {
	api := &apiHandler{manager: manager}
	mux := http.NewServeMux()

	// API routes
	mux.Handle("/tables", enforceJSONHandler(setupCORS(http.HandlerFunc(api.handleGetTables))))
	mux.Handle("/partitions", enforceJSONHandler(setupCORS(http.HandlerFunc(api.handleGetPartitions))))

	return mux
}

// StaticHandler returns an http.Handler that serves only the static UI files
// This allows users to serve the UI from their own static file server
func StaticHandler() http.Handler {
	fsys, err := fs.Sub(uiFS, "web/dist")
	if err != nil {
		panic(err)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		// Remove leading slash for embedded files
		if strings.HasPrefix(path, "/") {
			path = path[1:]
		}

		// Get file from embedded filesystem
		file, err := fsys.Open(path)
		if err != nil {
			// If file not found, serve index.html for SPA routing
			if path != "index.html" {
				indexFile, err := fsys.Open("index.html")
				if err != nil {
					http.NotFound(w, r)
					return
				}
				defer indexFile.Close()
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				http.ServeFile(w, r, "index.html")
				return
			}
			http.NotFound(w, r)
			return
		}
		defer file.Close()

		// Set proper MIME type
		ext := filepath.Ext(path)
		mimeType := mime.TypeByExtension(ext)
		if mimeType == "" {
			switch ext {
			case ".js":
				mimeType = "application/javascript"
			case ".css":
				mimeType = "text/css"
			case ".html":
				mimeType = "text/html; charset=utf-8"
			case ".svg":
				mimeType = "image/svg+xml"
			case ".ico":
				mimeType = "image/x-icon"
			default:
				mimeType = "application/octet-stream"
			}
		}
		w.Header().Set("Content-Type", mimeType)

		// Serve the file using http.FileServer
		fileServer := http.FileServer(http.FS(fsys))
		fileServer.ServeHTTP(w, r)
	})
}

func setupCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func enforceJSONHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType := r.Header.Get("Content-Type")

		if contentType != "" {
			mt, _, err := mime.ParseMediaType(contentType)
			if err != nil {
				http.Error(w, "Malformed Content-Type header", http.StatusBadRequest)
				return
			}

			if mt != "application/json" {
				http.Error(w, "Content-Type header must be application/json", http.StatusUnsupportedMediaType)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

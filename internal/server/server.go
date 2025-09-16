package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pavel-fokin/files-stash/internal/files"
	"github.com/pavel-fokin/files-stash/internal/fs"
	"github.com/pavel-fokin/files-stash/internal/sqlite"
)

type Config struct {
	AdminToken string        `env:"FILES_STASH_ADMIN_TOKEN,required"`
	DataDir    string        `env:"FILES_STASH_DATA_DIR,required"`
	HmacKey    string        `env:"FILES_STASH_HMAC_KEY,required"`
	MaxSize    int64         `env:"FILES_STASH_MAX_SIZE,required"`
	TTL        time.Duration `env:"FILES_STASH_TTL,required"`
	DBPath     string        `env:"FILES_STASH_DB_PATH,required"`
}

func New(cfg *Config) *http.Server {
	// Initialize structured logger with JSON handler
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Initialize storage and repository
	storage := fs.NewStorage(cfg.DataDir)
	repo, err := sqlite.NewRepository(cfg.DBPath)
	if err != nil {
		slog.Error("Failed to initialize repository", "error", err)
		panic(fmt.Sprintf("Failed to initialize repository: %v", err))
	}

	// Initialize file service
	fileService := files.NewService(storage, repo, cfg.HmacKey, cfg.TTL)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthz)
	mux.HandleFunc("POST /v1/files", auth(cfg.AdminToken, uploadFile(cfg, fileService)))
	mux.HandleFunc("GET /v1/files", auth(cfg.AdminToken, listFiles(cfg, fileService)))
	mux.HandleFunc("GET /v1/files/latest/{tag}", getLatestFileByTag(cfg, fileService))
	mux.HandleFunc("DELETE /v1/files/{id}", auth(cfg.AdminToken, deleteFile(cfg, fileService)))
	mux.HandleFunc("GET /v1/files/{id}", signedDownload(cfg, fileService))

	// Wrap the handler with logging middleware
	handler := loggingMiddleware(limitBody(mux, cfg.MaxSize))

	return &http.Server{
		Addr:         ":8080",
		Handler:      handler,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}

func healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func uploadFile(cfg *Config, fileService *files.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse multipart form
		err := r.ParseMultipartForm(cfg.MaxSize)
		if err != nil {
			http.Error(w, "Failed to parse multipart form", http.StatusBadRequest)
			return
		}

		// Get file from form
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "No file provided", http.StatusBadRequest)
			return
		}
		defer file.Close()

		// Create upload request
		uploadReq := &files.UploadRequest{
			Name:     header.Filename,
			MimeType: header.Header.Get("Content-Type"),
			Tag:      r.FormValue("tag"),
			Content:  file,
		}

		// Upload file
		result, err := fileService.Upload(uploadReq)
		if err != nil {
			slog.Error("Upload failed", "error", err, "filename", header.Filename)
			http.Error(w, "Upload failed", http.StatusInternalServerError)
			return
		}

		// Return success response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(result); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func getLatestFileByTag(cfg *Config, fileService *files.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tag := r.PathValue("tag")
		slog.Info("Getting latest file by tag", "tag", tag)

		result, err := fileService.GetLatestByTag(tag)
		if err != nil {
			slog.Error("Get latest by tag failed", "error", err, "tag", tag)
			http.Error(w, "Failed to get latest file by tag", http.StatusNotFound)
			return
		}

		http.Redirect(w, r, result.URL, http.StatusFound)
	}
}

func deleteFile(cfg *Config, fileService *files.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		slog.Info("Deleting file", "file_id", id)

		// Delete file
		err := fileService.Delete(id)
		if err != nil {
			slog.Error("Delete failed", "error", err, "file_id", id)
			http.Error(w, "Delete failed", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func listFiles(cfg *Config, fileService *files.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slog.Info("Listing files")

		// Get list of files
		files, err := fileService.List()
		if err != nil {
			slog.Error("List files failed", "error", err)
			http.Error(w, "Failed to list files", http.StatusInternalServerError)
			return
		}

		// Set response headers
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// Return JSON response
		if err := json.NewEncoder(w).Encode(files); err != nil {
			slog.Error("Failed to encode files list", "error", err)
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}
	}
}

func signedDownload(cfg *Config, fileService *files.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		signature := r.URL.Query().Get("signature")
		slog.Info("Downloading file", "file_id", id)

		// Download file with signature verification
		file, content, err := fileService.Download(id, signature)
		if err != nil {
			slog.Error("Download failed", "error", err, "file_id", id)
			http.Error(w, "Download failed", http.StatusNotFound)
			return
		}

		// Set response headers
		w.Header().Set("Content-Type", file.MimeType)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", file.Name))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", file.Size))

		// Stream file content
		if content != nil {
			defer content.Close()
			w.WriteHeader(http.StatusOK)
			io.Copy(w, content)
		} else {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("File content not available"))
		}
	}
}

func auth(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func limitBody(next http.Handler, maxSize int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Create a limited reader that will return an error if the limit is exceeded
		limitedReader := http.MaxBytesReader(w, r.Body, maxSize)
		r.Body = limitedReader

		// For multipart requests, parse the form to trigger size validation
		if r.Header.Get("Content-Type") == "multipart/form-data" {
			if err := r.ParseMultipartForm(maxSize); err != nil {
				if err.Error() == "http: request body too large" {
					http.Error(w, "Request entity too large", http.StatusRequestEntityTooLarge)
					return
				}
			}
		} else {
			// For non-multipart requests, we need to read the body to trigger the size check
			// We'll read it into a buffer and then create a new reader for the next handler
			body, err := io.ReadAll(r.Body)
			if err != nil {
				if err.Error() == "http: request body too large" {
					http.Error(w, "Request entity too large", http.StatusRequestEntityTooLarge)
					return
				}
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(strings.NewReader(string(body)))
		}

		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs HTTP requests with structured logging
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response writer wrapper to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Process the request
		next.ServeHTTP(wrapped, r)

		// Calculate response time
		duration := time.Since(start)

		// Log the request with structured data
		slog.Info("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"query", r.URL.RawQuery,
			"status", wrapped.statusCode,
			"duration_ms", duration.Milliseconds(),
			"remote_addr", r.RemoteAddr,
			"user_agent", r.UserAgent(),
		)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

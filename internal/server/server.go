package server

import (
	"fmt"
	"io"
	"log"
	"net/http"
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
	// Initialize storage and repository
	storage := fs.NewStorage(cfg.DataDir)
	repo, err := sqlite.NewRepository(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize repository: %v", err)
	}

	// Initialize file service
	fileService := files.NewService(storage, repo, cfg.HmacKey, cfg.TTL)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthz)
	mux.HandleFunc("POST /v1/upload", auth(cfg.AdminToken, upload(cfg, fileService)))
	mux.HandleFunc("DELETE /v1/files/{id}", auth(cfg.AdminToken, deleteFile(cfg, fileService)))
	mux.HandleFunc("GET /v1/files/{id}", signedDownload(cfg, fileService))

	return &http.Server{
		Addr:         ":8080",
		Handler:      limitBody(mux, cfg.MaxSize),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}

func healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func upload(cfg *Config, fileService *files.Service) http.HandlerFunc {
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
			Content:  file,
		}

		// Upload file
		result, err := fileService.Upload(uploadReq)
		if err != nil {
			log.Printf("Upload failed: %v", err)
			http.Error(w, "Upload failed", http.StatusInternalServerError)
			return
		}

		// Return success response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		// Simple JSON response
		response := fmt.Sprintf(`{"id":"%s","name":"%s","size":%d,"mime_type":"%s","url":"%s"}`,
			result.ID, result.Name, result.Size, result.MimeType, result.URL)
		w.Write([]byte(response))
	}
}

func deleteFile(cfg *Config, fileService *files.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		log.Printf("deleting file %s", id)

		// Delete file
		err := fileService.Delete(id)
		if err != nil {
			log.Printf("Delete failed: %v", err)
			http.Error(w, "Delete failed", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func signedDownload(cfg *Config, fileService *files.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		signature := r.URL.Query().Get("signature")
		log.Printf("downloading file %s", id)

		// Download file with signature verification
		file, content, err := fileService.Download(id, signature)
		if err != nil {
			log.Printf("Download failed: %v", err)
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

package server

import (
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

type Config struct {
	AdminToken string        `env:"ADMIN_TOKEN,required"`
	DataDir    string        `env:"DATA_DIR,required"`
	HmacKey    string        `env:"HMAC_KEY,required"`
	MaxSize    int64         `env:"MAX_SIZE,required"`
	TTL        time.Duration `env:"TTL,required"`
}

func New(cfg *Config) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthz)
	mux.HandleFunc("POST /v1/upload", auth(cfg.AdminToken, upload(cfg)))
	mux.HandleFunc("DELETE /v1/files/{id}", auth(cfg.AdminToken, deleteFile(cfg)))
	mux.HandleFunc("GET /v1/files/{id}", signedDownload(cfg))

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

func upload(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("uploading file")
		w.WriteHeader(http.StatusNotImplemented)
	}
}

func deleteFile(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		log.Printf("deleting file %s", id)
		w.WriteHeader(http.StatusNotImplemented)
	}
}

func signedDownload(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		log.Printf("downloading file %s", id)
		w.WriteHeader(http.StatusNotImplemented)
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

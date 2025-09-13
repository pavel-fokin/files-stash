package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHealthz(t *testing.T) {
	req, err := http.NewRequest("GET", "/healthz", nil)
	assert.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(healthz)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestAuthMiddleware(t *testing.T) {
	tests := []struct {
		name          string
		token         string
		header        string
		expectedCode  int
		expectedBody  string
	}{
		{
			name:          "valid token",
			token:         "secret",
			header:        "Bearer secret",
			expectedCode:  http.StatusOK,
			expectedBody:  "",
		},
		{
			name:          "invalid token",
			token:         "secret",
			header:        "Bearer wrong",
			expectedCode:  http.StatusUnauthorized,
			expectedBody:  "Unauthorized\n",
		},
		{
			name:          "no header",
			token:         "secret",
			header:        "",
			expectedCode:  http.StatusUnauthorized,
			expectedBody:  "Unauthorized\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "/", nil)
			assert.NoError(t, err)
			req.Header.Set("Authorization", tt.header)

			rr := httptest.NewRecorder()
			handler := auth(tt.token, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			handler.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedCode, rr.Code)
			assert.Equal(t, tt.expectedBody, rr.Body.String())
		})
	}
}

func TestLimitBodyMiddleware(t *testing.T) {
	handler := limitBody(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), 10)

	t.Run("body within limit", func(t *testing.T) {
		req, err := http.NewRequest("POST", "/", strings.NewReader("123456789"))
		assert.NoError(t, err)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("body exceeds limit", func(t *testing.T) {
		req, err := http.NewRequest("POST", "/", strings.NewReader("12345678901"))
		assert.NoError(t, err)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusRequestEntityTooLarge, rr.Code)
	})
}

func TestNotImplementedHandlers(t *testing.T) {
	cfg := &Config{
		AdminToken: "secret",
		DataDir:    "/tmp",
		HmacKey:    "hmac-secret",
		MaxSize:    1024,
		TTL:        24 * time.Hour,
	}

	endpoints := map[string]http.HandlerFunc{
		"POST /v1/upload":      upload(cfg),
		"DELETE /v1/files/1": deleteFile(cfg),
		"GET /v1/files/1":    signedDownload(cfg),
	}

	for name, handler := range endpoints {
		t.Run(name, func(t *testing.T) {
			parts := strings.Split(name, " ")
			method, path := parts[0], parts[1]

			req, err := http.NewRequest(method, path, nil)
			assert.NoError(t, err)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusNotImplemented, rr.Code)
		})
	}
}

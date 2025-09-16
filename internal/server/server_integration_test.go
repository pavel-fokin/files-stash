
package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	adminToken = "test-token"
	hmacKey    = "test-key"
)

func setupTestServer(t *testing.T) (*http.Server, func()) {
	dataDir, err := os.MkdirTemp("", "files-stash-test")
	require.NoError(t, err)

	dbPath := filepath.Join(dataDir, "test.db")

	cfg := &Config{
		AdminToken: adminToken,
		DataDir:    dataDir,
		HmacKey:    hmacKey,
		MaxSize:    1024,
		TTL:        5 * time.Minute,
		DBPath:     dbPath,
	}

	srv := New(cfg)

	cleanup := func() {
		os.RemoveAll(dataDir)
	}

	return srv, cleanup
}

func TestIntegration(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	// 1. Upload a file
	var fileID, fileURL string
	t.Run("Upload", func(t *testing.T) {
		body := new(bytes.Buffer)
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("file", "test.txt")
		require.NoError(t, err)
		_, err = io.WriteString(part, "test file content")
		require.NoError(t, err)
		writer.Close()

		req, err := http.NewRequest("POST", ts.URL+"/v1/files", body)
		require.NoError(t, err)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		// Extract file ID and URL from response
		// A simple way for now, assuming a specific JSON structure.
		// In a real app, you'd unmarshal the JSON.
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var result struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		}
		err = json.Unmarshal(respBody, &result)
		require.NoError(t, err)

		fileID = result.ID
		fileURL = result.URL

		require.NotEmpty(t, fileID)
		require.NotEmpty(t, fileURL)
	})

	// 2. Download the file
	t.Run("Download", func(t *testing.T) {
		require.NotEmpty(t, fileURL, "fileURL should not be empty")
		req, err := http.NewRequest("GET", ts.URL+fileURL, nil)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, "test file content", string(respBody))
	})

	// 3. Upload a file with a tag
	var taggedFileURL string
	t.Run("Upload with tag", func(t *testing.T) {
		body := new(bytes.Buffer)
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("file", "test.txt")
		require.NoError(t, err)
		_, err = io.WriteString(part, "tagged file content")
		require.NoError(t, err)
		writer.WriteField("tag", "latest")
		writer.Close()

		req, err := http.NewRequest("POST", ts.URL+"/v1/files", body)
		require.NoError(t, err)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var result struct {
			URL string `json:"url"`
		}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)
		taggedFileURL = result.URL
	})

	// 4. Download the tagged file
	t.Run("Download tagged file", func(t *testing.T) {
		req, err := http.NewRequest("GET", ts.URL+"/v1/files/latest/latest", nil)
		require.NoError(t, err)

		// prevent redirects
		client := &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusFound, resp.StatusCode)
		assert.Equal(t, taggedFileURL, resp.Header.Get("Location"))
	})

	// 5. Delete the file
	t.Run("Delete", func(t *testing.T) {
		require.NotEmpty(t, fileID, "fileID should not be empty")
		req, err := http.NewRequest("DELETE", ts.URL+"/v1/files/"+fileID, nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	// 6. Try to download the deleted file
	t.Run("Download after delete", func(t *testing.T) {
		require.NotEmpty(t, fileURL, "fileURL should not be empty")
		req, err := http.NewRequest("GET", ts.URL+fileURL, nil)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

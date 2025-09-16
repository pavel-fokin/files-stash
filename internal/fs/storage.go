package fs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/pavel-fokin/files-stash/internal/files"
)

// Storage implements files.FileStorage using the filesystem
type Storage struct {
	dataDir string
}

// NewStorage creates a new filesystem storage
func NewStorage(dataDir string) *Storage {
	return &Storage{
		dataDir: dataDir,
	}
}

// Save stores a file and returns its metadata
func (s *Storage) Save(id string, name string, mimeType string, content io.Reader) (*files.File, error) {
	// Create file path
	filePath := filepath.Join(s.dataDir, id)

	// Create directory if it doesn't exist
	if err := os.MkdirAll(s.dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Create file
	file, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Copy content to file
	size, err := io.Copy(file, content)
	if err != nil {
		// Clean up file if copy fails
		os.Remove(filePath)
		return nil, fmt.Errorf("failed to write file content: %w", err)
	}

	return &files.File{
		ID:        id,
		Name:      name,
		Size:      size,
		MimeType:  mimeType,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour), // Default TTL, will be overridden by service
	}, nil
}

// Delete removes a file by ID
func (s *Storage) Delete(id string) error {
	filePath := filepath.Join(s.dataDir, id)

	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			return nil // File already deleted
		}
		return fmt.Errorf("failed to delete file: %w", err)
	}

	return nil
}

// GetContent returns a reader for the file content
func (s *Storage) GetContent(id string) (io.ReadCloser, error) {
	filePath := filepath.Join(s.dataDir, id)

	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found")
		}
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	return file, nil
}

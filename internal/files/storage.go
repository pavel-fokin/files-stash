package files

import (
	"io"
	"time"
)

// File represents a stored file with its metadata
type File struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	MimeType  string    `json:"mime_type"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// FileStorage defines the interface for file storage operations
type FileStorage interface {
	// Save stores a file and returns its metadata
	Save(id string, name string, mimeType string, content io.Reader) (*File, error)

	// Get retrieves a file by ID
	Get(id string) (*File, error)

	// GetContent returns a reader for the file content
	GetContent(id string) (io.ReadCloser, error)

	// Delete removes a file by ID
	Delete(id string) error

	// Exists checks if a file exists
	Exists(id string) bool
}

// FileRepository defines the interface for file metadata persistence
type FileRepository interface {
	// Create stores file metadata
	Create(file *File) error

	// FindByID retrieves file metadata by ID
	FindByID(id string) (*File, error)

	// List retrieves all file metadata
	List() ([]*File, error)

	// Delete removes file metadata by ID
	Delete(id string) error

	// CleanupExpired removes expired file metadata
	CleanupExpired() error
}

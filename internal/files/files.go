package files

import (
	"io"
	"time"
)

// File represents the metadata of a stored file
type File struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Tag       string    `json:"tag,omitempty"`
	Size      int64     `json:"size"`
	MimeType  string    `json:"mime_type"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// FileRepository defines the interface for storing and retrieving file metadata
type FileRepository interface {
	Create(file *File) error
	FindByID(id string) (*File, error)
	FindByTag(tag string) (*File, error)
	Delete(id string) error
	List() ([]*File, error)
}

// FileStorage defines the interface for the physical file storage
type FileStorage interface {
	Save(id, name, mimeType string, content io.Reader) (*File, error)
	GetContent(id string) (io.ReadCloser, error)
	Delete(id string) error
}

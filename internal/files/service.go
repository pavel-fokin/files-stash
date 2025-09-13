package files

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"time"
)

// Service provides application-level file operations
type Service struct {
	storage FileStorage
	repo    FileRepository
	hmacKey string
	ttl     time.Duration
}

// NewService creates a new file service
func NewService(storage FileStorage, repo FileRepository, hmacKey string, ttl time.Duration) *Service {
	return &Service{
		storage: storage,
		repo:    repo,
		hmacKey: hmacKey,
		ttl:     ttl,
	}
}

// UploadRequest represents a file upload request
type UploadRequest struct {
	Name     string
	MimeType string
	Content  io.Reader
}

// UploadResult represents the result of a file upload
type UploadResult struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	MimeType  string    `json:"mime_type"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	URL       string    `json:"url"`
}

// Upload stores a file and returns its metadata with a signed URL
func (s *Service) Upload(req *UploadRequest) (*UploadResult, error) {
	// Generate unique file ID
	id := s.generateID()

	// Calculate file size by reading content
	size, data, err := s.calculateSize(req.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate file size: %w", err)
	}

	// Create file metadata
	now := time.Now()
	file := &File{
		ID:        id,
		Name:      req.Name,
		Size:      size,
		MimeType:  req.MimeType,
		CreatedAt: now,
		ExpiresAt: now.Add(s.ttl),
	}

	// Save file to storage
	savedFile, err := s.storage.Save(id, req.Name, req.MimeType, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to save file: %w", err)
	}

	// Save metadata to repository
	if err := s.repo.Create(file); err != nil {
		// Clean up file if metadata save fails
		s.storage.Delete(id)
		return nil, fmt.Errorf("failed to save file metadata: %w", err)
	}

	// Generate signed URL
	url, err := s.generateSignedURL(id)
	if err != nil {
		return nil, fmt.Errorf("failed to generate signed URL: %w", err)
	}

	return &UploadResult{
		ID:        savedFile.ID,
		Name:      savedFile.Name,
		Size:      savedFile.Size,
		MimeType:  savedFile.MimeType,
		CreatedAt: savedFile.CreatedAt,
		ExpiresAt: savedFile.ExpiresAt,
		URL:       url,
	}, nil
}

// Download retrieves a file by ID with signature verification
func (s *Service) Download(id string, signature string) (*File, io.ReadCloser, error) {
	// Verify signature
	if !s.verifySignature(id, signature) {
		return nil, nil, fmt.Errorf("invalid signature")
	}

	// Check if file exists in repository
	file, err := s.repo.FindByID(id)
	if err != nil {
		return nil, nil, fmt.Errorf("file not found: %w", err)
	}

	// Check if file is expired
	if time.Now().After(file.ExpiresAt) {
		// Clean up expired file
		s.storage.Delete(id)
		s.repo.Delete(id)
		return nil, nil, fmt.Errorf("file has expired")
	}

	// Get file content from storage
	content, err := s.storage.GetContent(id)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to retrieve file content: %w", err)
	}

	return file, content, nil
}

// Delete removes a file by ID
func (s *Service) Delete(id string) error {
	// Delete from storage
	if err := s.storage.Delete(id); err != nil {
		return fmt.Errorf("failed to delete file from storage: %w", err)
	}

	// Delete metadata from repository
	if err := s.repo.Delete(id); err != nil {
		return fmt.Errorf("failed to delete file metadata: %w", err)
	}

	return nil
}

// generateID creates a unique file identifier
func (s *Service) generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// calculateSize reads the content to determine file size
func (s *Service) calculateSize(content io.Reader) (int64, []byte, error) {
	// This is a simplified implementation
	// In a real scenario, you might want to use a more efficient method
	data, err := io.ReadAll(content)
	if err != nil {
		return 0, nil, err
	}
	return int64(len(data)), data, nil
}

// generateSignedURL creates a signed URL for file access
func (s *Service) generateSignedURL(id string) (string, error) {
	signature := s.createSignature(id)
	return fmt.Sprintf("/v1/files/%s?signature=%s", id, signature), nil
}

// createSignature generates HMAC signature for file ID
func (s *Service) createSignature(id string) string {
	h := hmac.New(sha256.New, []byte(s.hmacKey))
	h.Write([]byte(id))
	return hex.EncodeToString(h.Sum(nil))
}

// verifySignature validates HMAC signature for file ID
func (s *Service) verifySignature(id string, signature string) bool {
	expectedSignature := s.createSignature(id)
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

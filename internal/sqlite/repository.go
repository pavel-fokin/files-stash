package sqlite

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/pavel-fokin/files-stash/internal/files"
	_ "modernc.org/sqlite"
)

// Repository implements files.FileRepository using SQLite
type Repository struct {
	db *sql.DB
}

// NewRepository creates a new SQLite repository
func NewRepository(dbPath string) (*Repository, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	repo := &Repository{db: db}

	// Initialize database schema
	if err := repo.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return repo, nil
}

// Close closes the database connection
func (r *Repository) Close() error {
	return r.db.Close()
}

// initSchema creates the necessary database tables
func (r *Repository) initSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS files (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		size INTEGER NOT NULL,
		mime_type TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		expires_at DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_files_expires_at ON files(expires_at);
	`

	_, err := r.db.Exec(query)
	return err
}

// Create stores file metadata
func (r *Repository) Create(file *files.File) error {
	query := `
	INSERT INTO files (id, name, size, mime_type, created_at, expires_at)
	VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.Exec(query,
		file.ID,
		file.Name,
		file.Size,
		file.MimeType,
		file.CreatedAt,
		file.ExpiresAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create file record: %w", err)
	}

	return nil
}

// FindByID retrieves file metadata by ID
func (r *Repository) FindByID(id string) (*files.File, error) {
	query := `
	SELECT id, name, size, mime_type, created_at, expires_at
	FROM files
	WHERE id = ?
	`

	var file files.File
	err := r.db.QueryRow(query, id).Scan(
		&file.ID,
		&file.Name,
		&file.Size,
		&file.MimeType,
		&file.CreatedAt,
		&file.ExpiresAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("file not found")
		}
		return nil, fmt.Errorf("failed to find file: %w", err)
	}

	return &file, nil
}

// List retrieves all file metadata
func (r *Repository) List() ([]*files.File, error) {
	query := `
	SELECT id, name, size, mime_type, created_at, expires_at
	FROM files
	ORDER BY created_at DESC
	`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query files: %w", err)
	}
	defer rows.Close()

	var fileList []*files.File
	for rows.Next() {
		var file files.File
		err := rows.Scan(
			&file.ID,
			&file.Name,
			&file.Size,
			&file.MimeType,
			&file.CreatedAt,
			&file.ExpiresAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file row: %w", err)
		}
		fileList = append(fileList, &file)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating file rows: %w", err)
	}

	return fileList, nil
}

// Delete removes file metadata by ID
func (r *Repository) Delete(id string) error {
	query := `DELETE FROM files WHERE id = ?`

	result, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete file record: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("file not found")
	}

	return nil
}

// CleanupExpired removes expired file metadata
func (r *Repository) CleanupExpired() error {
	query := `DELETE FROM files WHERE expires_at < ?`

	now := time.Now()
	_, err := r.db.Exec(query, now)
	if err != nil {
		return fmt.Errorf("failed to cleanup expired files: %w", err)
	}

	return nil
}

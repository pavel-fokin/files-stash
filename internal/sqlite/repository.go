package sqlite

import (
	"database/sql"
	"fmt"
	"strings"

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

// initSchema creates and migrates the necessary database tables
func (r *Repository) initSchema() error {
	// Create the table if it doesn't exist, but without the tag column initially
	// to support migrating from an older schema.
	createTableQuery := `
	CREATE TABLE IF NOT EXISTS files (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		size INTEGER NOT NULL,
		mime_type TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		expires_at DATETIME NOT NULL
	);`
	if _, err := r.db.Exec(createTableQuery); err != nil {
		return fmt.Errorf("failed to create files table: %w", err)
	}

	// Add the tag column, ignoring the error if it already exists.
	// This is a simple migration strategy.
	alterTableQuery := `ALTER TABLE files ADD COLUMN tag TEXT;`
	if _, err := r.db.Exec(alterTableQuery); err != nil {
		if !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("failed to add tag column: %w", err)
		}
	}

	// Create indexes, which is safe now that we know the tag column exists.
	createIndexesQuery := `
	CREATE INDEX IF NOT EXISTS idx_files_expires_at ON files(expires_at);
	CREATE INDEX IF NOT EXISTS idx_files_tag_created_at ON files(tag, created_at);
	`
	if _, err := r.db.Exec(createIndexesQuery); err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	return nil
}

// Create stores file metadata
func (r *Repository) Create(file *files.File) error {
	query := `
	INSERT INTO files (id, name, tag, size, mime_type, created_at, expires_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.Exec(query,
		file.ID,
		file.Name,
		file.Tag,
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
	SELECT id, name, tag, size, mime_type, created_at, expires_at
	FROM files
	WHERE id = ?
	`

	var file files.File
	var tag sql.NullString
	err := r.db.QueryRow(query, id).Scan(
		&file.ID,
		&file.Name,
		&tag,
		&file.Size,
		&file.MimeType,
		&file.CreatedAt,
		&file.ExpiresAt,
	)
	if tag.Valid {
		file.Tag = tag.String
	}

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("file not found")
		}
		return nil, fmt.Errorf("failed to find file: %w", err)
	}

	return &file, nil
}

// FindByTag retrieves the latest file metadata by tag
func (r *Repository) FindByTag(tag string) (*files.File, error) {
	query := `
	SELECT id, name, tag, size, mime_type, created_at, expires_at
	FROM files
	WHERE tag = ?
	ORDER BY created_at DESC
	LIMIT 1
	`

	var file files.File
	var sqlTag sql.NullString
	err := r.db.QueryRow(query, tag).Scan(
		&file.ID,
		&file.Name,
		&sqlTag,
		&file.Size,
		&file.MimeType,
		&file.CreatedAt,
		&file.ExpiresAt,
	)
	if sqlTag.Valid {
		file.Tag = sqlTag.String
	}

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("file not found")
		}
		return nil, fmt.Errorf("failed to find file by tag: %w", err)
	}

	return &file, nil
}

// List retrieves all file metadata
func (r *Repository) List() ([]*files.File, error) {
	query := `
	SELECT id, name, tag, size, mime_type, created_at, expires_at
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
		var tag sql.NullString
		err := rows.Scan(
			&file.ID,
			&file.Name,
			&tag,
			&file.Size,
			&file.MimeType,
			&file.CreatedAt,
			&file.ExpiresAt,
		)
		if tag.Valid {
			file.Tag = tag.String
		}
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

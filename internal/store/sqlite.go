package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteStore tracks seen job IDs in a SQLite database for deduplication.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens (or creates) a SQLite database at dbPath and ensures the
// seen_jobs table exists.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite db: %w", err)
	}

	// Verify the connection is alive.
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging sqlite db: %w", err)
	}

	createTable := `CREATE TABLE IF NOT EXISTS seen_jobs (
		job_id     TEXT PRIMARY KEY,
		first_seen DATETIME DEFAULT CURRENT_TIMESTAMP
	)`
	if _, err := db.Exec(createTable); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating seen_jobs table: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// HasSeen returns true if the given job ID has already been recorded.
func (s *SQLiteStore) HasSeen(jobID string) (bool, error) {
	var exists int
	err := s.db.QueryRow("SELECT 1 FROM seen_jobs WHERE job_id = ?", jobID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking seen status for %s: %w", jobID, err)
	}
	return true, nil
}

// MarkSeen records a job ID as seen. If it already exists the call is a no-op.
func (s *SQLiteStore) MarkSeen(jobID string) error {
	_, err := s.db.Exec("INSERT OR IGNORE INTO seen_jobs (job_id) VALUES (?)", jobID)
	if err != nil {
		return fmt.Errorf("marking job %s as seen: %w", jobID, err)
	}
	return nil
}

// Cleanup deletes seen-job entries older than the given duration.
func (s *SQLiteStore) Cleanup(olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)
	_, err := s.db.Exec("DELETE FROM seen_jobs WHERE first_seen < ?", cutoff)
	if err != nil {
		return fmt.Errorf("cleaning up seen jobs older than %v: %w", olderThan, err)
	}
	return nil
}

// IsEmpty returns true if the seen_jobs table has no entries.
func (s *SQLiteStore) IsEmpty() (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM seen_jobs").Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking if store is empty: %w", err)
	}
	return count == 0, nil
}

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

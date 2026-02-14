package store

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMarkSeenThenHasSeen(t *testing.T) {
	s := newTestStore(t)

	if err := s.MarkSeen("job-123"); err != nil {
		t.Fatalf("MarkSeen: %v", err)
	}

	seen, err := s.HasSeen("job-123")
	if err != nil {
		t.Fatalf("HasSeen: %v", err)
	}
	if !seen {
		t.Error("expected HasSeen to return true after MarkSeen")
	}
}

func TestHasSeenUnknownReturnsFalse(t *testing.T) {
	s := newTestStore(t)

	seen, err := s.HasSeen("does-not-exist")
	if err != nil {
		t.Fatalf("HasSeen: %v", err)
	}
	if seen {
		t.Error("expected HasSeen to return false for unknown job ID")
	}
}

func TestMarkSeenIdempotent(t *testing.T) {
	s := newTestStore(t)

	if err := s.MarkSeen("job-456"); err != nil {
		t.Fatalf("first MarkSeen: %v", err)
	}
	if err := s.MarkSeen("job-456"); err != nil {
		t.Fatalf("second MarkSeen (duplicate): %v", err)
	}

	seen, err := s.HasSeen("job-456")
	if err != nil {
		t.Fatalf("HasSeen: %v", err)
	}
	if !seen {
		t.Error("expected HasSeen to return true after duplicate MarkSeen")
	}
}

func TestCleanupRemovesOldKeepsFresh(t *testing.T) {
	s := newTestStore(t)

	// Insert an "old" entry by writing directly with a past timestamp.
	_, err := s.db.Exec(
		"INSERT INTO seen_jobs (job_id, first_seen) VALUES (?, ?)",
		"old-job", time.Now().Add(-48*time.Hour),
	)
	if err != nil {
		t.Fatalf("inserting old job: %v", err)
	}

	// Insert a fresh entry via the normal API (timestamp = now).
	if err := s.MarkSeen("fresh-job"); err != nil {
		t.Fatalf("MarkSeen fresh: %v", err)
	}

	// Cleanup anything older than 24 hours.
	if err := s.Cleanup(24 * time.Hour); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	// Old job should be gone.
	seen, err := s.HasSeen("old-job")
	if err != nil {
		t.Fatalf("HasSeen old: %v", err)
	}
	if seen {
		t.Error("expected old job to be cleaned up")
	}

	// Fresh job should remain.
	seen, err = s.HasSeen("fresh-job")
	if err != nil {
		t.Fatalf("HasSeen fresh: %v", err)
	}
	if !seen {
		t.Error("expected fresh job to survive cleanup")
	}
}

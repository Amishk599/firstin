package poller

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/amishk599/firstin/internal/model"
)

// --- Mock/Fake Implementations ---

// MockFetcher returns a canned slice of jobs or an error.
type MockFetcher struct {
	Jobs []model.Job
	Err  error
}

func (m *MockFetcher) FetchJobs(_ context.Context) ([]model.Job, error) {
	return m.Jobs, m.Err
}

// InMemoryStore is a map-based store for testing dedup.
type InMemoryStore struct {
	seen map[string]bool
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{seen: make(map[string]bool)}
}

func (s *InMemoryStore) HasSeen(jobID string) (bool, error) {
	return s.seen[jobID], nil
}

func (s *InMemoryStore) MarkSeen(jobID string) error {
	s.seen[jobID] = true
	return nil
}

func (s *InMemoryStore) Cleanup(_ time.Duration) error { return nil }

// RecordingNotifier records which jobs were sent to Notify.
type RecordingNotifier struct {
	Notified []model.Job
}

func (n *RecordingNotifier) Notify(jobs []model.Job) error {
	n.Notified = append(n.Notified, jobs...)
	return nil
}

// AcceptAllFilter matches every job.
type AcceptAllFilter struct{}

func (f *AcceptAllFilter) Match(_ model.Job) bool { return true }

// RejectAllFilter rejects every job.
type RejectAllFilter struct{}

func (f *RejectAllFilter) Match(_ model.Job) bool { return false }

// --- Helpers ---

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func makeJobs(ids ...string) []model.Job {
	jobs := make([]model.Job, len(ids))
	for i, id := range ids {
		jobs[i] = model.Job{
			ID:       id,
			Company:  "testco",
			Title:    "Software Engineer",
			Location: "US",
			URL:      "https://example.com/" + id,
			Source:   "test",
		}
	}
	return jobs
}

// --- Tests (max 5) ---

func TestPoll_FilterAndDedup(t *testing.T) {
	// 5 fetched, filter accepts all, store has seen "2" → notifier gets 4, store marks 4.
	store := NewInMemoryStore()
	store.MarkSeen("2")

	notifier := &RecordingNotifier{}
	poller := NewCompanyPoller(
		"testco",
		&MockFetcher{Jobs: makeJobs("1", "2", "3", "4", "5")},
		&AcceptAllFilter{},
		store,
		notifier,
		discardLogger(),
	)

	if err := poller.Poll(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := len(notifier.Notified); got != 4 {
		t.Errorf("notified = %d, want 4", got)
	}

	// All 5 should now be marked seen.
	for _, id := range []string{"1", "2", "3", "4", "5"} {
		if seen, _ := store.HasSeen(id); !seen {
			t.Errorf("job %s should be marked seen", id)
		}
	}
}

func TestPoll_FetchError(t *testing.T) {
	notifier := &RecordingNotifier{}
	poller := NewCompanyPoller(
		"failco",
		&MockFetcher{Err: errors.New("network down")},
		&AcceptAllFilter{},
		NewInMemoryStore(),
		notifier,
		discardLogger(),
	)

	err := poller.Poll(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if len(notifier.Notified) != 0 {
		t.Error("notifier should not be called on fetch error")
	}
}

func TestPoll_AllAlreadySeen(t *testing.T) {
	store := NewInMemoryStore()
	store.MarkSeen("1")
	store.MarkSeen("2")

	notifier := &RecordingNotifier{}
	poller := NewCompanyPoller(
		"testco",
		&MockFetcher{Jobs: makeJobs("1", "2")},
		&AcceptAllFilter{},
		store,
		notifier,
		discardLogger(),
	)

	if err := poller.Poll(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(notifier.Notified) != 0 {
		t.Error("notifier should not be called when all jobs already seen")
	}
}

func TestPoll_FilterRejectsAll(t *testing.T) {
	notifier := &RecordingNotifier{}
	poller := NewCompanyPoller(
		"testco",
		&MockFetcher{Jobs: makeJobs("1", "2", "3")},
		&RejectAllFilter{},
		NewInMemoryStore(),
		notifier,
		discardLogger(),
	)

	if err := poller.Poll(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(notifier.Notified) != 0 {
		t.Error("notifier should not be called when filter rejects all")
	}
}

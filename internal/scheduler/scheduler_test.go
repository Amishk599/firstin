package scheduler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amishk599/firstin/internal/model"
	"github.com/amishk599/firstin/internal/poller"
)

// --- Mock implementations ---

type CountingFetcher struct {
	calls atomic.Int32
}

func (f *CountingFetcher) FetchJobs(_ context.Context) ([]model.Job, error) {
	f.calls.Add(1)
	return nil, nil
}

type ErrorFetcher struct {
	calls atomic.Int32
}

func (f *ErrorFetcher) FetchJobs(_ context.Context) ([]model.Job, error) {
	f.calls.Add(1)
	return nil, errors.New("fetch failed")
}

type NoOpStore struct{}

func (s *NoOpStore) HasSeen(_ string) (bool, error) { return false, nil }
func (s *NoOpStore) MarkSeen(_ string) error         { return nil }
func (s *NoOpStore) Cleanup(_ time.Duration) error   { return nil }
func (s *NoOpStore) IsEmpty() (bool, error)          { return false, nil }

type NoOpNotifier struct{}

func (n *NoOpNotifier) Notify(_ []model.Job) error { return nil }

type AcceptAllFilter struct{}

func (f *AcceptAllFilter) Match(_ model.Job) bool { return true }

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func makePoller(name string, fetcher model.JobFetcher) *poller.CompanyPoller {
	return poller.NewCompanyPoller(
		name,
		fetcher,
		&AcceptAllFilter{},
		&NoOpStore{},
		&NoOpNotifier{},
		discardLogger(),
	)
}

// --- Tests (max 5) ---

func TestRun_CancelReturnsPromptly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := makePoller("testco", &CountingFetcher{})
	s := NewScheduler([]*poller.CompanyPoller{p}, 1*time.Hour, discardLogger())

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	// Give it time to start and run the immediate poll cycle.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil error on cancel, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not return within 2s after cancel")
	}
}

func TestRun_PollersCalledOnEachTick(t *testing.T) {
	fetcher := &CountingFetcher{}

	pollers := []*poller.CompanyPoller{
		makePoller("co1", fetcher),
	}

	// Use a short interval so we get the immediate cycle + at least one tick.
	// Single poller avoids the 1s inter-company sleep.
	ctx, cancel := context.WithCancel(context.Background())
	s := NewScheduler(pollers, 100*time.Millisecond, discardLogger())

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	// Wait for immediate cycle + at least one tick.
	time.Sleep(250 * time.Millisecond)
	cancel()
	<-done

	// Fetcher should have been called at least twice (immediate + 1 tick).
	if got := fetcher.calls.Load(); got < 2 {
		t.Errorf("fetcher calls = %d, want >= 2", got)
	}
}

func TestRun_OnePollerErrorOthersStillRun(t *testing.T) {
	errFetcher := &ErrorFetcher{}
	okFetcher := &CountingFetcher{}

	pollers := []*poller.CompanyPoller{
		makePoller("failing", errFetcher),
		makePoller("healthy", okFetcher),
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := NewScheduler(pollers, 1*time.Hour, discardLogger())

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	// Wait for the immediate cycle to complete (includes 1s inter-company sleep).
	time.Sleep(1500 * time.Millisecond)
	cancel()
	<-done

	if got := errFetcher.calls.Load(); got < 1 {
		t.Errorf("error fetcher calls = %d, want >= 1", got)
	}
	if got := okFetcher.calls.Load(); got < 1 {
		t.Errorf("ok fetcher calls = %d, want >= 1 (should run despite sibling error)", got)
	}
}

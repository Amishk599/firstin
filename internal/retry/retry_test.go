package retry

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/amishk599/firstin/internal/model"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// mockFetcher calls a function on each invocation, tracking call count.
type mockFetcher struct {
	calls int
	fn    func(attempt int) ([]model.Job, error)
}

func (m *mockFetcher) FetchJobs(_ context.Context) ([]model.Job, error) {
	m.calls++
	return m.fn(m.calls)
}

func TestRetry_SucceedsOnFirstAttempt(t *testing.T) {
	jobs := []model.Job{{ID: "1", Title: "Engineer"}}
	mock := &mockFetcher{fn: func(_ int) ([]model.Job, error) {
		return jobs, nil
	}}

	rf := NewRetryFetcher(mock, 2, 10*time.Millisecond, discardLogger())
	got, err := rf.FetchJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "1" {
		t.Fatalf("unexpected jobs: %v", got)
	}
	if mock.calls != 1 {
		t.Fatalf("expected 1 call, got %d", mock.calls)
	}
}

func TestRetry_RetriesOn5xx_SucceedsOnSecondAttempt(t *testing.T) {
	jobs := []model.Job{{ID: "1"}}
	mock := &mockFetcher{fn: func(attempt int) ([]model.Job, error) {
		if attempt == 1 {
			return nil, &model.HTTPError{StatusCode: 503, Err: errors.New("service unavailable")}
		}
		return jobs, nil
	}}

	rf := NewRetryFetcher(mock, 2, 10*time.Millisecond, discardLogger())
	got, err := rf.FetchJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 job, got %d", len(got))
	}
	if mock.calls != 2 {
		t.Fatalf("expected 2 calls, got %d", mock.calls)
	}
}

func TestRetry_DoesNotRetryOn4xx(t *testing.T) {
	mock := &mockFetcher{fn: func(_ int) ([]model.Job, error) {
		return nil, &model.HTTPError{StatusCode: 404, Err: errors.New("not found")}
	}}

	rf := NewRetryFetcher(mock, 2, 10*time.Millisecond, discardLogger())
	_, err := rf.FetchJobs(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var httpErr *model.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 404 {
		t.Fatalf("expected HTTPError with status 404, got %v", err)
	}
	if mock.calls != 1 {
		t.Fatalf("expected 1 call (no retry), got %d", mock.calls)
	}
}

func TestRetry_GivesUpAfterMaxRetries(t *testing.T) {
	mock := &mockFetcher{fn: func(_ int) ([]model.Job, error) {
		return nil, &model.HTTPError{StatusCode: 500, Err: errors.New("internal error")}
	}}

	rf := NewRetryFetcher(mock, 2, 10*time.Millisecond, discardLogger())
	_, err := rf.FetchJobs(context.Background())
	if err == nil {
		t.Fatal("expected error after max retries, got nil")
	}
	// 1 initial + 2 retries = 3
	if mock.calls != 3 {
		t.Fatalf("expected 3 calls (1 + 2 retries), got %d", mock.calls)
	}
}

func TestRetry_RespectsContextCancellation(t *testing.T) {
	mock := &mockFetcher{fn: func(_ int) ([]model.Job, error) {
		return nil, &model.HTTPError{StatusCode: 500, Err: errors.New("internal error")}
	}}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately so the backoff sleep is interrupted.
	cancel()

	rf := NewRetryFetcher(mock, 2, time.Second, discardLogger())
	_, err := rf.FetchJobs(ctx)
	if err == nil {
		t.Fatal("expected error from context cancellation, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	// Should have made initial call, then been cancelled during backoff.
	if mock.calls != 1 {
		t.Fatalf("expected 1 call before cancellation, got %d", mock.calls)
	}
}

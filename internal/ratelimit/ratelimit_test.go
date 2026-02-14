package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/amishk599/firstin/internal/model"
)

func TestWait_SameATS_EnforcesMinDelay(t *testing.T) {
	limiter := NewATSRateLimiter(100 * time.Millisecond)
	ctx := context.Background()

	// First call should return immediately.
	if err := limiter.Wait(ctx, "greenhouse"); err != nil {
		t.Fatalf("first wait: %v", err)
	}

	start := time.Now()
	if err := limiter.Wait(ctx, "greenhouse"); err != nil {
		t.Fatalf("second wait: %v", err)
	}
	elapsed := time.Since(start)

	// Should have waited at least ~100ms (allow 80ms for timer jitter).
	if elapsed < 80*time.Millisecond {
		t.Errorf("expected >= 80ms wait, got %v", elapsed)
	}
}

func TestWait_DifferentATS_NoCrossBlocking(t *testing.T) {
	limiter := NewATSRateLimiter(200 * time.Millisecond)
	ctx := context.Background()

	// Call for greenhouse.
	if err := limiter.Wait(ctx, "greenhouse"); err != nil {
		t.Fatalf("greenhouse wait: %v", err)
	}

	// Immediately call for lever — should NOT block.
	start := time.Now()
	if err := limiter.Wait(ctx, "lever"); err != nil {
		t.Fatalf("lever wait: %v", err)
	}
	elapsed := time.Since(start)

	if elapsed > 50*time.Millisecond {
		t.Errorf("expected lever wait to be near-instant, got %v", elapsed)
	}
}

func TestWait_ContextCancellation(t *testing.T) {
	limiter := NewATSRateLimiter(5 * time.Second) // long delay
	ctx := context.Background()

	// First call to seed the last-call time.
	if err := limiter.Wait(ctx, "greenhouse"); err != nil {
		t.Fatalf("first wait: %v", err)
	}

	// Cancel the context before the wait completes.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := limiter.Wait(ctx, "greenhouse")
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
}

// --- Mock for RateLimitedFetcher test ---

type recordingFetcher struct {
	called bool
}

func (f *recordingFetcher) FetchJobs(_ context.Context) ([]model.Job, error) {
	f.called = true
	return nil, nil
}

func TestRateLimitedFetcher_WaitsBeforeDelegating(t *testing.T) {
	limiter := NewATSRateLimiter(100 * time.Millisecond)
	inner := &recordingFetcher{}
	fetcher := NewRateLimitedFetcher(inner, limiter, "greenhouse")
	ctx := context.Background()

	// First call — seeds limiter, then delegates.
	if _, err := fetcher.FetchJobs(ctx); err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if !inner.called {
		t.Fatal("inner fetcher was not called on first fetch")
	}

	// Reset.
	inner.called = false

	// Second call — should wait for the rate limiter.
	start := time.Now()
	if _, err := fetcher.FetchJobs(ctx); err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	elapsed := time.Since(start)

	if !inner.called {
		t.Fatal("inner fetcher was not called on second fetch")
	}
	if elapsed < 80*time.Millisecond {
		t.Errorf("expected >= 80ms wait on second fetch, got %v", elapsed)
	}
}

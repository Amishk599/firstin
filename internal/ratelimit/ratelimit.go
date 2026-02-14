package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/amishk599/firstin/internal/model"
)

// ATSRateLimiter enforces a minimum delay between requests to the same ATS backend.
type ATSRateLimiter struct {
	mu       sync.Mutex
	lastCall map[string]time.Time // key: ATS name
	minDelay time.Duration        // delay (seconds) between requests to same ATS
}

// NewATSRateLimiter creates a rate limiter that enforces minDelay between
// consecutive requests to the same ATS provider.
func NewATSRateLimiter(minDelay time.Duration) *ATSRateLimiter {
	return &ATSRateLimiter{
		lastCall: make(map[string]time.Time),
		minDelay: minDelay,
	}
}

// Wait blocks until enough time has passed since the last request to the given ATS.
// Returns an error if the context is cancelled while waiting.
func (r *ATSRateLimiter) Wait(ctx context.Context, ats string) error {
	r.mu.Lock()
	last, ok := r.lastCall[ats]
	now := time.Now()

	if !ok {
		// First request for this ATS — no wait needed.
		r.lastCall[ats] = now
		r.mu.Unlock()
		return nil
	}

	elapsed := now.Sub(last)
	if elapsed >= r.minDelay {
		// Enough time has passed — proceed immediately.
		r.lastCall[ats] = now
		r.mu.Unlock()
		return nil
	}

	// Need to wait for the remainder.
	remaining := r.minDelay - elapsed
	r.mu.Unlock()

	select {
	case <-ctx.Done():
		return fmt.Errorf("rate limiter wait for %s: %w", ats, ctx.Err())
	case <-time.After(remaining):
	}

	// Record the actual time after waiting.
	r.mu.Lock()
	r.lastCall[ats] = time.Now()
	r.mu.Unlock()

	return nil
}

// RateLimitedFetcher is a decorator that enforces ATS-level rate limiting
// before delegating to the wrapped JobFetcher.
type RateLimitedFetcher struct {
	inner   model.JobFetcher
	limiter *ATSRateLimiter
	ats     string // which ATS this fetcher targets
}

// NewRateLimitedFetcher wraps a JobFetcher with ATS-level rate limiting.
// All fetchers targeting the same ATS should share the same limiter instance.
func NewRateLimitedFetcher(inner model.JobFetcher, limiter *ATSRateLimiter, ats string) *RateLimitedFetcher {
	return &RateLimitedFetcher{
		inner:   inner,
		limiter: limiter,
		ats:     ats,
	}
}

// FetchJobs waits for the rate limiter to allow a request, then delegates to
// the wrapped fetcher.
func (f *RateLimitedFetcher) FetchJobs(ctx context.Context) ([]model.Job, error) {
	if err := f.limiter.Wait(ctx, f.ats); err != nil {
		return nil, err
	}
	return f.inner.FetchJobs(ctx)
}

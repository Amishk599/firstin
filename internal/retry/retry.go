package retry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/amishk599/firstin/internal/model"
)

// RetryFetcher is a decorator that retries transient failures with exponential
// backoff and jitter before delegating to the wrapped JobFetcher.
type RetryFetcher struct {
	inner      model.JobFetcher
	maxRetries int
	baseDelay  time.Duration
	logger     *slog.Logger
}

// NewRetryFetcher wraps a JobFetcher with retry logic.
// maxRetries is the number of additional attempts after the first failure (default: 2).
// baseDelay is the delay before the first retry (default: 5s), doubled on each subsequent retry.
func NewRetryFetcher(inner model.JobFetcher, maxRetries int, baseDelay time.Duration, logger *slog.Logger) *RetryFetcher {
	return &RetryFetcher{
		inner:      inner,
		maxRetries: maxRetries,
		baseDelay:  baseDelay,
		logger:     logger,
	}
}

// FetchJobs attempts to fetch jobs, retrying on transient errors.
func (f *RetryFetcher) FetchJobs(ctx context.Context) ([]model.Job, error) {
	jobs, err := f.inner.FetchJobs(ctx)
	if err == nil {
		return jobs, nil
	}

	if !isRetryable(err) {
		return nil, err
	}

	var lastErr error = err
	for attempt := 1; attempt <= f.maxRetries; attempt++ {
		delay := f.backoffDelay(attempt, lastErr)

		f.logger.Warn("retrying after transient error",
			"attempt", attempt,
			"max_retries", f.maxRetries,
			"delay", delay,
			"error", lastErr,
		)

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("retry cancelled: %w", ctx.Err())
		case <-time.After(delay):
		}

		jobs, err = f.inner.FetchJobs(ctx)
		if err == nil {
			return jobs, nil
		}

		if !isRetryable(err) {
			return nil, err
		}
		lastErr = err
	}

	return nil, lastErr
}

// backoffDelay computes the delay for a given attempt with ±30% jitter.
// If the error includes a Retry-After duration (HTTP 429), that takes precedence.
func (f *RetryFetcher) backoffDelay(attempt int, err error) time.Duration {
	var httpErr *model.HTTPError
	if errors.As(err, &httpErr) && httpErr.RetryAfter > 0 {
		return httpErr.RetryAfter
	}

	// Exponential: baseDelay * 2^(attempt-1)
	delay := f.baseDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
	}

	// Apply ±30% jitter
	jitter := float64(delay) * 0.3
	delay = time.Duration(float64(delay) + (rand.Float64()*2-1)*jitter)

	return delay
}

// isRetryable returns true if the error represents a transient failure worth retrying.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Context cancellation — never retry.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	var httpErr *model.HTTPError
	if errors.As(err, &httpErr) {
		// 429 Too Many Requests — retryable.
		if httpErr.StatusCode == 429 {
			return true
		}
		// 5xx — retryable.
		if httpErr.StatusCode >= 500 {
			return true
		}
		// 4xx (not 429) — not retryable.
		return false
	}

	// Non-HTTP errors (network, DNS, etc.) — retryable.
	return true
}

package model

import (
	"fmt"
	"time"
)

// HTTPError wraps an HTTP status code so retry logic can inspect it.
type HTTPError struct {
	StatusCode int
	RetryAfter time.Duration // from Retry-After header, zero if absent
	Err        error
}

func (e *HTTPError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("HTTP %d: %v", e.StatusCode, e.Err)
	}
	return fmt.Sprintf("HTTP %d", e.StatusCode)
}

func (e *HTTPError) Unwrap() error {
	return e.Err
}

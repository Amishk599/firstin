package adapter

import (
	"strconv"
	"time"
)

// parseRetryAfter parses the Retry-After header value into a duration.
// Supports seconds format (e.g. "120"). Returns zero if absent or unparseable.
func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}
	seconds, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

package model

import (
	"context"
	"time"
)

// Unified representation of a job listing from any ATS.
type Job struct {
	ID        string     // unique per platform
	Company   string     // company name
	Title     string     // job title
	Location  string     // location string
	URL       string     // direct apply link
	PostedAt  *time.Time // nullable (not all APIs provide this)
	FirstSeen time.Time  // our clock (set on first encounter)
	Source    string     // ATS name
}

// JobFetcher fetches job listings from a source (e.g. Greenhouse).
type JobFetcher interface {
	FetchJobs(ctx context.Context) ([]Job, error)
}

// JobStore tracks which job IDs have been seen for deduplication.
type JobStore interface {
	HasSeen(jobID string) (bool, error)
	MarkSeen(jobID string) error
	Cleanup(olderThan time.Duration) error
}

// Notifier sends notifications for new job matches.
type Notifier interface {
	Notify(jobs []Job) error
}

// JobFilter decides whether a job matches the user's criteria.
type JobFilter interface {
	Match(job Job) bool
}

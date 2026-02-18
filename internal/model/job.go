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
	Detail    *JobDetail // optional enriched metadata from detail endpoints
}

// JobDetail holds source-specific metadata populated during FetchJobs or on-demand via FetchJobDetail.
type JobDetail struct {
	ApplyURL      string     // separate apply link (lever applyUrl, workday externalUrl)
	PostedOn      string     // raw posted-on string (workday: "Posted Today")
	StartDate     *time.Time // workday start date
	UpdatedAt      *time.Time // greenhouse updated_at
	FirstPublished *time.Time // greenhouse first_published_at
	PublishedAt    *time.Time // ashby publishedAt, lever createdAt
	RequisitionID string     // greenhouse requisition_id
	PayRanges     []PayRange // greenhouse salary info
}

// PayRange represents a salary/pay range from Greenhouse.
type PayRange struct {
	MinCents     int64
	MaxCents     int64
	CurrencyType string
	Title        string
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
	IsEmpty() (bool, error)
}

// Notifier sends notifications for new job matches.
type Notifier interface {
	Notify(jobs []Job) error
}

// JobFilter decides whether a job matches the user's criteria.
type JobFilter interface {
	Match(job Job) bool
}

// JobDetailFetcher fetches enriched detail for a job on demand.
// Adapters that support a detail endpoint (Greenhouse, Workday) implement this.
type JobDetailFetcher interface {
	FetchJobDetail(ctx context.Context, job Job) (Job, error)
}

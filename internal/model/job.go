package model

import (
	"context"
	"time"
)

// Unified representation of a job listing from any ATS.
type Job struct {
	ID       string // unique per platform
	Company  string // company name
	Title    string // job title
	Location string // location string
	URL      string // direct apply link

	// PostedAt is the canonical freshness signal used by the poller and TUI sort.
	// Each adapter maps its publication timestamp here:
	//   Greenhouse → first_published (list endpoint)
	//   Lever      → createdAt (Unix ms)
	//   Ashby      → publishedAt (RFC3339)
	//   Workday    → startDate if present, else postedOn approximated to midnight UTC
	// Nil when the source provides no usable timestamp.
	PostedAt *time.Time

	// FirstSeen is stamped by our clock on the job's first encounter.
	// Not yet wired — currently remains zero value.
	FirstSeen time.Time

	Source string     // ATS name: "greenhouse", "lever", "ashby", "workday"
	Detail *JobDetail // optional enriched metadata; nil until populated
}

// JobDetail holds ATS-specific metadata. Fields are populated during FetchJobs
// (list-level data) or lazily via FetchJobDetail (detail-endpoint data).
// Not every field applies to every ATS — see inline notes.
type JobDetail struct {
	// ApplyURL is a separate apply link distinct from the job listing URL.
	// Set by: Lever (applyUrl), Workday (externalUrl).
	ApplyURL string

	// PostedOn is the raw relative date string from Workday ("Posted Today",
	// "Posted 3 Days Ago"). Workday does not return absolute timestamps at
	// the list level, so this string is kept for display and approximation.
	// Not set by any other ATS.
	PostedOn string

	// StartDate is the job start date from Workday's detail endpoint.
	// When present it is also used as PostedAt (best available timestamp for Workday).
	// Not set by any other ATS.
	StartDate *time.Time

	// UpdatedAt is greenhouse's updated_at — the last time the job record was
	// mutated (description edits, compliance updates, bulk syncs, etc.).
	// This is NOT a publication timestamp and must not be used for freshness checks.
	// Set by: Greenhouse (list and detail endpoints).
	UpdatedAt *time.Time

	// FirstPublished is greenhouse's first_published — when the job first went
	// publicly live. This is the correct freshness signal for Greenhouse jobs and
	// is what populates Job.PostedAt. Populated here as supplementary display data
	// after a detail fetch (FetchJobDetail).
	// Set by: Greenhouse (detail endpoint only).
	FirstPublished *time.Time

	// PublishedAt is the publication timestamp from Ashby and Lever.
	// Ashby: publishedAt (RFC3339). Lever: createdAt converted from Unix ms.
	// Mirrors Job.PostedAt for these ATS; kept here for display in the TUI detail view.
	PublishedAt *time.Time

	RequisitionID string     // greenhouse requisition_id
	PayRanges     []PayRange // greenhouse pay_input_ranges (salary info)
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

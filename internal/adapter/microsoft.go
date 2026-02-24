package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/amishk599/firstin/internal/model"
)

const (
	microsoftBaseURL      = "https://apply.careers.microsoft.com"
	microsoftPageSize     = 10
	microsoftCutoff       = 24 * time.Hour
	microsoftAuditMaxPages = 20 // caps audit mode at 200 jobs (20 pages × 10)
)

// microsoftPosition represents a single position in the Microsoft search API response.
type microsoftPosition struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Locations   []string `json:"locations"`
	PostedTs    int64    `json:"postedTs"`
	PositionURL string   `json:"positionUrl"`
}

// microsoftSearchResponse is the top-level Microsoft search API response.
type microsoftSearchResponse struct {
	Data struct {
		Positions []microsoftPosition `json:"positions"`
		Count     int                 `json:"count"`
	} `json:"data"`
}

// microsoftDetailData holds the fields we need from the position detail response.
type microsoftDetailData struct {
	JobDescription string `json:"jobDescription"`
	PublicURL      string `json:"publicUrl"`
}

// microsoftDetailResponse is the response from the Microsoft position detail endpoint.
type microsoftDetailResponse struct {
	Data microsoftDetailData `json:"data"`
}

// MicrosoftAdapter fetches jobs from the Microsoft careers API.
type MicrosoftAdapter struct {
	companyName string
	client      *http.Client
	auditMode   bool // when true: return all listings regardless of freshness
}

// NewMicrosoftAdapter creates a new adapter for Microsoft careers.
func NewMicrosoftAdapter(companyName string, client *http.Client) *MicrosoftAdapter {
	return &MicrosoftAdapter{
		companyName: companyName,
		client:      client,
	}
}

// SetAuditMode enables audit mode: all listings are returned regardless of freshness.
func (a *MicrosoftAdapter) SetAuditMode(enabled bool) {
	a.auditMode = enabled
}

// FetchJobs retrieves jobs from Microsoft careers and normalizes them into the
// unified Job model. In normal mode only jobs posted within the last 24 hours
// are returned. In audit mode all listings are returned regardless of freshness.
func (a *MicrosoftAdapter) FetchJobs(ctx context.Context) ([]model.Job, error) {
	positions, err := a.fetchAllPositions(ctx)
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().UTC().Add(-microsoftCutoff)
	jobs := make([]model.Job, 0, len(positions))
	for _, p := range positions {
		if p.PostedTs == 0 {
			continue
		}
		postedAt := time.Unix(p.PostedTs, 0).UTC()
		if !a.auditMode && postedAt.Before(cutoff) {
			continue
		}
		jobs = append(jobs, a.jobFromPosition(p, postedAt))
	}

	return jobs, nil
}

// fetchAllPositions paginates the Microsoft search API, stopping early once a
// full page contains no positions posted within the last 24 hours.
func (a *MicrosoftAdapter) fetchAllPositions(ctx context.Context) ([]microsoftPosition, error) {
	cutoff := time.Now().UTC().Add(-microsoftCutoff)
	var all []microsoftPosition
	start := 0

	for {
		positions, count, err := a.fetchPage(ctx, start)
		if err != nil {
			return nil, err
		}

		all = append(all, positions...)

		// Early exit: if no position on this page was posted within the cutoff,
		// older pages will only get more stale — stop paginating.
		// Skipped in audit mode since we want all listings.
		if !a.auditMode {
			hasAnyFresh := false
			for _, p := range positions {
				if p.PostedTs > 0 && time.Unix(p.PostedTs, 0).UTC().After(cutoff) {
					hasAnyFresh = true
					break
				}
			}
			if !hasAnyFresh {
				break
			}
		}

		start += microsoftPageSize
		if start >= count {
			break
		}
		if a.auditMode && start >= microsoftAuditMaxPages*microsoftPageSize {
			break
		}
	}

	return all, nil
}

// fetchPage fetches a single page of search results at the given start offset.
func (a *MicrosoftAdapter) fetchPage(ctx context.Context, start int) ([]microsoftPosition, int, error) {
	u, _ := url.Parse(microsoftBaseURL + "/api/pcsx/search")
	q := u.Query()
	q.Set("domain", "microsoft.com")
	q.Set("query", "software engineer")
	q.Set("location", "United States")
	q.Set("start", fmt.Sprintf("%d", start))
	q.Set("sort_by", "timestamp")
	q.Set("filter_include_remote", "1")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, 0, fmt.Errorf("microsoft fetch page (start=%d): %w", start, err)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("microsoft fetch page (start=%d): %w", start, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, &model.HTTPError{
			StatusCode: resp.StatusCode,
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
			Err:        fmt.Errorf("microsoft fetch page (start=%d): unexpected status %d", start, resp.StatusCode),
		}
	}

	var msResp microsoftSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&msResp); err != nil {
		return nil, 0, fmt.Errorf("microsoft fetch page (start=%d) decode: %w", start, err)
	}

	return msResp.Data.Positions, msResp.Data.Count, nil
}

// jobFromPosition normalizes a microsoftPosition into the unified Job model.
func (a *MicrosoftAdapter) jobFromPosition(p microsoftPosition, postedAt time.Time) model.Job {
	location := ""
	if len(p.Locations) > 0 {
		location = p.Locations[0]
	}

	jobURL := microsoftBaseURL + p.PositionURL

	return model.Job{
		ID:       fmt.Sprintf("%d", p.ID),
		Company:  a.companyName,
		Title:    p.Name,
		Location: location,
		URL:      jobURL,
		PostedAt: &postedAt,
		Source:   "microsoft",
		Detail:   &model.JobDetail{PublishedAt: &postedAt},
	}
}

// FetchJobDetail enriches a job with its full description and canonical URL
// from the Microsoft position detail endpoint.
func (a *MicrosoftAdapter) FetchJobDetail(ctx context.Context, job model.Job) (model.Job, error) {
	if job.Detail != nil && job.Detail.Description != "" {
		return job, nil
	}

	u, _ := url.Parse(microsoftBaseURL + "/api/pcsx/position_details")
	q := u.Query()
	q.Set("position_id", job.ID)
	q.Set("domain", "microsoft.com")
	q.Set("hl", "en")
	q.Set("queried_location", "United States")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return job, fmt.Errorf("microsoft detail request for job %s: %w", job.ID, err)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return job, fmt.Errorf("microsoft detail fetch for job %s: %w", job.ID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return job, &model.HTTPError{
			StatusCode: resp.StatusCode,
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
			Err:        fmt.Errorf("microsoft detail fetch for job %s: unexpected status %d", job.ID, resp.StatusCode),
		}
	}

	var detail microsoftDetailResponse
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return job, fmt.Errorf("microsoft detail decode for job %s: %w", job.ID, err)
	}

	if job.Detail == nil {
		job.Detail = &model.JobDetail{}
	}
	if detail.Data.JobDescription != "" {
		job.Detail.Description = extractText(detail.Data.JobDescription)
	}
	if detail.Data.PublicURL != "" {
		job.URL = detail.Data.PublicURL
	}

	return job, nil
}

package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/amishk599/firstin/internal/model"
)

const greenhouseBaseURL = "https://boards-api.greenhouse.io/v1/boards"

var htmlTagRegex = regexp.MustCompile(`<[^>]*>`)

// greenhouseJob represents a single job in the Greenhouse API response.
type greenhouseJob struct {
	ID          int64             `json:"id"`
	Title       string            `json:"title"`
	Location    greenhouseLocation `json:"location"`
	AbsoluteURL string            `json:"absolute_url"`
	UpdatedAt   string            `json:"updated_at"`
}

type greenhouseLocation struct {
	Name string `json:"name"`
}

// greenhouseResponse is the top-level Greenhouse jobs API response.
type greenhouseResponse struct {
	Jobs []greenhouseJob `json:"jobs"`
}

// greenhouseJobDetail is the response from the Greenhouse job detail endpoint.
type greenhouseJobDetail struct {
	ID              int64                `json:"id"`
	Title           string               `json:"title"`
	UpdatedAt       string               `json:"updated_at"`
	FirstPublished  string               `json:"first_published"`
	RequisitionID   string               `json:"requisition_id"`
	Location        greenhouseLocation   `json:"location"`
	Content         string               `json:"content"`
	AbsoluteURL     string               `json:"absolute_url"`
	InternalJobID   int64                `json:"internal_job_id"`
	PayInputRanges  []greenhousePayRange `json:"pay_input_ranges"`
}

type greenhousePayRange struct {
	MinCents     int64  `json:"min_cents"`
	MaxCents     int64  `json:"max_cents"`
	CurrencyType string `json:"currency_type"`
	Title        string `json:"title"`
	Blurb        string `json:"blurb"`
}

// GreenhouseAdapter fetches jobs from the Greenhouse public boards API.
type GreenhouseAdapter struct {
	boardToken  string
	companyName string
	client      *http.Client
}

// NewGreenhouseAdapter creates a new adapter for a Greenhouse board.
func NewGreenhouseAdapter(boardToken string, companyName string, client *http.Client) *GreenhouseAdapter {
	return &GreenhouseAdapter{
		boardToken:  boardToken,
		companyName: companyName,
		client:      client,
	}
}

// FetchJobs retrieves all jobs from the Greenhouse board and normalizes them
// into the unified Job model.
func (a *GreenhouseAdapter) FetchJobs(ctx context.Context) ([]model.Job, error) {
	url := fmt.Sprintf("%s/%s/jobs", greenhouseBaseURL, a.boardToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("greenhouse fetch for %s: %w", a.boardToken, err)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("greenhouse fetch for %s: %w", a.boardToken, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &model.HTTPError{
			StatusCode: resp.StatusCode,
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
			Err:        fmt.Errorf("greenhouse fetch for %s: unexpected status %d", a.boardToken, resp.StatusCode),
		}
	}

	var ghResp greenhouseResponse
	if err := json.NewDecoder(resp.Body).Decode(&ghResp); err != nil {
		return nil, fmt.Errorf("greenhouse fetch for %s: %w", a.boardToken, err)
	}

	jobs := make([]model.Job, 0, len(ghResp.Jobs))
	for _, gj := range ghResp.Jobs {
		job := model.Job{
			ID:       fmt.Sprintf("%d", gj.ID),
			Company:  a.companyName,
			Title:    gj.Title,
			Location: gj.Location.Name,
			URL:      gj.AbsoluteURL,
			Source:   "greenhouse",
		}

		if gj.UpdatedAt != "" {
			t, err := time.Parse(time.RFC3339, gj.UpdatedAt)
			if err == nil {
				job.PostedAt = &t
				job.Detail = &model.JobDetail{UpdatedAt: &t}
			}
		}

		jobs = append(jobs, job)
	}

	return jobs, nil
}

// fetchDetail retrieves full job details from the Greenhouse job detail endpoint.
func (a *GreenhouseAdapter) fetchDetail(ctx context.Context, jobID int64) (greenhouseJobDetail, error) {
	url := fmt.Sprintf("%s/%s/jobs/%d", greenhouseBaseURL, a.boardToken, jobID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return greenhouseJobDetail{}, fmt.Errorf("greenhouse detail request for %s job %d: %w", a.companyName, jobID, err)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return greenhouseJobDetail{}, fmt.Errorf("greenhouse detail fetch for %s job %d: %w", a.companyName, jobID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return greenhouseJobDetail{}, &model.HTTPError{
			StatusCode: resp.StatusCode,
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
			Err:        fmt.Errorf("greenhouse detail fetch for %s job %d: unexpected status %d", a.companyName, jobID, resp.StatusCode),
		}
	}

	var detail greenhouseJobDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return greenhouseJobDetail{}, fmt.Errorf("greenhouse detail decode for %s job %d: %w", a.companyName, jobID, err)
	}

	return detail, nil
}

// FetchJobDetail enriches a job with data from the Greenhouse detail endpoint.
func (a *GreenhouseAdapter) FetchJobDetail(ctx context.Context, job model.Job) (model.Job, error) {
	var jobID int64
	if _, err := fmt.Sscanf(job.ID, "%d", &jobID); err != nil {
		return job, fmt.Errorf("greenhouse detail: invalid job ID %q: %w", job.ID, err)
	}

	detail, err := a.fetchDetail(ctx, jobID)
	if err != nil {
		return job, err
	}

	if job.Detail == nil {
		job.Detail = &model.JobDetail{}
	}

	if detail.UpdatedAt != "" {
		if t, err := time.Parse(time.RFC3339, detail.UpdatedAt); err == nil {
			job.Detail.UpdatedAt = &t
		}
	}
	if detail.FirstPublished != "" {
		if t, err := time.Parse(time.RFC3339, detail.FirstPublished); err == nil {
			job.Detail.FirstPublished = &t
		}
	}
	job.Detail.RequisitionID = detail.RequisitionID

	for _, pr := range detail.PayInputRanges {
		job.Detail.PayRanges = append(job.Detail.PayRanges, model.PayRange{
			MinCents:     pr.MinCents,
			MaxCents:     pr.MaxCents,
			CurrencyType: pr.CurrencyType,
			Title:        pr.Title,
		})
	}

	return job, nil
}

// extractText converts the Greenhouse content field to plain text.
// The content is HTML-encoded HTML (e.g. "&lt;p&gt;" in the JSON string), so
// we first unescape entities to get real HTML, then strip all tags and
// collapse leftover whitespace into a single clean string.
func extractText(content string) string {
	unescaped := html.UnescapeString(content)
	plain := htmlTagRegex.ReplaceAllString(unescaped, "")
	return strings.Join(strings.Fields(plain), " ")
}

package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/amishk599/firstin/internal/model"
)

const ashbyBaseURL = "https://api.ashbyhq.com/posting-api/job-board"

// ashbyJob represents a single job in the Ashby API response.
type ashbyJob struct {
	Title       string `json:"title"`
	Location    string `json:"location"`
	JobUrl      string `json:"jobUrl"`
	PublishedAt string `json:"publishedAt"`
	IsListed    bool   `json:"isListed"`
}

// ashbyResponse is the top-level Ashby job board API response.
type ashbyResponse struct {
	Jobs []ashbyJob `json:"jobs"`
}

// AshbyAdapter fetches jobs from the Ashby public job board API.
type AshbyAdapter struct {
	boardToken  string
	companyName string
	client      *http.Client
}

// NewAshbyAdapter creates a new adapter for an Ashby job board.
func NewAshbyAdapter(boardToken string, companyName string, client *http.Client) *AshbyAdapter {
	return &AshbyAdapter{
		boardToken:  boardToken,
		companyName: companyName,
		client:      client,
	}
}

// FetchJobs retrieves all jobs from the Ashby job board and normalizes them
// into the unified Job model.
func (a *AshbyAdapter) FetchJobs(ctx context.Context) ([]model.Job, error) {
	url := fmt.Sprintf("%s/%s", ashbyBaseURL, a.boardToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("ashby fetch for %s: %w", a.boardToken, err)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ashby fetch for %s: %w", a.boardToken, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &model.HTTPError{
			StatusCode: resp.StatusCode,
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
			Err:        fmt.Errorf("ashby fetch for %s: unexpected status %d", a.boardToken, resp.StatusCode),
		}
	}

	var ashbyResp ashbyResponse
	if err := json.NewDecoder(resp.Body).Decode(&ashbyResp); err != nil {
		return nil, fmt.Errorf("ashby fetch for %s: %w", a.boardToken, err)
	}

	jobs := make([]model.Job, 0, len(ashbyResp.Jobs))
	for _, aj := range ashbyResp.Jobs {
		if !aj.IsListed {
			continue
		}

		job := model.Job{
			ID:      aj.JobUrl,
			Company: a.companyName,
			Title:   aj.Title,
			Location: aj.Location,
			URL:     aj.JobUrl,
			Source:  "ashby",
		}

		if aj.PublishedAt != "" {
			t, err := time.Parse(time.RFC3339, aj.PublishedAt)
			if err == nil {
				job.PostedAt = &t
				job.Detail = &model.JobDetail{PublishedAt: &t}
			}
		}

		jobs = append(jobs, job)
	}

	return jobs, nil
}

package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/amishk599/firstin/internal/model"
)

const gemBaseURL = "https://api.gem.com/job_board/v0"

type gemJob struct {
	ID             string      `json:"id"`
	Title          string      `json:"title"`
	Location       gemLocation `json:"location"`
	AbsoluteURL    string      `json:"absolute_url"`
	FirstPublished string      `json:"first_published_at"`
	UpdatedAt      string      `json:"updated_at"`
	Content        string      `json:"content"`
	ContentPlain   string      `json:"content_plain"`
}

type gemLocation struct {
	Name string `json:"name"`
}

// GemAdapter fetches jobs from the Gem public job board API.
type GemAdapter struct {
	boardToken  string
	companyName string
	client      *http.Client
}

// NewGemAdapter creates a new adapter for a Gem job board.
func NewGemAdapter(boardToken string, companyName string, client *http.Client) *GemAdapter {
	return &GemAdapter{
		boardToken:  boardToken,
		companyName: companyName,
		client:      client,
	}
}

// FetchJobs retrieves all jobs from the Gem board and normalizes them
// into the unified Job model.
func (a *GemAdapter) FetchJobs(ctx context.Context) ([]model.Job, error) {
	url := fmt.Sprintf("%s/%s/job_posts/", gemBaseURL, a.boardToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("gem fetch for %s: %w", a.boardToken, err)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gem fetch for %s: %w", a.boardToken, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &model.HTTPError{
			StatusCode: resp.StatusCode,
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
			Err:        fmt.Errorf("gem fetch for %s: unexpected status %d", a.boardToken, resp.StatusCode),
		}
	}

	var gemJobs []gemJob
	if err := json.NewDecoder(resp.Body).Decode(&gemJobs); err != nil {
		return nil, fmt.Errorf("gem fetch for %s: %w", a.boardToken, err)
	}

	jobs := make([]model.Job, 0, len(gemJobs))
	for _, gj := range gemJobs {
		job := model.Job{
			ID:       gj.ID,
			Company:  a.companyName,
			Title:    gj.Title,
			Location: gj.Location.Name,
			URL:      gj.AbsoluteURL,
			Source:   "gem",
		}

		if gj.FirstPublished != "" {
			if t, err := time.Parse(time.RFC3339, gj.FirstPublished); err == nil {
				job.PostedAt = &t
			}
		}
		if gj.UpdatedAt != "" {
			if t, err := time.Parse(time.RFC3339, gj.UpdatedAt); err == nil {
				job.Detail = &model.JobDetail{UpdatedAt: &t}
			}
		}
		desc := gj.ContentPlain
		if desc == "" && gj.Content != "" {
			desc = extractText(gj.Content)
		}
		if desc != "" {
			if job.Detail == nil {
				job.Detail = &model.JobDetail{}
			}
			job.Detail.Description = desc
		}

		jobs = append(jobs, job)
	}

	return jobs, nil
}

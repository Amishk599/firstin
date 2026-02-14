package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/amishk599/firstin/internal/model"
)

const greenhouseBaseURL = "https://boards-api.greenhouse.io/v1/boards"

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
		return nil, fmt.Errorf("greenhouse fetch for %s: unexpected status %d", a.boardToken, resp.StatusCode)
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
			}
		}

		jobs = append(jobs, job)
	}

	return jobs, nil
}

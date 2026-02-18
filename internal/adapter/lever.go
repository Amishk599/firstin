package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/amishk599/firstin/internal/model"
)

const leverBaseURL = "https://api.lever.co/v0/postings"

// leverCategories represents the categories object in a Lever job.
type leverCategories struct {
	Team         string   `json:"team"`
	Department   string   `json:"department"`
	Location     string   `json:"location"`
	Commitment   string   `json:"commitment"`
	AllLocations []string `json:"allLocations"`
}

// leverJob represents a single job in the Lever API response.
type leverJob struct {
	ID               string          `json:"id"`
	Text             string          `json:"text"`
	Description      string          `json:"description"`
	DescriptionPlain string          `json:"descriptionPlain"`
	Categories       leverCategories `json:"categories"`
	CreatedAt        int64           `json:"createdAt"`
	WorkplaceType    string          `json:"workplaceType"`
	HostedURL        string          `json:"hostedUrl"`
	ApplyURL         string          `json:"applyUrl"`
}

// LeverAdapter fetches jobs from the Lever public postings API.
type LeverAdapter struct {
	companySlug string
	companyName string
	client      *http.Client
}

// NewLeverAdapter creates a new adapter for a Lever board.
func NewLeverAdapter(companySlug string, companyName string, client *http.Client) *LeverAdapter {
	return &LeverAdapter{
		companySlug: companySlug,
		companyName: companyName,
		client:      client,
	}
}

// FetchJobs retrieves all jobs from the Lever board and normalizes them
// into the unified Job model.
func (a *LeverAdapter) FetchJobs(ctx context.Context) ([]model.Job, error) {
	url := fmt.Sprintf("%s/%s?mode=json", leverBaseURL, a.companySlug)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("lever fetch for %s: %w", a.companySlug, err)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lever fetch for %s: %w", a.companySlug, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &model.HTTPError{
			StatusCode: resp.StatusCode,
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
			Err:        fmt.Errorf("lever fetch for %s: unexpected status %d", a.companySlug, resp.StatusCode),
		}
	}

	var leverJobs []leverJob
	if err := json.NewDecoder(resp.Body).Decode(&leverJobs); err != nil {
		return nil, fmt.Errorf("lever fetch for %s: %w", a.companySlug, err)
	}

	jobs := make([]model.Job, 0, len(leverJobs))
	for _, lj := range leverJobs {
		// Determine location: prefer allLocations if available, fallback to location
		location := lj.Categories.Location
		if len(lj.Categories.AllLocations) > 0 {
			location = strings.Join(lj.Categories.AllLocations, ", ")
		}

		// Convert createdAt (Unix milliseconds) to time.Time
		var postedAt *time.Time
		if lj.CreatedAt > 0 {
			t := time.UnixMilli(lj.CreatedAt)
			postedAt = &t
		}

		job := model.Job{
			ID:       lj.ID,
			Company:  a.companyName,
			Title:    lj.Text,
			Location: location,
			URL:      lj.HostedURL,
			PostedAt: postedAt,
			Source:   "lever",
			Detail: &model.JobDetail{
				PublishedAt: postedAt,
				ApplyURL:    lj.ApplyURL,
			},
		}

		jobs = append(jobs, job)
	}

	return jobs, nil
}

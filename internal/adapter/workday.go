package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/amishk599/firstin/internal/model"
)

const workdayPageSize = 20

// workdayListingResponse is the response from the Workday jobs listing endpoint.
type workdayListingResponse struct {
	Total       int              `json:"total"`
	JobPostings []workdayListing `json:"jobPostings"`
}

type workdayListing struct {
	Title         string   `json:"title"`
	ExternalPath  string   `json:"externalPath"`
	LocationsText string   `json:"locationsText"`
	PostedOn      string   `json:"postedOn"`
	BulletFields  []string `json:"bulletFields"`
}

// workdayListingRequest is the POST body for the Workday jobs listing endpoint.
type workdayListingRequest struct {
	AppliedFacets map[string]any `json:"appliedFacets"`
	Limit         int            `json:"limit"`
	Offset        int            `json:"offset"`
	SearchText    string         `json:"searchText"`
}

// workdayDetailResponse is the response from the Workday job detail endpoint.
type workdayDetailResponse struct {
	JobPostingInfo workdayJobDetail `json:"jobPostingInfo"`
}

type workdayJobDetail struct {
	JobReqID            string         `json:"jobReqId"`
	Title               string         `json:"title"`
	Location            string         `json:"location"`
	PostedOn            string         `json:"postedOn"`
	StartDate           string         `json:"startDate"`
	ExternalURL         string         `json:"externalUrl"`
	Country             workdayCountry `json:"country"`
	AdditionalLocations []string       `json:"additionalLocations"`
}

type workdayCountry struct {
	Descriptor string `json:"descriptor"`
}

// WorkdayAdapter fetches jobs from a Workday career site.
type WorkdayAdapter struct {
	baseURL     string
	companyName string
	client      *http.Client
	preFilter   model.JobFilter // optional: used to skip detail fetches for listings that clearly won't match
	auditMode   bool            // when true: return all listings, only detail-fetch fresh ones
}

// NewWorkdayAdapter creates a new adapter for a Workday career site.
// An optional preFilter can be provided to skip detail API calls for listings that clearly won't match
// pass nil to disable pre-filtering.
func NewWorkdayAdapter(baseURL string, companyName string, client *http.Client, preFilter model.JobFilter) *WorkdayAdapter {
	return &WorkdayAdapter{
		baseURL:     strings.TrimRight(baseURL, "/"),
		companyName: companyName,
		client:      client,
		preFilter:   preFilter,
	}
}

// SetAuditMode enables audit mode: all listings are returned regardless of freshness,
// but only fresh listings get a detail fetch. Stale listings are returned with
// listing-level data only (locationsText as location, no apply URL).
func (a *WorkdayAdapter) SetAuditMode(enabled bool) {
	a.auditMode = enabled
}

// FetchJobs retrieves jobs from the Workday career site using a two-phase approach:
// 1. Paginate through POST /jobs to get all listings, pre-filtering by freshness.
// 2. GET /job/{externalPath} for each fresh listing to get full details.
//
// In audit mode, all listings are returned but only fresh ones get a detail fetch.
// Stale listings are returned with listing-level data only.
func (a *WorkdayAdapter) FetchJobs(ctx context.Context) ([]model.Job, error) {
	listings, err := a.fetchAllListings(ctx)
	if err != nil {
		return nil, err
	}

	var jobs []model.Job
	for _, l := range listings {
		fresh := isFreshPosting(l.PostedOn)

		if !fresh && !a.auditMode {
			continue
		}
		if fresh && !a.listingPassesPreFilter(l) {
			continue
		}

		if fresh {
			job, err := a.fetchDetail(ctx, l)
			if err != nil {
				return nil, err
			}
			jobs = append(jobs, job)
		} else {
			// Audit mode: return stale listings with listing-level data only
			jobs = append(jobs, a.jobFromListing(l))
		}
	}

	return jobs, nil
}

func (a *WorkdayAdapter) fetchAllListings(ctx context.Context) ([]workdayListing, error) {
	var all []workdayListing
	offset := 0

	for {
		body := workdayListingRequest{
			AppliedFacets: map[string]any{},
			Limit:         workdayPageSize,
			Offset:        offset,
			SearchText:    "",
		}

		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("workday listing marshal for %s: %w", a.companyName, err)
		}

		url := a.baseURL + "/jobs"
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
		if err != nil {
			return nil, fmt.Errorf("workday listing request for %s: %w", a.companyName, err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := a.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("workday listing fetch for %s: %w", a.companyName, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, &model.HTTPError{
				StatusCode: resp.StatusCode,
				RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
				Err:        fmt.Errorf("workday listing fetch for %s: unexpected status %d", a.companyName, resp.StatusCode),
			}
		}

		var listResp workdayListingResponse
		if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
			return nil, fmt.Errorf("workday listing decode for %s: %w", a.companyName, err)
		}

		all = append(all, listResp.JobPostings...)

		// Early exit: if the last listing on this page is stale, all subsequent
		// pages will be even older (Workday returns jobs in reverse chronological
		// order), so there's no point fetching more. Skipped in audit mode since
		// we want all listings.
		if !a.auditMode && len(listResp.JobPostings) > 0 {
			last := listResp.JobPostings[len(listResp.JobPostings)-1]
			if !isFreshPosting(last.PostedOn) {
				break
			}
		}

		offset += workdayPageSize
		if offset >= listResp.Total {
			break
		}
	}

	return all, nil
}

// jobFromListing builds a basic Job from listing-level data (no detail fetch).
// Used in audit mode for stale listings where we skip the detail API call.
func (a *WorkdayAdapter) jobFromListing(l workdayListing) model.Job {
	job := model.Job{
		ID:       l.ExternalPath,
		Company:  a.companyName,
		Title:    l.Title,
		Location: l.LocationsText,
		Source:   "workday",
	}
	job.PostedAt = parsePostedOn(l.PostedOn)
	return job
}

func (a *WorkdayAdapter) fetchDetail(ctx context.Context, listing workdayListing) (model.Job, error) {
	url := a.baseURL + "/" + listing.ExternalPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return model.Job{}, fmt.Errorf("workday detail request for %s: %w", a.companyName, err)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return model.Job{}, fmt.Errorf("workday detail fetch for %s: %w", a.companyName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return model.Job{}, &model.HTTPError{
			StatusCode: resp.StatusCode,
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
			Err:        fmt.Errorf("workday detail fetch for %s: unexpected status %d", a.companyName, resp.StatusCode),
		}
	}

	var detail workdayDetailResponse
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return model.Job{}, fmt.Errorf("workday detail decode for %s: %w", a.companyName, err)
	}

	info := detail.JobPostingInfo

	location := info.Location
	if len(info.AdditionalLocations) > 0 {
		location = location + "; " + strings.Join(info.AdditionalLocations, "; ")
	}

	job := model.Job{
		ID:       info.JobReqID,
		Company:  a.companyName,
		Title:    info.Title,
		Location: location,
		URL:      info.ExternalURL,
		Source:   "workday",
	}

	// Prefer startDate (format "2006-01-02"), fall back to postedOn parsing
	if info.StartDate != "" {
		if t, err := time.Parse("2006-01-02", info.StartDate); err == nil {
			job.PostedAt = &t
		}
	}
	if job.PostedAt == nil {
		job.PostedAt = parsePostedOn(info.PostedOn)
	}

	return job, nil
}

var ambiguousLocationRegex = regexp.MustCompile(`^\d+ Locations?$`)

// listingPassesPreFilter checks whether a listing is worth fetching details for.
// If no preFilter is configured, all listings pass. When a preFilter is set:
//   - Title is always checked.
//   - Location is checked only when locationsText is a specific location (e.g.
//     "India, Pune"). Ambiguous values like "2 Locations" skip the location check
//     because we won't know the real location until the detail fetch.
func (a *WorkdayAdapter) listingPassesPreFilter(l workdayListing) bool {
	if a.preFilter == nil {
		return true
	}

	if isAmbiguousLocation(l.LocationsText) {
		// Location is ambiguous (e.g. "2 Locations") — we can't filter on it
		// without risking false negatives. Let it through; the poller's filter
		// will check the real location after the detail fetch.
		return true
	}

	candidate := model.Job{
		Title:    l.Title,
		Location: l.LocationsText,
	}
	return a.preFilter.Match(candidate)
}

// isAmbiguousLocation returns true for Workday location strings like
// "2 Locations" or "5 Locations" where the actual location is unknown.
func isAmbiguousLocation(loc string) bool {
	return ambiguousLocationRegex.MatchString(loc)
}

// isFreshPosting returns true if the postedOn string indicates a recent posting
// (today or yesterday). Used to pre-filter listings before fetching details.
func isFreshPosting(postedOn string) bool {
	switch postedOn {
	case "Posted Today", "Posted Yesterday":
		return true
	}
	// Also accept "Posted N Days Ago" where N <= 1
	if n, ok := parseDaysAgo(postedOn); ok && n <= 1 {
		return true
	}
	return false
}

var daysAgoRegex = regexp.MustCompile(`^Posted (\d+) Days? Ago$`)

// parsePostedOn converts a Workday relative date string to an approximate timestamp.
func parsePostedOn(postedOn string) *time.Time {
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	switch postedOn {
	case "Posted Today":
		return &today
	case "Posted Yesterday":
		t := today.AddDate(0, 0, -1)
		return &t
	}

	if n, ok := parseDaysAgo(postedOn); ok {
		t := today.AddDate(0, 0, -n)
		return &t
	}

	// "Posted 30+ Days Ago" or unknown → nil
	return nil
}

func parseDaysAgo(s string) (int, bool) {
	matches := daysAgoRegex.FindStringSubmatch(s)
	if matches == nil {
		return 0, false
	}
	n, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

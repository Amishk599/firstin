package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amishk599/firstin/internal/model"
)

func TestWorkdayFetchJobs_Success(t *testing.T) {
	listingResp := `{
		"total": 1,
		"jobPostings": [
			{
				"title": "Software Engineer",
				"externalPath": "job/Software-Engineer/JR328732",
				"locationsText": "San Francisco, CA",
				"postedOn": "Posted Today",
				"bulletFields": ["Full-Time"]
			}
		]
	}`

	detailResp := `{
		"jobPostingInfo": {
			"jobReqId": "JR328732",
			"title": "Software Engineer",
			"location": "San Francisco, CA",
			"postedOn": "Posted Today",
			"startDate": "2026-02-17",
			"externalUrl": "https://salesforce.wd12.myworkdayjobs.com/Slack/job/Software-Engineer/JR328732",
			"country": {"descriptor": "United States of America"},
			"additionalLocations": ["New York, NY"],
			"jobDescription": "<p>Build scalable systems.</p>"
		}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.Write([]byte(listingResp))
		} else {
			w.Write([]byte(detailResp))
		}
	}))
	defer srv.Close()

	a := newWorkdayTestAdapter(srv, "TestCo")

	jobs, err := a.FetchJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}

	j := jobs[0]
	if j.ID != "JR328732" {
		t.Errorf("expected ID JR328732, got %s", j.ID)
	}
	if j.Company != "TestCo" {
		t.Errorf("expected company TestCo, got %s", j.Company)
	}
	if j.Title != "Software Engineer" {
		t.Errorf("expected title Software Engineer, got %s", j.Title)
	}
	if j.Location != "San Francisco, CA; New York, NY" {
		t.Errorf("expected location 'San Francisco, CA; New York, NY', got %s", j.Location)
	}
	if j.Source != "workday" {
		t.Errorf("expected source workday, got %s", j.Source)
	}
	if j.PostedAt == nil {
		t.Fatal("expected PostedAt to be set")
	}
	if j.PostedAt.Year() != 2026 || j.PostedAt.Month() != 2 || j.PostedAt.Day() != 17 {
		t.Errorf("unexpected PostedAt: %v", j.PostedAt)
	}
	if j.Detail == nil || j.Detail.Description != "Build scalable systems." {
		t.Errorf("expected description 'Build scalable systems.', got %v", j.Detail)
	}
}

func TestWorkdayFetchJobs_PaginationContinuesWhenLastIsFresh(t *testing.T) {
	postCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			var reqBody workdayListingRequest
			json.NewDecoder(r.Body).Decode(&reqBody)
			postCount++

			if reqBody.Offset == 0 {
				// First page: all fresh → should continue to next page
				listings := make([]workdayListing, 20)
				for i := range listings {
					listings[i] = workdayListing{
						Title:        fmt.Sprintf("Fresh Job %d", i),
						ExternalPath: fmt.Sprintf("job/Fresh-%d/JR%d", i, i),
						PostedOn:     "Posted Today",
					}
				}
				resp := workdayListingResponse{Total: 25, JobPostings: listings}
				json.NewEncoder(w).Encode(resp)
			} else {
				// Second page: last listing is stale → stops here
				listings := []workdayListing{
					{Title: "Fresh 20", ExternalPath: "job/Fresh-20/JR20", PostedOn: "Posted Today"},
					{Title: "Old Job", PostedOn: "Posted 30+ Days Ago"},
				}
				resp := workdayListingResponse{Total: 25, JobPostings: listings}
				json.NewEncoder(w).Encode(resp)
			}
		} else {
			// Detail fetch — return a simple detail for any request
			detail := workdayDetailResponse{
				JobPostingInfo: workdayJobDetail{
					JobReqID:    "JR001",
					Title:       "Some Job",
					Location:    "Remote",
					ExternalURL: "https://example.com/job",
					StartDate:   "2026-02-17",
				},
			}
			json.NewEncoder(w).Encode(detail)
		}
	}))
	defer srv.Close()

	a := newWorkdayTestAdapter(srv, "TestCo")

	_, err := a.FetchJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if postCount != 2 {
		t.Errorf("expected 2 POST requests (last on page 1 was fresh, page 2 has stale tail), got %d", postCount)
	}
}

func TestWorkdayFetchJobs_PaginationStopsWhenWholePageIsStale(t *testing.T) {
	postCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			postCount++
			// Every listing on the page is stale → should stop after page 1.
			// Previously the early exit only checked the last entry, which missed
			// fresh jobs earlier on the page when Workday uses non-chronological
			// ordering. Now the whole page must be stale to trigger early exit.
			listings := make([]workdayListing, 20)
			for i := range 20 {
				listings[i] = workdayListing{
					Title:    fmt.Sprintf("Old Job %d", i),
					PostedOn: "Posted 7 Days Ago",
				}
			}
			resp := workdayListingResponse{Total: 100, JobPostings: listings}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer srv.Close()

	a := newWorkdayTestAdapter(srv, "TestCo")

	_, err := a.FetchJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if postCount != 1 {
		t.Errorf("expected 1 POST request (early exit: entire page was stale), got %d", postCount)
	}
}

func TestWorkdayFetchJobs_FreshnessFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			resp := workdayListingResponse{
				Total: 3,
				JobPostings: []workdayListing{
					{Title: "Old Job", PostedOn: "Posted 30+ Days Ago", ExternalPath: "job/old/1"},
					{Title: "Week Old", PostedOn: "Posted 7 Days Ago", ExternalPath: "job/week/2"},
					{Title: "Yesterday Job", PostedOn: "Posted Yesterday", ExternalPath: "job/yesterday/3"},
				},
			}
			json.NewEncoder(w).Encode(resp)
		} else {
			detail := workdayDetailResponse{
				JobPostingInfo: workdayJobDetail{
					JobReqID:    "JR003",
					Title:       "Yesterday Job",
					Location:    "NYC",
					ExternalURL: "https://example.com/job/3",
					PostedOn:    "Posted Yesterday",
				},
			}
			json.NewEncoder(w).Encode(detail)
		}
	}))
	defer srv.Close()

	a := newWorkdayTestAdapter(srv, "TestCo")

	jobs, err := a.FetchJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only "Posted Yesterday" should pass the freshness pre-filter
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job (only yesterday), got %d", len(jobs))
	}
	if jobs[0].Title != "Yesterday Job" {
		t.Errorf("expected 'Yesterday Job', got %s", jobs[0].Title)
	}
}

func TestWorkdayFetchJobs_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	a := newWorkdayTestAdapter(srv, "FailCo")

	_, err := a.FetchJobs(context.Background())
	if err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
}

// mockFilter rejects jobs whose location doesn't contain the keyword (case-insensitive).
type mockFilter struct {
	locationKeyword string
}

func (f *mockFilter) Match(j model.Job) bool {
	if f.locationKeyword == "" {
		return true
	}
	return strings.Contains(strings.ToLower(j.Location), strings.ToLower(f.locationKeyword))
}

func TestWorkdayFetchJobs_PreFilterSkipsNonMatchingListings(t *testing.T) {
	detailFetched := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			resp := workdayListingResponse{
				Total: 3,
				JobPostings: []workdayListing{
					{
						// Specific location "India, Pune" won't match "US" → should be skipped
						Title:        "Software Engineer",
						ExternalPath: "job/SWE/JR001",
						LocationsText: "India, Pune",
						PostedOn:     "Posted Today",
					},
					{
						// Ambiguous "2 Locations" → can't pre-filter on location, should pass through
						Title:        "Backend Engineer",
						ExternalPath: "job/BE/JR002",
						LocationsText: "2 Locations",
						PostedOn:     "Posted Today",
					},
					{
						// Specific location "San Francisco, US" matches → should pass through
						Title:        "Platform Engineer",
						ExternalPath: "job/PE/JR003",
						LocationsText: "San Francisco, US",
						PostedOn:     "Posted Today",
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
		} else {
			detailFetched++
			detail := workdayDetailResponse{
				JobPostingInfo: workdayJobDetail{
					JobReqID:    "JR999",
					Title:       "Some Job",
					Location:    "Somewhere, US",
					ExternalURL: "https://example.com/job",
					StartDate:   "2026-02-17",
				},
			}
			json.NewEncoder(w).Encode(detail)
		}
	}))
	defer srv.Close()

	f := &mockFilter{locationKeyword: "US"}
	a := newWorkdayTestAdapterWithFilter(srv, "TestCo", f)

	jobs, err := a.FetchJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// JR001 (India, Pune) should be skipped by pre-filter.
	// JR002 (2 Locations) and JR003 (San Francisco, US) should get detail fetched.
	if detailFetched != 2 {
		t.Errorf("expected 2 detail fetches (skip India, keep ambiguous + US), got %d", detailFetched)
	}
	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(jobs))
	}
}

func TestParsePostedOn(t *testing.T) {
	tests := []struct {
		input    string
		wantNil  bool
		wantDays int // days before today (0 = today)
	}{
		{"Posted Today", false, 0},
		{"Posted Yesterday", false, 1},
		{"Posted 3 Days Ago", false, 3},
		{"Posted 1 Day Ago", false, 1},
		{"Posted 30+ Days Ago", true, 0},
		{"Unknown format", true, 0},
		{"", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parsePostedOn(tt.input)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil for %q, got %v", tt.input, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected non-nil for %q", tt.input)
			}
		})
	}
}

// newWorkdayTestAdapter creates a WorkdayAdapter wired to a test server.
func newWorkdayTestAdapter(srv *httptest.Server, company string) *WorkdayAdapter {
	return newWorkdayTestAdapterWithFilter(srv, company, nil)
}

func newWorkdayTestAdapterWithFilter(srv *httptest.Server, company string, preFilter model.JobFilter) *WorkdayAdapter {
	a := NewWorkdayAdapter(srv.URL, company, srv.Client(), preFilter, slog.Default())
	a.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}
	return a
}

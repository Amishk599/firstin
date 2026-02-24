package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/amishk599/firstin/internal/model"
)

// freshMsTs returns a Unix timestamp 30 minutes ago (within the 24h cutoff).
func freshMsTs() int64 {
	return time.Now().Add(-30 * time.Minute).Unix()
}

// staleMsTs returns a Unix timestamp 48 hours ago (outside the 24h cutoff).
func staleMsTs() int64 {
	return time.Now().Add(-48 * time.Hour).Unix()
}

func TestMicrosoftAdapter_FetchJobs_Success(t *testing.T) {
	fresh1 := freshMsTs()
	fresh2 := freshMsTs()
	stale := staleMsTs()

	searchPayload := map[string]any{
		"status": 200,
		"data": map[string]any{
			"positions": []map[string]any{
				{
					"id":          int64(1970393556619327),
					"name":        "Senior Software Engineer",
					"locations":   []string{"United States, Multiple Locations, Multiple Locations"},
					"postedTs":    fresh1,
					"positionUrl": "/careers/job/1970393556619327",
				},
				{
					"id":          int64(1970393556657166),
					"name":        "Senior Unreal Engineer",
					"locations":   []string{"United States, Washington, Redmond"},
					"postedTs":    fresh2,
					"positionUrl": "/careers/job/1970393556657166",
				},
				{
					"id":          int64(9999999999999999),
					"name":        "Old Role",
					"locations":   []string{"United States, California, San Francisco"},
					"postedTs":    stale,
					"positionUrl": "/careers/job/9999999999999999",
				},
			},
			"count": 3,
		},
	}

	srv := newMicrosoftTestServer(t, searchPayload, nil)
	defer srv.Close()

	a := newMicrosoftTestAdapter(srv, "Microsoft")
	jobs, err := a.FetchJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 fresh jobs, got %d", len(jobs))
	}

	j := jobs[0]
	if j.ID != "1970393556619327" {
		t.Errorf("expected ID 1970393556619327, got %s", j.ID)
	}
	if j.Company != "Microsoft" {
		t.Errorf("expected company Microsoft, got %s", j.Company)
	}
	if j.Title != "Senior Software Engineer" {
		t.Errorf("expected title 'Senior Software Engineer', got %s", j.Title)
	}
	if j.Location != "United States, Multiple Locations, Multiple Locations" {
		t.Errorf("unexpected location: %s", j.Location)
	}
	if j.URL != microsoftBaseURL+"/careers/job/1970393556619327" {
		t.Errorf("unexpected URL: %s", j.URL)
	}
	if j.Source != "microsoft" {
		t.Errorf("expected source 'microsoft', got %s", j.Source)
	}
	if j.PostedAt == nil {
		t.Fatal("expected PostedAt to be set")
	}
	if j.PostedAt.Unix() != fresh1 {
		t.Errorf("expected PostedAt %d, got %d", fresh1, j.PostedAt.Unix())
	}
	if j.Detail == nil || j.Detail.PublishedAt == nil {
		t.Fatal("expected Detail.PublishedAt to be set")
	}
}

func TestMicrosoftAdapter_FetchJobs_PaginationEarlyExit(t *testing.T) {
	pageRequests := 0

	// All positions are stale — adapter should stop after the first page.
	searchPayload := map[string]any{
		"status": 200,
		"data": map[string]any{
			"positions": []map[string]any{
				{
					"id":          int64(111),
					"name":        "Old Engineer",
					"locations":   []string{"United States"},
					"postedTs":    staleMsTs(),
					"positionUrl": "/careers/job/111",
				},
			},
			"count": 50, // signals more pages exist
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/pcsx/search" {
			pageRequests++
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(searchPayload)
		}
	}))
	defer srv.Close()

	a := newMicrosoftTestAdapter(srv, "Microsoft")
	jobs, err := a.FetchJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("expected 0 jobs (all stale), got %d", len(jobs))
	}
	if pageRequests != 1 {
		t.Errorf("expected exactly 1 page request (early exit), got %d", pageRequests)
	}
}

func TestMicrosoftAdapter_FetchJobs_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	a := newMicrosoftTestAdapter(srv, "Microsoft")
	_, err := a.FetchJobs(context.Background())
	if err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
}

func TestMicrosoftAdapter_FetchJobDetail_Success(t *testing.T) {
	detailPayload := map[string]any{
		"status": 200,
		"data": map[string]any{
			"id":             int64(1970393556619327),
			"name":           "Senior Software Engineer",
			"jobDescription": "<b>Overview</b><br><p>The Azure Core New Tech team is seeking engineers.</p>",
			"publicUrl":      "https://apply.careers.microsoft.com/careers/job/1970393556619327",
		},
	}

	srv := newMicrosoftTestServer(t, nil, detailPayload)
	defer srv.Close()

	a := newMicrosoftTestAdapter(srv, "Microsoft")

	postedAt := time.Now().Add(-1 * time.Hour).UTC()
	job := jobFromPositionHelper(a, microsoftPosition{
		ID:          1970393556619327,
		Name:        "Senior Software Engineer",
		Locations:   []string{"United States"},
		PostedTs:    postedAt.Unix(),
		PositionURL: "/careers/job/1970393556619327",
	}, postedAt)

	enriched, err := a.FetchJobDetail(context.Background(), job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enriched.URL != "https://apply.careers.microsoft.com/careers/job/1970393556619327" {
		t.Errorf("expected canonical publicUrl, got %s", enriched.URL)
	}
	if enriched.Detail == nil || enriched.Detail.Description == "" {
		t.Fatal("expected Description to be populated")
	}
	if enriched.Detail.Description != "OverviewThe Azure Core New Tech team is seeking engineers." {
		t.Errorf("unexpected description: %q", enriched.Detail.Description)
	}
}

func TestMicrosoftAdapter_FetchJobs_EmptyLocations(t *testing.T) {
	searchPayload := map[string]any{
		"status": 200,
		"data": map[string]any{
			"positions": []map[string]any{
				{
					"id":          int64(42),
					"name":        "Software Engineer",
					"locations":   []string{},
					"postedTs":    freshMsTs(),
					"positionUrl": "/careers/job/42",
				},
			},
			"count": 1,
		},
	}

	srv := newMicrosoftTestServer(t, searchPayload, nil)
	defer srv.Close()

	a := newMicrosoftTestAdapter(srv, "Microsoft")
	jobs, err := a.FetchJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Location != "" {
		t.Errorf("expected empty location, got %q", jobs[0].Location)
	}
}

// --- helpers ---

// newMicrosoftTestServer creates a test server that serves the given payloads
// for the search and detail endpoints respectively (either can be nil).
func newMicrosoftTestServer(t *testing.T, searchPayload, detailPayload any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/pcsx/search":
			if searchPayload != nil {
				json.NewEncoder(w).Encode(searchPayload)
			}
		case "/api/pcsx/position_details":
			if detailPayload != nil {
				json.NewEncoder(w).Encode(detailPayload)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// newMicrosoftTestAdapter creates a MicrosoftAdapter wired to a test server.
func newMicrosoftTestAdapter(srv *httptest.Server, company string) *MicrosoftAdapter {
	a := NewMicrosoftAdapter(company, srv.Client())
	a.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}
	return a
}

// jobFromPositionHelper calls the unexported jobFromPosition for test setup.
func jobFromPositionHelper(a *MicrosoftAdapter, p microsoftPosition, postedAt time.Time) model.Job {
	return a.jobFromPosition(p, postedAt)
}

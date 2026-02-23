package adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLeverAdapter_FetchJobs_Success(t *testing.T) {
	payload := `[
		{
			"id": "ff7ef527-b0d3-4c44-836a-8d6b58ac321e",
			"text": "Software Engineer",
			"description": "<div>Full HTML description</div>",
			"descriptionPlain": "Plain text job description",
			"categories": {
				"team": "Engineering",
				"department": "Platform",
				"location": "San Francisco, CA",
				"commitment": "Full-time",
				"allLocations": ["San Francisco, CA", "Remote"]
			},
			"createdAt": 1769784074110,
			"workplaceType": "hybrid",
			"hostedUrl": "https://jobs.lever.co/acme/ff7ef527-b0d3-4c44-836a-8d6b58ac321e",
			"applyUrl": "https://jobs.lever.co/acme/ff7ef527-b0d3-4c44-836a-8d6b58ac321e/apply"
		},
		{
			"id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			"text": "Backend Engineer",
			"description": "<div>Backend job description</div>",
			"descriptionPlain": "Backend job description",
			"categories": {
				"team": "Engineering",
				"department": "Backend",
				"location": "Remote",
				"commitment": "Full-time",
				"allLocations": ["Remote"]
			},
			"createdAt": 1769870474110,
			"workplaceType": "remote",
			"hostedUrl": "https://jobs.lever.co/acme/a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			"applyUrl": "https://jobs.lever.co/acme/a1b2c3d4-e5f6-7890-abcd-ef1234567890/apply"
		}
	]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(payload))
	}))
	defer srv.Close()

	adapter := newLeverTestAdapter(srv, "acme", "Acme Corp")

	jobs, err := adapter.FetchJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}

	// Verify first job
	j := jobs[0]
	if j.ID != "ff7ef527-b0d3-4c44-836a-8d6b58ac321e" {
		t.Errorf("expected ID ff7ef527-b0d3-4c44-836a-8d6b58ac321e, got %s", j.ID)
	}
	if j.Company != "Acme Corp" {
		t.Errorf("expected company Acme Corp, got %s", j.Company)
	}
	if j.Title != "Software Engineer" {
		t.Errorf("expected title Software Engineer, got %s", j.Title)
	}
	if j.Location != "San Francisco, CA, Remote" {
		t.Errorf("expected location 'San Francisco, CA, Remote', got %s", j.Location)
	}
	if j.URL != "https://jobs.lever.co/acme/ff7ef527-b0d3-4c44-836a-8d6b58ac321e" {
		t.Errorf("expected hostedUrl, got %s", j.URL)
	}
	if j.Source != "lever" {
		t.Errorf("expected source lever, got %s", j.Source)
	}
	if j.PostedAt == nil {
		t.Fatal("expected PostedAt to be set from createdAt")
	}
	expected := time.UnixMilli(1769784074110).UTC()
	if !j.PostedAt.Equal(expected) {
		t.Errorf("expected PostedAt %v, got %v", expected, j.PostedAt)
	}

	if j.Detail == nil {
		t.Fatal("expected Detail to be set for first job")
	}
	if j.Detail.Description != "Plain text job description" {
		t.Errorf("expected Description 'Plain text job description', got %s", j.Detail.Description)
	}

	// Verify second job
	j2 := jobs[1]
	if j2.ID != "a1b2c3d4-e5f6-7890-abcd-ef1234567890" {
		t.Errorf("expected ID a1b2c3d4-e5f6-7890-abcd-ef1234567890, got %s", j2.ID)
	}
	if j2.Location != "Remote" {
		t.Errorf("expected location Remote, got %s", j2.Location)
	}
	if j2.PostedAt == nil {
		t.Fatal("expected PostedAt to be set for second job")
	}
	if j2.Detail == nil {
		t.Fatal("expected Detail to be set for second job")
	}
	if j2.Detail.Description != "Backend job description" {
		t.Errorf("expected Description 'Backend job description', got %s", j2.Detail.Description)
	}
}

func TestLeverAdapter_FetchJobs_EmptyBoard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	adapter := newLeverTestAdapter(srv, "empty-co", "Empty Co")

	jobs, err := adapter.FetchJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("expected 0 jobs, got %d", len(jobs))
	}
}

func TestLeverAdapter_FetchJobs_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	adapter := newLeverTestAdapter(srv, "bad-co", "Bad Co")

	_, err := adapter.FetchJobs(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestLeverAdapter_FetchJobs_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	adapter := newLeverTestAdapter(srv, "fail-co", "Fail Co")

	_, err := adapter.FetchJobs(context.Background())
	if err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
}

func TestLeverAdapter_FetchJobs_LocationFallback(t *testing.T) {
	payload := `[
		{
			"id": "test-id-123",
			"text": "Test Engineer",
			"description": "<div>Test</div>",
			"descriptionPlain": "Test",
			"categories": {
				"team": "Engineering",
				"department": "QA",
				"location": "New York, NY",
				"commitment": "Full-time",
				"allLocations": []
			},
			"createdAt": 1769784074110,
			"workplaceType": "onsite",
			"hostedUrl": "https://jobs.lever.co/acme/test-id-123",
			"applyUrl": "https://jobs.lever.co/acme/test-id-123/apply"
		}
	]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(payload))
	}))
	defer srv.Close()

	adapter := newLeverTestAdapter(srv, "acme", "Acme Corp")

	jobs, err := adapter.FetchJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}

	if jobs[0].Location != "New York, NY" {
		t.Errorf("expected fallback to categories.location, got %s", jobs[0].Location)
	}
	if jobs[0].URL != "https://jobs.lever.co/acme/test-id-123" {
		t.Errorf("expected hostedUrl, got %s", jobs[0].URL)
	}
}

// --- helpers ---

// newLeverTestAdapter creates a LeverAdapter wired to a test server.
func newLeverTestAdapter(srv *httptest.Server, slug, company string) *LeverAdapter {
	a := NewLeverAdapter(slug, company, srv.Client())
	a.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}
	return a
}

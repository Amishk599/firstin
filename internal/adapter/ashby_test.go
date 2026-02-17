package adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAshbyFetchJobs_Success(t *testing.T) {
	payload := `{
		"apiVersion": "1",
		"jobs": [
			{
				"title": "Software Engineer",
				"location": "San Francisco, CA",
				"jobUrl": "https://jobs.ashbyhq.com/acme/abc-123",
				"publishedAt": "2026-02-13T10:00:00Z",
				"isListed": true
			},
			{
				"title": "Backend Engineer",
				"location": "Remote, US",
				"jobUrl": "https://jobs.ashbyhq.com/acme/def-456",
				"publishedAt": "2026-02-13T11:30:00Z",
				"isListed": true
			},
			{
				"title": "Unlisted Role",
				"location": "NYC",
				"jobUrl": "https://jobs.ashbyhq.com/acme/ghi-789",
				"publishedAt": "2026-02-13T12:00:00Z",
				"isListed": false
			}
		]
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(payload))
	}))
	defer srv.Close()

	adapter := newAshbyTestAdapter(srv, "acme", "Acme Corp")

	jobs, err := adapter.FetchJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs (unlisted filtered), got %d", len(jobs))
	}

	// Verify first job
	j := jobs[0]
	if j.ID != "https://jobs.ashbyhq.com/acme/abc-123" {
		t.Errorf("expected ID https://jobs.ashbyhq.com/acme/abc-123, got %s", j.ID)
	}
	if j.Company != "Acme Corp" {
		t.Errorf("expected company Acme Corp, got %s", j.Company)
	}
	if j.Title != "Software Engineer" {
		t.Errorf("expected title Software Engineer, got %s", j.Title)
	}
	if j.Location != "San Francisco, CA" {
		t.Errorf("expected location San Francisco, CA, got %s", j.Location)
	}
	if j.URL != "https://jobs.ashbyhq.com/acme/abc-123" {
		t.Errorf("expected URL https://jobs.ashbyhq.com/acme/abc-123, got %s", j.URL)
	}
	if j.Source != "ashby" {
		t.Errorf("expected source ashby, got %s", j.Source)
	}
	if j.PostedAt == nil {
		t.Fatal("expected PostedAt to be set")
	}
	if j.PostedAt.Year() != 2026 || j.PostedAt.Month() != 2 || j.PostedAt.Day() != 13 {
		t.Errorf("unexpected PostedAt: %v", j.PostedAt)
	}
}

func TestAshbyFetchJobs_EmptyBoard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"apiVersion": "1", "jobs": []}`))
	}))
	defer srv.Close()

	adapter := newAshbyTestAdapter(srv, "empty-co", "Empty Co")

	jobs, err := adapter.FetchJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("expected 0 jobs, got %d", len(jobs))
	}
}

func TestAshbyFetchJobs_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	adapter := newAshbyTestAdapter(srv, "bad-co", "Bad Co")

	_, err := adapter.FetchJobs(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestAshbyFetchJobs_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	adapter := newAshbyTestAdapter(srv, "fail-co", "Fail Co")

	_, err := adapter.FetchJobs(context.Background())
	if err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
}

// newAshbyTestAdapter creates an AshbyAdapter wired to a test server.
func newAshbyTestAdapter(srv *httptest.Server, token, company string) *AshbyAdapter {
	a := NewAshbyAdapter(token, company, srv.Client())
	a.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}
	return a
}

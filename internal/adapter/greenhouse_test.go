package adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchJobs_Success(t *testing.T) {
	payload := `{
		"jobs": [
			{
				"id": 12345,
				"title": "Software Engineer",
				"location": {"name": "San Francisco, CA"},
				"absolute_url": "https://boards.greenhouse.io/acme/jobs/12345",
				"updated_at": "2026-02-13T10:00:00Z"
			},
			{
				"id": 67890,
				"title": "Backend Engineer",
				"location": {"name": "Remote, US"},
				"absolute_url": "https://boards.greenhouse.io/acme/jobs/67890",
				"updated_at": "2026-02-13T11:30:00Z"
			}
		]
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(payload))
	}))
	defer srv.Close()

	adapter := NewGreenhouseAdapter("acme", "Acme Corp", srv.Client())
	// Override the base URL by pointing the adapter at our test server.
	// We do this by replacing the global base URL temporarily — but since we
	// can't easily do that, let's use a different approach: point the HTTP
	// client at the test server via a custom transport.
	adapter.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			// Rewrite the URL to hit the test server instead.
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	jobs, err := adapter.FetchJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}

	// Verify first job
	j := jobs[0]
	if j.ID != "12345" {
		t.Errorf("expected ID 12345, got %s", j.ID)
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
	if j.Source != "greenhouse" {
		t.Errorf("expected source greenhouse, got %s", j.Source)
	}
	if j.PostedAt == nil {
		t.Fatal("expected PostedAt to be set")
	}
	if j.PostedAt.Year() != 2026 || j.PostedAt.Month() != 2 || j.PostedAt.Day() != 13 {
		t.Errorf("unexpected PostedAt: %v", j.PostedAt)
	}
}

func TestFetchJobs_EmptyBoard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jobs": []}`))
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv, "empty-co", "Empty Co")

	jobs, err := adapter.FetchJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("expected 0 jobs, got %d", len(jobs))
	}
}

func TestFetchJobs_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv, "bad-co", "Bad Co")

	_, err := adapter.FetchJobs(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestFetchJobs_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv, "fail-co", "Fail Co")

	_, err := adapter.FetchJobs(context.Background())
	if err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
}

// --- helpers ---

// roundTripFunc adapts a function into an http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// newTestAdapter creates a GreenhouseAdapter wired to a test server.
func newTestAdapter(srv *httptest.Server, token, company string) *GreenhouseAdapter {
	a := NewGreenhouseAdapter(token, company, srv.Client())
	a.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}
	return a
}

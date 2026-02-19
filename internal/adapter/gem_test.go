package adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amishk599/firstin/internal/model"
)

func TestGemFetchJobs_Success(t *testing.T) {
	payload := `[
		{
			"id": "abc-123",
			"title": "Software Engineer",
			"location": {"name": "San Francisco, CA"},
			"absolute_url": "https://jobs.gem.com/retool/jobs/abc-123",
			"first_published_at": "2026-02-10T09:00:00Z",
			"updated_at": "2026-02-13T10:00:00Z"
		},
		{
			"id": "def-456",
			"title": "Backend Engineer",
			"location": {"name": "Remote, US"},
			"absolute_url": "https://jobs.gem.com/retool/jobs/def-456",
			"first_published_at": "2026-02-11T14:00:00Z",
			"updated_at": "2026-02-13T11:30:00Z"
		}
	]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(payload))
	}))
	defer srv.Close()

	a := newTestGemAdapter(srv, "retool", "Retool")

	jobs, err := a.FetchJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}

	j := jobs[0]
	if j.ID != "abc-123" {
		t.Errorf("expected ID abc-123, got %s", j.ID)
	}
	if j.Company != "Retool" {
		t.Errorf("expected company Retool, got %s", j.Company)
	}
	if j.Title != "Software Engineer" {
		t.Errorf("expected title Software Engineer, got %s", j.Title)
	}
	if j.Location != "San Francisco, CA" {
		t.Errorf("expected location San Francisco, CA, got %s", j.Location)
	}
	if j.Source != "gem" {
		t.Errorf("expected source gem, got %s", j.Source)
	}
	if j.PostedAt == nil {
		t.Fatal("expected PostedAt to be set")
	}
	if j.PostedAt.Year() != 2026 || j.PostedAt.Month() != 2 || j.PostedAt.Day() != 10 {
		t.Errorf("unexpected PostedAt: %v", j.PostedAt)
	}
	if j.URL != "https://jobs.gem.com/retool/jobs/abc-123" {
		t.Errorf("unexpected URL: %s", j.URL)
	}
}

func TestGemFetchJobs_EmptyBoard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	a := newTestGemAdapter(srv, "empty-co", "Empty Co")

	jobs, err := a.FetchJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("expected 0 jobs, got %d", len(jobs))
	}
}

func TestGemFetchJobs_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	a := newTestGemAdapter(srv, "bad-co", "Bad Co")

	_, err := a.FetchJobs(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestGemFetchJobs_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	a := newTestGemAdapter(srv, "fail-co", "Fail Co")

	_, err := a.FetchJobs(context.Background())
	if err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
	var httpErr *model.HTTPError
	if !isHTTPError(err, &httpErr) || httpErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected HTTPError with status 500, got: %v", err)
	}
}

func TestGemFetchJobs_MissingTimestamp(t *testing.T) {
	payload := `[
		{
			"id": "no-ts-789",
			"title": "Platform Engineer",
			"location": {"name": "Remote"},
			"absolute_url": "https://jobs.gem.com/retool/jobs/no-ts-789"
		}
	]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(payload))
	}))
	defer srv.Close()

	a := newTestGemAdapter(srv, "retool", "Retool")

	jobs, err := a.FetchJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].PostedAt != nil {
		t.Errorf("expected PostedAt to be nil, got %v", jobs[0].PostedAt)
	}
}

// --- helpers ---

func newTestGemAdapter(srv *httptest.Server, token, company string) *GemAdapter {
	a := NewGemAdapter(token, company, srv.Client())
	a.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}
	return a
}

func isHTTPError(err error, target **model.HTTPError) bool {
	if he, ok := err.(*model.HTTPError); ok {
		*target = he
		return true
	}
	return false
}

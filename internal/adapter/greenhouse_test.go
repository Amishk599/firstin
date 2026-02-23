package adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amishk599/firstin/internal/model"
)

func TestFetchJobs_Success(t *testing.T) {
	payload := `{
		"jobs": [
			{
				"id": 12345,
				"title": "Software Engineer",
				"location": {"name": "San Francisco, CA"},
				"absolute_url": "https://boards.greenhouse.io/acme/jobs/12345",
				"first_published": "2026-02-10T09:00:00Z",
				"updated_at": "2026-02-13T10:00:00Z"
			},
			{
				"id": 67890,
				"title": "Backend Engineer",
				"location": {"name": "Remote, US"},
				"absolute_url": "https://boards.greenhouse.io/acme/jobs/67890",
				"first_published": "2026-02-11T14:00:00Z",
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
	if j.PostedAt.Year() != 2026 || j.PostedAt.Month() != 2 || j.PostedAt.Day() != 10 {
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

func TestFetchDetail_Success(t *testing.T) {
	payload := `{
		"id": 44444,
		"title": "Product Engineer",
		"updated_at": "2026-02-13T10:00:00Z",
		"requisition_id": "50",
		"location": {"name": "San Francisco, CA"},
		"content": "&lt;p&gt;This is the job description.&lt;/p&gt;",
		"absolute_url": "https://your.co/careers?gh_jid=44444",
		"internal_job_id": 55555,
		"pay_input_ranges": [
			{
				"min_cents": 5000000,
				"max_cents": 7500000,
				"currency_type": "USD",
				"title": "NYC Salary Range",
				"blurb": "In order to provide transparency..."
			}
		]
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/boards/acme/jobs/44444" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(payload))
	}))
	defer srv.Close()

	a := newTestAdapter(srv, "acme", "Acme Corp")
	stub := model.Job{ID: "44444", Company: "Acme Corp", Source: "greenhouse"}
	job, err := a.FetchJobDetail(context.Background(), stub)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job.Detail == nil {
		t.Fatal("expected Detail to be populated")
	}
	if job.Detail.RequisitionID != "50" {
		t.Errorf("expected requisition ID 50, got %s", job.Detail.RequisitionID)
	}
	if len(job.Detail.PayRanges) != 1 || job.Detail.PayRanges[0].MinCents != 5000000 {
		t.Errorf("unexpected pay ranges: %+v", job.Detail.PayRanges)
	}
	if job.Detail.Description != "This is the job description." {
		t.Errorf("expected description 'This is the job description.', got %q", job.Detail.Description)
	}
}

func TestFetchDetail_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	a := newTestAdapter(srv, "acme", "Acme Corp")
	_, err := a.fetchDetail(context.Background(), 99999)
	if err == nil {
		t.Fatal("expected error for HTTP 404, got nil")
	}
}

func TestExtractText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "double-encoded HTML from Greenhouse API",
			input: "This is the job description. &lt;p&gt;Any HTML included.&lt;/p&gt;",
			want:  "This is the job description. Any HTML included.",
		},
		{
			name:  "typical job description with nested tags and whitespace",
			input: "&lt;p&gt;We are hiring.&lt;/p&gt;\n&lt;ul&gt;\n  &lt;li&gt;Write code&lt;/li&gt;\n  &lt;li&gt;Review PRs&lt;/li&gt;\n&lt;/ul&gt;",
			want:  "We are hiring. Write code Review PRs",
		},
		{
			name:  "plain text with no HTML",
			input: "No tags here.",
			want:  "No tags here.",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractText(tc.input)
			if got != tc.want {
				t.Errorf("extractText(%q)\n got  %q\n want %q", tc.input, got, tc.want)
			}
		})
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

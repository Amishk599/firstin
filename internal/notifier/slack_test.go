package notifier

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amishk599/firstin/internal/model"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func timePtr(t time.Time) *time.Time { return &t }

func sampleJob(title, company string) model.Job {
	return model.Job{
		ID:       "123",
		Company:  company,
		Title:    title,
		Location: "Remote, US",
		URL:      "https://example.com/apply",
		PostedAt: timePtr(time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)),
		Source:   "greenhouse",
	}
}

func TestSlackNotifier_EmptyJobs(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewSlackNotifier(srv.URL, srv.Client(), discardLogger())

	if err := n.Notify(nil); err != nil {
		t.Errorf("Notify(nil) = %v, want nil", err)
	}
	if err := n.Notify([]model.Job{}); err != nil {
		t.Errorf("Notify([]) = %v, want nil", err)
	}
	if c := calls.Load(); c != 0 {
		t.Errorf("expected 0 HTTP calls, got %d", c)
	}
}

func TestSlackNotifier_SingleJob(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewSlackNotifier(srv.URL, srv.Client(), discardLogger())
	job := sampleJob("Backend Engineer", "Acme Corp")

	if err := n.Notify([]model.Job{job}); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}

	var payload slackPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	header := payload.Blocks[0]
	if header.Text.Text != "🚀 Acme Corp: Backend Engineer" {
		t.Errorf("header text = %q, want company: title", header.Text.Text)
	}

	companyField := payload.Blocks[1].Fields[0]
	if companyField.Text != "*Company:*\nAcme Corp" {
		t.Errorf("company field = %q", companyField.Text)
	}

	actionURL := payload.Blocks[3].Elements[0].URL
	if actionURL != "https://example.com/apply" {
		t.Errorf("action URL = %q", actionURL)
	}
}

func TestSlackNotifier_MultipleJobs(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewSlackNotifier(srv.URL, srv.Client(), discardLogger())
	jobs := []model.Job{
		sampleJob("Engineer 1", "A"),
		sampleJob("Engineer 2", "B"),
		sampleJob("Engineer 3", "C"),
	}

	if err := n.Notify(jobs); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}
	if c := calls.Load(); c != 3 {
		t.Errorf("expected 3 HTTP calls, got %d", c)
	}
}

func TestSlackNotifier_SlackReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := NewSlackNotifier(srv.URL, srv.Client(), discardLogger())
	jobs := []model.Job{
		sampleJob("Fails", "A"),
		sampleJob("Fails", "B"),
	}

	// Mock returns 500 for all — but we send 2, let's test partial by having a mix.
	// For this test, all fail so Notify should return error.
	// Use a separate test for partial failure.
	err := n.Notify(jobs)
	if err == nil {
		t.Error("expected error when all messages fail, got nil")
	}
}

func TestSlackNotifier_AllFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := NewSlackNotifier(srv.URL, srv.Client(), discardLogger())
	jobs := []model.Job{
		sampleJob("A", "X"),
		sampleJob("B", "Y"),
		sampleJob("C", "Z"),
	}

	err := n.Notify(jobs)
	if err == nil {
		t.Error("expected error when all messages fail, got nil")
	}
}

func TestSlackNotifier_PartialFailure(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := calls.Add(1)
		if c == 1 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	n := NewSlackNotifier(srv.URL, srv.Client(), discardLogger())
	jobs := []model.Job{
		sampleJob("Fails", "A"),
		sampleJob("Succeeds", "B"),
	}

	if err := n.Notify(jobs); err != nil {
		t.Errorf("expected nil (partial success), got %v", err)
	}
}

func TestSlackNotifier_RateLimited(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := calls.Add(1)
		if c == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	n := NewSlackNotifier(srv.URL, srv.Client(), discardLogger())
	err := n.Notify([]model.Job{sampleJob("Rate Limited Job", "Test")})
	if err != nil {
		t.Fatalf("expected nil after retry, got %v", err)
	}
	if c := calls.Load(); c != 2 {
		t.Errorf("expected 2 HTTP calls (initial + retry), got %d", c)
	}
}

func TestSlackNotifier_PayloadFormat(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewSlackNotifier(srv.URL, srv.Client(), discardLogger())
	job := model.Job{
		ID:       "456",
		Company:  "TestCo",
		Title:    "SRE",
		Location: "NYC",
		URL:      "https://example.com/sre",
		Source:   "greenhouse",
		// PostedAt is nil — should display "Just detected"
	}

	if err := n.Notify([]model.Job{job}); err != nil {
		t.Fatalf("Notify() = %v", err)
	}

	var payload slackPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(payload.Blocks) != 5 {
		t.Fatalf("expected 5 blocks, got %d", len(payload.Blocks))
	}

	// header
	if payload.Blocks[0].Type != "header" {
		t.Errorf("block[0] type = %q, want header", payload.Blocks[0].Type)
	}
	// section (company/location)
	if payload.Blocks[1].Type != "section" || len(payload.Blocks[1].Fields) != 2 {
		t.Errorf("block[1] not a 2-field section")
	}
	// section (posted/source)
	if payload.Blocks[2].Type != "section" || len(payload.Blocks[2].Fields) != 2 {
		t.Errorf("block[2] not a 2-field section")
	}
	postedField := payload.Blocks[2].Fields[0].Text
	if postedField != "*Posted:*\nJust detected" {
		t.Errorf("posted field = %q, want 'Just detected' for nil PostedAt", postedField)
	}
	// actions
	if payload.Blocks[3].Type != "actions" || len(payload.Blocks[3].Elements) != 1 {
		t.Errorf("block[3] not a single-element actions block")
	}
	if payload.Blocks[3].Elements[0].Style != "primary" {
		t.Errorf("button style = %q, want primary", payload.Blocks[3].Elements[0].Style)
	}
	// divider
	if payload.Blocks[4].Type != "divider" {
		t.Errorf("block[4] type = %q, want divider", payload.Blocks[4].Type)
	}
}

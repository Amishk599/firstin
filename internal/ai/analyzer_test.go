package ai

import (
	"context"
	"errors"
	"testing"
	"text/template"

	"github.com/amishk599/firstin/internal/model"
)

// mockProvider is a stub LLMProvider for testing.
type mockProvider struct {
	response string
	err      error
}

func (m *mockProvider) Complete(_ context.Context, _ string) (string, error) {
	return m.response, m.err
}

func newTestAnalyzer(provider LLMProvider) *LLMJobAnalyzer {
	tmpl := template.Must(template.New("test").Parse("desc: {{.Description}}"))
	return NewLLMJobAnalyzer(provider, tmpl, nil)
}

// jobWithDesc returns a Job with the given description in its Detail field.
func jobWithDesc(desc string) model.Job {
	return model.Job{
		ID:      "j1",
		Company: "testco",
		Title:   "Software Engineer",
		Detail:  &model.JobDetail{Description: desc},
	}
}

func TestAnalyze_SkipsJobWithNoDescription(t *testing.T) {
	analyzer := newTestAnalyzer(&mockProvider{}) // provider never called
	job := model.Job{ID: "j1", Detail: nil}

	result, err := analyzer.Analyze(context.Background(), job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Insights != nil {
		t.Error("expected nil Insights when no description")
	}
}

func TestAnalyze_PopulatesInsights(t *testing.T) {
	validJSON := `{
		"role_type": "backend",
		"years_exp": "3-5 years",
		"tech_stack": ["Go", "Kubernetes"],
		"key_points": ["Build distributed systems", "Join a small team", "High ownership role"]
	}`
	analyzer := newTestAnalyzer(&mockProvider{response: validJSON})

	result, err := analyzer.Analyze(context.Background(), jobWithDesc("we use Go and Kubernetes"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Insights == nil {
		t.Fatal("expected non-nil Insights")
	}
	if result.Insights.RoleType != "backend" {
		t.Errorf("RoleType = %q, want backend", result.Insights.RoleType)
	}
	if result.Insights.YearsExp != "3-5 years" {
		t.Errorf("YearsExp = %q, want 3-5 years", result.Insights.YearsExp)
	}
	if len(result.Insights.TechStack) != 2 {
		t.Errorf("TechStack len = %d, want 2", len(result.Insights.TechStack))
	}
	if result.Insights.KeyPoints[0] != "Build distributed systems" {
		t.Errorf("KeyPoints[0] = %q", result.Insights.KeyPoints[0])
	}
}

func TestAnalyze_ProviderError_ReturnsOriginalJob(t *testing.T) {
	analyzer := newTestAnalyzer(&mockProvider{err: errors.New("network error")})

	job := jobWithDesc("some description")
	_, err := analyzer.Analyze(context.Background(), job)
	if err == nil {
		t.Fatal("expected error from provider failure")
	}
}

func TestParseInsights_ParsesCleanJSON(t *testing.T) {
	// OpenAI structured outputs guarantees clean JSON — no fences, no preamble.
	input := `{"role_type":"infra","years_exp":"5+ years","tech_stack":["Terraform"],"key_points":["a","b","c"]}`

	insights, err := parseInsights(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if insights.RoleType != "infra" {
		t.Errorf("RoleType = %q, want infra", insights.RoleType)
	}
}

func TestParseInsights_CapsTechStackAtEight(t *testing.T) {
	input := `{"role_type":"backend","years_exp":"not specified","tech_stack":["Go","Rust","Java","Python","C++","Kafka","Redis","Postgres","gRPC"],"key_points":["a","b","c"]}`

	insights, err := parseInsights(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(insights.TechStack) != 8 {
		t.Errorf("TechStack len = %d, want 8 (capped)", len(insights.TechStack))
	}
}

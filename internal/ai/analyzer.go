package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"text/template"

	"github.com/amishk599/firstin/internal/model"
)

// LLMJobAnalyzer implements poller.JobAnalyzer using an LLM.
type LLMJobAnalyzer struct {
	provider LLMProvider
	tmpl     *template.Template
	logger   *slog.Logger
}

// NewLLMJobAnalyzer creates an analyzer that enriches jobs with LLM-generated insights.
func NewLLMJobAnalyzer(provider LLMProvider, tmpl *template.Template, logger *slog.Logger) *LLMJobAnalyzer {
	return &LLMJobAnalyzer{
		provider: provider,
		tmpl:     tmpl,
		logger:   logger,
	}
}

// Analyze enriches job with AI-generated insights. Returns the original job unchanged
// when the description is unavailable or the LLM call fails.
func (a *LLMJobAnalyzer) Analyze(ctx context.Context, job model.Job) (model.Job, error) {
	if job.Detail == nil || job.Detail.Description == "" {
		return job, nil
	}

	var promptBuf bytes.Buffer
	if err := a.tmpl.Execute(&promptBuf, struct{ Description string }{
		Description: job.Detail.Description,
	}); err != nil {
		return job, fmt.Errorf("render prompt: %w", err)
	}

	raw, err := a.provider.Complete(ctx, promptBuf.String())
	if err != nil {
		return job, fmt.Errorf("llm complete: %w", err)
	}

	insights, err := parseInsights(raw)
	if err != nil {
		return job, fmt.Errorf("parse insights: %w", err)
	}

	job.Insights = insights
	return job, nil
}

// rawInsights is the JSON shape returned by the LLM (matches jobInsightsSchema).
type rawInsights struct {
	RoleType  string   `json:"role_type"`
	YearsExp  string   `json:"years_exp"`
	TechStack []string `json:"tech_stack"`
	KeyPoints []string `json:"key_points"`
}

// parseInsights deserializes the LLM response into a JobInsights struct.
// OpenAI structured outputs guarantees valid JSON conforming to jobInsightsSchema,
// so no code-fence stripping or defensive trimming is needed.
func parseInsights(raw string) (*model.JobInsights, error) {
	var ri rawInsights
	if err := json.Unmarshal([]byte(raw), &ri); err != nil {
		return nil, fmt.Errorf("unmarshal insights JSON: %w", err)
	}

	insights := &model.JobInsights{
		RoleType:  ri.RoleType,
		YearsExp:  ri.YearsExp,
		TechStack: ri.TechStack,
	}

	// Populate exactly 3 key points; schema enforces minItems/maxItems: 3.
	for i := 0; i < 3 && i < len(ri.KeyPoints); i++ {
		insights.KeyPoints[i] = ri.KeyPoints[i]
	}

	// Cap tech stack at 8 items as a defensive guard.
	if len(insights.TechStack) > 8 {
		insights.TechStack = insights.TechStack[:8]
	}

	return insights, nil
}

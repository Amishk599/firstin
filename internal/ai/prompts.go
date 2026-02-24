package ai

import (
	_ "embed"
	"text/template"
)

//go:embed prompts/job_analysis.md
var jobAnalysisPromptRaw string

// JobAnalysisTemplate is the parsed prompt template for job analysis.
// Parsed once at package init; reused on every Analyze call.
var JobAnalysisTemplate = template.Must(template.New("job_analysis").Parse(jobAnalysisPromptRaw))

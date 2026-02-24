package ai

import (
	"context"

	"github.com/amishk599/firstin/internal/model"
)

// NopJobAnalyzer is a no-op analyzer used when ai.enabled is false.
// It returns the job unchanged with no LLM calls.
type NopJobAnalyzer struct{}

// NewNopJobAnalyzer returns a NopJobAnalyzer.
func NewNopJobAnalyzer() *NopJobAnalyzer {
	return &NopJobAnalyzer{}
}

// Analyze returns the job unchanged.
func (n *NopJobAnalyzer) Analyze(_ context.Context, job model.Job) (model.Job, error) {
	return job, nil
}

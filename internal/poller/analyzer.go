package poller

import (
	"context"

	"github.com/amishk599/firstin/internal/model"
)

// JobAnalyzer enriches a Job with AI-generated insights.
// Returns the original job unchanged when enrichment is unavailable or disabled.
type JobAnalyzer interface {
	Analyze(ctx context.Context, job model.Job) (model.Job, error)
}

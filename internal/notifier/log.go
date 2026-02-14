package notifier

import (
	"log/slog"

	"github.com/amishk599/firstin/internal/model"
)

// Ensure LogNotifier implements model.Notifier.
var _ model.Notifier = (*LogNotifier)(nil)

// LogNotifier writes new job matches to the given logger as structured messages.
type LogNotifier struct {
	logger *slog.Logger
}

// NewLogNotifier returns a notifier that logs each job via slog.
func NewLogNotifier(logger *slog.Logger) *LogNotifier {
	return &LogNotifier{logger: logger}
}

// Notify logs each job with company, title, location, URL, and posted_at.
// Returns nil (stdout logging does not fail).
func (n *LogNotifier) Notify(jobs []model.Job) error {
	for _, j := range jobs {
		args := []any{"company", j.Company, "title", j.Title, "location", j.Location, "url", j.URL}
		if j.PostedAt != nil {
			args = append(args, "posted_at", *j.PostedAt)
		}
		n.logger.Info("new job", args...)
	}
	return nil
}

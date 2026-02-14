package notifier

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/amishk599/firstin/internal/model"
)

func TestLogNotifier_Notify_zeroJobs(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	n := NewLogNotifier(logger)
	err := n.Notify(nil)
	if err != nil {
		t.Errorf("Notify(nil) = %v, want nil", err)
	}
	err = n.Notify([]model.Job{})
	if err != nil {
		t.Errorf("Notify([]) = %v, want nil", err)
	}
}

func TestLogNotifier_Notify_multipleJobs_returnsNil(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	n := NewLogNotifier(logger)
	posted := time.Now().Add(-30 * time.Minute)
	jobs := []model.Job{
		{Company: "Acme", Title: "Engineer", Location: "Remote", URL: "https://example.com/1", PostedAt: &posted},
		{Company: "Beta", Title: "Developer", Location: "US", URL: "https://example.com/2"},
	}
	err := n.Notify(jobs)
	if err != nil {
		t.Errorf("Notify(jobs) = %v, want nil", err)
	}
}

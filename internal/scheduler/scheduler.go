package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/amishk599/firstin/internal/poller"
)

// Scheduler owns the main loop: ticks on an interval and runs each poller sequentially.
type Scheduler struct {
	pollers  []*poller.CompanyPoller
	interval time.Duration
	logger   *slog.Logger
}

// NewScheduler creates a scheduler that polls all companies at the given interval.
func NewScheduler(pollers []*poller.CompanyPoller, interval time.Duration, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		pollers:  pollers,
		interval: interval,
		logger:   logger,
	}
}

// Run starts the polling loop. It runs one immediate cycle, then ticks on the
// configured interval. It returns nil when ctx is cancelled (graceful shutdown).
func (s *Scheduler) Run(ctx context.Context) error {
	s.logger.Info("starting scheduler",
		"interval", s.interval.String(),
		"companies", len(s.pollers),
	)

	// Run one immediate poll cycle.
	s.pollAll(ctx)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("shutting down scheduler")
			return nil
		case <-time.After(s.interval):
			s.pollAll(ctx)
		}
	}
}

// pollAll runs Poll on each poller sequentially with a small pause between companies.
func (s *Scheduler) pollAll(ctx context.Context) {
	for i, p := range s.pollers {
		if ctx.Err() != nil {
			return
		}

		if err := p.Poll(ctx); err != nil {
			s.logger.Error("poll failed",
				"company", p.Name,
				"error", err,
			)
		}

		// Small sleep between companies to be polite, except after the last one.
		if i < len(s.pollers)-1 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(1 * time.Second):
			}
		}
	}
}

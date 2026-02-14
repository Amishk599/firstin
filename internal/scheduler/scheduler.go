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
	cycle := 1
	s.logger.Info("starting poll cycle", "cycle", cycle)
	s.pollAll(ctx)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("shutting down scheduler", "completed_cycles", cycle)
			return nil
		case <-time.After(s.interval):
			cycle++
			s.logger.Info("starting poll cycle", "cycle", cycle)
			s.pollAll(ctx)
		}
	}
}

// pollAll runs Poll on each poller sequentially.
// Rate limiting between requests to the same ATS is handled by the
// RateLimitedFetcher decorator
func (s *Scheduler) pollAll(ctx context.Context) {
	for _, p := range s.pollers {
		if ctx.Err() != nil {
			return
		}

		if err := p.Poll(ctx); err != nil {
			s.logger.Error("poll failed",
				"company", p.Name,
				"error", err,
			)
		}
	}
}

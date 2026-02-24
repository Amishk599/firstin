package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/amishk599/firstin/internal/poller"
)

// Scheduler runs one long-lived goroutine per ATS group. Each goroutine polls
// its companies sequentially with minDelay between same-ATS requests, then
// sleeps polling_interval before the next pass. Rate limiting is structural.
type Scheduler struct {
	pollers   []*poller.CompanyPoller
	interval  time.Duration
	minDelay  time.Duration
	atsDelays map[string]time.Duration
	logger    *slog.Logger
}

// NewScheduler creates a scheduler that groups pollers by ATS and runs one goroutine per group.
func NewScheduler(pollers []*poller.CompanyPoller, interval, minDelay time.Duration, atsDelays map[string]time.Duration, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		pollers:   pollers,
		interval:  interval,
		minDelay:  minDelay,
		atsDelays: atsDelays,
		logger:    logger,
	}
}

// minDelayFor returns the per-ATS delay if configured, otherwise the global minDelay.
func (s *Scheduler) minDelayFor(ats string) time.Duration {
	if d, ok := s.atsDelays[ats]; ok {
		return d
	}
	return s.minDelay
}

// groupByATS returns pollers grouped by ATS name. Order within each group preserves config order.
func (s *Scheduler) groupByATS() map[string][]*poller.CompanyPoller {
	groups := make(map[string][]*poller.CompanyPoller)
	for _, p := range s.pollers {
		groups[p.ATS] = append(groups[p.ATS], p)
	}
	return groups
}

// Run starts one goroutine per ATS group. Each goroutine runs its own loop
// until ctx is cancelled. Returns nil on graceful shutdown.
func (s *Scheduler) Run(ctx context.Context) error {
	groups := s.groupByATS()

	s.logger.Info("starting scheduler",
		"interval", s.interval.String(),
		"min_delay", s.minDelay.String(),
		"ats_overrides", len(s.atsDelays),
		"companies", len(s.pollers),
		"ats_groups", len(groups),
	)

	var wg sync.WaitGroup
	for ats, pollers := range groups {
		ats, pollers := ats, pollers
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.runATSLoop(ctx, ats, pollers)
		}()
	}

	wg.Wait()
	s.logger.Info("scheduler stopped")
	return nil
}

// runATSLoop runs the poll loop for one ATS group: poll each company sequentially
// with minDelay between them, then sleep interval before the next full pass.
func (s *Scheduler) runATSLoop(ctx context.Context, ats string, pollers []*poller.CompanyPoller) {
	for {
		for i, p := range pollers {
			if ctx.Err() != nil {
				return
			}
			if err := p.Poll(ctx); err != nil {
				s.logger.Error("poll failed",
					"company", p.Name,
					"ats", ats,
					"error", err,
				)
			}
			// Sleep min_delay between same-ATS companies not after the last
			if i < len(pollers)-1 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(s.minDelayFor(ats)):
				}
			}
		}
		// Sleep polling_interval before next full pass
		select {
		case <-ctx.Done():
			return
		case <-time.After(s.interval):
		}
	}
}

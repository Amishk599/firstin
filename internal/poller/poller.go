package poller

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/amishk599/firstin/internal/model"
)

// CompanyPoller owns the full poll pipeline for a single company:
// fetch → filter → dedup → notify → mark seen.
type CompanyPoller struct {
	Name     string
	fetcher  model.JobFetcher
	filter   model.JobFilter
	store    model.JobStore
	notifier model.Notifier
	logger   *slog.Logger
}

// NewCompanyPoller creates a poller wired with all its dependencies.
func NewCompanyPoller(
	name string,
	fetcher model.JobFetcher,
	filter model.JobFilter,
	store model.JobStore,
	notifier model.Notifier,
	logger *slog.Logger,
) *CompanyPoller {
	return &CompanyPoller{
		Name:     name,
		fetcher:  fetcher,
		filter:   filter,
		store:    store,
		notifier: notifier,
		logger:   logger,
	}
}

// Poll runs one poll cycle: fetch → filter → freshness → dedup → notify → mark seen.
// On the very first run (empty store), jobs are seeded as seen without notifying.
func (p *CompanyPoller) Poll(ctx context.Context) error {
	firstRun, err := p.store.IsEmpty()
	if err != nil {
		return fmt.Errorf("polling %s: checking if first run: %w", p.Name, err)
	}

	jobs, err := p.fetcher.FetchJobs(ctx)
	if err != nil {
		return fmt.Errorf("polling %s: %w", p.Name, err)
	}

	p.logger.Debug("fetched jobs from API",
		"company", p.Name,
		"total", len(jobs),
	)

	now := time.Now()

	var matched []model.Job
	var filteredOut, staleOut int
	for _, job := range jobs {
		if !p.filter.Match(job) {
			filteredOut++
			continue
		}
		// Freshness check: skip jobs posted more than 1 hour ago.
		// Skip on first run — we need to seed all matching jobs so future
		// polls can detect new ones by comparison.
		if !firstRun && job.PostedAt != nil && job.PostedAt.Before(now.Add(-1*time.Hour)) {
			staleOut++
			continue
		}
		matched = append(matched, job)
	}

	p.logger.Debug("filter pipeline results",
		"company", p.Name,
		"fetched", len(jobs),
		"filtered_out", filteredOut,
		"stale_out", staleOut,
		"matched", len(matched),
	)

	var newJobs []model.Job
	for _, job := range matched {
		seen, err := p.store.HasSeen(job.ID)
		if err != nil {
			return fmt.Errorf("polling %s: checking seen status: %w", p.Name, err)
		}
		if !seen {
			newJobs = append(newJobs, job)
		}
	}

	// First-run suppression: seed the store without notifying.
	if firstRun {
		for _, job := range newJobs {
			if err := p.store.MarkSeen(job.ID); err != nil {
				return fmt.Errorf("polling %s: seeding seen: %w", p.Name, err)
			}
		}
		p.logger.Info("initial seed: marked existing jobs as seen",
			"company", p.Name,
			"seeded", len(newJobs),
		)
		return nil
	}

	if len(newJobs) > 0 {
		if err := p.notifier.Notify(newJobs); err != nil {
			return fmt.Errorf("polling %s: notifying: %w", p.Name, err)
		}
	}

	for _, job := range newJobs {
		if err := p.store.MarkSeen(job.ID); err != nil {
			return fmt.Errorf("polling %s: marking seen: %w", p.Name, err)
		}
	}

	p.logger.Info("polled company",
		"company", p.Name,
		"fetched", len(jobs),
		"matched", len(matched),
		"new", len(newJobs),
	)

	return nil
}

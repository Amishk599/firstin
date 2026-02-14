package poller

import (
	"context"
	"fmt"
	"log/slog"

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

// Poll runs one poll cycle: fetch jobs, filter, dedup, notify, and mark seen.
func (p *CompanyPoller) Poll(ctx context.Context) error {
	jobs, err := p.fetcher.FetchJobs(ctx)
	if err != nil {
		return fmt.Errorf("polling %s: %w", p.Name, err)
	}

	var matched []model.Job
	for _, job := range jobs {
		if p.filter.Match(job) {
			matched = append(matched, job)
		}
	}

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

package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/amishk599/firstin/internal/adapter"
	"github.com/amishk599/firstin/internal/config"
	"github.com/amishk599/firstin/internal/filter"
	"github.com/amishk599/firstin/internal/model"
	"github.com/amishk599/firstin/internal/notifier"
	"github.com/amishk599/firstin/internal/poller"
	"github.com/amishk599/firstin/internal/scheduler"
	"github.com/amishk599/firstin/internal/store"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	debug := flag.Bool("debug", false, "enable debug logging")
	dryRun := flag.Bool("dry-run", false, "poll once, print matches, do not mark as seen, then exit")
	flag.Parse()

	logLevel := slog.LevelInfo
	if *debug {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger.Info("config loaded",
		"interval", cfg.PollingInterval.String(),
		"companies", len(cfg.Companies),
		"title_keywords", cfg.Filters.TitleKeywords,
		"locations", cfg.Filters.Locations,
	)

	// In dry-run mode, use a NopStore so nothing is persisted.
	var jobStore model.JobStore
	if *dryRun {
		logger.Info("dry-run mode enabled, no jobs will be marked as seen")
		jobStore = store.NewNopStore()
	} else {
		sqlStore, err := store.NewSQLiteStore("jobs.db")
		if err != nil {
			logger.Error("failed to open store", "error", err)
			os.Exit(1)
		}
		defer sqlStore.Close()
		jobStore = sqlStore
	}

	logNotifier := notifier.NewLogNotifier(logger)

	jobFilter := filter.NewTitleAndLocationFilter(
		cfg.Filters.TitleKeywords,
		cfg.Filters.Locations,
	)

	httpClient := &http.Client{Timeout: 30 * time.Second}

	var pollers []*poller.CompanyPoller
	for _, company := range cfg.Companies {
		if !company.Enabled {
			continue
		}

		switch company.ATS {
		case "greenhouse":
			fetcher := adapter.NewGreenhouseAdapter(company.BoardToken, company.Name, httpClient)
			p := poller.NewCompanyPoller(company.Name, fetcher, jobFilter, jobStore, logNotifier, logger)
			pollers = append(pollers, p)
			logger.Info("registered company", "name", company.Name, "ats", company.ATS)
		default:
			logger.Warn("unsupported ATS, skipping", "company", company.Name, "ats", company.ATS)
		}
	}

	if len(pollers) == 0 {
		logger.Error("no companies to poll")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// In dry-run mode, poll each company once and exit.
	if *dryRun {
		for _, p := range pollers {
			if err := p.Poll(ctx); err != nil {
				logger.Error("poll failed", "company", p.Name, "error", err)
			}
		}
		logger.Info("dry-run complete")
		return
	}

	sched := scheduler.NewScheduler(pollers, cfg.PollingInterval, logger)
	if err := sched.Run(ctx); err != nil {
		logger.Error("scheduler error", "error", err)
		os.Exit(1)
	}

	logger.Info("goodbye")
}

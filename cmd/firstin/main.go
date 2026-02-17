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
	"github.com/amishk599/firstin/internal/ratelimit"
	"github.com/amishk599/firstin/internal/scheduler"
	"github.com/amishk599/firstin/internal/store"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	debug := flag.Bool("debug", false, "enable debug logging")
	dryRun := flag.Bool("dry-run", false, "poll once, print matches, do not mark as seen, then exit")
	testSlack := flag.Bool("test-slack", false, "send a test message to Slack and exit")
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
		"max_age", cfg.Filters.MaxAge.String(),
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

	jobFilter := filter.NewTitleAndLocationFilter(
		cfg.Filters.TitleKeywords,
		cfg.Filters.Locations,
	)

	httpClient := &http.Client{Timeout: 30 * time.Second}

	var n model.Notifier
	switch cfg.Notification.Type {
	case "slack":
		n = notifier.NewSlackNotifier(cfg.Notification.WebhookURL, httpClient, logger)
		logger.Info("using slack notifier")
	default:
		n = notifier.NewLogNotifier(logger)
	}

	if *testSlack {
		if cfg.Notification.Type != "slack" {
			logger.Error("--test-slack requires notification.type to be \"slack\" in config")
			os.Exit(1)
		}
		if err := notifier.SendTestMessage(n); err != nil {
			logger.Error("test slack message failed", "error", err)
			os.Exit(1)
		}
		logger.Info("test slack message sent successfully")
		return
	}

	// Shared ATS-level rate limiter - all companies on the same ATS share this instance.
	limiter := ratelimit.NewATSRateLimiter(cfg.RateLimit.MinDelay)
	logger.Info("rate limiter configured", "min_delay", cfg.RateLimit.MinDelay.String())

	var pollers []*poller.CompanyPoller
	for _, company := range cfg.Companies {
		if !company.Enabled {
			continue
		}

		var fetcher model.JobFetcher
		switch company.ATS {
		case "greenhouse":
			fetcher = adapter.NewGreenhouseAdapter(company.BoardToken, company.Name, httpClient)
		case "ashby":
			fetcher = adapter.NewAshbyAdapter(company.BoardToken, company.Name, httpClient)
		default:
			logger.Warn("unsupported ATS, skipping", "company", company.Name, "ats", company.ATS)
			continue
		}

		// Wrap with ATS-level rate limiting
		fetcher = ratelimit.NewRateLimitedFetcher(fetcher, limiter, company.ATS)

		p := poller.NewCompanyPoller(company.Name, fetcher, jobFilter, jobStore, n, cfg.Filters.MaxAge, logger)
		pollers = append(pollers, p)
		logger.Info("registered company", "name", company.Name, "ats", company.ATS)
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

package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/amishk599/firstin/internal/adapter"
	"github.com/amishk599/firstin/internal/audit"
	"github.com/amishk599/firstin/internal/config"
	"github.com/amishk599/firstin/internal/filter"
	"github.com/amishk599/firstin/internal/model"
	"github.com/amishk599/firstin/internal/notifier"
	"github.com/amishk599/firstin/internal/poller"
	"github.com/amishk599/firstin/internal/retry"
	"github.com/amishk599/firstin/internal/scheduler"
	"github.com/amishk599/firstin/internal/store"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	debug := flag.Bool("debug", false, "enable debug logging")
	dryRun := flag.Bool("dry-run", false, "poll once, print matches, do not mark as seen, then exit")
	testSlack := flag.Bool("test-slack", false, "send a test message to Slack and exit")
	auditMode := flag.Bool("audit", false, "interactive filter audit: pick a company, see all jobs vs filtered")
	flag.Parse()

	logger := setupLogger(*debug)

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	if *auditMode {
		runAudit(cfg, &http.Client{Timeout: 30 * time.Second}, logger)
		return
	}

	logger.Info("config loaded",
		"interval", cfg.PollingInterval.String(),
		"companies", len(cfg.Companies),
		"title_keywords", len(cfg.Filters.TitleKeywords),
		"locations", len(cfg.Filters.Locations),
		"max_age", cfg.Filters.MaxAge.String(),
	)

	jobStore, cleanup := setupStore(*dryRun, logger)
	defer cleanup()

	httpClient := &http.Client{Timeout: 30 * time.Second}
	jobFilter := filter.NewTitleAndLocationFilter(
		cfg.Filters.TitleKeywords,
		cfg.Filters.TitleExcludeKeywords,
		cfg.Filters.Locations,
		cfg.Filters.ExcludeLocations,
	)
	n := setupNotifier(cfg, httpClient, logger)

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

	pollers := buildPollers(cfg, jobFilter, jobStore, n, httpClient, logger)
	if len(pollers) == 0 {
		logger.Error("no companies to poll")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if *dryRun {
		runDryRun(ctx, pollers, logger)
		return
	}

	sched := scheduler.NewScheduler(pollers, cfg.PollingInterval, cfg.RateLimit.MinDelay, logger)
	if err := sched.Run(ctx); err != nil {
		logger.Error("scheduler error", "error", err)
		os.Exit(1)
	}

	logger.Info("goodbye")
}

func setupLogger(debug bool) *slog.Logger {
	logLevel := slog.LevelInfo
	if debug {
		logLevel = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
}

func setupStore(dryRun bool, logger *slog.Logger) (model.JobStore, func()) {
	if dryRun {
		logger.Info("dry-run mode enabled, no jobs will be marked as seen")
		return store.NewNopStore(), func() {}
	}

	sqlStore, err := store.NewSQLiteStore("jobs.db")
	if err != nil {
		logger.Error("failed to open store", "error", err)
		os.Exit(1)
	}
	return sqlStore, func() { sqlStore.Close() }
}

func setupNotifier(cfg *config.Config, httpClient *http.Client, logger *slog.Logger) model.Notifier {
	switch cfg.Notification.Type {
	case "slack":
		logger.Info("using slack notifier")
		return notifier.NewSlackNotifier(cfg.Notification.WebhookURL, httpClient, logger)
	default:
		return notifier.NewLogNotifier(logger)
	}
}

func createFetcher(company config.CompanyConfig, httpClient *http.Client, logger *slog.Logger) (model.JobFetcher, bool) {
	switch company.ATS {
	case "greenhouse":
		return adapter.NewGreenhouseAdapter(company.BoardToken, company.Name, httpClient), true
	case "ashby":
		return adapter.NewAshbyAdapter(company.BoardToken, company.Name, httpClient), true
	case "lever":
		return adapter.NewLeverAdapter(company.BoardToken, company.Name, httpClient), true
	default:
		logger.Warn("unsupported ATS, skipping", "company", company.Name, "ats", company.ATS)
		return nil, false
	}
}

func buildPollers(cfg *config.Config, jobFilter model.JobFilter, jobStore model.JobStore, n model.Notifier, httpClient *http.Client, logger *slog.Logger) []*poller.CompanyPoller {
	logger.Info("scheduler min_delay", "min_delay", cfg.RateLimit.MinDelay.String())

	var pollers []*poller.CompanyPoller
	for _, company := range cfg.Companies {
		if !company.Enabled {
			continue
		}

		fetcher, ok := createFetcher(company, httpClient, logger)
		if !ok {
			continue
		}

		fetcher = retry.NewRetryFetcher(fetcher, 2, 5*time.Second, logger)
		p := poller.NewCompanyPoller(company.Name, company.ATS, fetcher, jobFilter, jobStore, n, cfg.Filters.MaxAge, logger)
		pollers = append(pollers, p)
		logger.Info("registered company", "name", company.Name, "ats", company.ATS)
	}
	return pollers
}

func runDryRun(ctx context.Context, pollers []*poller.CompanyPoller, logger *slog.Logger) {
	// Poll only one company per ATS
	seen := make(map[string]bool)
	for _, p := range pollers {
		if seen[p.ATS] {
			logger.Info("skipping (ATS already tested)", "company", p.Name, "ats", p.ATS)
			continue
		}
		seen[p.ATS] = true
		if err := p.Poll(ctx); err != nil {
			logger.Error("poll failed", "company", p.Name, "error", err)
		}
	}
	logger.Info("dry-run complete")
}

func runAudit(cfg *config.Config, httpClient *http.Client, logger *slog.Logger) {
	var enabled []config.CompanyConfig
	for _, c := range cfg.Companies {
		if c.Enabled {
			enabled = append(enabled, c)
		}
	}
	if len(enabled) == 0 {
		fmt.Println("No enabled companies in config.")
		return
	}

	choice, err := audit.RunCompanyPicker(enabled)
	if err != nil {
		fmt.Printf("Picker error: %v\n", err)
		return
	}
	if choice < 0 {
		return
	}
	company := enabled[choice]

	fetcher, ok := createFetcher(company, httpClient, logger)
	if !ok {
		fmt.Printf("Unsupported ATS: %s\n", company.ATS)
		return
	}

	fmt.Printf("\nFetching jobs from %s...\n", company.Name)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	jobs, err := fetcher.FetchJobs(ctx)
	if err != nil {
		fmt.Printf("Error fetching jobs: %v\n", err)
		return
	}
	fmt.Printf("Fetched %d jobs.\n", len(jobs))

	jobFilter := filter.NewTitleAndLocationFilter(
		cfg.Filters.TitleKeywords,
		cfg.Filters.TitleExcludeKeywords,
		cfg.Filters.Locations,
		cfg.Filters.ExcludeLocations,
	)
	var matched []model.Job
	for _, j := range jobs {
		if jobFilter.Match(j) {
			matched = append(matched, j)
		}
	}

	if err := audit.RunAuditTUI(jobs, matched, cfg.Filters); err != nil {
		fmt.Printf("TUI error: %v\n", err)
	}
}

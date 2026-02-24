package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/amishk599/firstin/internal/filter"
	"github.com/amishk599/firstin/internal/scheduler"
	"github.com/amishk599/firstin/internal/store"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the polling daemon",
	Long:  "Start the scheduler daemon; blocks until SIGINT/SIGTERM.",
	RunE:  runStart,
}

func init() {
	rootCmd.AddCommand(startCmd)
}

func runStart(cmd *cobra.Command, args []string) error {
	logger := setupLogger(debug)

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger.Info("config loaded",
		"interval", cfg.PollingInterval.String(),
		"companies", len(cfg.Companies),
		"title_keywords", len(cfg.Filters.TitleKeywords),
		"locations", len(cfg.Filters.Locations),
		"max_age", cfg.Filters.MaxAge.String(),
	)

	sqlStore, err := store.NewSQLiteStore("jobs.db")
	if err != nil {
		logger.Error("failed to open store", "error", err)
		os.Exit(1)
	}
	defer sqlStore.Close()

	httpClient := &http.Client{Timeout: 30 * time.Second}
	jobFilter := filter.NewTitleAndLocationFilter(
		cfg.Filters.TitleKeywords,
		cfg.Filters.TitleExcludeKeywords,
		cfg.Filters.Locations,
		cfg.Filters.ExcludeLocations,
	)
	n := setupNotifier(cfg, httpClient, logger)
	analyzer := setupAnalyzer(cfg, logger)

	pollers := buildPollers(cfg, jobFilter, sqlStore, n, analyzer, httpClient, logger)
	if len(pollers) == 0 {
		logger.Error("no companies to poll")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	sched := scheduler.NewScheduler(pollers, cfg.PollingInterval, cfg.RateLimit.MinDelay, cfg.RateLimit.ATSOverrides, logger)
	if err := sched.Run(ctx); err != nil {
		logger.Error("scheduler error", "error", err)
		os.Exit(1)
	}

	logger.Info("goodbye")
	return nil
}

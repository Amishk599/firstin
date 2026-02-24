package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/amishk599/firstin/internal/filter"
	"github.com/amishk599/firstin/internal/store"
	"github.com/spf13/cobra"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Poll once, print matches, exit",
	Long:  "One-shot poll: fetches one company per ATS, prints matched jobs, exits. Does not write to the store.",
	RunE:  runCheck,
}

func init() {
	rootCmd.AddCommand(checkCmd)
}

func runCheck(cmd *cobra.Command, args []string) error {
	logger := setupLogger(debug)

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger.Info("check mode: no jobs will be marked as seen")

	httpClient := &http.Client{Timeout: 30 * time.Second}
	jobFilter := filter.NewTitleAndLocationFilter(
		cfg.Filters.TitleKeywords,
		cfg.Filters.TitleExcludeKeywords,
		cfg.Filters.Locations,
		cfg.Filters.ExcludeLocations,
	)
	n := setupNotifier(cfg, httpClient, logger)
	analyzer := setupAnalyzer(cfg, logger)
	nopStore := store.NewNopStore()

	pollers := buildPollers(cfg, jobFilter, nopStore, n, analyzer, httpClient, logger)
	if len(pollers) == 0 {
		logger.Error("no companies to poll")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Poll only one company per ATS type
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

	logger.Info("check complete")
	return nil
}

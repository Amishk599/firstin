package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/amishk599/firstin/internal/adapter"
	"github.com/amishk599/firstin/internal/audit"
	"github.com/amishk599/firstin/internal/config"
	"github.com/amishk599/firstin/internal/filter"
	"github.com/amishk599/firstin/internal/model"
	"github.com/amishk599/firstin/internal/poller"
	"github.com/spf13/cobra"
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Browse jobs interactively (TUI)",
	Long:  "Shows the company picker TUI, then launches the split-pane audit view.",
	RunE:  runAuditCmd,
}

func init() {
	rootCmd.AddCommand(auditCmd)
}

func runAuditCmd(cmd *cobra.Command, args []string) error {
	logger := setupLogger(debug)

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	// Use a discard logger for setupAnalyzer — audit mode runs a TUI and any
	// log output before the alt-screen starts corrupts the display.
	silentLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	analyzer := setupAnalyzer(cfg, silentLogger)
	runAudit(cfg, httpClient, analyzer, logger)
	return nil
}

func runAudit(cfg *config.Config, httpClient *http.Client, analyzer poller.JobAnalyzer, logger *slog.Logger) {
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

	for {
		choice, err := audit.RunCompanyPicker(enabled)
		if err != nil {
			fmt.Printf("Picker error: %v\n", err)
			return
		}
		if choice < 0 {
			return
		}
		company := enabled[choice]

		fetcher, ok := createFetcher(company, httpClient, nil, logger)
		if !ok {
			fmt.Printf("Unsupported ATS: %s\n", company.ATS)
			continue
		}
		// In audit mode, adapters that support it should return all listings
		// (not just fresh ones) so the full job board is visible.
		if wa, ok := fetcher.(*adapter.WorkdayAdapter); ok {
			wa.SetAuditMode(true)
		}
		if ma, ok := fetcher.(*adapter.MicrosoftAdapter); ok {
			ma.SetAuditMode(true)
		}

		jobs, err := audit.RunLoader(company.Name, fetcher.FetchJobs)
		if err != nil {
			fmt.Printf("Error fetching jobs: %v\n", err)
			continue
		}

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

		var detailFetcher model.JobDetailFetcher
		if df, ok := fetcher.(model.JobDetailFetcher); ok {
			detailFetcher = df
		}

		wantQuit, err := audit.RunAuditTUI(jobs, matched, cfg.Filters, detailFetcher, analyzer)
		if err != nil {
			fmt.Printf("TUI error: %v\n", err)
		}
		if wantQuit {
			return
		}
		// else: loop → back to picker
	}
}

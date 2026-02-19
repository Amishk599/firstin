package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/amishk599/firstin/internal/adapter"
	"github.com/amishk599/firstin/internal/config"
	"github.com/amishk599/firstin/internal/model"
	"github.com/amishk599/firstin/internal/notifier"
	"github.com/amishk599/firstin/internal/poller"
	"github.com/amishk599/firstin/internal/retry"
	"github.com/spf13/cobra"
)

var (
	cfgPath string
	debug   bool
)

var rootCmd = &cobra.Command{
	Use:   "firstin",
	Short: "Job radar — be first in the door",
	Long:  "FirstIn polls company career pages and alerts you to new engineering roles.",
	// Default to `start` so that `firstin` with no args runs the daemon.
	// This preserves compatibility with systemd unit files that invoke the binary directly.
	RunE: runStart,
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgPath, "config", "c", "", "path to config file (default: FIRSTIN_CONFIG env var or ./config.yaml)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
}

// loadConfig resolves the config path and parses it.
// Priority: explicit path arg > FIRSTIN_CONFIG env var > "./config.yaml"
func loadConfig(path string) (*config.Config, error) {
	if path == "" {
		if env := os.Getenv("FIRSTIN_CONFIG"); env != "" {
			path = env
		} else {
			path = "config.yaml"
		}
	}
	return config.Load(path)
}

func setupLogger(dbg bool) *slog.Logger {
	logLevel := slog.LevelInfo
	if dbg {
		logLevel = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
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

func createFetcher(company config.CompanyConfig, httpClient *http.Client, jobFilter model.JobFilter, logger *slog.Logger) (model.JobFetcher, bool) {
	switch company.ATS {
	case "greenhouse":
		return adapter.NewGreenhouseAdapter(company.BoardToken, company.Name, httpClient), true
	case "ashby":
		return adapter.NewAshbyAdapter(company.BoardToken, company.Name, httpClient), true
	case "lever":
		return adapter.NewLeverAdapter(company.BoardToken, company.Name, httpClient), true
	case "workday":
		return adapter.NewWorkdayAdapter(company.WorkdayURL, company.Name, httpClient, jobFilter), true
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

		fetcher, ok := createFetcher(company, httpClient, jobFilter, logger)
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

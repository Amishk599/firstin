package main

import (
	"net/http"
	"os"
	"time"

	"github.com/amishk599/firstin/internal/notifier"
	"github.com/spf13/cobra"
)

var notifyCmd = &cobra.Command{
	Use:   "notify",
	Short: "Notification subcommands",
}

var notifyTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Send a test notification",
	Long:  "Sends a test notification using the configured notifier.",
	RunE:  runNotifyTest,
}

func init() {
	rootCmd.AddCommand(notifyCmd)
	notifyCmd.AddCommand(notifyTestCmd)
}

func runNotifyTest(cmd *cobra.Command, args []string) error {
	logger := setupLogger(debug)

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	n := setupNotifier(cfg, httpClient, logger)

	if err := notifier.SendTestMessage(n); err != nil {
		logger.Error("test notification failed", "error", err)
		os.Exit(1)
	}
	logger.Info("test notification sent successfully")
	return nil
}

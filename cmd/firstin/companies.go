package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var companiesCmd = &cobra.Command{
	Use:   "companies",
	Short: "List all configured companies",
	Long:  "Reads the config and prints a table of all configured companies.",
	RunE:  runCompanies,
}

func init() {
	rootCmd.AddCommand(companiesCmd)
}

func runCompanies(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%-25s %-15s %s\n", "Company", "ATS", "Status")
	fmt.Println(strings.Repeat("─", 47))

	enabled, disabled := 0, 0
	for _, c := range cfg.Companies {
		status := "enabled"
		if !c.Enabled {
			status = "disabled"
			disabled++
		} else {
			enabled++
		}
		fmt.Printf("%-25s %-15s %s\n", c.Name, c.ATS, status)
	}

	fmt.Printf("\nTotal: %d companies (%d enabled, %d disabled)\n", len(cfg.Companies), enabled, disabled)
	return nil
}

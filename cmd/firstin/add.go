package main

import (
	"errors"
	"fmt"

	"github.com/amishk599/firstin/internal/audit"
	"github.com/amishk599/firstin/internal/config"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new company to config",
	Long:  "Interactively enter company details and append them to config.yaml.",
	RunE:  runAddCmd,
}

func init() {
	rootCmd.AddCommand(addCmd)
}

func runAddCmd(cmd *cobra.Command, args []string) error {
	company, err := audit.RunNewCompanyForm()
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil
		}
		return err
	}

	path := resolveConfigPath(cfgPath)
	if err := config.AppendCompany(path, company); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("Added %s (%s) to %s\n", company.Name, company.ATS, path)
	return nil
}

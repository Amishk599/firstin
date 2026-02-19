package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

const version = "0.1.0"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version info",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("firstin %s\n", version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

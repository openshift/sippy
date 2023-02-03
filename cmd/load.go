package cmd

import (
	"github.com/spf13/cobra"
)

var loadCmd = &cobra.Command{
	Use:   "load",
	Short: "Load data from various import sources",
}

func init() {
	rootCmd.AddCommand(loadCmd)
}

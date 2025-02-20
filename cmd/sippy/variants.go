package main

import (
	"github.com/spf13/cobra"
)

func NewVariantsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "variants",
		Short: "Subcommands available for working with job variants",
	}

	cmd.AddCommand(NewVariantSnapshotCommand())
	cmd.AddCommand(NewVariantsGenerateCommand())
	return cmd
}

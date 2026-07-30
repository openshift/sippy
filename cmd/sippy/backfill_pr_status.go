package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/openshift/sippy/pkg/dataloader/prmergesyncloader"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/github"
	"github.com/openshift/sippy/pkg/flags"
)

type BackfillPRStatusFlags struct {
	DBFlags   *flags.PostgresFlags
	BatchSize int
	Pause     int
	Limit     int
}

func NewBackfillPRStatusFlags() *BackfillPRStatusFlags {
	return &BackfillPRStatusFlags{
		DBFlags:   flags.NewPostgresDatabaseFlags(),
		BatchSize: 50,
		Pause:     60,
	}
}

func (f *BackfillPRStatusFlags) BindFlags(fs *pflag.FlagSet) {
	f.DBFlags.BindFlags(fs)
	fs.IntVar(&f.BatchSize, "batch-size", f.BatchSize, "Number of PRs to look up per batch")
	fs.IntVar(&f.Pause, "pause", f.Pause, "Seconds to pause between batches")
	fs.IntVar(&f.Limit, "limit", f.Limit, "Maximum total PRs to process (0 = no limit)")
}

func NewBackfillPRStatusCommand() *cobra.Command {
	f := NewBackfillPRStatusFlags()

	cmd := &cobra.Command{
		Use:   "backfill-pr-status",
		Short: "Backfill merged_at on prow_pull_requests by querying GitHub for each PR",
		Long: `Query all prow_pull_requests rows that have no merged_at set (and no sibling
row with merged_at set), look up each PR on GitHub, and set merged_at for PRs
that have merged. Processes in batches with pauses to stay within rate limits.

Naturally resumable: each run re-queries what is still unset, so previously
resolved PRs are skipped.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBackfillPRStatus(f)
		},
	}

	f.BindFlags(cmd.Flags())
	return cmd
}

func runBackfillPRStatus(f *BackfillPRStatusFlags) error {
	ctx := context.Background()

	dbc, err := f.DBFlags.GetDBClient()
	if err != nil {
		return fmt.Errorf("getting db client: %w", err)
	}

	ghClient := github.New(ctx, github.OpenshiftOrg)
	loader := prmergesyncloader.New(ctx, dbc, ghClient)

	return loader.Backfill(f.BatchSize, f.Pause, f.Limit)
}

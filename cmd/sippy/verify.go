package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/civil"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/db/verify"
	"github.com/openshift/sippy/pkg/flags"
	"github.com/openshift/sippy/pkg/flags/configflags"
	"github.com/openshift/sippy/pkg/variantregistry"
)

type VerifyFlags struct {
	Date             string
	Checks           []string
	Release          string
	DBFlags          *flags.PostgresFlags
	BigQueryFlags    *flags.BigQueryFlags
	GoogleCloudFlags *flags.GoogleCloudFlags
	ConfigFlags      *configflags.ConfigFlags
}

func NewVerifyFlags(now time.Time) *VerifyFlags {
	return &VerifyFlags{
		Date:             civil.DateOf(now.UTC()).AddDays(-2).String(),
		DBFlags:          flags.NewPostgresDatabaseFlags(),
		BigQueryFlags:    flags.NewBigQueryFlags(),
		GoogleCloudFlags: flags.NewGoogleCloudFlags(),
		ConfigFlags:      configflags.NewConfigFlags(),
	}
}

func (f *VerifyFlags) BindFlags(fs *pflag.FlagSet) {
	f.DBFlags.BindFlags(fs)
	f.BigQueryFlags.BindFlags(fs)
	f.GoogleCloudFlags.BindFlags(fs)
	f.ConfigFlags.BindFlags(fs)
	fs.StringVar(&f.Date, "date", f.Date, "UTC calendar date to verify (YYYY-MM-DD; defaults to the day before yesterday)")
	fs.StringArrayVar(&f.Checks, "check", nil, "Check to run; repeat for multiple checks (bq-completeness, daily-totals, cumulative-summaries; defaults to all)")
	fs.StringVar(&f.Release, "release", "", "Verify only this release (defaults to every configured and discovered release)")
}

type verifyCommandDependencies struct {
	now time.Time
	run func(context.Context, *VerifyFlags, civil.Date, []verify.Check) (verify.Result, error)
}

func NewVerifyCommand() *cobra.Command {
	return newVerifyCommandWithDependencies(verifyCommandDependencies{now: time.Now(), run: runVerify})
}

func newVerifyCommandWithDependencies(dependencies verifyCommandDependencies) *cobra.Command {
	f := NewVerifyFlags(dependencies.now)
	cmd := &cobra.Command{
		Use:          "verify",
		Short:        "Verify daily data integrity without modifying storage",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			date, err := civil.ParseDate(f.Date)
			if err != nil {
				return fmt.Errorf("invalid --date %q: expected YYYY-MM-DD: %w", f.Date, err)
			}
			checks, err := verify.ParseChecks(f.Checks)
			if err != nil {
				return err
			}
			result, runErr := dependencies.run(cmd.Context(), f, date, checks)
			if runErr != nil && len(result.Summaries) == 0 {
				for _, check := range checks {
					result.Summaries = append(result.Summaries, verify.Summary{
						Check: check, Release: f.Release, Date: date, Passed: false, Error: runErr.Error(),
					})
				}
			}
			result.Sort()
			result.Log(log.StandardLogger())
			if runErr != nil {
				return runErr
			}
			if !result.Passed() {
				return fmt.Errorf("one or more verification checks failed")
			}
			return nil
		},
	}
	f.BindFlags(cmd.Flags())
	return cmd
}

func runVerify(ctx context.Context, verifyFlags *VerifyFlags, date civil.Date, checks []verify.Check) (verify.Result, error) {
	dbc, err := verifyFlags.DBFlags.GetDBClient()
	if err != nil {
		return verify.Result{}, fmt.Errorf("getting PostgreSQL client: %w", err)
	}

	runner := verify.Runner{PostgreSQL: verify.NewPostgreSQL(dbc)}
	var bqClient *bqcachedclient.Client
	if verify.ContainsCheck(checks, verify.CheckBQCompleteness) {
		var initializationErrors []error
		config, configErr := verifyFlags.ConfigFlags.GetConfig()
		if configErr != nil {
			initializationErrors = append(initializationErrors, fmt.Errorf("loading Sippy config: %w", configErr))
		} else {
			runner.Config = config
			overrides, overrideErr := variantregistry.BuildSyntheticReleaseJobOverrides(config.Releases)
			if overrideErr != nil {
				initializationErrors = append(initializationErrors, fmt.Errorf("building synthetic release overrides: %w", overrideErr))
			} else {
				runner.SyntheticReleaseOverrides = overrides
			}
		}

		opCtx, queryCtx := bqcachedclient.OpCtxForCronEnv(ctx, "verify")
		bqClient, err = verifyFlags.BigQueryFlags.GetBigQueryClient(
			queryCtx, opCtx, nil, verifyFlags.GoogleCloudFlags.ServiceAccountCredentialFile,
		)
		if err != nil {
			initializationErrors = append(initializationErrors, fmt.Errorf("initializing BigQuery client: %w", err))
		} else {
			runner.BigQuery = verify.NewBigQuery(bqClient)
			defer func() {
				if closeErr := bqClient.BQ.Close(); closeErr != nil {
					log.WithError(closeErr).Warn("closing BigQuery client")
				}
			}()
		}
		runner.BigQueryInitializationError = errors.Join(initializationErrors...)
	}

	return runner.Run(ctx, verify.Options{Date: date, Checks: checks, Release: verifyFlags.Release}), nil
}

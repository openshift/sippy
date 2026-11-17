package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/storage"
	"github.com/openshift/sippy/pkg/api"
	sippyv1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/dataloader/prowloader"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/bqjunit"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/pgwriter"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/flags"
	"github.com/openshift/sippy/pkg/flags/configflags"
	"github.com/openshift/sippy/pkg/variantregistry"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const historicalGCSConcurrency = 20

// historicalRawBatchWriter bounds memory and preserves one transaction per
// batch while GCS workers produce results independently.
type historicalRawBatchWriter struct {
	batch     []pgwriter.JobRunResult
	batchSize int
	write     func([]pgwriter.JobRunResult) error
	persisted int
}

// add flushes a full batch so persistence can keep pace with source reads.
func (w *historicalRawBatchWriter) add(result *pgwriter.JobRunResult) error {
	w.batch = append(w.batch, *result)
	if len(w.batch) < w.batchSize {
		return nil
	}
	return w.flush()
}

// flush persists the remaining results, including a final partial batch.
func (w *historicalRawBatchWriter) flush() error {
	if len(w.batch) == 0 {
		return nil
	}
	if err := w.write(w.batch); err != nil {
		return err
	}
	w.persisted += len(w.batch)
	w.batch = w.batch[:0]
	return nil
}

// historicalLoaderFlags groups the clients and configuration shared with the
// live loader while the backfill command selects an explicit historical range.
type historicalLoaderFlags struct {
	BigQueryFlags    *flags.BigQueryFlags
	GoogleCloudFlags *flags.GoogleCloudFlags
	PostgresFlags    *flags.PostgresFlags
	ConfigFlags      *configflags.ConfigFlags
	ModeFlags        *flags.ModeFlags
}

// historicalRunFlags holds only the backfill's range and source controls.
type historicalRunFlags struct {
	StartDate      string
	EndDate        string
	TestSource     string
	SkipPartitions bool
}

// prowBackfillFlags separates shared loader setup from historical options.
type prowBackfillFlags struct {
	Loader historicalLoaderFlags
	Run    historicalRunFlags
}

// newProwBackfillFlags defaults to GCS with BigQuery fallback when available.
func newProwBackfillFlags() *prowBackfillFlags {
	return &prowBackfillFlags{
		Loader: historicalLoaderFlags{
			BigQueryFlags:    flags.NewBigQueryFlags(),
			GoogleCloudFlags: flags.NewGoogleCloudFlags(),
			PostgresFlags:    flags.NewPostgresDatabaseFlags(),
			ConfigFlags:      configflags.NewConfigFlags(),
			ModeFlags:        flags.NewModeFlags(),
		},
		Run: historicalRunFlags{TestSource: "auto"},
	}
}

// bindCommon accepts the same connection and configuration flags as other
// Sippy commands while adding the historical range and partition controls.
func (f *prowBackfillFlags) bindCommon(fs *pflag.FlagSet) {
	f.Loader.BigQueryFlags.BindFlags(fs)
	f.Loader.GoogleCloudFlags.BindFlags(fs)
	f.Loader.PostgresFlags.BindFlags(fs)
	f.Loader.ConfigFlags.BindFlags(fs)
	f.Loader.ModeFlags.BindFlags(fs)
	fs.StringVar(&f.Run.StartDate, "start-date", "", "Inclusive UTC start date (YYYY-MM-DD)")
	fs.StringVar(&f.Run.EndDate, "end-date", "", "Inclusive UTC end date (YYYY-MM-DD)")
	fs.BoolVar(&f.Run.SkipPartitions, "skip-partition-validation", false, "Skip ensuring partitions exist for the requested range")
}

// NewProwBackfillCommand exposes one bounded raw-data command; summaries are
// rebuilt separately after the raw import finishes.
func NewProwBackfillCommand() *cobra.Command {
	f := newProwBackfillFlags()
	cmd := &cobra.Command{Use: "prow-backfill", Short: "Run a bounded historical Prow job backfill", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		start, end, err := f.parseRange()
		if err != nil {
			return err
		}
		if err := validateTestSource(f.Run.TestSource); err != nil {
			return err
		}
		return runHistoricalRaw(cmd.Context(), f, start, end)
	}}
	f.bindCommon(cmd.Flags())
	cmd.Flags().StringVar(&f.Run.TestSource, "test-source", f.Run.TestSource, "Test source: auto, gcs, or bigquery")
	return cmd
}

// parseRange turns inclusive UTC dates into a half-open interval for querying
// and partition creation.
func (f *prowBackfillFlags) parseRange() (time.Time, time.Time, error) {
	since, err := civil.ParseDate(f.Run.StartDate)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid --start-date: %w", err)
	}
	until, err := civil.ParseDate(f.Run.EndDate)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid --end-date: %w", err)
	}
	start := time.Date(since.Year, since.Month, since.Day, 0, 0, 0, 0, time.UTC)
	end := time.Date(until.Year, until.Month, until.Day, 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	if !start.Before(end) {
		return time.Time{}, time.Time{}, fmt.Errorf("--start-date must be before --end-date")
	}
	return start, end, nil
}

// historicalReleaseNames uses PostgreSQL definitions so historical loading
// follows the same release selection as the normal loader.
func historicalReleaseNames(ctx context.Context, dbc *db.DB) ([]string, error) {
	releaseConfigs, err := api.GetReleasesFromDB(ctx, dbc)
	if err != nil {
		return nil, fmt.Errorf("loading release definitions from PostgreSQL: %w", err)
	}
	return releaseNames(releaseConfigs)
}

// releaseNames requires configured PostgreSQL releases before any backfill
// work begins; an empty set would silently skip all candidates.
func releaseNames(releaseConfigs []sippyv1.Release) ([]string, error) {
	if len(releaseConfigs) == 0 {
		return nil, fmt.Errorf("no release definitions found in PostgreSQL; populate release_definitions before running prow-backfill")
	}
	releases := make([]string, 0, len(releaseConfigs))
	for _, release := range releaseConfigs {
		releases = append(releases, release.Release)
	}
	return releases, nil
}

// loader constructs the historical loader with the current release rules and
// returns cleanup for its source clients.
func (f *prowBackfillFlags) loader(ctx context.Context) (*prowloader.ProwLoader, func(), error) {
	if f.Loader.ConfigFlags.Path == "" {
		return nil, nil, fmt.Errorf("--config is required for historical Prow backfill")
	}
	if err := validateHistoricalDatasets(f.Loader.BigQueryFlags.BigQueryDataset, os.Getenv(prowloader.LabelsDatasetEnv)); err != nil {
		return nil, nil, err
	}
	dbc, err := f.Loader.PostgresFlags.GetDBClient()
	if err != nil {
		return nil, nil, fmt.Errorf("initializing PostgreSQL: %w", err)
	}
	config, err := f.Loader.ConfigFlags.GetConfig()
	if err != nil {
		return nil, nil, err
	}
	releases, err := historicalReleaseNames(ctx, dbc)
	if err != nil {
		return nil, nil, err
	}
	opCtx, queryCtx := bqcachedclient.OpCtxForCronEnv(ctx, "prow-backfill")
	bq, err := bqcachedclient.New(queryCtx, opCtx, nil, f.Loader.GoogleCloudFlags.ServiceAccountCredentialFile, f.Loader.BigQueryFlags.BigQueryProject, f.Loader.BigQueryFlags.BigQueryDataset, f.Loader.BigQueryFlags.ReleasesTable)
	if err != nil {
		return nil, nil, fmt.Errorf("initializing BigQuery: %w", err)
	}
	var gcsClient *storage.Client
	if shouldInitializeGCS(f.Run.TestSource, f.Loader.GoogleCloudFlags.ServiceAccountCredentialFile, f.Loader.GoogleCloudFlags.OAuthClientCredentialFile) {
		gcsClient, err = gcs.NewGCSClient(ctx, f.Loader.GoogleCloudFlags.ServiceAccountCredentialFile, f.Loader.GoogleCloudFlags.OAuthClientCredentialFile)
		if err != nil {
			return nil, nil, fmt.Errorf("initializing GCS: %w", err)
		}
	}
	overrides, err := variantregistry.BuildSyntheticReleaseJobOverrides(config.Releases)
	if err != nil {
		return nil, nil, err
	}
	loader := prowloader.New(ctx, dbc, gcsClient, bq, nil, f.Loader.ModeFlags.GetVariantManager(ctx, bq), f.Loader.ModeFlags.GetSyntheticTestManager(), releases, config, nil, nil, nil, overrides)
	loader.SetQuietHistoricalLogs(true)
	return loader, func() {
		if err := bq.BQ.Close(); err != nil {
			log.WithError(err).Warn("closing historical BigQuery client")
		}
		if gcsClient != nil {
			if err := gcsClient.Close(); err != nil {
				log.WithError(err).Warn("closing historical GCS client")
			}
		}
	}, nil
}

// validateHistoricalDatasets rejects unsafe dataset identifiers before the
// variant manager or label prefetch can interpolate them into BigQuery SQL.
func validateHistoricalDatasets(dataset, labelsDataset string) error {
	if err := bqcachedclient.ValidateDatasetIdentifier(dataset); err != nil {
		return fmt.Errorf("invalid --bigquery-dataset: %w", err)
	}
	if labelsDataset != "" {
		if err := bqcachedclient.ValidateDatasetIdentifier(labelsDataset); err != nil {
			return fmt.Errorf("invalid %s: %w", prowloader.LabelsDatasetEnv, err)
		}
	}
	return nil
}

// validateTestSource rejects unsupported source modes before opening clients.
func validateTestSource(source string) error {
	if source != "auto" && source != "gcs" && source != "bigquery" {
		return fmt.Errorf("invalid --test-source %q", source)
	}
	return nil
}

// shouldInitializeGCS avoids requiring GCS credentials for BigQuery-only runs.
func shouldInitializeGCS(source, serviceAccountCredentialFile, oauthCredentialFile string) bool {
	if source == "gcs" {
		return true
	}
	return source == "auto" && (serviceAccountCredentialFile != "" || oauthCredentialFile != "")
}

// shouldFallbackToBigQuery treats any existing GCS build prefix as authoritative,
// even when that build has no JUnit XML files. An uncertain prefix state from
// a GCS error must stop the backfill rather than cause a permanent BQ import.
func shouldFallbackToBigQuery(source string, gcsBuildPrefixExists bool, lookupErr error) bool {
	return source == "auto" && lookupErr == nil && !gcsBuildPrefixExists
}

// runHistoricalRaw processes days newest first and leaves summary rebuilding
// to the existing summary commands.
func runHistoricalRaw(ctx context.Context, f *prowBackfillFlags, start, end time.Time) error {
	loader, cleanup, err := f.loader(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	if !f.Run.SkipPartitions {
		if err := loader.EnsureHistoricalPartitions(start, end); err != nil {
			return err
		}
	}
	junitReader, err := bqjunit.New(loader.BigQueryClient())
	if err != nil {
		return err
	}
	if f.Run.TestSource != "gcs" {
		if err := junitReader.InspectSchema(ctx); err != nil {
			return fmt.Errorf("inspecting BigQuery schema: %w", err)
		}
		log.WithFields(log.Fields{"start": start, "end": end, "testSource": f.Run.TestSource}).Info("historical test source schema inspected")
	}
	for dayEnd := end; dayEnd.After(start); {
		dayStart := dayEnd.AddDate(0, 0, -1)
		if dayStart.Before(start) {
			dayStart = start
		}
		candidates, err := loader.HistoricalJobCandidates(ctx, dayStart, dayEnd, true)
		if err != nil {
			return err
		}
		if len(candidates) > 0 {
			log.WithFields(log.Fields{"start": dayStart, "end": dayEnd, "jobs": len(candidates)}).Info("loading historical test data for day")
			results, err := loadHistoricalRawResults(ctx, loader, f, junitReader, candidates)
			if err != nil {
				return err
			}
			log.WithFields(log.Fields{"runs": results, "start": dayStart, "end": dayEnd, "source": f.Run.TestSource}).Info("historical raw backfill day complete")
		} else {
			log.WithFields(log.Fields{"start": dayStart, "end": dayEnd}).Info("no new historical jobs for day")
		}
		dayEnd = dayStart
	}
	return nil
}

// historicalJUnitBatch groups builds by JUnit table and modified-time window
// so one query can cover many builds without rescanning daily partitions.
type historicalJUnitBatch struct {
	Table    string
	Start    time.Time
	End      time.Time
	BuildIDs []string
}

// historicalJUnitBatches bounds each table query by its candidates' start and
// completion times and keeps build IDs ordered for deterministic parameters.
func historicalJUnitBatches(candidates []prowloader.HistoricalJob) ([]historicalJUnitBatch, error) {
	type tableRange struct {
		start time.Time
		end   time.Time
		ids   map[string]struct{}
	}
	ranges := map[string]*tableRange{}
	for _, candidate := range candidates {
		start := candidate.ProwJob.Status.StartTime.UTC()
		if start.IsZero() {
			return nil, fmt.Errorf("historical job %s has no start time", candidate.ProwJob.Status.BuildID)
		}
		end := start.AddDate(0, 0, 1)
		if completion := candidate.ProwJob.Status.CompletionTime; completion != nil && completion.After(start) {
			end = completion.UTC()
		}
		table := "junit"
		if strings.Contains(candidate.ProwJob.Status.URL, "/pr-logs/") {
			table = "junit_pr"
		}
		if ranges[table] == nil {
			ranges[table] = &tableRange{start: start, end: end, ids: map[string]struct{}{}}
		} else {
			if start.Before(ranges[table].start) {
				ranges[table].start = start
			}
			if end.After(ranges[table].end) {
				ranges[table].end = end
			}
		}
		ranges[table].ids[candidate.ProwJob.Status.BuildID] = struct{}{}
	}

	batches := make([]historicalJUnitBatch, 0)
	for table, rangeForTable := range ranges {
		buildIDs := make([]string, 0, len(rangeForTable.ids))
		for id := range rangeForTable.ids {
			buildIDs = append(buildIDs, id)
		}
		sort.Strings(buildIDs)
		batches = append(batches, historicalJUnitBatch{Table: table, Start: rangeForTable.start, End: rangeForTable.end, BuildIDs: buildIDs})
	}
	sort.Slice(batches, func(i, j int) bool {
		return batches[i].Table < batches[j].Table
	})
	return batches, nil
}

package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
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
	"k8s.io/apimachinery/pkg/util/sets"
)

const historicalGCSConcurrency = 20

type historicalRawBatchWriter struct {
	batch     []pgwriter.JobRunResult
	batchSize int
	write     func([]pgwriter.JobRunResult) error
	persisted int
}

func (w *historicalRawBatchWriter) add(result *pgwriter.JobRunResult) error {
	w.batch = append(w.batch, *result)
	if len(w.batch) < w.batchSize {
		return nil
	}
	return w.flush()
}

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

type prowBackfillFlags struct {
	BigQueryFlags    *flags.BigQueryFlags
	GoogleCloudFlags *flags.GoogleCloudFlags
	PostgresFlags    *flags.PostgresFlags
	ConfigFlags      *configflags.ConfigFlags
	ModeFlags        *flags.ModeFlags
	StartDate        string
	EndDate          string
	TestSource       string
	DryRun           bool
	SkipPartitions   bool
}

func newProwBackfillFlags() *prowBackfillFlags {
	return &prowBackfillFlags{
		BigQueryFlags:    flags.NewBigQueryFlags(),
		GoogleCloudFlags: flags.NewGoogleCloudFlags(),
		PostgresFlags:    flags.NewPostgresDatabaseFlags(),
		ConfigFlags:      configflags.NewConfigFlags(),
		ModeFlags:        flags.NewModeFlags(),
		TestSource:       "auto",
	}
}

func (f *prowBackfillFlags) bindCommon(fs *pflag.FlagSet) {
	f.BigQueryFlags.BindFlags(fs)
	f.GoogleCloudFlags.BindFlags(fs)
	f.PostgresFlags.BindFlags(fs)
	f.ConfigFlags.BindFlags(fs)
	f.ModeFlags.BindFlags(fs)
	fs.StringVar(&f.StartDate, "start-date", "", "Inclusive UTC start date (YYYY-MM-DD)")
	fs.StringVar(&f.EndDate, "end-date", "", "Inclusive UTC end date (YYYY-MM-DD)")
	fs.BoolVar(&f.DryRun, "dry-run", false, "Validate inputs and report work without writing PostgreSQL")
	fs.BoolVar(&f.SkipPartitions, "skip-partition-validation", false, "Skip ensuring partitions exist for the requested range")
}

func NewProwBackfillCommand() *cobra.Command {
	f := newProwBackfillFlags()
	cmd := &cobra.Command{Use: "prow-backfill", Short: "Run a bounded historical Prow job backfill", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		start, end, err := f.parseRange()
		if err != nil {
			return err
		}
		if err := validateTestSource(f.TestSource); err != nil {
			return err
		}
		return runHistoricalRaw(cmd.Context(), f, start, end)
	}}
	f.bindCommon(cmd.Flags())
	cmd.Flags().StringVar(&f.TestSource, "test-source", f.TestSource, "Test source: auto, gcs, or bigquery")
	return cmd
}

func (f *prowBackfillFlags) parseRange() (time.Time, time.Time, error) {
	since, err := civil.ParseDate(f.StartDate)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid --start-date: %w", err)
	}
	until, err := civil.ParseDate(f.EndDate)
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

func historicalReleaseNames(ctx context.Context, dbc *db.DB, bq *bqcachedclient.Client) ([]string, error) {
	releaseConfigs, err := api.GetReleasesFromDB(ctx, dbc)
	if err != nil {
		return nil, fmt.Errorf("loading release definitions from PostgreSQL: %w", err)
	}
	if len(releaseConfigs) == 0 {
		releaseConfigs, err = api.GetReleasesFromBigQuery(ctx, bq)
		if err != nil {
			return nil, fmt.Errorf("loading release definitions from BigQuery: %w", err)
		}
	}
	return releaseNames(releaseConfigs), nil
}

func releaseNames(releaseConfigs []sippyv1.Release) []string {
	releases := make([]string, 0, len(releaseConfigs))
	for _, release := range releaseConfigs {
		releases = append(releases, release.Release)
	}
	return releases
}

func (f *prowBackfillFlags) loader(ctx context.Context) (*prowloader.ProwLoader, func(), error) {
	if f.ConfigFlags.Path == "" {
		return nil, nil, fmt.Errorf("--config is required for historical Prow backfill")
	}
	dbc, err := f.PostgresFlags.GetDBClient()
	if err != nil {
		return nil, nil, fmt.Errorf("initializing PostgreSQL: %w", err)
	}
	config, err := f.ConfigFlags.GetConfig()
	if err != nil {
		return nil, nil, err
	}
	opCtx, queryCtx := bqcachedclient.OpCtxForCronEnv(ctx, "prow-backfill")
	bq, err := bqcachedclient.New(queryCtx, opCtx, nil, f.GoogleCloudFlags.ServiceAccountCredentialFile, f.BigQueryFlags.BigQueryProject, f.BigQueryFlags.BigQueryDataset, f.BigQueryFlags.ReleasesTable)
	if err != nil {
		return nil, nil, fmt.Errorf("initializing BigQuery: %w", err)
	}
	releases, err := historicalReleaseNames(ctx, dbc, bq)
	if err != nil {
		return nil, nil, err
	}
	var gcsClient *storage.Client
	if shouldInitializeGCS(f.TestSource, f.GoogleCloudFlags.ServiceAccountCredentialFile, f.GoogleCloudFlags.OAuthClientCredentialFile) {
		gcsClient, err = gcs.NewGCSClient(ctx, f.GoogleCloudFlags.ServiceAccountCredentialFile, f.GoogleCloudFlags.OAuthClientCredentialFile)
		if err != nil {
			return nil, nil, fmt.Errorf("initializing GCS: %w", err)
		}
	}
	overrides, err := variantregistry.BuildSyntheticReleaseJobOverrides(config.Releases)
	if err != nil {
		return nil, nil, err
	}
	loader := prowloader.New(ctx, dbc, gcsClient, bq, nil, f.ModeFlags.GetVariantManager(ctx, bq), f.ModeFlags.GetSyntheticTestManager(), releases, config, nil, nil, nil, overrides)
	loader.SetQuietHistoricalLogs(true)
	return loader, func() {
		_ = bq.BQ.Close()
		if gcsClient != nil {
			_ = gcsClient.Close()
		}
	}, nil
}

func validateTestSource(source string) error {
	if source != "auto" && source != "gcs" && source != "bigquery" {
		return fmt.Errorf("invalid --test-source %q", source)
	}
	return nil
}

func shouldInitializeGCS(source, serviceAccountCredentialFile, oauthCredentialFile string) bool {
	if source == "gcs" {
		return true
	}
	return source == "auto" && (serviceAccountCredentialFile != "" || oauthCredentialFile != "")
}

func shouldFallbackToBigQuery(source string, gcsBuildPrefixExists bool) bool {
	return source == "auto" && !gcsBuildPrefixExists
}

func historicalGCSFallbackReason(buildPrefixExists bool, err error) string {
	if err != nil {
		return "gcs_error"
	}
	if !buildPrefixExists {
		return "missing_gcs_prefix"
	}
	return "nil_gcs_result"
}

func runHistoricalRaw(ctx context.Context, f *prowBackfillFlags, start, end time.Time) error {
	loader, cleanup, err := f.loader(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	if !f.DryRun && !f.SkipPartitions {
		if err := loader.EnsureHistoricalPartitions(start, end); err != nil {
			return err
		}
	}
	analyzer, err := bqjunit.New(loader.BigQueryClient())
	if err != nil {
		return err
	}
	if err := analyzer.InspectSchema(ctx); err != nil {
		return fmt.Errorf("inspecting BigQuery schema: %w", err)
	}
	log.WithFields(log.Fields{"start": start, "end": end, "testSource": f.TestSource}).Info("historical test source schema inspected")
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
			results, err := loadHistoricalRawResults(ctx, loader, f, analyzer, candidates)
			if err != nil {
				return err
			}
			if f.DryRun {
				log.WithFields(log.Fields{"runs": results, "start": dayStart, "end": dayEnd, "source": f.TestSource}).Info("dry-run historical raw backfill day complete")
			} else {
				log.WithFields(log.Fields{"runs": results, "start": dayStart, "end": dayEnd, "source": f.TestSource}).Info("historical raw backfill day complete")
			}
		} else {
			log.WithFields(log.Fields{"start": dayStart, "end": dayEnd}).Info("no new historical jobs for day")
		}
		dayEnd = dayStart
	}
	return nil
}

func loadHistoricalRawResults(ctx context.Context, loader *prowloader.ProwLoader, f *prowBackfillFlags, analyzer *bqjunit.Client, candidates []prowloader.HistoricalJob) (int, error) {
	const historicalBatchSize = 100

	fallbackCandidates := make([]prowloader.HistoricalJob, 0, len(candidates))
	fallbackReasons := make(map[string]int)
	processed := 0
	progressInterval := len(candidates) / 10
	if progressInterval < 1 {
		progressInterval = 1
	}

	batchWriter := historicalRawBatchWriter{
		batch:     make([]pgwriter.JobRunResult, 0, historicalBatchSize),
		batchSize: historicalBatchSize,
		write: func(batch []pgwriter.JobRunResult) error {
			if f.DryRun {
				return nil
			}
			return loader.PersistHistoricalRaw(ctx, batch)
		},
	}
	flush := func() error {
		before := batchWriter.persisted
		if err := batchWriter.flush(); err != nil {
			return err
		}
		if batchWriter.persisted != before {
			log.WithFields(log.Fields{"batch": batchWriter.persisted - before, "persisted": batchWriter.persisted, "total": len(candidates)}).Info("persisted historical raw results")
		}
		return nil
	}
	acceptResult := func(result *pgwriter.JobRunResult) error {
		processed++
		if processed == 1 || processed%progressInterval == 0 || processed == len(candidates) {
			log.WithFields(log.Fields{"processed": processed, "total": len(candidates), "source": f.TestSource}).Info("loaded historical test results")
		}
		before := batchWriter.persisted
		if err := batchWriter.add(result); err != nil {
			return fmt.Errorf("writing historical raw batch: %w", err)
		}
		if batchWriter.persisted != before {
			log.WithFields(log.Fields{"batch": batchWriter.persisted - before, "persisted": batchWriter.persisted, "total": len(candidates)}).Info("persisted historical raw results")
		}
		return nil
	}

	tryGCS := f.TestSource != "bigquery" && (f.TestSource == "gcs" || shouldInitializeGCS(f.TestSource, f.GoogleCloudFlags.ServiceAccountCredentialFile, f.GoogleCloudFlags.OAuthClientCredentialFile))
	if tryGCS {
		log.WithFields(log.Fields{"jobs": len(candidates)}).Info("loading historical test data from GCS")
	}
	if !tryGCS {
		fallbackCandidates = append(fallbackCandidates, candidates...)
	} else {
		type gcsResult struct {
			candidate         prowloader.HistoricalJob
			result            *pgwriter.JobRunResult
			err               error
			buildPrefixExists bool
		}
		workCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		jobs := make(chan prowloader.HistoricalJob)
		gcsResults := make(chan gcsResult, historicalGCSConcurrency)
		var workers sync.WaitGroup
		for i := 0; i < historicalGCSConcurrency; i++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for candidate := range jobs {
					result, buildPrefixExists, err := loader.BuildHistoricalTestResult(workCtx, candidate, "gcs", nil)
					select {
					case gcsResults <- gcsResult{candidate: candidate, result: result, err: err, buildPrefixExists: buildPrefixExists}:
					case <-workCtx.Done():
						return
					}
				}
			}()
		}
		go func() {
			defer close(gcsResults)
			for _, candidate := range candidates {
				select {
				case jobs <- candidate:
				case <-workCtx.Done():
					close(jobs)
					workers.Wait()
					return
				}
			}
			close(jobs)
			workers.Wait()
		}()
		for loaded := range gcsResults {
			if loaded.err != nil || loaded.result == nil {
				if f.TestSource == "gcs" {
					if loaded.err != nil {
						return 0, fmt.Errorf("loading GCS tests for build %s: %w", loaded.candidate.ProwJob.Status.BuildID, loaded.err)
					}
					return 0, fmt.Errorf("loading GCS tests for build %s returned no tests", loaded.candidate.ProwJob.Status.BuildID)
				}
				if !shouldFallbackToBigQuery(f.TestSource, loaded.buildPrefixExists) {
					if loaded.err != nil {
						return 0, fmt.Errorf("loading GCS tests for build %s: %w", loaded.candidate.ProwJob.Status.BuildID, loaded.err)
					}
					return 0, fmt.Errorf("loading GCS tests for build %s returned no result", loaded.candidate.ProwJob.Status.BuildID)
				}
				reason := historicalGCSFallbackReason(loaded.buildPrefixExists, loaded.err)
				fallbackReasons[reason]++
				fallbackCandidates = append(fallbackCandidates, loaded.candidate)
				continue
			}
			if err := acceptResult(loaded.result); err != nil {
				return 0, err
			}
		}
	}
	if len(fallbackCandidates) > 0 && f.TestSource == "auto" {
		log.WithFields(log.Fields{"jobs": len(fallbackCandidates), "reasons": fallbackReasons}).Info("falling back to BigQuery for historical test data")
	}
	batches, err := historicalJUnitBatches(fallbackCandidates)
	if err != nil {
		return 0, err
	}
	candidatesByBuildID := make(map[string]prowloader.HistoricalJob, len(fallbackCandidates))
	for _, candidate := range fallbackCandidates {
		candidatesByBuildID[candidate.ProwJob.Status.BuildID] = candidate
	}
	processedBuilds := sets.New[string]()
	bqBuildsWithRows := sets.New[string]()
	bqRows := 0
	for _, batch := range batches {
		rowsByBuild := make(map[string][]prowloader.HistoricalBQTestRow)
		currentBuildID := ""
		flushBuild := func() error {
			if currentBuildID == "" {
				return nil
			}
			candidate, ok := candidatesByBuildID[currentBuildID]
			if !ok {
				return fmt.Errorf("BigQuery returned unknown historical build %s", currentBuildID)
			}
			result, _, err := loader.BuildHistoricalTestResult(ctx, candidate, "bigquery", rowsByBuild[currentBuildID])
			if err != nil {
				return fmt.Errorf("loading BigQuery tests for build %s: %w", currentBuildID, err)
			}
			processedBuilds.Insert(currentBuildID)
			delete(rowsByBuild, currentBuildID)
			currentBuildID = ""
			return acceptResult(result)
		}

		log.WithFields(log.Fields{"table": batch.Table, "builds": len(batch.BuildIDs), "junitStart": batch.Start, "junitEnd": batch.End}).Info("loading historical JUnit range from BigQuery")
		_, err := analyzer.StreamRows(ctx, batch.BuildIDs, bqjunit.QueryMode{ModifiedSince: batch.Start, ModifiedUntil: batch.End, Table: batch.Table, PreserveModifiedWindow: true}, func(row bqjunit.TestRow) error {
			bqBuildsWithRows.Insert(row.BuildID)
			bqRows++
			if currentBuildID != "" && currentBuildID != row.BuildID {
				if err := flushBuild(); err != nil {
					return err
				}
			}
			currentBuildID = row.BuildID
			rowsByBuild[row.BuildID] = append(rowsByBuild[row.BuildID], prowloader.HistoricalBQTestRow{BuildID: row.BuildID, TestName: row.TestName, Suite: row.Suite, Success: row.Success, Skipped: row.Skipped, FlakeCount: row.FlakeCount, Duration: row.Duration, FailureOutput: row.FailureContent, Lifecycle: row.Lifecycle})
			return nil
		})
		if err != nil {
			return 0, fmt.Errorf("reading BigQuery %s: %w", batch.Table, err)
		}
		if err := flushBuild(); err != nil {
			return 0, err
		}
		log.WithFields(log.Fields{"table": batch.Table, "builds": len(batch.BuildIDs), "junitStart": batch.Start, "junitEnd": batch.End}).Info("historical JUnit range complete")
	}
	for _, candidate := range fallbackCandidates {
		buildID := candidate.ProwJob.Status.BuildID
		if processedBuilds.Has(buildID) {
			continue
		}
		result, _, err := loader.BuildHistoricalTestResult(ctx, candidate, "bigquery", nil)
		if err != nil {
			return 0, fmt.Errorf("loading BigQuery tests for build %s: %w", buildID, err)
		}
		if err := acceptResult(result); err != nil {
			return 0, err
		}
	}
	if err := flush(); err != nil {
		return 0, fmt.Errorf("writing final historical raw batch: %w", err)
	}
	if len(fallbackCandidates) > 0 {
		log.WithFields(log.Fields{
			"jobs":               len(fallbackCandidates),
			"buildsWithJUnit":    bqBuildsWithRows.Len(),
			"buildsWithoutJUnit": len(fallbackCandidates) - bqBuildsWithRows.Len(),
			"junitRows":          bqRows,
		}).Info("historical BigQuery test data summary")
	}
	return batchWriter.persisted, nil
}

type historicalJUnitBatch struct {
	Table    string
	Start    time.Time
	End      time.Time
	BuildIDs []string
}

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

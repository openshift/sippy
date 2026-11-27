package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/openshift/sippy/pkg/dataloader/prowloader"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/bqjunit"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/pgwriter"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
)

const historicalRawBatchSize = 100

// historicalResultSink owns progress reporting and the single database writer
// while either source produces complete job results.
type historicalResultSink struct {
	batch            historicalRawBatchWriter
	processed        int
	total            int
	progressInterval int
	source           string
}

// newHistoricalResultSink keeps source readers independent of persistence.
func newHistoricalResultSink(ctx context.Context, loader *prowloader.ProwLoader, total int, source string) *historicalResultSink {
	interval := total / 10
	if interval < 1 {
		interval = 1
	}
	return &historicalResultSink{
		batch: historicalRawBatchWriter{
			batch:     make([]pgwriter.JobRunResult, 0, historicalRawBatchSize),
			batchSize: historicalRawBatchSize,
			write: func(batch []pgwriter.JobRunResult) error {
				return loader.PersistHistoricalRaw(ctx, batch)
			},
		},
		total: total, progressInterval: interval, source: source,
	}
}

// accept persists full batches as GCS or BigQuery produces complete jobs.
func (s *historicalResultSink) accept(result *pgwriter.JobRunResult) error {
	s.processed++
	if s.processed == 1 || s.processed%s.progressInterval == 0 || s.processed == s.total {
		log.WithFields(log.Fields{"processed": s.processed, "total": s.total, "source": s.source}).Info("loaded historical test results")
	}
	before := s.batch.persisted
	if err := s.batch.add(result); err != nil {
		return fmt.Errorf("writing historical raw batch: %w", err)
	}
	s.logPersisted(before)
	return nil
}

// flush persists the last partial batch after all source reads complete.
func (s *historicalResultSink) flush() error {
	before := s.batch.persisted
	if err := s.batch.flush(); err != nil {
		return fmt.Errorf("writing final historical raw batch: %w", err)
	}
	s.logPersisted(before)
	return nil
}

// logPersisted reports each committed batch without logging every job.
func (s *historicalResultSink) logPersisted(before int) {
	if s.batch.persisted != before {
		log.WithFields(log.Fields{"batch": s.batch.persisted - before, "persisted": s.batch.persisted, "total": s.total}).Info("persisted historical raw results")
	}
}

// loadHistoricalRawResults coordinates the selected source, fallback, and
// final batch flush without owning either source's read loop.
func loadHistoricalRawResults(ctx context.Context, loader *prowloader.ProwLoader, f *prowBackfillFlags, junitReader *bqjunit.Client, candidates []prowloader.HistoricalJob) (int, error) {
	sink := newHistoricalResultSink(ctx, loader, len(candidates), f.Run.TestSource)
	fallback := candidates
	fallbackReasons := make(map[string]int)
	tryGCS := f.Run.TestSource != "bigquery" && (f.Run.TestSource == "gcs" || shouldInitializeGCS(f.Run.TestSource, f.Loader.GoogleCloudFlags.ServiceAccountCredentialFile, f.Loader.GoogleCloudFlags.OAuthClientCredentialFile))
	if tryGCS {
		log.WithFields(log.Fields{"jobs": len(candidates)}).Info("loading historical test data from GCS")
		var err error
		fallback, err = loadHistoricalGCS(ctx, loader, f.Run.TestSource, candidates, sink)
		if err != nil {
			return 0, err
		}
		fallbackReasons["missing_gcs_prefix"] = len(fallback)
	}
	if len(fallback) > 0 && f.Run.TestSource == "auto" {
		log.WithFields(log.Fields{"jobs": len(fallback), "reasons": fallbackReasons}).Info("falling back to BigQuery for historical test data")
	}
	if err := loadHistoricalBigQuery(ctx, loader, junitReader, fallback, sink); err != nil {
		return 0, err
	}
	if err := sink.flush(); err != nil {
		return 0, err
	}
	return sink.batch.persisted, nil
}

// historicalGCSResult carries one worker's job and prefix state to the
// consumer, which alone decides fallback and writes completed jobs.
type historicalGCSResult struct {
	candidate    prowloader.HistoricalJob
	result       *pgwriter.JobRunResult
	prefixExists bool
	err          error
}

// loadHistoricalGCS runs parallel GCS readers and returns only jobs whose
// build prefix is confirmed absent for a BigQuery fallback.
func loadHistoricalGCS(ctx context.Context, loader *prowloader.ProwLoader, source string, candidates []prowloader.HistoricalJob, sink *historicalResultSink) ([]prowloader.HistoricalJob, error) {
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan prowloader.HistoricalJob)
	results := make(chan historicalGCSResult, historicalGCSConcurrency)
	var workers sync.WaitGroup
	for i := 0; i < historicalGCSConcurrency; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for candidate := range jobs {
				result, prefixExists, err := loader.BuildHistoricalGCSResult(workCtx, candidate)
				select {
				case results <- historicalGCSResult{candidate: candidate, result: result, prefixExists: prefixExists, err: err}:
				case <-workCtx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(results)
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

	fallback := make([]prowloader.HistoricalJob, 0)
	for loaded := range results {
		if loaded.err != nil || loaded.result == nil {
			if shouldFallbackToBigQuery(source, loaded.prefixExists, loaded.err) {
				fallback = append(fallback, loaded.candidate)
				continue
			}
			buildID := loaded.candidate.ProwJob.Status.BuildID
			if loaded.err != nil {
				return nil, fmt.Errorf("loading GCS tests for build %s: %w", buildID, loaded.err)
			}
			if source == "gcs" {
				return nil, fmt.Errorf("loading GCS tests for build %s returned no tests", buildID)
			}
			return nil, fmt.Errorf("loading GCS tests for build %s returned no result", buildID)
		}
		if err := sink.accept(loaded.result); err != nil {
			return nil, err
		}
	}
	return fallback, nil
}

// historicalBigQueryBuild retains only one ordered build's JUnit rows until
// its complete job result is handed to the database writer.
type historicalBigQueryBuild struct {
	loader     *prowloader.ProwLoader
	candidates map[string]prowloader.HistoricalJob
	processed  sets.Set[string]
	sink       *historicalResultSink
	buildID    string
	rows       []bqjunit.TestRow
}

// accept advances the ordered BigQuery stream one build at a time.
func (b *historicalBigQueryBuild) accept(row bqjunit.TestRow) error {
	if b.buildID != "" && b.buildID != row.BuildID {
		if err := b.flush(); err != nil {
			return err
		}
	}
	b.buildID = row.BuildID
	b.rows = append(b.rows, row)
	return nil
}

// flush converts and releases a complete build before accepting another.
func (b *historicalBigQueryBuild) flush() error {
	if b.buildID == "" {
		return nil
	}
	candidate, ok := b.candidates[b.buildID]
	if !ok {
		return fmt.Errorf("BigQuery returned unknown historical build %s", b.buildID)
	}
	result, err := b.loader.BuildHistoricalBigQueryResult(candidate, b.rows)
	if err != nil {
		return fmt.Errorf("loading BigQuery tests for build %s: %w", b.buildID, err)
	}
	b.processed.Insert(b.buildID)
	b.buildID = ""
	b.rows = nil
	return b.sink.accept(result)
}

// loadHistoricalBigQuery scans each table's candidate window once and
// supplies synthetic results for builds without JUnit rows.
func loadHistoricalBigQuery(ctx context.Context, loader *prowloader.ProwLoader, junitReader *bqjunit.Client, candidates []prowloader.HistoricalJob, sink *historicalResultSink) error {
	batches, err := historicalJUnitBatches(candidates)
	if err != nil {
		return err
	}
	candidatesByBuildID := make(map[string]prowloader.HistoricalJob, len(candidates))
	for _, candidate := range candidates {
		candidatesByBuildID[candidate.ProwJob.Status.BuildID] = candidate
	}
	processedBuilds := sets.New[string]()
	buildsWithRows := sets.New[string]()
	junitRows := 0
	for _, batch := range batches {
		build := historicalBigQueryBuild{loader: loader, candidates: candidatesByBuildID, processed: processedBuilds, sink: sink}
		log.WithFields(log.Fields{"table": batch.Table, "builds": len(batch.BuildIDs), "junitStart": batch.Start, "junitEnd": batch.End}).Info("loading historical JUnit range from BigQuery")
		err := junitReader.StreamRows(ctx, batch.BuildIDs, bqjunit.QueryMode{ModifiedSince: batch.Start, ModifiedUntil: batch.End, Table: batch.Table}, func(row bqjunit.TestRow) error {
			buildsWithRows.Insert(row.BuildID)
			junitRows++
			return build.accept(row)
		})
		if err != nil {
			return fmt.Errorf("reading BigQuery %s: %w", batch.Table, err)
		}
		if err := build.flush(); err != nil {
			return err
		}
		log.WithFields(log.Fields{"table": batch.Table, "builds": len(batch.BuildIDs), "junitStart": batch.Start, "junitEnd": batch.End}).Info("historical JUnit range complete")
	}
	for _, candidate := range candidates {
		buildID := candidate.ProwJob.Status.BuildID
		if processedBuilds.Has(buildID) {
			continue
		}
		result, err := loader.BuildHistoricalBigQueryResult(candidate, nil)
		if err != nil {
			return fmt.Errorf("loading BigQuery tests for build %s: %w", buildID, err)
		}
		if err := sink.accept(result); err != nil {
			return err
		}
	}
	if len(candidates) > 0 {
		log.WithFields(log.Fields{
			"jobs":               len(candidates),
			"buildsWithJUnit":    buildsWithRows.Len(),
			"buildsWithoutJUnit": len(candidates) - buildsWithRows.Len(),
			"junitRows":          junitRows,
		}).Info("historical BigQuery test data summary")
	}
	return nil
}

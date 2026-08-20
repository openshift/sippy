// Package infrafailurebackfill backfills InfraFailure job run labels from
// BigQuery into PostgreSQL.
//
// The BigQuery job_labels table is the source of truth for which prow job runs
// were labeled InfraFailure. This package reads those runs within a time window
// and, for any run that does not yet carry the label in PostgreSQL, calls
// infrafailure.RecordInfraFailure to atomically apply the label and remove the
// run's contribution from the summary tables. RecordInfraFailure is idempotent,
// so the backfill is safe to run repeatedly.
//
// The exported logic is split so the pure pieces (time-window resolution, query
// construction, batching, and classification) are unit-testable without a
// BigQuery or PostgreSQL client, per the project's testing conventions. The
// narrow client calls are wired behind function fields on Backfiller so the
// orchestration can be exercised with closures in tests.
package infrafailurebackfill

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
	"k8s.io/apimachinery/pkg/util/sets"

	bqclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/infrafailure"
)

const (
	// defaultDays is the default lookback window when --since is not supplied.
	defaultDays = 90
	// defaultBatchSize is the default number of runs processed per batch.
	defaultBatchSize = 100
)

// datasetPattern bounds the characters allowed in a BigQuery dataset name. The
// dataset is interpolated directly into the query's table reference (it cannot
// be a query parameter), so it is validated against this allow-list to prevent
// SQL injection. It matches the characters valid in a BigQuery project/dataset
// identifier: letters, digits, underscores, dots, and dashes.
var datasetPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

// Options configures a backfill run.
type Options struct {
	// Since is the start of the time window as a date string (YYYY-MM-DD).
	// When set it takes precedence over Days.
	Since string
	// Days is the lookback window in days from now, used when Since is empty.
	Days int
	// DryRun reports what would be done without modifying PostgreSQL.
	DryRun bool
	// BatchSize is the number of runs processed per batch.
	BatchSize int
}

// Stats reports the outcome of a backfill run.
type Stats struct {
	// TotalBQRuns is the number of distinct InfraFailure runs found in BigQuery.
	TotalBQRuns int
	// AlreadyLabeled is the number of runs that already carried the label in PG.
	AlreadyLabeled int
	// NewlySynced is the number of runs newly labeled (or that would be labeled
	// in dry-run mode).
	NewlySynced int
	// NotFoundInPG is the number of BigQuery runs with no matching prow_job_runs
	// row in PostgreSQL (e.g. runs older than the retained partitions).
	NotFoundInPG int
	// Errors is the number of runs that failed to sync.
	Errors int
}

// batchLabelStatus holds, for a single batch, the set of run IDs that exist in
// PostgreSQL and the subset of those that already carry the InfraFailure label.
type batchLabelStatus struct {
	existing sets.Set[int64]
	labeled  sets.Set[int64]
}

// Backfiller backfills InfraFailure labels from BigQuery into PostgreSQL.
type Backfiller struct {
	bq   *bqclient.Client
	dbc  *db.DB
	opts Options

	// Narrow client calls are wired behind function fields so orchestration can
	// be unit-tested with closures. New wires the real implementations.
	fetchInfraFailureIDs func(ctx context.Context, startDate civil.Date) ([]int64, error)
	findLabelStatus      func(ctx context.Context, batch []int64) (batchLabelStatus, error)
	recordInfraFailure   func(ctx context.Context, prowJobRunID int64) (infrafailure.RecordOutcome, error)
}

// New constructs a Backfiller wired to the given BigQuery and PostgreSQL
// clients. Zero-valued Days and BatchSize fall back to their defaults.
func New(bq *bqclient.Client, dbc *db.DB, opts Options) *Backfiller {
	if opts.Days <= 0 {
		opts.Days = defaultDays
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = defaultBatchSize
	}
	b := &Backfiller{bq: bq, dbc: dbc, opts: opts}
	b.fetchInfraFailureIDs = b.fetchInfraFailureIDsFromBQ
	b.findLabelStatus = b.findLabelStatusInPG
	b.recordInfraFailure = func(ctx context.Context, prowJobRunID int64) (infrafailure.RecordOutcome, error) {
		return infrafailure.RecordInfraFailureWithOutcome(ctx, b.dbc.DB, prowJobRunID)
	}
	return b
}

// Run executes the backfill and returns the resulting statistics.
func (b *Backfiller) Run(ctx context.Context) (*Stats, error) {
	startDate, err := resolveStartDate(b.opts.Since, b.opts.Days, time.Now())
	if err != nil {
		return nil, err
	}

	logger := log.WithFields(log.Fields{
		"since":     startDate.String(),
		"dryRun":    b.opts.DryRun,
		"batchSize": b.opts.BatchSize,
	})
	logger.Info("starting InfraFailure backfill")

	ids, err := b.fetchInfraFailureIDs(ctx, startDate)
	if err != nil {
		return nil, fmt.Errorf("fetching InfraFailure runs from BigQuery: %w", err)
	}

	stats := &Stats{TotalBQRuns: len(ids)}
	logger.WithField("count", len(ids)).Info("fetched InfraFailure runs from BigQuery")

	batches := batchIDs(ids, b.opts.BatchSize)
	for i, batch := range batches {
		if err := b.processBatch(ctx, batch, stats); err != nil {
			return stats, fmt.Errorf("processing batch %d of %d: %w", i+1, len(batches), err)
		}
		log.WithFields(log.Fields{
			"batch":          i + 1,
			"of":             len(batches),
			"alreadyLabeled": stats.AlreadyLabeled,
			"newlySynced":    stats.NewlySynced,
			"notFoundInPG":   stats.NotFoundInPG,
			"errors":         stats.Errors,
		}).Debug("processed batch")
	}

	logger.WithFields(log.Fields{
		"totalBQRuns":    stats.TotalBQRuns,
		"alreadyLabeled": stats.AlreadyLabeled,
		"newlySynced":    stats.NewlySynced,
		"notFoundInPG":   stats.NotFoundInPG,
		"errors":         stats.Errors,
	}).Info("InfraFailure backfill complete")

	return stats, nil
}

// processBatch classifies a batch against PostgreSQL and syncs the runs that are
// missing the label, accumulating results into stats.
func (b *Backfiller) processBatch(ctx context.Context, batch []int64, stats *Stats) error {
	status, err := b.findLabelStatus(ctx, batch)
	if err != nil {
		return fmt.Errorf("looking up InfraFailure label status: %w", err)
	}

	toSync, alreadyLabeled, notInPG := classifyBatch(batch, status)
	stats.AlreadyLabeled += len(alreadyLabeled)
	stats.NotFoundInPG += len(notInPG)

	if b.opts.DryRun {
		// Report what would be synced without touching the database.
		stats.NewlySynced += len(toSync)
		for _, id := range toSync {
			log.WithField("prowJobRunID", id).Info("dry-run: would record InfraFailure")
		}
		return nil
	}

	for _, id := range toSync {
		outcome, err := b.recordInfraFailure(ctx, id)
		if err != nil {
			stats.Errors++
			log.WithError(err).WithField("prowJobRunID", id).Error("failed to record InfraFailure")
			continue
		}
		// toSync was classified as present-but-unlabeled by an earlier lookup, but
		// a concurrent labeler or a pruned partition can change that before we
		// record. Trust the recorded outcome so the stats stay accurate.
		switch outcome {
		case infrafailure.OutcomeRunNotFound:
			stats.NotFoundInPG++
			log.WithField("prowJobRunID", id).Debug("run not found in PostgreSQL at record time")
		case infrafailure.OutcomeAlreadyLabeled:
			stats.AlreadyLabeled++
			log.WithField("prowJobRunID", id).Debug("run already labeled InfraFailure at record time")
		default:
			stats.NewlySynced++
			log.WithField("prowJobRunID", id).Debug("recorded InfraFailure")
		}
	}
	return nil
}

// fetchInfraFailureIDsFromBQ queries the BigQuery job_labels table for distinct
// prow job run IDs labeled InfraFailure with a start time within the window.
func (b *Backfiller) fetchInfraFailureIDsFromBQ(ctx context.Context, startDate civil.Date) ([]int64, error) {
	sql, params, err := buildInfraFailureQuery(b.bq.Dataset, startDate)
	if err != nil {
		return nil, err
	}
	q := b.bq.Query(ctx, bqlabel.InfraFailureBackfill, sql)
	q.Parameters = params
	bqclient.LogQueryWithParamsReplaced(log.StandardLogger(), q)

	it, err := q.Read(ctx)
	if err != nil {
		return nil, err
	}

	var ids []int64
	for {
		var row struct {
			ProwJobBuildID string `bigquery:"prowjob_build_id"`
		}
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return ids, err
		}
		// prowjob_build_id is a STRING in BigQuery but maps to the int64
		// prow_job_runs.id primary key in PostgreSQL.
		id, perr := strconv.ParseInt(row.ProwJobBuildID, 10, 64)
		if perr != nil {
			log.WithError(perr).WithField("prowjob_build_id", row.ProwJobBuildID).
				Warn("skipping non-numeric prowjob_build_id from job_labels")
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// findLabelStatusInPG looks up which of the given run IDs exist in PostgreSQL and
// which already carry the InfraFailure label.
func (b *Backfiller) findLabelStatusInPG(ctx context.Context, batch []int64) (batchLabelStatus, error) {
	status := batchLabelStatus{existing: sets.New[int64](), labeled: sets.New[int64]()}
	if len(batch) == 0 {
		return status, nil
	}

	var rows []struct {
		ID      int64 `gorm:"column:id"`
		Labeled bool  `gorm:"column:labeled"`
	}
	// 'InfraFailure' is embedded literally (matching pkg/db/infrafailure and the
	// component readiness postgres provider) to avoid constructing dynamic SQL.
	// It must stay in sync with infrafailure.LabelInfraFailure.
	err := b.dbc.DB.WithContext(ctx).
		Table("prow_job_runs").
		Select("id, (labels @> ARRAY['InfraFailure']) AS labeled").
		Where("id IN ?", batch).
		Scan(&rows).Error
	if err != nil {
		return batchLabelStatus{}, err
	}

	for _, r := range rows {
		status.existing.Insert(r.ID)
		if r.Labeled {
			status.labeled.Insert(r.ID)
		}
	}
	return status, nil
}

// resolveStartDate resolves the start of the time window. A non-empty since
// (YYYY-MM-DD) takes precedence; otherwise the window starts days before now.
func resolveStartDate(since string, days int, now time.Time) (civil.Date, error) {
	if since != "" {
		d, err := civil.ParseDate(since)
		if err != nil {
			return civil.Date{}, fmt.Errorf("invalid --since %q: %w", since, err)
		}
		return d, nil
	}
	if days <= 0 {
		return civil.Date{}, fmt.Errorf("--days must be positive, got %d", days)
	}
	return civil.DateOf(now.UTC().AddDate(0, 0, -days)), nil
}

// buildInfraFailureQuery builds the BigQuery SQL and parameters for fetching the
// distinct InfraFailure run IDs whose prowjob_start falls within the window.
// It is a pure function so it can be unit-tested without a BigQuery client.
//
// The dataset cannot be passed as a query parameter (it is part of the table
// reference), so it is interpolated into the SQL. It is validated against
// datasetPattern first to prevent SQL injection.
func buildInfraFailureQuery(dataset string, startDate civil.Date) (string, []bigquery.QueryParameter, error) {
	if !datasetPattern.MatchString(dataset) {
		return "", nil, fmt.Errorf("invalid BigQuery dataset %q: must match %s", dataset, datasetPattern.String())
	}
	table := fmt.Sprintf("`%s.job_labels`", dataset)
	sql := fmt.Sprintf(`SELECT DISTINCT prowjob_build_id
FROM %s
WHERE label = @label
  AND DATE(prowjob_start) >= @startDate`, table)
	params := []bigquery.QueryParameter{
		{Name: "label", Value: infrafailure.LabelInfraFailure},
		{Name: "startDate", Value: startDate},
	}
	return sql, params, nil
}

// batchIDs splits ids into contiguous batches of at most size. A non-positive
// size falls back to defaultBatchSize.
func batchIDs(ids []int64, size int) [][]int64 {
	if size <= 0 {
		size = defaultBatchSize
	}
	if len(ids) == 0 {
		return nil
	}
	// Ceiling division without the (len+size-1) form, which could overflow for
	// very large inputs.
	capacity := len(ids) / size
	if len(ids)%size != 0 {
		capacity++
	}
	batches := make([][]int64, 0, capacity)
	for i := 0; i < len(ids); i += size {
		end := i + size
		if end > len(ids) {
			end = len(ids)
		}
		batches = append(batches, ids[i:end])
	}
	return batches
}

// classifyBatch partitions a batch into runs to sync (present but unlabeled),
// runs already labeled, and runs not present in PostgreSQL, given the PG lookup
// result. It is a pure function so it can be unit-tested without a client.
func classifyBatch(batch []int64, status batchLabelStatus) (toSync, alreadyLabeled, notInPG []int64) {
	for _, id := range batch {
		switch {
		case status.labeled.Has(id):
			alreadyLabeled = append(alreadyLabeled, id)
		case status.existing.Has(id):
			toSync = append(toSync, id)
		default:
			notInPG = append(notInPG, id)
		}
	}
	return toSync, alreadyLabeled, notInPG
}

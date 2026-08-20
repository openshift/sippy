// Package infrafailure records infrastructure-failure job runs and removes
// their contribution from Sippy's pre-aggregated summary tables.
//
// A job run labeled InfraFailure should not count toward test pass rates. To
// keep the summary tables (test_daily_totals and test_cumulative_summaries)
// consistent with that rule, RecordInfraFailure subtracts a run's test results
// from those tables at the moment the label is applied.
//
// The central invariant is: "InfraFailure label in PostgreSQL is equivalent to
// subtraction done." The conditional UPDATE gate enforces it -- the label is
// only ever set as part of the same transaction that performs the subtraction.
package infrafailure

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/db/query"
)

// LabelInfraFailure is the job run label applied to runs that failed for
// infrastructure reasons. Runs carrying this label are excluded from Sippy's
// summary tables and non-summary reporting queries.
//
// The raw SQL below embeds this value literally (matching the existing filter
// in the component readiness postgres provider) to keep the queries readable
// and to avoid constructing SQL dynamically.
const LabelInfraFailure = "InfraFailure"

// setInfraFailureLabelSQL sets the InfraFailure label on a single prow job run
// and, as a side effect, acquires the PostgreSQL row lock. It is the first
// operation performed so the lock is held for the remainder of the transaction.
//
// The NULL-safe predicate ensures runs with no labels (the common case) are
// still updated: array_append treats a NULL array as empty, yielding
// {InfraFailure}. When the label is already present the UPDATE matches no rows
// (RowsAffected == 0), which the caller treats as an idempotent no-op because
// the subtraction has already been performed.
const setInfraFailureLabelSQL = `
UPDATE prow_job_runs
SET labels = array_append(labels, 'InfraFailure')
WHERE id = ?
  AND (labels IS NULL OR NOT (labels @> ARRAY['InfraFailure']))`

// createDeltasTempTableSQL materializes one job run's test results into a temp
// table of per-summary-key deltas. The table is dropped automatically when the
// transaction ends (ON COMMIT DROP); the two UPDATE statements below read from
// it so the scan and aggregation run once rather than once per statement.
//
// The grouping matches the write path (pgwriter.createBatchDeltas and
// dailysummary.insertSQL) exactly so the subtraction targets the same rows the
// addition created: keyed by (release, date, test_id, suite_id, lifecycle,
// prow_job_id), using date(prow_job_run_timestamp) with no timezone conversion
// and COALESCE(suite_id, 0). Status values are 1=success, 12=failure,
// 13=flake (see sippyprocessing/v1 TestStatus constants).
//
// The release and timestamp placeholders carry the run's partition key values
// so the planner can prune to the run's single partition (equality on the raw
// partition-key columns) instead of scanning every partition for the run id.
const createDeltasTempTableSQL = `
CREATE TEMP TABLE infra_failure_deltas ON COMMIT DROP AS
SELECT
	prow_job_run_release AS release,
	date(prow_job_run_timestamp) AS date,
	test_id,
	COALESCE(suite_id, 0) AS suite_id,
	lifecycle,
	prow_job_id,
	COUNT(*) FILTER (WHERE status = 1) AS successes,
	COUNT(*) FILTER (WHERE status = 12) AS failures,
	COUNT(*) FILTER (WHERE status = 13) AS flakes,
	COUNT(*) AS runs
FROM prow_job_run_tests
WHERE prow_job_run_id = ? AND deleted_at IS NULL
	AND prow_job_run_release = ?
	AND prow_job_run_timestamp = ?
GROUP BY prow_job_run_release, date(prow_job_run_timestamp), test_id,
	COALESCE(suite_id, 0), lifecycle, prow_job_id`

// subtractDailyTotalsSQL removes the run's per-day counts from
// test_daily_totals for each affected summary key, reading the deltas from the
// materialized temp table.
const subtractDailyTotalsSQL = `
UPDATE test_daily_totals dt SET
	successes = dt.successes - d.successes,
	failures = dt.failures - d.failures,
	flakes = dt.flakes - d.flakes,
	runs = dt.runs - d.runs
FROM infra_failure_deltas d
WHERE dt.release = d.release
	AND dt.date = d.date
	AND dt.test_id = d.test_id
	AND dt.suite_id = d.suite_id
	AND dt.lifecycle = d.lifecycle
	AND dt.prow_job_id = d.prow_job_id`

// subtractCumulativeSummariesSQL cascades the same subtraction into the
// cumulative prefix sums, reading the deltas from the materialized temp table.
// Because each row holds a running total ordered by date, a constant-offset
// subtraction is applied to every row from the affected date onward
// (date >= d.date).
const subtractCumulativeSummariesSQL = `
UPDATE test_cumulative_summaries cs SET
	prefix_sum_successes = cs.prefix_sum_successes - d.successes,
	prefix_sum_failures = cs.prefix_sum_failures - d.failures,
	prefix_sum_flakes = cs.prefix_sum_flakes - d.flakes,
	prefix_sum_runs = cs.prefix_sum_runs - d.runs
FROM infra_failure_deltas d
WHERE cs.release = d.release
	AND cs.test_id = d.test_id
	AND cs.suite_id = d.suite_id
	AND cs.lifecycle = d.lifecycle
	AND cs.prow_job_id = d.prow_job_id
	AND cs.date >= d.date`

// RecordInfraFailure marks a prow job run as an infrastructure failure and
// removes its contribution from the pre-aggregated summary tables. It opens its
// own transaction on dbc (scoped to ctx) so the label set and the summary
// subtraction commit (or roll back) atomically. dbc may be a plain database
// connection or an existing transaction; gorm nests the latter as a savepoint.
//
// The operation is idempotent: if the run already carries the InfraFailure
// label the function returns nil without touching the summary tables, because
// the invariant guarantees the subtraction was already performed.
func RecordInfraFailure(ctx context.Context, dbc *gorm.DB, prowJobRunID int64) error {
	return dbc.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return recordInfraFailureInTx(tx, prowJobRunID)
	})
}

// recordInfraFailureInTx performs the label set and summary subtraction on the
// supplied transaction. Returning an error rolls back the transaction so
// neither the label nor the subtraction persists, preserving the invariant.
func recordInfraFailureInTx(tx *gorm.DB, prowJobRunID int64) error {
	logger := log.WithField("prowJobRunID", prowJobRunID)

	// Conditional UPDATE as the first operation: set the label and acquire the
	// row lock together. RowsAffected == 0 means the label was already set, so
	// the subtraction is already done -- nothing further to do.
	res := tx.Exec(setInfraFailureLabelSQL, prowJobRunID)
	if res.Error != nil {
		return fmt.Errorf("setting InfraFailure label on prow_job_run %d: %w", prowJobRunID, res.Error)
	}
	if res.RowsAffected == 0 {
		logger.Debug("prow job run already labeled InfraFailure; summary subtraction already applied, skipping")
		return nil
	}
	logger.Debug("set InfraFailure label on prow job run")

	// The atomic gate passed (the label was newly applied), so remove the run's
	// contribution from the summary tables in the same transaction.
	return subtractFromSummaries(tx, prowJobRunID)
}

// subtractFromSummaries removes a single prow job run's test results from the
// pre-aggregated summary tables (test_daily_totals and
// test_cumulative_summaries). It does not touch the InfraFailure label: callers
// own the label and any gating that decides whether the subtraction should run.
// All work happens on the supplied transaction so it commits or rolls back
// atomically with the caller's other writes.
func subtractFromSummaries(tx *gorm.DB, prowJobRunID int64) error {
	logger := log.WithField("prowJobRunID", prowJobRunID)

	// Read the run's partition keys (release and timestamp) so the delta scan
	// below can prune to the run's single partition instead of scanning every
	// partition for the run id.
	partKeys, err := query.LookupProwJobRunPartitionKeys(tx, prowJobRunID)
	if err != nil {
		return fmt.Errorf("reading partition keys for prow_job_run %d: %w", prowJobRunID, err)
	}

	// Drop any stale temp table before recreating it. ON COMMIT DROP ties the
	// table's lifetime to the outermost transaction, not to a savepoint, so when
	// the subtraction runs twice inside the same outer transaction (tx is
	// already a transaction, so gorm nests each call as a savepoint) the second
	// CREATE would collide with the table left by the first. Dropping first makes
	// the CREATE idempotent within the outer transaction.
	if err := tx.Exec("DROP TABLE IF EXISTS infra_failure_deltas").Error; err != nil {
		return fmt.Errorf("dropping stale infra_failure_deltas temp table for prow_job_run %d: %w", prowJobRunID, err)
	}

	// Materialize the per-summary-key deltas once so both UPDATEs below read
	// them from a single scan and aggregation rather than re-evaluating the
	// join twice.
	if err := tx.Exec(createDeltasTempTableSQL, prowJobRunID, partKeys.ProwJobRelease, partKeys.Timestamp).Error; err != nil {
		return fmt.Errorf("materializing deltas for prow_job_run %d: %w", prowJobRunID, err)
	}

	// Subtract the run's counts from the per-day totals.
	dailyRes := tx.Exec(subtractDailyTotalsSQL)
	if dailyRes.Error != nil {
		return fmt.Errorf("subtracting daily totals for prow_job_run %d: %w", prowJobRunID, dailyRes.Error)
	}
	logger.WithField("rowsAffected", dailyRes.RowsAffected).Debug("subtracted infra-failure run from test_daily_totals")

	// Cascade the subtraction into the cumulative prefix sums from the affected
	// date onward.
	cumulativeRes := tx.Exec(subtractCumulativeSummariesSQL)
	if cumulativeRes.Error != nil {
		return fmt.Errorf("subtracting cumulative summaries for prow_job_run %d: %w", prowJobRunID, cumulativeRes.Error)
	}
	logger.WithField("rowsAffected", cumulativeRes.RowsAffected).Debug("subtracted infra-failure run from test_cumulative_summaries")

	return nil
}

// SubtractNewInfraFailure removes a prow job run's contribution from the summary
// tables when the run is being newly labeled InfraFailure, without setting the
// label itself. It is the re-evaluator's entry point: the re-evaluator owns the
// prow_job_runs.labels array (it fully replaces it) and applies InfraFailure
// there, so this function must not touch the label.
//
// It is idempotent with respect to the summary subtraction. If the row already
// carries the InfraFailure label the subtraction was already performed (the
// invariant guarantees it), so this returns nil without touching the summary
// tables. Otherwise it performs the subtraction on the supplied transaction.
//
// Callers must run this inside the same row-locked transaction that later
// replaces the labels array so the containment check and the subtraction stay
// consistent with a concurrent RecordInfraFailure.
func SubtractNewInfraFailure(tx *gorm.DB, prowJobRunID int64) error {
	var found int
	res := tx.Raw(
		"SELECT 1 FROM prow_job_runs WHERE id = ? AND labels @> ARRAY[?] LIMIT 1",
		prowJobRunID, LabelInfraFailure).Scan(&found)
	if res.Error != nil {
		return fmt.Errorf("checking InfraFailure label on prow_job_run %d: %w", prowJobRunID, res.Error)
	}
	if res.RowsAffected > 0 {
		log.WithField("prowJobRunID", prowJobRunID).Debug("prow job run already labeled InfraFailure; summary subtraction already applied, skipping")
		return nil
	}
	return subtractFromSummaries(tx, prowJobRunID)
}

package infrafailure

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/db/query"
)

const restoreInfraFailureDeltasSQL = `
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

// RestoreInfraFailureToSummaries adds a run's test contribution back to the
// summary tables. Callers must invoke it only when removing an existing
// InfraFailure label inside the same row-locked transaction as that removal.
func RestoreInfraFailureToSummaries(tx *gorm.DB, prowJobRunID int64, partKeys query.ProwJobRunPartitionKeys) error {
	if err := tx.Exec("DROP TABLE IF EXISTS infra_failure_deltas").Error; err != nil {
		return fmt.Errorf("dropping stale infra_failure_deltas temp table for prow_job_run %d: %w", prowJobRunID, err)
	}
	if err := tx.Exec(restoreInfraFailureDeltasSQL, prowJobRunID, partKeys.ProwJobRelease, partKeys.Timestamp).Error; err != nil {
		return fmt.Errorf("materializing restore deltas for prow_job_run %d: %w", prowJobRunID, err)
	}
	if err := tx.Exec(`
UPDATE test_daily_totals dt SET
	successes = dt.successes + d.successes,
	failures = dt.failures + d.failures,
	flakes = dt.flakes + d.flakes,
	runs = dt.runs + d.runs
FROM infra_failure_deltas d
WHERE dt.release = d.release
	AND dt.date = d.date
	AND dt.test_id = d.test_id
	AND dt.suite_id = d.suite_id
	AND dt.lifecycle = d.lifecycle
	AND dt.prow_job_id = d.prow_job_id`).Error; err != nil {
		return fmt.Errorf("restoring daily totals for prow_job_run %d: %w", prowJobRunID, err)
	}
	if err := tx.Exec(`
UPDATE test_cumulative_summaries cs SET
	prefix_sum_successes = cs.prefix_sum_successes + d.successes,
	prefix_sum_failures = cs.prefix_sum_failures + d.failures,
	prefix_sum_flakes = cs.prefix_sum_flakes + d.flakes,
	prefix_sum_runs = cs.prefix_sum_runs + d.runs
FROM infra_failure_deltas d
WHERE cs.release = d.release
	AND cs.test_id = d.test_id
	AND cs.suite_id = d.suite_id
	AND cs.lifecycle = d.lifecycle
	AND cs.prow_job_id = d.prow_job_id
	AND cs.date >= d.date`).Error; err != nil {
		return fmt.Errorf("restoring cumulative summaries for prow_job_run %d: %w", prowJobRunID, err)
	}
	return nil
}

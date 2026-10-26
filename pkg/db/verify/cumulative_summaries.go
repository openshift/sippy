package verify

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"cloud.google.com/go/civil"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type CumulativeSummariesVerifier struct {
	PostgreSQL CumulativeSummariesReader
}

func (v *CumulativeSummariesVerifier) Verify(ctx context.Context, scope Scope) Result {
	result := Result{}
	for _, release := range scope.Releases {
		releaseStartedAt := time.Now()
		log.WithFields(log.Fields{
			"check":   CheckCumulativeSummaries,
			"date":    scope.Date,
			"release": release,
		}).Info("verification release started")
		discrepancyStart := len(result.Discrepancies)
		comparison, err := v.PostgreSQL.CumulativeComparison(ctx, release, scope.Date, func(discrepancy CumulativeComparisonDiscrepancy) {
			key := discrepancy.Key
			key.Release = release
			key.Date = scope.Date
			result.Discrepancies = append(result.Discrepancies, Discrepancy{
				Check:    CheckCumulativeSummaries,
				Release:  release,
				Date:     scope.Date,
				Kind:     discrepancy.Kind,
				Key:      key.String(),
				Field:    discrepancy.Field,
				Expected: discrepancy.Expected,
				Actual:   discrepancy.Actual,
				Detail:   discrepancy.Detail,
			})
		})
		if err != nil {
			result.Discrepancies = result.Discrepancies[:discrepancyStart]
			log.WithError(err).WithFields(log.Fields{
				"check":   CheckCumulativeSummaries,
				"date":    scope.Date,
				"elapsed": time.Since(releaseStartedAt),
				"release": release,
			}).Error("verification release failed")
			result.Summaries = append(result.Summaries, operationalSummary(CheckCumulativeSummaries, release, scope.Date, err))
			continue
		}
		releaseSummary := summary(CheckCumulativeSummaries, release, scope.Date, comparison.ExpectedRows, comparison.ActualRows, comparison.Discrepancies)
		log.WithFields(log.Fields{
			"check":         CheckCumulativeSummaries,
			"date":          scope.Date,
			"elapsed":       time.Since(releaseStartedAt),
			"discrepancies": comparison.Discrepancies,
			"expectedRows":  comparison.ExpectedRows,
			"release":       release,
			"actualRows":    comparison.ActualRows,
		}).Info("verification release complete")
		result.Summaries = append(result.Summaries, releaseSummary)
	}
	return result
}

// CumulativeComparison compares the expected and stored cumulative rows for a
// release and date without materializing the source row sets in Go.
func (p *PostgreSQL) CumulativeComparison(ctx context.Context, release string, date civil.Date, handle CumulativeComparisonHandler) (CumulativeComparison, error) {
	if err := p.validate(); err != nil {
		return CumulativeComparison{}, err
	}
	if handle == nil {
		return CumulativeComparison{}, fmt.Errorf("cumulative comparison handler is required")
	}
	var result CumulativeComparison
	queryStartedAt := time.Now()
	err := p.dbc.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rows, err := tx.Raw(cumulativeComparisonQuery, date.AddDays(-1), release, date, release, date, release).Rows()
		if err != nil {
			return fmt.Errorf("querying cumulative comparison: %w", err)
		}
		result, err = streamCumulativeComparisonRows(rows, handle)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return CumulativeComparison{}, fmt.Errorf("comparing cumulative summaries for release %s: %w", release, err)
	}
	log.WithFields(log.Fields{
		"actualRows":    result.ActualRows,
		"check":         CheckCumulativeSummaries,
		"discrepancies": result.Discrepancies,
		"elapsed":       time.Since(queryStartedAt),
		"expectedRows":  result.ExpectedRows,
		"query":         "cumulative-comparison",
		"release":       release,
	}).Info("verification PostgreSQL query complete")
	return result, nil
}

func streamCumulativeComparisonRows(rows *sql.Rows, handle CumulativeComparisonHandler) (result CumulativeComparison, err error) {
	defer func() {
		if closeErr := rows.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("closing cumulative comparison rows: %w", closeErr)
		}
	}()

	summarySeen := false
	for rows.Next() {
		var rowType string
		var expectedRows, actualRows sql.NullInt64
		var testID, prowJobID, suiteID sql.NullInt64
		var lifecycle, kind, field, expected, actual, detail sql.NullString
		if scanErr := rows.Scan(
			&rowType, &expectedRows, &actualRows,
			&testID, &prowJobID, &suiteID, &lifecycle,
			&kind, &field, &expected, &actual, &detail,
		); scanErr != nil {
			return CumulativeComparison{}, fmt.Errorf("scanning cumulative comparison row: %w", scanErr)
		}
		switch rowType {
		case "summary":
			if !expectedRows.Valid || !actualRows.Valid {
				return CumulativeComparison{}, fmt.Errorf("cumulative comparison summary is missing row counts")
			}
			result.ExpectedRows = int(expectedRows.Int64)
			result.ActualRows = int(actualRows.Int64)
			summarySeen = true
		case "discrepancy":
			if !testID.Valid || !prowJobID.Valid || !suiteID.Valid || !lifecycle.Valid || !kind.Valid || !field.Valid || !expected.Valid || !actual.Valid || !detail.Valid {
				return CumulativeComparison{}, fmt.Errorf("cumulative comparison discrepancy is missing fields")
			}
			testIDValue, err := parseCumulativeID(testID)
			if err != nil {
				return CumulativeComparison{}, fmt.Errorf("parsing cumulative test ID: %w", err)
			}
			prowJobIDValue, err := parseCumulativeID(prowJobID)
			if err != nil {
				return CumulativeComparison{}, fmt.Errorf("parsing cumulative Prow job ID: %w", err)
			}
			suiteIDValue, err := parseCumulativeID(suiteID)
			if err != nil {
				return CumulativeComparison{}, fmt.Errorf("parsing cumulative suite ID: %w", err)
			}
			handle(CumulativeComparisonDiscrepancy{
				Key: SummaryKey{
					TestID: testIDValue, ProwJobID: prowJobIDValue,
					SuiteID: suiteIDValue, Lifecycle: lifecycle.String,
				},
				Kind: kind.String, Field: field.String, Expected: expected.String, Actual: actual.String, Detail: detail.String,
			})
			result.Discrepancies++
		default:
			return CumulativeComparison{}, fmt.Errorf("unknown cumulative comparison row type %q", rowType)
		}
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return CumulativeComparison{}, fmt.Errorf("reading cumulative comparison rows: %w", rowsErr)
	}
	if !summarySeen {
		return CumulativeComparison{}, fmt.Errorf("cumulative comparison did not return a summary")
	}
	return result, nil
}

func parseCumulativeID(value sql.NullInt64) (uint64, error) {
	if !value.Valid {
		return 0, fmt.Errorf("ID is NULL")
	}
	id, err := strconv.ParseUint(strconv.FormatInt(value.Int64, 10), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid ID %d: %w", value.Int64, err)
	}
	return id, nil
}

// cumulativeComparisonQuery compares the expected and target row sets once,
// then returns one summary row and every discrepancy row.
const cumulativeComparisonQuery = `
	WITH previous AS (
		SELECT test_id, prow_job_id, suite_id, lifecycle,
		       prefix_sum_successes AS successes, prefix_sum_failures AS failures,
		       prefix_sum_flakes AS flakes, prefix_sum_runs AS runs
		FROM test_cumulative_summaries
		WHERE date = ? AND release = ?
	), daily AS (
		SELECT test_id, prow_job_id, suite_id, lifecycle, successes, failures, flakes, runs
		FROM test_daily_totals
		WHERE date = ? AND release = ?
	), expected AS (
		SELECT COALESCE(previous.test_id, daily.test_id) AS test_id,
		       COALESCE(previous.prow_job_id, daily.prow_job_id) AS prow_job_id,
		       COALESCE(previous.suite_id, daily.suite_id) AS suite_id,
		       COALESCE(previous.lifecycle, daily.lifecycle) AS lifecycle,
		       COALESCE(previous.successes, 0) + COALESCE(daily.successes, 0) AS successes,
		       COALESCE(previous.failures, 0) + COALESCE(daily.failures, 0) AS failures,
		       COALESCE(previous.flakes, 0) + COALESCE(daily.flakes, 0) AS flakes,
		       COALESCE(previous.runs, 0) + COALESCE(daily.runs, 0) AS runs
		FROM previous
		FULL OUTER JOIN daily USING (test_id, prow_job_id, suite_id, lifecycle)
	), target AS (
		SELECT test_id, prow_job_id, suite_id, lifecycle,
		       prefix_sum_successes AS successes, prefix_sum_failures AS failures,
		       prefix_sum_flakes AS flakes, prefix_sum_runs AS runs
		FROM test_cumulative_summaries
		WHERE date = ? AND release = ?
	), comparison AS MATERIALIZED (
		SELECT COALESCE(expected.test_id, target.test_id) AS test_id,
		       COALESCE(expected.prow_job_id, target.prow_job_id) AS prow_job_id,
		       COALESCE(expected.suite_id, target.suite_id) AS suite_id,
		       COALESCE(expected.lifecycle, target.lifecycle) AS lifecycle,
		       expected.test_id IS NOT NULL AS expected_present,
		       target.test_id IS NOT NULL AS actual_present,
		       expected.successes AS expected_successes, target.successes AS actual_successes,
		       expected.failures AS expected_failures, target.failures AS actual_failures,
		       expected.flakes AS expected_flakes, target.flakes AS actual_flakes,
		       expected.runs AS expected_runs, target.runs AS actual_runs
		FROM expected
		FULL OUTER JOIN target USING (test_id, prow_job_id, suite_id, lifecycle)
	), counts AS (
		SELECT COUNT(*) FILTER (WHERE expected_present) AS expected_rows,
		       COUNT(*) FILTER (WHERE actual_present) AS actual_rows
		FROM comparison
	), differences AS (
		SELECT comparison.test_id, comparison.prow_job_id, comparison.suite_id, comparison.lifecycle,
		       difference.kind, difference.field, difference.expected, difference.actual, ''::text AS detail
		FROM comparison
		CROSS JOIN LATERAL (
			SELECT 'missing-row'::text AS kind, ''::text AS field, 'present'::text AS expected, 'missing'::text AS actual
			WHERE comparison.expected_present AND NOT comparison.actual_present
			UNION ALL
			SELECT 'unexpected-row'::text, ''::text, 'missing'::text, 'present'::text
			WHERE NOT comparison.expected_present AND comparison.actual_present
			  AND (comparison.actual_successes <> 0 OR comparison.actual_failures <> 0
			       OR comparison.actual_flakes <> 0 OR comparison.actual_runs <> 0)
			UNION ALL
			SELECT 'count-mismatch'::text, mismatch.field, mismatch.expected_count::text, mismatch.actual_count::text
			FROM (VALUES
				('successes', comparison.expected_successes, comparison.actual_successes),
				('failures', comparison.expected_failures, comparison.actual_failures),
				('flakes', comparison.expected_flakes, comparison.actual_flakes),
				('runs', comparison.expected_runs, comparison.actual_runs)
			) AS mismatch(field, expected_count, actual_count)
			WHERE comparison.expected_present AND comparison.actual_present
			  AND mismatch.expected_count <> mismatch.actual_count
		) AS difference
	)
	SELECT 'summary'::text AS row_type, counts.expected_rows, counts.actual_rows,
	       NULL::bigint AS test_id, NULL::bigint AS prow_job_id, NULL::bigint AS suite_id,
	       NULL::text AS lifecycle, NULL::text AS kind, NULL::text AS field,
	       NULL::text AS expected, NULL::text AS actual, NULL::text AS detail
	FROM counts
	UNION ALL
	SELECT 'discrepancy'::text, NULL::bigint, NULL::bigint,
	       differences.test_id, differences.prow_job_id, differences.suite_id,
	       differences.lifecycle, differences.kind, differences.field,
	       differences.expected, differences.actual, differences.detail
	FROM differences
`

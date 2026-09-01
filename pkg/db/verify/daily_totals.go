package verify

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/civil"
)

type DailyTotalsVerifier struct {
	PostgreSQL DailyTotalsReader
}

func (v *DailyTotalsVerifier) Verify(ctx context.Context, scope Scope) Result {
	result := Result{}
	for _, release := range scope.Releases {
		raw, stored, err := v.PostgreSQL.DailyRows(ctx, release, scope.Date)
		if err != nil {
			result.Summaries = append(result.Summaries, operationalSummary(CheckDailyTotals, release, scope.Date, err))
			continue
		}
		summary, discrepancies := CompareDaily(release, scope.Date, raw, stored)
		result.Summaries = append(result.Summaries, summary)
		result.Discrepancies = append(result.Discrepancies, discrepancies...)
	}
	return result
}

func (p *PostgreSQL) DailyRows(ctx context.Context, release string, date civil.Date) ([]DailyRow, []DailyRow, error) {
	if err := p.validate(); err != nil {
		return nil, nil, err
	}
	start := date.In(time.UTC)
	end := date.AddDays(1).In(time.UTC)
	raw, err := p.queryDaily(ctx, `
		SELECT
			pjrt.test_id,
			pjrt.prow_job_id,
			COALESCE(pjrt.suite_id, 0) AS suite_id,
			pjrt.lifecycle,
			COUNT(*) FILTER (WHERE pjrt.status = 1) AS successes,
			COUNT(*) FILTER (WHERE pjrt.status = 12) AS failures,
			COUNT(*) FILTER (WHERE pjrt.status = 13) AS flakes,
			COUNT(*) AS runs
		FROM prow_job_run_tests pjrt
		JOIN prow_job_runs pjr
		  ON pjr.id = pjrt.prow_job_run_id
		 AND pjr.prow_job_release = pjrt.prow_job_run_release
		 AND pjr.timestamp = pjrt.prow_job_run_timestamp
		WHERE pjrt.prow_job_run_timestamp >= ?
		  AND pjrt.prow_job_run_timestamp < ?
		  AND pjrt.prow_job_run_release = ?
		  AND (pjr.labels IS NULL OR NOT (pjr.labels @> ARRAY['InfraFailure']))
		GROUP BY pjrt.test_id, pjrt.prow_job_id, COALESCE(pjrt.suite_id, 0), pjrt.lifecycle
		ORDER BY pjrt.test_id, pjrt.prow_job_id, COALESCE(pjrt.suite_id, 0), pjrt.lifecycle
	`, start, end, release)
	if err != nil {
		return nil, nil, fmt.Errorf("querying raw daily totals for release %s: %w", release, err)
	}
	summaryRows, err := p.queryDaily(ctx, `
		SELECT test_id, prow_job_id, suite_id, lifecycle, successes, failures, flakes, runs
		FROM test_daily_totals
		WHERE date = ? AND release = ?
		ORDER BY test_id, prow_job_id, suite_id, lifecycle
	`, date, release)
	if err != nil {
		return nil, nil, fmt.Errorf("querying stored daily totals for release %s: %w", release, err)
	}
	return raw, summaryRows, nil
}

func (p *PostgreSQL) queryDaily(ctx context.Context, query string, args ...any) ([]DailyRow, error) {
	type row struct {
		TestID    uint64
		ProwJobID uint64
		SuiteID   uint64
		Lifecycle string
		Successes int64
		Failures  int64
		Flakes    int64
		Runs      int64
	}
	var rows []row
	if err := p.dbc.DB.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]DailyRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, DailyRow{
			Key:    SummaryKey{TestID: row.TestID, ProwJobID: row.ProwJobID, SuiteID: row.SuiteID, Lifecycle: row.Lifecycle},
			Counts: Counts{Successes: row.Successes, Failures: row.Failures, Flakes: row.Flakes, Runs: row.Runs},
		})
	}
	return result, nil
}

func CompareDaily(release string, date civil.Date, rawRows, summaryRows []DailyRow) (Summary, []Discrepancy) {
	return compareCounts(CheckDailyTotals, release, date, rawRows, summaryRows)
}

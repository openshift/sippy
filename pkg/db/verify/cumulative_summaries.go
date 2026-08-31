package verify

import (
	"context"
	"fmt"

	"cloud.google.com/go/civil"
)

type CumulativeSummariesVerifier struct {
	PostgreSQL CumulativeSummariesReader
}

func (v *CumulativeSummariesVerifier) Verify(ctx context.Context, scope Scope) Result {
	result := Result{}
	for _, release := range scope.Releases {
		rows, err := v.PostgreSQL.CumulativeRows(ctx, release, scope.Date)
		if err != nil {
			result.Summaries = append(result.Summaries, operationalSummary(CheckCumulativeSummaries, release, scope.Date, err))
			continue
		}
		summary, discrepancies := CompareCumulative(release, scope.Date, rows)
		result.Summaries = append(result.Summaries, summary)
		result.Discrepancies = append(result.Discrepancies, discrepancies...)
	}
	return result
}

func (p *PostgreSQL) CumulativeRows(ctx context.Context, release string, date civil.Date) (CumulativeRows, error) {
	if err := p.validate(); err != nil {
		return CumulativeRows{}, err
	}
	type row struct {
		Source    string
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
	err := p.dbc.DB.WithContext(ctx).Raw(`
		SELECT 'previous' AS source, test_id, prow_job_id, suite_id, lifecycle,
		       prefix_sum_successes AS successes, prefix_sum_failures AS failures,
		       prefix_sum_flakes AS flakes, prefix_sum_runs AS runs
		FROM test_cumulative_summaries
		WHERE date = ? AND release = ?
		UNION ALL
		SELECT 'daily' AS source, test_id, prow_job_id, suite_id, lifecycle,
		       successes, failures, flakes, runs
		FROM test_daily_totals
		WHERE date = ? AND release = ?
		UNION ALL
		SELECT 'target' AS source, test_id, prow_job_id, suite_id, lifecycle,
		       prefix_sum_successes AS successes, prefix_sum_failures AS failures,
		       prefix_sum_flakes AS flakes, prefix_sum_runs AS runs
		FROM test_cumulative_summaries
		WHERE date = ? AND release = ?
		ORDER BY source, test_id, prow_job_id, suite_id, lifecycle
	`, date.AddDays(-1), release, date, release, date, release).Scan(&rows).Error
	if err != nil {
		return CumulativeRows{}, fmt.Errorf("querying cumulative inputs for release %s: %w", release, err)
	}
	result := CumulativeRows{}
	for _, row := range rows {
		value := DailyRow{
			Key:    SummaryKey{TestID: row.TestID, ProwJobID: row.ProwJobID, SuiteID: row.SuiteID, Lifecycle: row.Lifecycle},
			Counts: Counts{Successes: row.Successes, Failures: row.Failures, Flakes: row.Flakes, Runs: row.Runs},
		}
		switch row.Source {
		case "previous":
			result.Previous = append(result.Previous, value)
		case "daily":
			result.Daily = append(result.Daily, value)
		case "target":
			result.Target = append(result.Target, value)
		}
	}
	return result, nil
}

func CompareCumulative(release string, date civil.Date, rows CumulativeRows) (Summary, []Discrepancy) {
	previous := rowsByKey(scopedRows(rows.Previous, release, date))
	daily := rowsByKey(scopedRows(rows.Daily, release, date))
	expected := make(map[SummaryKey]Counts, len(previous)+len(daily))
	for key, counts := range previous {
		expected[key] = counts
	}
	for key, counts := range daily {
		expected[key] = expected[key].Add(counts)
	}
	expectedRows := make([]DailyRow, 0, len(expected))
	for key, counts := range expected {
		expectedRows = append(expectedRows, DailyRow{Key: key, Counts: counts})
	}
	return compareCounts(CheckCumulativeSummaries, release, date, expectedRows, rows.Target)
}

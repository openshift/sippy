package verify

import (
	"fmt"
	"sort"

	"cloud.google.com/go/civil"
)

func compareCounts(check Check, release string, date civil.Date, expectedRows, actualRows []DailyRow) (Summary, []Discrepancy) {
	expectedRows = scopedRows(expectedRows, release, date)
	actualRows = scopedRows(actualRows, release, date)
	expected := rowsByKey(expectedRows)
	actual := rowsByKey(actualRows)
	discrepancies := make([]Discrepancy, 0)
	keys := unionKeys(expected, actual)
	for _, key := range keys {
		expectedCounts, hasExpected := expected[key]
		actualCounts, hasActual := actual[key]
		if !hasExpected {
			if actualCounts.IsZero() {
				// A row can be left behind with all-zero counts once its only
				// contributing run is subtracted out (e.g. an InfraFailure
				// exclusion applied after the row was first written). That is
				// indistinguishable from the row never having existed.
				continue
			}
			discrepancies = append(discrepancies, Discrepancy{
				Check: check, Release: release, Date: date, Kind: "unexpected-row", Key: key.String(),
				Expected: "missing", Actual: "present",
			})
			continue
		}
		if !hasActual {
			discrepancies = append(discrepancies, Discrepancy{
				Check: check, Release: release, Date: date, Kind: "missing-row", Key: key.String(),
				Expected: "present", Actual: "missing",
			})
			continue
		}
		fields := []struct {
			name     string
			expected int64
			actual   int64
		}{
			{"successes", expectedCounts.Successes, actualCounts.Successes},
			{"failures", expectedCounts.Failures, actualCounts.Failures},
			{"flakes", expectedCounts.Flakes, actualCounts.Flakes},
			{"runs", expectedCounts.Runs, actualCounts.Runs},
		}
		for _, field := range fields {
			if field.expected != field.actual {
				discrepancies = append(discrepancies, Discrepancy{
					Check: check, Release: release, Date: date, Kind: "count-mismatch", Key: key.String(), Field: field.name,
					Expected: fmt.Sprint(field.expected), Actual: fmt.Sprint(field.actual),
				})
			}
		}
	}
	return summary(check, release, date, len(expected), len(actual), len(discrepancies)), discrepancies
}

func scopedRows(rows []DailyRow, release string, date civil.Date) []DailyRow {
	result := make([]DailyRow, len(rows))
	copy(result, rows)
	for i := range result {
		result[i].Key.Release = release
		result[i].Key.Date = date
	}
	return result
}

func summary(check Check, release string, date civil.Date, expected, actual, discrepancyCount int) Summary {
	return Summary{
		Check: check, Release: release, Date: date, Passed: discrepancyCount == 0,
		ExpectedRows: expected, ActualRows: actual, Discrepancies: discrepancyCount,
	}
}

func rowsByKey(rows []DailyRow) map[SummaryKey]Counts {
	result := make(map[SummaryKey]Counts, len(rows))
	for _, row := range rows {
		result[row.Key] = row.Counts
	}
	return result
}

func unionKeys(a, b map[SummaryKey]Counts) []SummaryKey {
	set := make(map[SummaryKey]struct{}, len(a)+len(b))
	for key := range a {
		set[key] = struct{}{}
	}
	for key := range b {
		set[key] = struct{}{}
	}
	keys := make([]SummaryKey, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
	return keys
}

package verify

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1config "github.com/openshift/sippy/pkg/apis/config/v1"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
)

var testDate = civil.Date{Year: 2026, Month: 8, Day: 25}

func TestParseChecks(t *testing.T) {
	tests := []struct {
		name    string
		values  []string
		want    []Check
		wantErr string
	}{
		{name: "default all", want: AllChecks},
		{name: "one", values: []string{"daily-totals"}, want: []Check{CheckDailyTotals}},
		{name: "repeatable canonical order and deduplication", values: []string{"cumulative-summaries", "bq-completeness", "cumulative-summaries"}, want: []Check{CheckBQCompleteness, CheckCumulativeSummaries}},
		{name: "invalid", values: []string{"unknown"}, wantErr: `invalid --check "unknown"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseChecks(tt.values)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestContainsCheck(t *testing.T) {
	checks := []Check{CheckBQCompleteness, CheckCumulativeSummaries}
	assert.True(t, ContainsCheck(checks, CheckBQCompleteness))
	assert.False(t, ContainsCheck(checks, CheckDailyTotals))
	assert.False(t, ContainsCheck(nil, CheckDailyTotals))
}

func TestCompareBuildIDs(t *testing.T) {
	tests := []struct {
		name      string
		bq        map[BuildID]struct{}
		pg        map[BuildID]struct{}
		malformed []string
		wantKinds []string
	}{
		{name: "equal", bq: idSet(1, 2), pg: idSet(1, 2)},
		{name: "both directions", bq: idSet(1), pg: idSet(2), wantKinds: []string{"missing-in-bigquery", "missing-in-postgres"}},
		{name: "malformed", malformed: []string{" bad "}, wantKinds: []string{"malformed-build-id"}},
		{name: "blank and whitespace normalize and deduplicate", malformed: []string{"", "  ", "\t"}, wantKinds: []string{"missing-build-id"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary, discrepancies := CompareBuildIDs("4.20", testDate, tt.bq, tt.pg, tt.malformed)
			assert.Equal(t, len(tt.wantKinds) == 0, summary.Passed)
			assert.Equal(t, len(tt.wantKinds), summary.Discrepancies)
			kinds := make([]string, len(discrepancies))
			for i := range discrepancies {
				kinds[i] = discrepancies[i].Kind
			}
			assert.ElementsMatch(t, tt.wantKinds, kinds)
			if tt.name == "malformed" {
				require.Len(t, discrepancies, 1)
				assert.Equal(t, "bad", discrepancies[0].Key)
			}
		})
	}
}

func TestCompareDaily(t *testing.T) {
	key := SummaryKey{TestID: 1, ProwJobID: 2, SuiteID: 0, Lifecycle: "blocking"}
	base := DailyRow{Key: key, Counts: Counts{Successes: 3, Failures: 2, Flakes: 1, Runs: 7}}
	tests := []struct {
		name      string
		expected  []DailyRow
		actual    []DailyRow
		wantKind  string
		wantField string
	}{
		{name: "equal", expected: []DailyRow{base}, actual: []DailyRow{base}},
		{name: "raw only", expected: []DailyRow{base}, wantKind: "missing-row"},
		{name: "summary only", actual: []DailyRow{base}, wantKind: "unexpected-row"},
		{name: "zero-count summary only is not a discrepancy", actual: []DailyRow{{Key: key, Counts: Counts{}}}},
		{name: "success mismatch", expected: []DailyRow{base}, actual: []DailyRow{{Key: key, Counts: Counts{Successes: 4, Failures: 2, Flakes: 1, Runs: 7}}}, wantKind: "count-mismatch", wantField: "successes"},
		{name: "failure mismatch", expected: []DailyRow{base}, actual: []DailyRow{{Key: key, Counts: Counts{Successes: 3, Failures: 3, Flakes: 1, Runs: 7}}}, wantKind: "count-mismatch", wantField: "failures"},
		{name: "flake mismatch", expected: []DailyRow{base}, actual: []DailyRow{{Key: key, Counts: Counts{Successes: 3, Failures: 2, Flakes: 2, Runs: 7}}}, wantKind: "count-mismatch", wantField: "flakes"},
		{name: "run mismatch", expected: []DailyRow{base}, actual: []DailyRow{{Key: key, Counts: Counts{Successes: 3, Failures: 2, Flakes: 1, Runs: 8}}}, wantKind: "count-mismatch", wantField: "runs"},
		{name: "lifecycle is in key", expected: []DailyRow{base}, actual: []DailyRow{{Key: SummaryKey{TestID: 1, ProwJobID: 2, SuiteID: 0, Lifecycle: "informing"}, Counts: base.Counts}}, wantKind: "missing-row"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary, discrepancies := CompareDaily("4.20", testDate, tt.expected, tt.actual)
			if tt.wantKind == "" {
				assert.True(t, summary.Passed)
				assert.Empty(t, discrepancies)
				return
			}
			assert.False(t, summary.Passed)
			require.NotEmpty(t, discrepancies)
			assert.Equal(t, tt.wantKind, discrepancies[0].Kind)
			assert.Equal(t, tt.wantField, discrepancies[0].Field)
		})
	}
}

func TestCountsIsZero(t *testing.T) {
	assert.True(t, Counts{}.IsZero())
	assert.False(t, Counts{Successes: 1}.IsZero())
	assert.False(t, Counts{Failures: 1}.IsZero())
	assert.False(t, Counts{Flakes: 1}.IsZero())
	assert.False(t, Counts{Runs: 1}.IsZero())
}

func TestCompareCumulative(t *testing.T) {
	key := SummaryKey{TestID: 1, ProwJobID: 2, SuiteID: 3, Lifecycle: "blocking"}
	previous := DailyRow{Key: key, Counts: Counts{Successes: 10, Failures: 2, Flakes: 1, Runs: 13}}
	daily := DailyRow{Key: key, Counts: Counts{Successes: 2, Failures: 1, Runs: 3}}
	tests := []struct {
		name string
		rows CumulativeRows
		pass bool
	}{
		{name: "first day", rows: CumulativeRows{Daily: []DailyRow{daily}, Target: []DailyRow{daily}}, pass: true},
		{name: "accumulation", rows: CumulativeRows{Previous: []DailyRow{previous}, Daily: []DailyRow{daily}, Target: []DailyRow{{Key: key, Counts: previous.Counts.Add(daily.Counts)}}}, pass: true},
		{name: "carry forward", rows: CumulativeRows{Previous: []DailyRow{previous}, Target: []DailyRow{previous}}, pass: true},
		{name: "broken carry forward", rows: CumulativeRows{Previous: []DailyRow{previous}}, pass: false},
		{name: "daily only missing target", rows: CumulativeRows{Daily: []DailyRow{daily}}, pass: false},
		{name: "unexpected target", rows: CumulativeRows{Target: []DailyRow{daily}}, pass: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary, discrepancies := CompareCumulative("4.20", testDate, tt.rows)
			assert.Equal(t, tt.pass, summary.Passed)
			assert.Equal(t, tt.pass, len(discrepancies) == 0)
		})
	}
}

func TestRunnerRunsAllSelectedChecksAfterFailures(t *testing.T) {
	pg := &fakePostgreSQL{
		releases:   []string{"pseudo", "4.20", "pseudo"},
		dailyErr:   map[string]error{"4.20": errors.New("daily unavailable")},
		cumulative: map[string]CumulativeRows{},
	}
	bq := &fakeBigQuery{jobs: []BQJob{{JobName: "job", BuildID: "bad"}}}
	runner := Runner{
		PostgreSQL: pg,
		BigQuery:   bq,
		Config: &v1config.SippyConfig{Releases: map[string]v1config.ReleaseConfig{
			"4.20": {Jobs: map[string]bool{"job": true}},
		}},
	}
	result := runner.Run(context.Background(), Options{Date: testDate, Checks: AllChecks})
	assert.False(t, result.Passed())
	assert.Equal(t, 1, bq.calls, "BQ is queried once globally")
	assert.Equal(t, []string{"4.20", "pseudo"}, pg.runIDCalls, "PostgreSQL completeness is queried once per release")
	assert.Equal(t, []string{"4.20", "pseudo"}, pg.dailyCalls)
	assert.Equal(t, []string{"4.20", "pseudo"}, pg.cumulativeCalls)
	assert.Len(t, result.Summaries, 6, "one summary per check and release")
	assert.True(t, sortIsStable(result.Discrepancies))
}

func TestRunnerBQInitializationFailureDoesNotSkipPostgreSQLChecks(t *testing.T) {
	pg := &fakePostgreSQL{releases: []string{"4.20"}, cumulative: map[string]CumulativeRows{}}
	runner := Runner{PostgreSQL: pg, BigQueryInitializationError: errors.New("credentials missing")}
	result := runner.Run(context.Background(), Options{Date: testDate, Checks: AllChecks})
	require.Len(t, result.Summaries, 3)
	assert.Equal(t, "credentials missing", result.Summaries[0].Error)
	assert.Equal(t, []string{"4.20"}, pg.dailyCalls)
	assert.Equal(t, []string{"4.20"}, pg.cumulativeCalls)
}

func TestRunnerDispatchesOnlySelectedChecks(t *testing.T) {
	tests := []struct {
		name                string
		check               Check
		wantBQCalls         int
		wantRunIDCalls      []string
		wantDailyCalls      []string
		wantCumulativeCalls []string
	}{
		{
			name:  "BigQuery completeness",
			check: CheckBQCompleteness, wantBQCalls: 1, wantRunIDCalls: []string{"4.20", "pseudo"},
		},
		{
			name:  "daily totals",
			check: CheckDailyTotals, wantDailyCalls: []string{"4.20", "pseudo"},
		},
		{
			name:  "cumulative summaries",
			check: CheckCumulativeSummaries, wantCumulativeCalls: []string{"4.20", "pseudo"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pg := &fakePostgreSQL{releases: []string{"pseudo", "4.20"}}
			bq := &fakeBigQuery{}
			runner := Runner{PostgreSQL: pg, BigQuery: bq}

			result := runner.Run(context.Background(), Options{Date: testDate, Checks: []Check{tt.check}})

			assert.True(t, result.Passed())
			assert.Len(t, result.Summaries, 2)
			assert.Equal(t, tt.wantBQCalls, bq.calls)
			assert.Equal(t, tt.wantRunIDCalls, pg.runIDCalls)
			assert.Equal(t, tt.wantDailyCalls, pg.dailyCalls)
			assert.Equal(t, tt.wantCumulativeCalls, pg.cumulativeCalls)
		})
	}
}

func TestRunnerNormalizesAndDeduplicatesBQBuildIDs(t *testing.T) {
	pg := &fakePostgreSQL{
		releases: []string{"4.20"},
		runIDs:   map[string]map[BuildID]struct{}{"4.20": idSet(1)},
	}
	bq := &fakeBigQuery{jobs: []BQJob{
		{JobName: "job", BuildID: "1"},
		{JobName: "job", BuildID: "001"},
		{JobName: "job", BuildID: " bad "},
		{JobName: "job", BuildID: " bad "},
	}}
	runner := Runner{
		PostgreSQL: pg,
		BigQuery:   bq,
		Config: &v1config.SippyConfig{Releases: map[string]v1config.ReleaseConfig{
			"4.20": {Jobs: map[string]bool{"job": true}},
		}},
	}
	result := runner.Run(context.Background(), Options{Date: testDate, Checks: []Check{CheckBQCompleteness}})
	require.Len(t, result.Summaries, 1)
	assert.Equal(t, 1, result.Summaries[0].ExpectedRows)
	assert.Equal(t, 1, result.Summaries[0].ActualRows)
	assert.Equal(t, 1, result.Summaries[0].Discrepancies)
	require.Len(t, result.Discrepancies, 1)
	assert.Equal(t, "malformed-build-id", result.Discrepancies[0].Kind)
	assert.Equal(t, "bad", result.Discrepancies[0].Key)
	assert.Equal(t, time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC), bq.start)
	assert.Equal(t, time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC), bq.end)
	assert.Equal(t, []string{"4.20"}, pg.runIDCalls)
}

func TestRunnerBQCompletenessContinuesAfterReleaseReadError(t *testing.T) {
	pg := &fakePostgreSQL{
		releases: []string{"pseudo", "4.20"},
		runIDs:   map[string]map[BuildID]struct{}{"pseudo": idSet(2)},
		runIDErr: map[string]error{"4.20": errors.New("release unavailable")},
	}
	bq := &fakeBigQuery{jobs: []BQJob{{JobName: "pseudo-job", BuildID: "2"}}}
	runner := Runner{
		PostgreSQL: pg,
		BigQuery:   bq,
		Config: &v1config.SippyConfig{Releases: map[string]v1config.ReleaseConfig{
			"pseudo": {Jobs: map[string]bool{"pseudo-job": true}},
		}},
	}

	result := runner.Run(context.Background(), Options{Date: testDate, Checks: []Check{CheckBQCompleteness}})

	assert.False(t, result.Passed())
	assert.Equal(t, 1, bq.calls, "BQ is queried once globally")
	assert.Equal(t, []string{"4.20", "pseudo"}, pg.runIDCalls)
	require.Len(t, result.Summaries, 2)
	assert.Equal(t, "4.20", result.Summaries[0].Release)
	assert.ErrorContains(t, errors.New(result.Summaries[0].Error), "release unavailable")
	assert.Equal(t, "pseudo", result.Summaries[1].Release)
	assert.True(t, result.Summaries[1].Passed, "other releases are still compared")
	assert.Empty(t, result.Discrepancies)
}

func TestRunnerReleaseSelection(t *testing.T) {
	pg := &fakePostgreSQL{releases: []string{"4.20", "pseudo"}}
	runner := Runner{PostgreSQL: pg}
	result := runner.Run(context.Background(), Options{Date: testDate, Checks: []Check{CheckDailyTotals}, Release: "pseudo"})
	assert.True(t, result.Passed())
	assert.Equal(t, []string{"pseudo"}, pg.dailyCalls)

	missing := runner.Run(context.Background(), Options{Date: testDate, Checks: []Check{CheckDailyTotals}, Release: "unknown"})
	assert.False(t, missing.Passed())
	require.Len(t, missing.Summaries, 1)
	assert.ErrorContains(t, errors.New(missing.Summaries[0].Error), "was not found")

	bq := &fakeBigQuery{}
	bqResult := (&Runner{PostgreSQL: pg, BigQuery: bq}).Run(context.Background(), Options{
		Date: testDate, Checks: []Check{CheckBQCompleteness}, Release: "pseudo",
	})
	assert.True(t, bqResult.Passed())
	assert.Equal(t, 1, bq.calls)
	assert.Equal(t, []string{"pseudo"}, pg.runIDCalls)
}

func TestResultSortAndFields(t *testing.T) {
	result := Result{
		Summaries: []Summary{
			{Check: CheckDailyTotals, Release: "z", Date: testDate, Passed: true},
			{Check: CheckBQCompleteness, Release: "a", Date: testDate, Passed: true},
		},
		Discrepancies: []Discrepancy{
			{Check: CheckDailyTotals, Release: "z", Date: testDate, Kind: "z"},
			{Check: CheckDailyTotals, Release: "a", Date: testDate, Kind: "a"},
		},
	}
	result.Sort()
	assert.Equal(t, CheckBQCompleteness, result.Summaries[0].Check)
	assert.Equal(t, "a", result.Discrepancies[0].Release)
	assert.Equal(t, "daily-totals", string(result.Summaries[1].Fields()["check"].(Check)))
	assert.Contains(t, result.Discrepancies[0].Fields(), "expected")
	assert.True(t, result.Passed())
	result.Summaries[0].Passed = false
	assert.False(t, result.Passed())
}

func TestReadersRejectNilClients(t *testing.T) {
	ctx := context.Background()
	start := testDate.In(time.UTC)
	end := testDate.AddDays(1).In(time.UTC)

	postgresReaders := []struct {
		name string
		call func(*PostgreSQL) error
	}{
		{name: "releases", call: func(p *PostgreSQL) error { _, err := p.Releases(ctx); return err }},
		{name: "Prow job run IDs", call: func(p *PostgreSQL) error { _, err := p.ProwJobRunIDs(ctx, "4.20", start, end); return err }},
		{name: "daily rows", call: func(p *PostgreSQL) error { _, _, err := p.DailyRows(ctx, "4.20", testDate); return err }},
		{name: "cumulative rows", call: func(p *PostgreSQL) error { _, err := p.CumulativeRows(ctx, "4.20", testDate); return err }},
	}
	for _, tt := range postgresReaders {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorContains(t, tt.call(NewPostgreSQL(nil)), "PostgreSQL client is not initialized")
			require.ErrorContains(t, tt.call(nil), "PostgreSQL client is not initialized")
		})
	}

	bigQueryReaders := []*BigQuery{nil, NewBigQuery(nil), NewBigQuery(&bqcachedclient.Client{})}
	for _, reader := range bigQueryReaders {
		_, err := reader.ProwJobs(ctx, start, end)
		require.ErrorContains(t, err, "BigQuery client is not initialized")
	}
}

func idSet(ids ...BuildID) map[BuildID]struct{} {
	result := make(map[BuildID]struct{}, len(ids))
	for _, id := range ids {
		result[id] = struct{}{}
	}
	return result
}

func sortIsStable(discrepancies []Discrepancy) bool {
	copyOf := append([]Discrepancy(nil), discrepancies...)
	result := Result{Discrepancies: copyOf}
	result.Sort()
	return reflect.DeepEqual(discrepancies, result.Discrepancies)
}

type fakePostgreSQL struct {
	releases        []string
	releasesErr     error
	runIDs          map[string]map[BuildID]struct{}
	runIDErr        map[string]error
	runIDCalls      []string
	daily           map[string][2][]DailyRow
	dailyErr        map[string]error
	dailyCalls      []string
	cumulative      map[string]CumulativeRows
	cumulativeErr   map[string]error
	cumulativeCalls []string
}

func (f *fakePostgreSQL) Releases(context.Context) ([]string, error) {
	return f.releases, f.releasesErr
}

func (f *fakePostgreSQL) ProwJobRunIDs(_ context.Context, release string, _, _ time.Time) (map[BuildID]struct{}, error) {
	f.runIDCalls = append(f.runIDCalls, release)
	if err := f.runIDErr[release]; err != nil {
		return nil, err
	}
	return f.runIDs[release], nil
}

func (f *fakePostgreSQL) DailyRows(_ context.Context, release string, _ civil.Date) ([]DailyRow, []DailyRow, error) {
	f.dailyCalls = append(f.dailyCalls, release)
	if err := f.dailyErr[release]; err != nil {
		return nil, nil, err
	}
	rows := f.daily[release]
	return rows[0], rows[1], nil
}

func (f *fakePostgreSQL) CumulativeRows(_ context.Context, release string, _ civil.Date) (CumulativeRows, error) {
	f.cumulativeCalls = append(f.cumulativeCalls, release)
	return f.cumulative[release], f.cumulativeErr[release]
}

type fakeBigQuery struct {
	jobs  []BQJob
	err   error
	calls int
	start time.Time
	end   time.Time
}

func (f *fakeBigQuery) ProwJobs(_ context.Context, start, end time.Time) ([]BQJob, error) {
	f.calls++
	f.start = start
	f.end = end
	return f.jobs, f.err
}

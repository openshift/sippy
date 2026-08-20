package infrafailurebackfill

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/sippy/pkg/db/infrafailure"
)

func TestResolveStartDate(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		since   string
		days    int
		want    civil.Date
		wantErr bool
	}{
		{
			name:  "since takes precedence over days",
			since: "2026-07-01",
			days:  90,
			want:  civil.Date{Year: 2026, Month: time.July, Day: 1},
		},
		{
			name:    "invalid since returns error",
			since:   "not-a-date",
			days:    90,
			wantErr: true,
		},
		{
			name: "days lookback when since empty",
			days: 90,
			want: civil.Date{Year: 2026, Month: time.May, Day: 22}, // 2026-08-20 minus 90 days
		},
		{
			name: "single day lookback",
			days: 1,
			want: civil.Date{Year: 2026, Month: time.August, Day: 19},
		},
		{
			name:    "non-positive days returns error",
			days:    0,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveStartDate(tc.since, tc.days, now)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (result %s)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("resolveStartDate() = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestBuildInfraFailureQuery(t *testing.T) {
	startDate := civil.Date{Year: 2026, Month: time.July, Day: 1}
	sql, params := buildInfraFailureQuery("ci_analysis_us", startDate)

	wantSubstrings := []string{
		"SELECT DISTINCT prowjob_build_id",
		"FROM `ci_analysis_us.job_labels`",
		"WHERE label = @label",
		"DATE(prowjob_start) >= @startDate",
	}
	for _, sub := range wantSubstrings {
		if !strings.Contains(sql, sub) {
			t.Errorf("query missing substring %q:\n%s", sub, sql)
		}
	}

	if len(params) != 2 {
		t.Fatalf("expected 2 params, got %d: %+v", len(params), params)
	}
	byName := map[string]interface{}{}
	for _, p := range params {
		byName[p.Name] = p.Value
	}
	if byName["label"] != infrafailure.LabelInfraFailure {
		t.Errorf("label param = %v, want %q", byName["label"], infrafailure.LabelInfraFailure)
	}
	if byName["startDate"] != startDate {
		t.Errorf("startDate param = %v, want %v", byName["startDate"], startDate)
	}

	// Ensure the returned params are usable as BigQuery parameters (type check).
	var _ []bigquery.QueryParameter = params
}

func TestBatchIDs(t *testing.T) {
	tests := []struct {
		name string
		ids  []int64
		size int
		want [][]int64
	}{
		{
			name: "empty input",
			ids:  nil,
			size: 10,
			want: nil,
		},
		{
			name: "evenly divisible",
			ids:  []int64{1, 2, 3, 4},
			size: 2,
			want: [][]int64{{1, 2}, {3, 4}},
		},
		{
			name: "remainder batch",
			ids:  []int64{1, 2, 3, 4, 5},
			size: 2,
			want: [][]int64{{1, 2}, {3, 4}, {5}},
		},
		{
			name: "size larger than input",
			ids:  []int64{1, 2, 3},
			size: 10,
			want: [][]int64{{1, 2, 3}},
		},
		{
			name: "non-positive size falls back to default",
			ids:  []int64{1, 2, 3},
			size: 0,
			want: [][]int64{{1, 2, 3}}, // defaultBatchSize (100) yields one batch
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := batchIDs(tc.ids, tc.size)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("batchIDs() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestClassifyBatch(t *testing.T) {
	tests := []struct {
		name        string
		batch       []int64
		existing    []int64
		labeled     []int64
		wantToSync  []int64
		wantAlready []int64
		wantNotInPG []int64
	}{
		{
			name:        "mixed",
			batch:       []int64{1, 2, 3, 4},
			existing:    []int64{1, 2, 3},
			labeled:     []int64{1},
			wantToSync:  []int64{2, 3},
			wantAlready: []int64{1},
			wantNotInPG: []int64{4},
		},
		{
			name:        "all already labeled",
			batch:       []int64{1, 2},
			existing:    []int64{1, 2},
			labeled:     []int64{1, 2},
			wantAlready: []int64{1, 2},
		},
		{
			name:        "none in pg",
			batch:       []int64{7, 8},
			existing:    []int64{},
			labeled:     []int64{},
			wantNotInPG: []int64{7, 8},
		},
		{
			name:       "all to sync",
			batch:      []int64{1, 2},
			existing:   []int64{1, 2},
			labeled:    []int64{},
			wantToSync: []int64{1, 2},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status := batchLabelStatus{
				existing: sets.New[int64](tc.existing...),
				labeled:  sets.New[int64](tc.labeled...),
			}
			toSync, already, notInPG := classifyBatch(tc.batch, status)
			if !reflect.DeepEqual(toSync, tc.wantToSync) {
				t.Errorf("toSync = %v, want %v", toSync, tc.wantToSync)
			}
			if !reflect.DeepEqual(already, tc.wantAlready) {
				t.Errorf("alreadyLabeled = %v, want %v", already, tc.wantAlready)
			}
			if !reflect.DeepEqual(notInPG, tc.wantNotInPG) {
				t.Errorf("notInPG = %v, want %v", notInPG, tc.wantNotInPG)
			}
		})
	}
}

func TestProcessBatch(t *testing.T) {
	batch := []int64{1, 2, 3, 4}
	status := batchLabelStatus{
		existing: sets.New[int64](1, 2, 3),
		labeled:  sets.New[int64](1),
	}

	tests := []struct {
		name        string
		dryRun      bool
		failOnID    int64 // record fails for this id (0 = never)
		wantStats   Stats
		wantRecords []int64
	}{
		{
			name:        "sync missing runs",
			wantStats:   Stats{AlreadyLabeled: 1, NewlySynced: 2, NotFoundInPG: 1},
			wantRecords: []int64{2, 3},
		},
		{
			name:        "dry run records nothing",
			dryRun:      true,
			wantStats:   Stats{AlreadyLabeled: 1, NewlySynced: 2, NotFoundInPG: 1},
			wantRecords: nil,
		},
		{
			name:        "record error is counted and skipped",
			failOnID:    3,
			wantStats:   Stats{AlreadyLabeled: 1, NewlySynced: 1, NotFoundInPG: 1, Errors: 1},
			wantRecords: []int64{2, 3},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var recorded []int64
			b := &Backfiller{
				opts: Options{DryRun: tc.dryRun},
				findLabelStatus: func(_ context.Context, _ []int64) (batchLabelStatus, error) {
					return status, nil
				},
				recordInfraFailure: func(_ context.Context, id int64) error {
					recorded = append(recorded, id)
					if id == tc.failOnID {
						return errTest
					}
					return nil
				},
			}

			var stats Stats
			if err := b.processBatch(context.Background(), batch, &stats); err != nil {
				t.Fatalf("processBatch returned error: %v", err)
			}
			if stats != tc.wantStats {
				t.Errorf("stats = %+v, want %+v", stats, tc.wantStats)
			}
			if !reflect.DeepEqual(recorded, tc.wantRecords) {
				t.Errorf("recorded = %v, want %v", recorded, tc.wantRecords)
			}
		})
	}
}

func TestRun(t *testing.T) {
	var recorded []int64
	b := &Backfiller{
		opts: Options{Days: 30, BatchSize: 2},
		fetchInfraFailureIDs: func(_ context.Context, _ civil.Date) ([]int64, error) {
			return []int64{1, 2, 3, 4, 5}, nil
		},
		findLabelStatus: func(_ context.Context, _ []int64) (batchLabelStatus, error) {
			// id 1 already labeled; 2,3,4 present unlabeled; 5 not in PG.
			return batchLabelStatus{
				existing: sets.New[int64](1, 2, 3, 4),
				labeled:  sets.New[int64](1),
			}, nil
		},
		recordInfraFailure: func(_ context.Context, id int64) error {
			recorded = append(recorded, id)
			return nil
		},
	}

	stats, err := b.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	want := Stats{TotalBQRuns: 5, AlreadyLabeled: 1, NewlySynced: 3, NotFoundInPG: 1, Errors: 0}
	if *stats != want {
		t.Errorf("stats = %+v, want %+v", *stats, want)
	}

	sort.Slice(recorded, func(i, j int) bool { return recorded[i] < recorded[j] })
	if !reflect.DeepEqual(recorded, []int64{2, 3, 4}) {
		t.Errorf("recorded = %v, want [2 3 4]", recorded)
	}
}

// errTest is a sentinel error used by record-failure test cases.
var errTest = &testError{"boom"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

package bqjunit

import (
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/stretchr/testify/require"
)

func TestJUnitQueryBoundsModifiedTimeAndBuildIDs(t *testing.T) {
	since := time.Date(2026, time.May, 22, 19, 30, 0, 0, time.FixedZone("EDT", -4*60*60))
	until := since.Add(90 * time.Minute)
	buildIDs := []string{"123", "456"}
	for _, table := range []string{junitTable, junitPRTable} {
		t.Run(table, func(t *testing.T) {
			query, params, err := junitQuery("project.dataset", table, buildIDs, QueryMode{ModifiedSince: since, ModifiedUntil: until})
			require.NoError(t, err)
			require.Contains(t, query, "FROM `project.dataset."+table+"`")
			require.Contains(t, query, "modified_time >= DATETIME(@modified_since)")
			require.Contains(t, query, "modified_time < DATETIME(@modified_until)")
			require.Contains(t, query, "prowjob_build_id IN UNNEST(@build_ids)")
			require.Contains(t, query, "ORDER BY prowjob_build_id")
			require.NotContains(t, query, "123")
			require.Equal(t, []bigquery.QueryParameter{
				{Name: "modified_since", Value: since.UTC()},
				{Name: "modified_until", Value: until.UTC()},
				{Name: "build_ids", Value: buildIDs},
			}, params)
		})
	}
}

func TestJUnitQueryRejectsInvalidInputs(t *testing.T) {
	since := time.Date(2026, time.May, 22, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		dataset string
		table   string
		until   time.Time
		wantErr string
	}{
		{name: "empty window", dataset: "project.dataset", table: junitTable, until: since, wantErr: "since before until"},
		{name: "unsafe dataset", dataset: "project.dataset`", table: junitTable, until: since.Add(time.Hour), wantErr: "invalid BigQuery dataset"},
		{name: "unsupported table", dataset: "project.dataset", table: "other", until: since.Add(time.Hour), wantErr: "unsupported JUnit table"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := junitQuery(tt.dataset, tt.table, []string{"123"}, QueryMode{ModifiedSince: since, ModifiedUntil: tt.until})
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestSchemaQueryKeepsTableNameParameterized(t *testing.T) {
	query, params, err := schemaQuery("project.dataset", "junit_pr` OR TRUE")
	require.NoError(t, err)
	require.Contains(t, query, "`project.dataset.INFORMATION_SCHEMA.COLUMNS`")
	require.Contains(t, query, "table_name = @table_name")
	require.NotContains(t, query, "OR TRUE")
	require.Equal(t, []bigquery.QueryParameter{{Name: "table_name", Value: "junit_pr` OR TRUE"}}, params)

	_, _, err = schemaQuery("project.dataset`", junitTable)
	require.ErrorContains(t, err, "invalid BigQuery dataset")
}

func TestSupportedJUnitTables(t *testing.T) {
	first, second := Tables()
	require.Equal(t, junitTable, first)
	require.Equal(t, junitPRTable, second)
}

func TestNormalizeTable(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
	}{
		{name: "pull request table", input: " junit_pr ", want: junitPRTable},
		{name: "standard table", input: junitTable, want: junitTable},
		{name: "unknown table defaults to standard", input: "unexpected", want: junitTable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, NormalizeTable(tc.input))
		})
	}
}

func TestBQTestRowConversionPreservesNullableValues(t *testing.T) {
	row := bqTestRow{
		BuildID:        bigquery.NullString{StringVal: "123", Valid: true},
		TestName:       bigquery.NullString{StringVal: "test", Valid: true},
		Suite:          bigquery.NullString{StringVal: "suite", Valid: true},
		Success:        bigquery.NullInt64{Int64: 1, Valid: true},
		Skipped:        bigquery.NullBool{Bool: true, Valid: true},
		FlakeCount:     bigquery.NullInt64{Int64: 2, Valid: true},
		Duration:       bigquery.NullFloat64{Float64: 1.25, Valid: true},
		FailureContent: bigquery.NullString{StringVal: "failure", Valid: true},
		Lifecycle:      bigquery.NullString{StringVal: "blocking", Valid: true},
	}
	require.Equal(t, TestRow{
		BuildID: "123", TestName: "test", Suite: "suite", Success: 1, Skipped: true, FlakeCount: 2,
		Duration: 1.25, FailureContent: "failure", Lifecycle: "blocking", LifecyclePresent: true,
	}, row.testRow())

	row.Lifecycle = bigquery.NullString{}
	require.Equal(t, "", row.testRow().Lifecycle)
	require.False(t, row.testRow().LifecyclePresent)
}

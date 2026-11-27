package bqjunit

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"

	bqclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
)

const (
	junitTable   = "junit"
	junitPRTable = "junit_pr"
)

// Client reads the bounded BigQuery JUnit data needed by the historical loader.
type Client struct {
	client  *bqclient.Client
	dataset string
	schema  map[string]map[string]struct{}
}

// QueryMode bounds a JUnit lookup to the modified-time partitions that can
// contain the selected build IDs.
type QueryMode struct {
	ModifiedSince time.Time
	ModifiedUntil time.Time
	Table         string
}

// TestRow is the BigQuery representation consumed by the historical loader.
type TestRow struct {
	BuildID          string
	TestName         string
	Suite            string
	Success          int64
	Skipped          bool
	FlakeCount       int64
	Duration         float64
	FailureContent   string
	Lifecycle        string
	LifecyclePresent bool
}

// New creates a BigQuery JUnit reader using the configured dataset.
func New(client *bqclient.Client) (*Client, error) {
	if client == nil {
		return nil, fmt.Errorf("BigQuery client is required")
	}
	if err := bqclient.ValidateDatasetIdentifier(client.Dataset); err != nil {
		return nil, err
	}
	return &Client{client: client, dataset: client.Dataset}, nil
}

// InspectSchema validates the columns used by both JUnit tables before reads.
func (c *Client) InspectSchema(ctx context.Context) error {
	if c == nil || c.client == nil {
		return fmt.Errorf("BigQuery JUnit client is not configured")
	}
	c.schema = make(map[string]map[string]struct{}, 2)
	for _, table := range []string{junitTable, junitPRTable} {
		query, params, err := schemaQuery(c.dataset, table)
		if err != nil {
			return err
		}
		q := c.client.Query(ctx, bqlabel.ProwLoaderProwJobs, query)
		q.Parameters = params
		it, err := q.Read(ctx)
		if err != nil {
			return fmt.Errorf("reading %s schema: %w", table, err)
		}
		columns := make(map[string]struct{})
		for {
			var row struct {
				Name string `bigquery:"column_name"`
			}
			if err := it.Next(&row); err == iterator.Done {
				break
			} else if err != nil {
				return fmt.Errorf("reading %s schema row: %w", table, err)
			}
			columns[row.Name] = struct{}{}
		}
		for _, column := range requiredColumns {
			if _, ok := columns[column]; !ok {
				return fmt.Errorf("%s schema is missing required column %q", table, column)
			}
		}
		c.schema[table] = columns
	}
	return nil
}

// StreamRows reads JUnit rows ordered by build ID and invokes handler for each
// row, allowing callers to release one build's rows before reading the next.
func (c *Client) StreamRows(ctx context.Context, buildIDs []string, mode QueryMode, handler func(TestRow) error) error {
	if c == nil || c.client == nil {
		return fmt.Errorf("BigQuery JUnit client is not configured")
	}
	if len(buildIDs) == 0 {
		return nil
	}
	table := mode.Table
	if table == "" {
		table = junitTable
	}
	if table != junitTable && table != junitPRTable {
		return fmt.Errorf("unsupported JUnit table %q", table)
	}
	if c.schema == nil {
		return fmt.Errorf("inspect BigQuery schema before querying JUnit data")
	}
	query, params, err := junitQuery(c.dataset, table, buildIDs, mode)
	if err != nil {
		return err
	}
	q := c.client.Query(ctx, bqlabel.ProwLoaderProwJobs, query)
	q.Parameters = params
	it, err := q.Read(ctx)
	if err != nil {
		return fmt.Errorf("reading bounded JUnit query: %w", err)
	}
	for {
		var row bqTestRow
		if err := it.Next(&row); err == iterator.Done {
			break
		} else if err != nil {
			return fmt.Errorf("reading JUnit row: %w", err)
		}
		if err := handler(row.testRow()); err != nil {
			return fmt.Errorf("processing JUnit row: %w", err)
		}
	}
	return nil
}

// bqTestRow retains nullability until conversion, especially for lifecycle.
type bqTestRow struct {
	BuildID        bigquery.NullString  `bigquery:"prowjob_build_id"`
	TestName       bigquery.NullString  `bigquery:"test_name"`
	Suite          bigquery.NullString  `bigquery:"testsuite"`
	Success        bigquery.NullInt64   `bigquery:"success_val"`
	Skipped        bigquery.NullBool    `bigquery:"skipped"`
	FlakeCount     bigquery.NullInt64   `bigquery:"flake_count"`
	Duration       bigquery.NullFloat64 `bigquery:"duration"`
	FailureContent bigquery.NullString  `bigquery:"failure_content"`
	Lifecycle      bigquery.NullString  `bigquery:"lifecycle"`
}

// testRow preserves whether lifecycle was present so the loader can apply its
// normal default only when the source field was absent.
func (r bqTestRow) testRow() TestRow {
	return TestRow{BuildID: r.BuildID.StringVal, TestName: r.TestName.StringVal, Suite: r.Suite.StringVal, Success: r.Success.Int64, Skipped: r.Skipped.Bool, FlakeCount: r.FlakeCount.Int64, Duration: r.Duration.Float64, FailureContent: r.FailureContent.StringVal, Lifecycle: r.Lifecycle.StringVal, LifecyclePresent: r.Lifecycle.Valid}
}

var requiredColumns = []string{"prowjob_build_id", "test_name", "testsuite", "success_val", "skipped", "flake_count", "duration_ms", "failure_content", "lifecycle", "modified_time"}

// schemaQuery parameterizes the table name and limits inspection to the
// configured dataset's schema metadata.
func schemaQuery(dataset, table string) (string, []bigquery.QueryParameter, error) {
	if err := bqclient.ValidateDatasetIdentifier(dataset); err != nil {
		return "", nil, err
	}
	return fmt.Sprintf("SELECT column_name FROM `%s.INFORMATION_SCHEMA.COLUMNS` WHERE table_name = @table_name ORDER BY ordinal_position", dataset), []bigquery.QueryParameter{{Name: "table_name", Value: table}}, nil
}

// junitQuery restricts partition scans by modified time and build ID; only
// validated dataset and table identifiers are interpolated into SQL.
func junitQuery(dataset, table string, buildIDs []string, mode QueryMode) (string, []bigquery.QueryParameter, error) {
	if !mode.ModifiedSince.UTC().Before(mode.ModifiedUntil.UTC()) {
		return "", nil, fmt.Errorf("JUnit modified-time window must have since before until")
	}
	if err := bqclient.ValidateDatasetIdentifier(dataset); err != nil {
		return "", nil, err
	}
	if table != junitTable && table != junitPRTable {
		return "", nil, fmt.Errorf("unsupported JUnit table %q", table)
	}
	query := fmt.Sprintf("SELECT prowjob_build_id, test_name, testsuite, success_val, skipped, flake_count, duration_ms / 1000.0 AS duration, failure_content, lifecycle\nFROM `%s.%s`\nWHERE modified_time >= DATETIME(@modified_since)\n  AND modified_time < DATETIME(@modified_until)\n  AND prowjob_build_id IN UNNEST(@build_ids)\nORDER BY prowjob_build_id, modified_time, testsuite, test_name", dataset, table)
	return query, []bigquery.QueryParameter{{Name: "modified_since", Value: mode.ModifiedSince.UTC()}, {Name: "modified_until", Value: mode.ModifiedUntil.UTC()}, {Name: "build_ids", Value: buildIDs}}, nil
}

// Tables returns the supported JUnit table names for callers building batches.
func Tables() (string, string) {
	tables := []string{junitTable, junitPRTable}
	sort.Strings(tables)
	return tables[0], tables[1]
}

// NormalizeTable accepts only the two supported JUnit tables.
func NormalizeTable(table string) string {
	if strings.TrimSpace(table) == junitPRTable {
		return junitPRTable
	}
	return junitTable
}

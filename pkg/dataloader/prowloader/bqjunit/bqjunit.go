package bqjunit

import (
	"context"
	"fmt"
	"regexp"
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

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+(?:\.[A-Za-z0-9_-]+){0,2}$`)

// Client reads the bounded BigQuery JUnit data needed by the historical loader.
type Client struct {
	client  *bqclient.Client
	dataset string
	schema  map[string]map[string]struct{}
}

// QueryMode bounds a JUnit lookup to the modified-time partitions that can
// contain the selected build IDs.
type QueryMode struct {
	ModifiedSince          time.Time
	ModifiedUntil          time.Time
	DryRun                 bool
	Table                  string
	PreserveModifiedWindow bool
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

// QueryCost records the bytes processed by a bounded query.
type QueryCost struct {
	BytesProcessed          int64
	EstimatedBytesProcessed int64
}

// New creates a BigQuery JUnit reader using the configured dataset.
func New(client *bqclient.Client) (*Client, error) {
	if client == nil {
		return nil, fmt.Errorf("BigQuery client is required")
	}
	if err := validateIdentifier(client.Dataset, "dataset"); err != nil {
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
func (c *Client) StreamRows(ctx context.Context, buildIDs []string, mode QueryMode, handler func(TestRow) error) (QueryCost, error) {
	if c == nil || c.client == nil {
		return QueryCost{}, fmt.Errorf("BigQuery JUnit client is not configured")
	}
	if len(buildIDs) == 0 {
		return QueryCost{}, nil
	}
	table := mode.Table
	if table == "" {
		table = junitTable
	}
	if table != junitTable && table != junitPRTable {
		return QueryCost{}, fmt.Errorf("unsupported JUnit table %q", table)
	}
	if c.schema == nil {
		return QueryCost{}, fmt.Errorf("inspect BigQuery schema before querying JUnit data")
	}
	batches := []QueryMode{mode}
	if !mode.PreserveModifiedWindow {
		var err error
		batches, err = splitQueryMode(mode)
		if err != nil {
			return QueryCost{}, err
		}
	}
	var total QueryCost
	for _, batch := range batches {
		query, params, err := junitQuery(c.dataset, table, buildIDs, batch)
		if err != nil {
			return total, err
		}
		q := c.client.Query(ctx, bqlabel.ProwLoaderProwJobs, query)
		q.Parameters = params
		q.DryRun = true
		job, err := q.Run(ctx)
		if err != nil {
			return total, fmt.Errorf("estimating bounded JUnit query: %w", err)
		}
		estimate := bytesFromJob(job)
		total.EstimatedBytesProcessed += estimate
		if batch.DryRun {
			total.BytesProcessed += estimate
			continue
		}
		q = c.client.Query(ctx, bqlabel.ProwLoaderProwJobs, query)
		q.Parameters = params
		it, err := q.Read(ctx)
		if err != nil {
			return total, fmt.Errorf("reading bounded JUnit query: %w", err)
		}
		for {
			var row bqTestRow
			if err := it.Next(&row); err == iterator.Done {
				break
			} else if err != nil {
				return total, fmt.Errorf("reading JUnit row: %w", err)
			}
			if err := handler(row.testRow()); err != nil {
				return total, fmt.Errorf("processing JUnit row: %w", err)
			}
		}
		total.BytesProcessed += bytesFromJob(it.SourceJob())
	}
	return total, nil
}

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

func (r bqTestRow) testRow() TestRow {
	return TestRow{BuildID: r.BuildID.StringVal, TestName: r.TestName.StringVal, Suite: r.Suite.StringVal, Success: r.Success.Int64, Skipped: r.Skipped.Bool, FlakeCount: r.FlakeCount.Int64, Duration: r.Duration.Float64, FailureContent: r.FailureContent.StringVal, Lifecycle: r.Lifecycle.StringVal, LifecyclePresent: r.Lifecycle.Valid}
}

var requiredColumns = []string{"prowjob_build_id", "test_name", "testsuite", "success_val", "skipped", "flake_count", "duration_ms", "failure_content", "lifecycle", "modified_time"}

func schemaQuery(dataset, table string) (string, []bigquery.QueryParameter, error) {
	if err := validateIdentifier(dataset, "dataset"); err != nil {
		return "", nil, err
	}
	return fmt.Sprintf("SELECT column_name FROM `%s.INFORMATION_SCHEMA.COLUMNS` WHERE table_name = @table_name ORDER BY ordinal_position", dataset), []bigquery.QueryParameter{{Name: "table_name", Value: table}}, nil
}

func junitQuery(dataset, table string, buildIDs []string, mode QueryMode) (string, []bigquery.QueryParameter, error) {
	if !mode.ModifiedSince.UTC().Before(mode.ModifiedUntil.UTC()) {
		return "", nil, fmt.Errorf("JUnit modified-time window must have since before until")
	}
	if err := validateIdentifier(dataset, "dataset"); err != nil {
		return "", nil, err
	}
	if table != junitTable && table != junitPRTable {
		return "", nil, fmt.Errorf("unsupported JUnit table %q", table)
	}
	query := fmt.Sprintf("SELECT prowjob_build_id, test_name, testsuite, success_val, skipped, flake_count, duration_ms / 1000.0 AS duration, failure_content, lifecycle\nFROM `%s.%s`\nWHERE modified_time >= DATETIME(@modified_since)\n  AND modified_time < DATETIME(@modified_until)\n  AND prowjob_build_id IN UNNEST(@build_ids)\nORDER BY prowjob_build_id, modified_time, testsuite, test_name", dataset, table)
	return query, []bigquery.QueryParameter{{Name: "modified_since", Value: mode.ModifiedSince.UTC()}, {Name: "modified_until", Value: mode.ModifiedUntil.UTC()}, {Name: "build_ids", Value: buildIDs}}, nil
}

func splitQueryMode(mode QueryMode) ([]QueryMode, error) {
	since, until := mode.ModifiedSince.UTC(), mode.ModifiedUntil.UTC()
	if since.IsZero() || until.IsZero() || !since.Before(until) {
		return nil, fmt.Errorf("JUnit modified-time window must have non-zero since before until")
	}
	var batches []QueryMode
	for start := since; start.Before(until); {
		end := time.Date(start.Year(), start.Month(), start.Day()+1, 0, 0, 0, 0, time.UTC)
		if end.After(until) {
			end = until
		}
		batches = append(batches, QueryMode{ModifiedSince: start, ModifiedUntil: end, DryRun: mode.DryRun, Table: mode.Table})
		start = end
	}
	return batches, nil
}

func bytesFromJob(job *bigquery.Job) int64 {
	if job == nil || job.LastStatus() == nil || job.LastStatus().Statistics == nil {
		return 0
	}
	if stats, ok := job.LastStatus().Statistics.Details.(*bigquery.QueryStatistics); ok {
		return stats.TotalBytesProcessed
	}
	return 0
}

func validateIdentifier(value, kind string) error {
	if !identifierPattern.MatchString(value) {
		return fmt.Errorf("invalid BigQuery %s %q", kind, value)
	}
	return nil
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

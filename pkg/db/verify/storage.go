package verify

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/civil"

	"github.com/openshift/sippy/pkg/db"
)

type ReleaseReader interface {
	Releases(context.Context) ([]string, error)
}

type ProwJobRunReader interface {
	ProwJobRunIDs(context.Context, string, time.Time, time.Time) (map[BuildID]struct{}, error)
}

type DailyTotalsReader interface {
	DailyRows(context.Context, string, civil.Date) ([]DailyRow, []DailyRow, error)
}

type CumulativeSummariesReader interface {
	CumulativeComparisonReader
}

// CumulativeComparisonReader reads the bounded result of a database-side
// cumulative summary comparison.
type CumulativeComparisonReader interface {
	CumulativeComparison(context.Context, string, civil.Date, CumulativeComparisonHandler) (CumulativeComparison, error)
}

type PostgreSQLReader interface {
	ReleaseReader
	ProwJobRunReader
	DailyTotalsReader
	CumulativeSummariesReader
}

// CumulativeComparison is the bounded outcome returned by a database-side
// cumulative summary comparison.
type CumulativeComparison struct {
	ExpectedRows  int
	ActualRows    int
	Discrepancies int
}

// CumulativeComparisonDiscrepancy is a database-side cumulative comparison
// difference before it is scoped to the requested release and date.
type CumulativeComparisonDiscrepancy struct {
	Key      SummaryKey
	Kind     string
	Field    string
	Expected string
	Actual   string
	Detail   string
}

// CumulativeComparisonHandler consumes one discrepancy while PostgreSQL rows
// are being streamed.
type CumulativeComparisonHandler func(CumulativeComparisonDiscrepancy)

type BigQueryReader interface {
	ForEachProwJob(context.Context, time.Time, time.Time, ProwJobHandler) error
}

// ProwJobHandler processes one BigQuery Prow job while the result stream is
// being consumed.
type ProwJobHandler func(BQJob) error

type PostgreSQL struct {
	dbc *db.DB
}

func NewPostgreSQL(dbc *db.DB) *PostgreSQL {
	return &PostgreSQL{dbc: dbc}
}

func (p *PostgreSQL) validate() error {
	if p == nil || p.dbc == nil || p.dbc.DB == nil {
		return fmt.Errorf("PostgreSQL client is not initialized")
	}
	return nil
}

func (p *PostgreSQL) Releases(ctx context.Context) ([]string, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	var releases []string
	err := p.dbc.DB.WithContext(ctx).Raw(`
		SELECT release
		FROM release_definitions
		WHERE deleted_at IS NULL AND release <> ''
		ORDER BY release
	`).Scan(&releases).Error
	if err != nil {
		return nil, fmt.Errorf("querying verification releases: %w", err)
	}
	return releases, nil
}

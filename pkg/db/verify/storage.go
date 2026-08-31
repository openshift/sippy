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
	CumulativeRows(context.Context, string, civil.Date) (CumulativeRows, error)
}

type PostgreSQLReader interface {
	ReleaseReader
	ProwJobRunReader
	DailyTotalsReader
	CumulativeSummariesReader
}

type BigQueryReader interface {
	ProwJobs(context.Context, time.Time, time.Time) ([]BQJob, error)
}

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
		UNION
		SELECT DISTINCT release
		FROM prow_jobs
		WHERE deleted_at IS NULL AND release <> ''
		ORDER BY release
	`).Scan(&releases).Error
	if err != nil {
		return nil, fmt.Errorf("querying verification releases: %w", err)
	}
	return releases, nil
}

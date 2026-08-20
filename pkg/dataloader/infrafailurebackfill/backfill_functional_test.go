package infrafailurebackfill

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	gormlogger "gorm.io/gorm/logger"

	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
	"github.com/openshift/sippy/pkg/db"
)

// TestBackfillFunctional exercises the InfraFailure backfill against real
// BigQuery (and, when a DSN is provided, PostgreSQL). It is skipped unless the
// required credentials are supplied via environment variables, following the
// pattern in releasesync_functional_test.go.
//
// Required:
//
//	GOOGLE_APPLICATION_CREDENTIALS - path to a GCP service account JSON key file
//	BIGQUERY_PROJECT               - GCP project ID (e.g. "openshift-gce-devel")
//	BIGQUERY_DATASET               - BigQuery dataset (e.g. "ci_analysis_us")
//
// Optional:
//
//	INFRA_FAILURE_SINCE - window start date YYYY-MM-DD (overrides the days lookback)
//	INFRA_FAILURE_DAYS  - lookback days when SINCE is unset (default 7)
//	SIPPY_DATABASE_DSN  - when set, additionally runs a full dry-run against PostgreSQL
func TestBackfillFunctional(t *testing.T) {
	credFile := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	bqProject := os.Getenv("BIGQUERY_PROJECT")
	bqDataset := os.Getenv("BIGQUERY_DATASET")
	if credFile == "" || bqProject == "" || bqDataset == "" {
		t.Skip("Set GOOGLE_APPLICATION_CREDENTIALS, BIGQUERY_PROJECT, and BIGQUERY_DATASET to run this test")
	}

	ctx := context.Background()
	opCtx := bqlabel.OperationalContext{
		App:         bqlabel.AppSippy,
		Command:     "functional-test",
		Environment: bqlabel.EnvCli,
	}
	bqc, err := bqcachedclient.New(ctx, opCtx, nil, credFile, bqProject, bqDataset, "")
	if err != nil {
		t.Fatalf("failed to create BigQuery client: %v", err)
	}

	days := 7
	if v := os.Getenv("INFRA_FAILURE_DAYS"); v != "" {
		if d, perr := strconv.Atoi(v); perr == nil {
			days = d
		}
	}
	since := os.Getenv("INFRA_FAILURE_SINCE")
	startDate, err := resolveStartDate(since, days, time.Now())
	if err != nil {
		t.Fatalf("resolving start date: %v", err)
	}

	// Exercise the real BigQuery query path.
	b := &Backfiller{bq: bqc}
	ids, err := b.fetchInfraFailureIDsFromBQ(ctx, startDate)
	if err != nil {
		t.Fatalf("fetching InfraFailure IDs from BigQuery: %v", err)
	}
	t.Logf("found %d InfraFailure runs in BigQuery since %s", len(ids), startDate)

	dsn := os.Getenv("SIPPY_DATABASE_DSN")
	if dsn == "" {
		t.Log("SIPPY_DATABASE_DSN not set; skipping PostgreSQL dry-run")
		return
	}

	dbc, err := db.New(dsn, gormlogger.Silent)
	if err != nil {
		t.Fatalf("connecting to PostgreSQL: %v", err)
	}

	backfiller := New(bqc, dbc, Options{Since: since, Days: days, DryRun: true})
	stats, err := backfiller.Run(ctx)
	if err != nil {
		t.Fatalf("dry-run backfill failed: %v", err)
	}
	t.Logf("dry-run stats: %+v", stats)
}

package jobrunscan

import (
	"context"
	"os"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/openshift/sippy/pkg/api/jobartifacts"
	bqclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
	"github.com/openshift/sippy/pkg/db"
	"google.golang.org/api/option"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// These tests exercise the ReEvaluator's write path (BQ, GCS, Postgres label
// updates) against real infrastructure. They verify that symptom evaluation
// produces correct results and that idempotent re-runs are stable. They require
// real GCP and database credentials and are skipped unless the necessary
// environment variables are set. To run:
//
//	GOOGLE_APPLICATION_CREDENTIALS=/path/to/key.json \
//	BIGQUERY_PROJECT=my-project \
//	BIGQUERY_DATASET=ci_analysis_us \
//	GCS_BUCKET=test-platform-results \
//	SIPPY_DATABASE_DSN=postgresql://user:pass@host:5432/dbname \
//	PROW_JOB_BUILD_ID=1234567890 \
//	go test -v -run TestReEvaluate ./pkg/api/jobrunscan/
//
// These tests call ReEvaluator methods directly, not through the async batch
// pipeline. The async pipeline (HTTP submit, River processing, status polling)
// is tested in pkg/sippyserver/workqueue/symptomre/e2e_test.go.
//
// To test the full async flow via HTTP (dry_run only, no BQ writes):
//
//	SIPPY_DATABASE_DSN=... GOOGLE_APPLICATION_CREDENTIALS=... GCS_BUCKET=... \
//	PROW_JOB_BUILD_ID=... \
//	go test -v -run TestE2E -timeout 5m ./pkg/sippyserver/workqueue/symptomre/

/* Example of invoking the async API:
   # Submit a batch:
   curl -X POST http://localhost:8080/api/jobs/runs/reevaluate \
        -H 'Content-Type: application/json' \
        -d '{"prow_job_build_ids": ["2061603073523978240"], "dry_run": true}'
   # Poll status (batch_id from the 202 response):
   curl http://localhost:8080/api/jobs/runs/reevaluate/<batch_id>
*/

func functionalTestReEvaluator(t *testing.T) *ReEvaluator {
	t.Helper()

	credFile := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	bqProject := os.Getenv("BIGQUERY_PROJECT")
	bqDataset := os.Getenv("BIGQUERY_DATASET")
	gcsBucket := os.Getenv("GCS_BUCKET")
	dbDSN := os.Getenv("SIPPY_DATABASE_DSN")
	buildID := os.Getenv("PROW_JOB_BUILD_ID")

	if credFile == "" || bqProject == "" || bqDataset == "" || gcsBucket == "" || dbDSN == "" || buildID == "" {
		t.Skip("Set GOOGLE_APPLICATION_CREDENTIALS, BIGQUERY_PROJECT, BIGQUERY_DATASET, GCS_BUCKET, SIPPY_DATABASE_DSN, and PROW_JOB_BUILD_ID to run this test")
	}

	ctx := context.Background()
	opCtx := bqlabel.OperationalContext{
		App:         bqlabel.AppSippy,
		Command:     "test",
		Environment: bqlabel.EnvCli,
	}
	bqC, err := bqclient.New(ctx, opCtx, nil, credFile, bqProject, bqDataset, "")
	if err != nil {
		t.Fatalf("creating BQ client: %v", err)
	}

	gcsC, err := storage.NewClient(ctx, option.WithCredentialsFile(credFile))
	if err != nil {
		t.Fatalf("creating GCS client: %v", err)
	}

	gormDB, err := gorm.Open(postgres.Open(dbDSN), &gorm.Config{})
	if err != nil {
		t.Fatalf("connecting to database: %v", err)
	}
	dbc := &db.DB{DB: gormDB}

	return NewReEvaluator(bqC, gcsC, gcsBucket, dbc, nil, jobartifacts.NewManager(ctx))
}

func TestReEvaluateEndToEnd(t *testing.T) {
	re := functionalTestReEvaluator(t)
	buildID := os.Getenv("PROW_JOB_BUILD_ID")

	results, err := re.ReEvaluateJobRuns(context.Background(), []string{buildID}, false)
	if err != nil {
		t.Fatalf("re-evaluation failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.ProwJobBuildID != buildID {
		t.Errorf("result build ID = %q, want %q", r.ProwJobBuildID, buildID)
	}
	t.Logf("result: status=%s, symptoms_evaluated=%d, symptoms_matched=%v, labels_applied=%v, bq=%d, gcs=%d, pg=%v",
		r.Status, r.SymptomsEvaluated, r.SymptomsMatched, r.LabelsApplied, r.BQEntriesWritten, r.GCSArtifactsWritten, r.PostgresUpdated)
	if r.Status != ReEvalSuccess {
		t.Errorf("expected success status, got %s: %s", r.Status, r.Error)
	}
}

func TestReEvaluateIdempotent(t *testing.T) {
	re := functionalTestReEvaluator(t)
	buildID := os.Getenv("PROW_JOB_BUILD_ID")

	// Run twice
	results1, err := re.ReEvaluateJobRuns(context.Background(), []string{buildID}, false)
	if err != nil {
		t.Fatalf("first re-evaluation failed: %v", err)
	}
	results2, err := re.ReEvaluateJobRuns(context.Background(), []string{buildID}, false)
	if err != nil {
		t.Fatalf("second re-evaluation failed: %v", err)
	}

	if len(results1) != 1 || len(results2) != 1 {
		t.Fatal("expected 1 result each")
	}
	r1, r2 := results1[0], results2[0]
	if !sameStrings(r1.SymptomsMatched, r2.SymptomsMatched) {
		t.Errorf("symptoms matched differ: %v vs %v", r1.SymptomsMatched, r2.SymptomsMatched)
	}
	if !sameStrings(r1.LabelsApplied, r2.LabelsApplied) {
		t.Errorf("labels applied differ: %v vs %v", r1.LabelsApplied, r2.LabelsApplied)
	}
}

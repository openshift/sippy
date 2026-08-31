package symptomre

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/option"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/api/jobartifacts"
	apijobrunscan "github.com/openshift/sippy/pkg/api/jobrunscan"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/sippyserver/workqueue"
)

// TestE2EAsyncSymptomReEvaluation exercises the full async re-evaluation pipeline
// end-to-end through HTTP: POST to submit a batch, then poll GET until complete.
//
// The test starts its own HTTP server (with the submit and status endpoints) and
// its own River worker client (processing batch fan-out and individual evaluations).
// It runs with dry_run=true by default, so no BigQuery or GCS writes occur.
//
// WARNING: this DOES run a DB migration, so write capability is required; use a DB snapshot.
//
// Required environment variables:
//
//	SIPPY_DATABASE_DSN      - PostgreSQL DSN with production-like data (symptoms, job runs, labels)
//	GOOGLE_APPLICATION_CREDENTIALS - path to GCS service account key file
//	GCS_BUCKET              - GCS bucket containing job run artifacts (e.g. "test-platform-results")
//	PROW_JOB_BUILD_ID       - comma-separated prow_job_build_ids to re-evaluate
//
// Example:
//
//	SIPPY_DATABASE_DSN=postgresql://user:pass@host:5432/sippy \
//	GOOGLE_APPLICATION_CREDENTIALS=/path/to/key.json \
//	GCS_BUCKET=test-platform-results \
//	PROW_JOB_BUILD_ID=2061603073523978240 \
//	go test -v -run TestE2E -timeout 5m ./pkg/sippyserver/workqueue/symptomre/
//
// This test always runs with dry_run=true. It does not configure a BigQuery client, so non-dry-run
// evaluation would fail. Full write-path testing (BQ, GCS, Postgres label updates) is covered by
// the existing functional tests in pkg/api/jobrunscan/reevaluate_functional_test.go.
func TestE2EAsyncSymptomReEvaluation(t *testing.T) {
	dsn := os.Getenv("SIPPY_DATABASE_DSN")
	credFile := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	gcsBucket := os.Getenv("GCS_BUCKET")
	buildIDsStr := os.Getenv("PROW_JOB_BUILD_ID")

	if dsn == "" || credFile == "" || gcsBucket == "" || buildIDsStr == "" {
		t.Skip("Set SIPPY_DATABASE_DSN, GOOGLE_APPLICATION_CREDENTIALS, GCS_BUCKET, and PROW_JOB_BUILD_ID to run this test")
	}

	buildIDs := strings.Split(buildIDsStr, ",")
	for i := range buildIDs {
		buildIDs[i] = strings.TrimSpace(buildIDs[i])
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	// --- Database setup ---
	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err, "GORM connection should open")
	dbc := &db.DB{DB: gormDB}

	pgxPool, err := workqueue.NewPgxV5Pool(ctx, dsn)
	require.NoError(t, err, "pgx/v5 pool creation should succeed")
	defer pgxPool.Close()

	migrator, err := rivermigrate.New(riverpgxv5.New(pgxPool), nil)
	require.NoError(t, err, "River migrator creation should succeed")
	_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	require.NoError(t, err, "River schema migration should succeed")

	require.NoError(t, gormDB.AutoMigrate(&Batch{}, &BatchItem{}),
		"batch table auto-migration should succeed")

	// --- GCS client ---
	gcsClient, err := storage.NewClient(ctx, option.WithCredentialsFile(credFile))
	require.NoError(t, err, "GCS client creation should succeed")
	defer gcsClient.Close()

	// --- ReEvaluator (BQ is nil for dry_run; GCS is real for artifact reads) ---
	reEvaluator := apijobrunscan.NewReEvaluator(nil, gcsClient, gcsBucket, dbc, nil, jobartifacts.NewManager(ctx))

	// --- River worker client ---
	workers := river.NewWorkers()
	batchWorker := NewProcessBatchWorker(reEvaluator, gormDB)
	reEvalWorker := NewReevaluateWorker(reEvaluator)
	river.AddWorker(workers, batchWorker)
	river.AddWorker(workers, reEvalWorker)

	riverConfig := &river.Config{
		Queues: map[string]river.QueueConfig{
			BatchQueue: {MaxWorkers: 1},
			ItemQueue:  {MaxWorkers: 4},
		},
	}
	workerClient, err := workqueue.NewWorkerClient(pgxPool, workers, riverConfig)
	require.NoError(t, err, "River worker client creation should succeed")
	batchWorker.SetRiverClient(workerClient)

	require.NoError(t, workerClient.Start(ctx), "River worker client should start")
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		_ = workerClient.Stop(stopCtx)
	}()

	// --- Insert-only client for Submitter (separate pool, like production) ---
	insertPool, err := workqueue.NewPgxV5Pool(ctx, dsn)
	require.NoError(t, err, "insert-only pgx/v5 pool should succeed")
	defer insertPool.Close()

	insertClient, err := workqueue.NewInsertOnlyClient(insertPool)
	require.NoError(t, err, "insert-only River client should succeed")

	submitter := NewSubmitter(gormDB, insertClient)
	querier := NewStatusQuerier(gormDB)

	// --- HTTP test server ---
	router := mux.NewRouter()
	router.HandleFunc("/api/jobs/runs/reevaluate", makeSubmitHandler(submitter)).Methods("POST")
	router.HandleFunc("/api/jobs/runs/reevaluate/{batch_id}", makeStatusHandler(querier)).Methods("GET")
	ts := httptest.NewServer(router)
	defer ts.Close()

	// --- Submit batch via HTTP POST ---
	reqBody := fmt.Sprintf(`{"prow_job_build_ids": [%s], "dry_run": true}`,
		quoteAndJoin(buildIDs))
	resp, err := http.Post(ts.URL+"/api/jobs/runs/reevaluate", "application/json", strings.NewReader(reqBody)) //nolint:gosec // test server URL
	require.NoError(t, err, "HTTP POST should succeed")
	defer resp.Body.Close()

	require.Equal(t, http.StatusAccepted, resp.StatusCode,
		"submit should return 202 Accepted")

	var submitResp struct {
		BatchID   uuid.UUID         `json:"batch_id"`
		Requested int               `json:"requested"`
		Links     map[string]string `json:"links"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&submitResp),
		"submit response should decode")
	assert.NotEqual(t, uuid.Nil, submitResp.BatchID,
		"batch ID should be non-nil")
	assert.Equal(t, len(buildIDs), submitResp.Requested,
		"requested count should match input")
	assert.Contains(t, submitResp.Links, "status",
		"response should contain a status HATEOAS link")
	t.Logf("submitted batch %s with %d items (dry_run=true)", submitResp.BatchID, submitResp.Requested)

	// --- Poll status until terminal ---
	statusURL := ts.URL + "/api/jobs/runs/reevaluate/" + submitResp.BatchID.String()
	var finalStatus BatchStatusResponse

	pollDeadline := time.Now().Add(3 * time.Minute)
	for {
		require.False(t, time.Now().After(pollDeadline),
			"batch should complete within 3 minutes")

		statusResp, err := http.Get(statusURL) //nolint:gosec // test URL from httptest
		require.NoError(t, err, "HTTP GET status should succeed")

		require.Equal(t, http.StatusOK, statusResp.StatusCode,
			"status endpoint should return 200")

		require.NoError(t, json.NewDecoder(statusResp.Body).Decode(&finalStatus),
			"status response should decode")
		statusResp.Body.Close()

		t.Logf("status=%s completed=%d failed=%d running=%d pending=%d",
			finalStatus.Status, finalStatus.Completed, finalStatus.Failed,
			finalStatus.Running, finalStatus.Pending)

		if finalStatus.Status == workqueue.BatchStatusComplete ||
			finalStatus.Status == workqueue.BatchStatusFailed {
			break
		}
		time.Sleep(2 * time.Second)
	}

	// --- Verify final state ---
	assert.Equal(t, submitResp.BatchID, finalStatus.BatchID,
		"status batch ID should match submitted batch")
	assert.Equal(t, len(buildIDs), finalStatus.Requested,
		"requested count should persist through completion")
	assert.Equal(t, len(buildIDs), finalStatus.Completed+finalStatus.Failed,
		"all items should reach a terminal state")
	assert.Len(t, finalStatus.Items, len(buildIDs),
		"status should report all items")

	if finalStatus.Status == workqueue.BatchStatusComplete {
		assert.Equal(t, len(buildIDs), finalStatus.Completed,
			"all items should complete successfully")
		assert.Zero(t, finalStatus.Failed,
			"no items should fail in a successful batch")
		t.Logf("batch completed successfully: %d items, %d enqueued, %d deduped",
			finalStatus.Requested, finalStatus.Enqueued, finalStatus.Deduped)
	} else {
		t.Logf("batch finished with failures: %d completed, %d failed",
			finalStatus.Completed, finalStatus.Failed)
		for _, item := range finalStatus.Items {
			if item.State != "completed" {
				t.Logf("  item %s: %s", item.ItemKey, item.State)
			}
		}
	}

	// Log per-item results for user verification
	for _, item := range finalStatus.Items {
		t.Logf("  item_key=%s state=%s", item.ItemKey, item.State)
	}

	// --- Cleanup test batch ---
	gormDB.Where("batch_id = ?", submitResp.BatchID).Delete(&BatchItem{})
	gormDB.Where("id = ?", submitResp.BatchID).Delete(&Batch{})
}

// makeSubmitHandler creates an HTTP handler for POST /api/jobs/runs/reevaluate
// that mirrors the production handler in sippyserver/job_run_scan.go.
func makeSubmitHandler(submitter *Submitter) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			ProwJobBuildIDs []string `json:"prow_job_build_ids"`
			DryRun          bool     `json:"dry_run"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			api.RespondWithJSON(http.StatusBadRequest, w, map[string]string{
				"message": "invalid request body: " + err.Error(),
			})
			return
		}

		deduped, err := apijobrunscan.ValidateReEvalRequest(body.ProwJobBuildIDs)
		if err != nil {
			api.RespondWithJSON(http.StatusBadRequest, w, map[string]string{
				"message": err.Error(),
			})
			return
		}

		result, err := submitter.Submit(req.Context(), deduped, body.DryRun)
		if err != nil {
			api.RespondWithJSON(http.StatusInternalServerError, w, map[string]string{
				"message": "submitting re-evaluation batch: " + err.Error(),
			})
			return
		}

		api.RespondWithJSON(http.StatusAccepted, w, map[string]interface{}{
			"batch_id":  result.BatchID,
			"requested": result.Requested,
			"links": map[string]string{
				"status": api.GetBaseURL(req) + req.URL.Path + "/" + result.BatchID.String(),
			},
		})
	}
}

// makeStatusHandler creates an HTTP handler for GET /api/jobs/runs/reevaluate/{batch_id}
// that mirrors the production handler in sippyserver/job_run_scan.go.
func makeStatusHandler(querier *StatusQuerier) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		batchIDStr := mux.Vars(req)["batch_id"]
		batchID, err := uuid.Parse(batchIDStr)
		if err != nil {
			api.RespondWithJSON(http.StatusBadRequest, w, map[string]string{
				"message": "invalid batch_id: " + err.Error(),
			})
			return
		}

		resp, err := querier.Query(batchID)
		if err != nil {
			api.RespondWithJSON(http.StatusInternalServerError, w, map[string]string{
				"message": err.Error(),
			})
			return
		}
		if resp == nil {
			api.RespondWithJSON(http.StatusNotFound, w, map[string]string{
				"message": "batch not found",
			})
			return
		}

		api.RespondWithJSON(http.StatusOK, w, resp)
	}
}

// quoteAndJoin formats string slices as JSON-style quoted, comma-separated values.
func quoteAndJoin(ss []string) string {
	quoted := make([]string, len(ss))
	for i, s := range ss {
		quoted[i] = `"` + s + `"`
	}
	return strings.Join(quoted, ", ")
}

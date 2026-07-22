package releaseloader

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path"
	"testing"
	"time"

	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
	"github.com/openshift/sippy/pkg/db/models"
)

// TestReleaseJobRunsLabelsFunctional is a functional test for debugging label fetching
// from BigQuery during release job run processing. It fetches real release details
// from the release controller, builds job runs, and applies labels via applyBulkLabels.
//
// Required environment variables:
//
//	GOOGLE_APPLICATION_CREDENTIALS - path to GCP service account JSON key file
//	BIGQUERY_PROJECT               - GCP project ID (e.g. "openshift-gce-devel")
//	BIGQUERY_DATASET               - BigQuery dataset (e.g. "ci_analysis_us")
//	RELEASE_TAG                    - release tag to test (e.g. "4.19.0-0.nightly-2026-05-17-125308")
//	RELEASE_STREAM                 - release stream (e.g. "4.19.0-0.nightly")
//	RELEASE_DOMAIN                 - release controller domain (default: "amd64.ocp.releases.ci.openshift.org")
//
// Optional:
//
//	JOB_LABELS_DATASET             - override dataset for the job_labels table
func TestReleaseJobRunsLabelsFunctional(t *testing.T) {
	credFile := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	bqProject := os.Getenv("BIGQUERY_PROJECT")
	bqDataset := os.Getenv("BIGQUERY_DATASET")
	releaseTag := os.Getenv("RELEASE_TAG")
	releaseStream := os.Getenv("RELEASE_STREAM")

	if credFile == "" || bqProject == "" || bqDataset == "" || releaseTag == "" || releaseStream == "" {
		t.Skip("Set GOOGLE_APPLICATION_CREDENTIALS, BIGQUERY_PROJECT, BIGQUERY_DATASET, RELEASE_TAG, and RELEASE_STREAM to run this test")
	}

	domain := os.Getenv("RELEASE_DOMAIN")
	if domain == "" {
		domain = "amd64.ocp.releases.ci.openshift.org"
	}

	ctx := context.Background()
	opCtx := bqlabel.OperationalContext{
		App:     bqlabel.AppSippy,
		Command: "functional-test",
	}
	bqClient, err := bqcachedclient.New(ctx, opCtx, nil, credFile, bqProject, bqDataset, "")
	if err != nil {
		t.Fatalf("Failed to create BigQuery client: %v", err)
	}

	// Fetch release details from the release controller
	detailsURL := url.URL{
		Scheme: "https",
		Host:   domain,
		Path:   path.Join("/api/v1/releasestream", releaseStream, "release", releaseTag),
	}
	t.Logf("Fetching release details from %s", detailsURL.String())

	httpClient := &http.Client{Timeout: 60 * time.Second}
	resp, err := httpClient.Get(detailsURL.String())
	if err != nil {
		t.Fatalf("Failed to fetch release details: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Release controller returned %d for %s", resp.StatusCode, detailsURL.String())
	}

	var details ReleaseDetails
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		t.Fatalf("Failed to decode release details: %v", err)
	}

	t.Logf("Release %s has %d blocking jobs, %d informing jobs",
		details.Name,
		len(details.Results["blockingJobs"]),
		len(details.Results["informingJobs"]),
	)

	loader := &ReleaseLoader{ctx: ctx, bqClient: bqClient}
	tag := &models.ReleaseTag{
		JobRuns:     loader.buildJobRuns(details),
		ReleaseTime: time.Now().Add(-14 * 24 * time.Hour),
	}
	loader.applyBulkLabels([]*models.ReleaseTag{tag})

	t.Logf("Produced %d job runs", len(tag.JobRuns))

	labelsFound := 0
	for _, jr := range tag.JobRuns {
		if len(jr.Labels) > 0 {
			labelsFound++
			t.Logf("  %s (build %d): labels=%v", jr.JobName, jr.Name, jr.Labels)
		}
	}

	t.Logf("Job runs with labels: %d / %d", labelsFound, len(tag.JobRuns))
	if labelsFound == 0 {
		t.Errorf("No labels found for any job run")
	}
}

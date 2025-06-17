package ai

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	log "github.com/sirupsen/logrus"

	jobQueries "github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/apis/openshift"
	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/dataloader/prowloader"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
)

// Prompt to use for jobRunAnalysis.
// TODO: Look into GCS bucket for node and machine info, maybe other small data from gather-extra
const jobRunAnalysisPrompt = `You are an expert in analyzing OpenShift CI job failures. You will receive
structured JSON input containing:

- The CI job name
- A high-level overall result for the job (e.g., success, install, test failure, other)
- A list of test failures and their output snippets
- Information about any cluster operators who are unavailable or degraded

Based on this, return a concise summary (1–3 sentences max) explaining why the job failed. Be clear, factual, and avoid
speculation. Do not guess at the meaning of any acronyms. Be brief.`

// ClusterOperatorStatus represents the status of a cluster operator
type ClusterOperatorStatus struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// JobRunData contains the comprehensive data for a job run including test failures and cluster operator status
type JobRunData struct {
	// Basic job information
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	Release   string `json:"release"`
	Cluster   string `json:"cluster"`
	URL       string `json:"url"`
	GCSBucket string `json:"gcsBucket"`

	// Timing information
	StartTime       time.Time     `json:"startTime"`
	Duration        time.Duration `json:"duration"`
	DurationSeconds float64       `json:"durationSeconds"`

	// Status and results
	OverallResult         string `json:"overallResult"`
	Reason                string `json:"reason"`
	Succeeded             bool   `json:"succeeded"`
	Failed                bool   `json:"failed"`
	InfrastructureFailure bool   `json:"infrastructureFailure"`
	KnownFailure          bool   `json:"knownFailure"`

	// Test information
	TestCount        int               `json:"testCount"`
	TestFailureCount int               `json:"testFailureCount"`
	TestFailures     map[string]string `json:"testFailures,omitempty"`

	// Job metadata
	Variants    []string `json:"variants,omitempty"`
	TestGridURL string   `json:"testGridURL,omitempty"`

	// Cluster and infrastructure information
	ClusterOperators []ClusterOperatorStatus `json:"clusterOperators,omitempty"`
}

// GetJobRunSummary extracts and returns the raw job run data without LLM analysis
func GetJobRunSummary(ctx context.Context, dbc *db.DB, gcsClient *storage.Client, jobRunID int64) (*JobRunData, error) {
	jLog := log.WithField("JobRunID", jobRunID)
	dbStart := time.Now()
	jLog.Info("Querying DB for job run data")
	jr, err := jobQueries.FetchJobRun(dbc, jobRunID, false, []string{"Tests.ProwJobRunTestOutput"}, jLog)
	if err != nil {
		return nil, err
	}
	jLog.Infof("DB query complete after %+v", time.Since(dbStart))

	failures := extractTestOutputs(jr)

	// Extract data from GCS bucket
	gcsPath, err := prowloader.GetGCSPathForProwJobURL(jLog, jr.URL)
	if err != nil {
		return nil, err
	}
	bkt := gcsClient.Bucket(jr.GCSBucket)
	gcsJr := gcs.NewGCSJobRun(bkt, gcsPath)

	clusterOperators := getUnavailableOrDegradedOperators(gcsJr, jLog)

	jrData := &JobRunData{
		// Basic job information
		ID:        jr.ID,
		Name:      jr.ProwJob.Name,
		Release:   jr.ProwJob.Release,
		Cluster:   jr.Cluster,
		URL:       jr.URL,
		GCSBucket: jr.GCSBucket,

		// Timing information
		StartTime:       jr.Timestamp,
		Duration:        jr.Duration,
		DurationSeconds: jr.Duration.Seconds(),

		// Status and results
		OverallResult:         string(jr.OverallResult),
		Reason:                jr.OverallResult.String(),
		Succeeded:             jr.Succeeded,
		Failed:                jr.Failed,
		InfrastructureFailure: jr.InfrastructureFailure,
		KnownFailure:          jr.KnownFailure,

		// Test information
		TestCount:        jr.TestCount,
		TestFailureCount: jr.TestFailures,
		TestFailures:     failures,

		// Job metadata
		Variants:    jr.ProwJob.Variants,
		TestGridURL: jr.ProwJob.TestGridURL,

		// Cluster and infrastructure information
		ClusterOperators: clusterOperators,
	}

	return jrData, nil
}

func AnalyzeJobRun(ctx context.Context, llmClient *LLMClient, dbc *db.DB, gcsClient *storage.Client, jobRunID int64) (string, error) {
	jrData, err := GetJobRunSummary(ctx, dbc, gcsClient, jobRunID)
	if err != nil {
		return "", err
	}

	jobRunSummary, err := json.MarshalIndent(jrData, "", "  ")
	if err != nil {
		return "", err
	}

	llmStart := time.Now()
	jLog := log.WithField("JobRunID", jobRunID).WithField("Name", jrData.Name)
	jLog.Info("Asking LLM for job run summary")
	jLog.Debugf("Job Run Data: %s", string(jobRunSummary))
	res, err := llmClient.Chat(ctx, jobRunAnalysisPrompt, string(jobRunSummary))
	if err == nil {
		jLog.Infof("LLM complete in %+v", time.Since(llmStart))
	}

	return res, err
}

func getUnavailableOrDegradedOperators(jr *gcs.GCSJobRun, jLog *log.Entry) []ClusterOperatorStatus {
	start := time.Now()
	jLog.Info("Fetching cluster operators...")
	// Operator statuses
	coData := jr.FindFirstFile("", regexp.MustCompile("clusteroperators.json"))
	if coData == nil {
		jLog.Infof("Cluster operators not found in %+v", time.Since(start))
		return nil
	}

	var statuses []ClusterOperatorStatus
	var coList openshift.ClusterOperatorList
	if err := json.Unmarshal(coData, &coList); err != nil {
		jLog.WithError(err).Warn("Failed to parse cluster operator list")
		return nil
	}
	for _, co := range coList.Items {
		for _, condition := range co.Status.Conditions {
			if (condition.Type == "Degraded" && condition.Status == "True") || (condition.Type == "Available" && condition.Status == "False") {
				statuses = append(statuses, ClusterOperatorStatus{
					Name:    co.Metadata.Name,
					Status:  condition.Status,
					Reason:  condition.Reason,
					Message: condition.Message,
				})
			}
		}

	}
	jLog.Infof("Cluster operators found in %+v", time.Since(start))
	return statuses
}

func extractTestOutputs(jr *models.ProwJobRun) map[string]string {
	failures := make(map[string]string)
	for _, test := range jr.Tests {
		// skip synthetic tests
		if strings.Contains(test.Test.Name, "sig-sippy") {
			continue
		}

		if v1.TestStatus(test.Status) == v1.TestStatusFailure {
			output := test.ProwJobRunTestOutput.Output
			// some tests are very chatty, get the last 256 characters where
			// the meat of the failure probably is.
			if len(output) > 256 {
				output = output[len(output)-256:]
			}
			failures[test.Test.Name] = output
		}
	}
	return failures
}

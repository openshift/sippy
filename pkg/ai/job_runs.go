package ai

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	jobQueries "github.com/openshift/sippy/pkg/api"
	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
)

// Prompt to use for jobRunAnalysis.
// TODO: Look into GCS bucket for node, machine, and operator info -- and whatever other small bits of data we can get.
const jobRunAnalysisPrompt = `You are an expert in analyzing OpenShift CI job failures. You will receive
structured JSON input containing:

- The CI job name
- A high-level overall result for the job (e.g., success, install, test failure, other)
- A list of test failures and their output snippets

Based on this, return a concise summary (1–3 sentences max) explaining why the job failed. Be clear, factual, and avoid
speculation. Be brief.`

type jobRunData struct {
	Name         string
	Reason       string
	TestFailures map[string]string
}

func AnalyzeJobRun(ctx context.Context, llmClient *LLMClient, dbc *db.DB, jobRunID int64) (string, error) {
	jLog := log.WithField("JobRunID", jobRunID)
	dbStart := time.Now()
	jLog.Info("Querying DB for job run data")
	jr, err := jobQueries.FetchJobRun(dbc, jobRunID, false, []string{"Tests.ProwJobRunTestOutput"}, jLog)
	if err != nil {
		return "", err
	}
	jLog.Infof("DB query complete after %+v", time.Since(dbStart))

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

	jrData := jobRunData{
		Name:         jr.ProwJob.Name,
		Reason:       jr.OverallResult.String(),
		TestFailures: failures,
	}

	jobRunSummary, err := json.MarshalIndent(jrData, "", "  ")
	if err != nil {
		return "", err
	}

	llmStart := time.Now()
	jLog = jLog.WithField("Name", jrData.Name)
	jLog.Info("Asking LLM for job run summary")
	res, err := llmClient.Chat(ctx, jobRunAnalysisPrompt, string(jobRunSummary))
	if err == nil {
		jLog.Infof("LLM complete in %+v", time.Since(llmStart))
	}

	return res, err
}

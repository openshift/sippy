package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"

	bq "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/util/param"
)

// PRTestResult represents a test result from a pull request job
type PRTestResult struct {
	ProwJobBuildID string    `json:"prowjob_build_id"`
	ProwJobName    string    `json:"prowjob_name"`
	ProwJobURL     string    `json:"prowjob_url"`
	PRSha          string    `json:"pr_sha"`
	ProwJobStart   time.Time `json:"prowjob_start"`
	TestName       string    `json:"test_name"`
	TestSuite      string    `json:"test_suite"`
	Success        bool      `json:"success"`
	Flaked         bool      `json:"flaked"`
	FailureContent string    `json:"failure_content"`
}

// GetPRTestResults fetches test failures for a specific pull request from BigQuery
// This queries both junit_pr and junit tables:
// - junit_pr: Contains results from normal presubmit jobs
// - junit: Contains results from /payload jobs (manually invoked jobs)
// Note: Only returns test failures (success = false), excluding flakes and passes
// includeSuccesses: Optional list of test name substrings to also include successes for
func GetPRTestResults(ctx context.Context, bqc *bq.Client, org, repo string, prNumber int, startDate, endDate time.Time, includeSuccesses []string) ([]PRTestResult, error) {
	log.WithFields(log.Fields{
		"org":               org,
		"repo":              repo,
		"pr_number":         prNumber,
		"start_date":        startDate.Format("2006-01-02"),
		"end_date":          endDate.Format("2006-01-02"),
		"include_successes": includeSuccesses,
	}).Info("querying test results for pull request")

	// Query junit_pr table (normal presubmit jobs)
	log.Infof("querying junit_pr table for org=%s, repo=%s, pr_number=%d, start=%s, end=%s",
		org, repo, prNumber, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	queryPR := buildPRTestResultsQuery(bqc, org, repo, prNumber, startDate, endDate, "junit_pr", includeSuccesses)
	bqc.ApplyQueryLabels(ctx, bqlabel.PRTestResults, queryPR)
	resultsPR, err := executePRTestResultsQuery(ctx, queryPR)
	if err != nil {
		log.WithError(err).Error("error querying junit_pr table")
		return nil, errors.Wrap(err, "failed to execute PR test results query for junit_pr table")
	}
	log.Infof("found %d test results from presubmit jobs (junit_pr)", len(resultsPR))

	// Query junit table (/payload jobs)
	log.Infof("querying junit table for org=%s, repo=%s, pr_number=%d, start=%s, end=%s",
		org, repo, prNumber, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	queryPayload := buildPRTestResultsQuery(bqc, org, repo, prNumber, startDate, endDate, "junit", includeSuccesses)
	bqc.ApplyQueryLabels(ctx, bqlabel.PRTestResults, queryPayload)
	resultsPayload, err := executePRTestResultsQuery(ctx, queryPayload)
	if err != nil {
		log.WithError(err).Error("error querying junit table")
		return nil, errors.Wrap(err, "failed to execute PR test results query for junit table")
	}
	log.Infof("found %d test results from /payload jobs (junit)", len(resultsPayload))

	// Combine results from both tables
	allResults := append(resultsPR, resultsPayload...)
	log.Infof("found %d total test results for PR %s/%s#%d", len(allResults), org, repo, prNumber)
	return allResults, nil
}

// buildPRTestResultsQuery constructs the BigQuery query to fetch test results for a PR
// junitTable should be either "junit_pr" (for normal presubmits) or "junit" (for /payload jobs)
// includeSuccesses: Optional list of test name substrings to also include successes for
func buildPRTestResultsQuery(bqc *bq.Client, org, repo string, prNumber int, startDate, endDate time.Time, junitTable string, includeSuccesses []string) *bigquery.Query {
	// Query joins jobs and specified junit table, filtering by org/repo/pr_number
	// Uses partitioning on prowjob_start and modified_time for efficiency
	// Note: junit_pr contains normal presubmit jobs, junit contains /payload jobs
	//
	// The query de-dupes test results because junit tables model the XML directly,
	// which means retried tests appear as multiple rows. We prefer:
	// 1. Flakes (flake_count > 0) - test that failed then passed on retry
	// 2. Successes (success_val > 0) - test that passed
	// 3. Failures (else) - test that failed

	// Build the WHERE clause for including successes
	// By default, only include failures (adjusted_flake_count = 0 AND adjusted_success_val = 0)
	// If includeSuccesses is specified, also include successes for matching test names
	whereClause := `
			AND (
				(deduped.adjusted_flake_count = 0 AND deduped.adjusted_success_val = 0)`

	if len(includeSuccesses) > 0 {
		whereClause += `
				OR (
					deduped.adjusted_success_val > 0
					AND (`
		for i := range includeSuccesses {
			if i > 0 {
				whereClause += " OR "
			}
			whereClause += fmt.Sprintf("deduped.test_name LIKE @IncludeSuccess%d", i)
		}
		whereClause += `
					)
				)`
	}
	whereClause += `
			)`

	queryString := fmt.Sprintf(`
		WITH deduped_testcases AS (
			SELECT
				junit.*,
				ROW_NUMBER() OVER(PARTITION BY prowjob_build_id, file_path, test_name, testsuite ORDER BY
					CASE
						WHEN flake_count > 0 THEN 0
						WHEN success_val > 0 THEN 1
						ELSE 2
					END) AS row_num,
				CASE
					WHEN flake_count > 0 THEN 0
					ELSE success_val
				END AS adjusted_success_val,
				CASE
					WHEN flake_count > 0 THEN 1
					ELSE 0
				END AS adjusted_flake_count
			FROM
				%s.%s AS junit
			WHERE
				junit.modified_time >= DATETIME(@StartDate)
				AND junit.modified_time < DATETIME(@EndDate)
				AND junit.skipped = false
		)
		SELECT
			jobs.prowjob_build_id,
			jobs.prowjob_job_name AS prowjob_name,
			jobs.prowjob_url,
			jobs.pr_sha,
			jobs.prowjob_start,
			deduped.test_name,
			deduped.testsuite,
			CASE
				WHEN deduped.adjusted_flake_count > 0 THEN TRUE
				ELSE FALSE
			END AS flaked,
			CASE
				WHEN deduped.adjusted_flake_count > 0 THEN TRUE
				WHEN deduped.adjusted_success_val > 0 THEN TRUE
				ELSE FALSE
			END AS success,
			deduped.failure_content
		FROM
			%s.jobs AS jobs
		INNER JOIN
			deduped_testcases AS deduped
		ON
			jobs.prowjob_build_id = deduped.prowjob_build_id
			AND deduped.row_num = 1
		WHERE
			jobs.org = @Org
			AND jobs.repo = @Repo
			AND jobs.pr_number = @PRNumber
			AND jobs.prowjob_start >= DATETIME(@StartDate)
			AND jobs.prowjob_start < DATETIME(@EndDate)%s
		ORDER BY
			jobs.prowjob_start DESC,
			deduped.test_name ASC
	`, bqc.Dataset, junitTable, bqc.Dataset, whereClause)

	query := bqc.BQ.Query(queryString)
	query.Parameters = []bigquery.QueryParameter{
		{
			Name:  "Org",
			Value: org,
		},
		{
			Name:  "Repo",
			Value: repo,
		},
		{
			Name:  "PRNumber",
			Value: strconv.Itoa(prNumber),
		},
		{
			Name:  "StartDate",
			Value: startDate,
		},
		{
			Name:  "EndDate",
			Value: endDate,
		},
	}

	// Add parameters for includeSuccesses LIKE clauses
	for i, testName := range includeSuccesses {
		query.Parameters = append(query.Parameters, bigquery.QueryParameter{
			Name:  fmt.Sprintf("IncludeSuccess%d", i),
			Value: "%" + testName + "%", // Wrap in % for SQL LIKE partial matching
		})
	}

	return query
}

// executePRTestResultsQuery executes the BigQuery query and returns the results
func executePRTestResultsQuery(ctx context.Context, query *bigquery.Query) ([]PRTestResult, error) {
	results := []PRTestResult{}

	it, err := query.Read(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "error reading from bigquery")
	}

	for {
		var row []bigquery.Value
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, errors.Wrap(err, "error iterating bigquery results")
		}

		result, err := deserializePRTestResult(row, it.Schema)
		if err != nil {
			log.WithError(err).Warn("error deserializing row, skipping")
			continue
		}

		results = append(results, result)
	}

	return results, nil
}

// deserializePRTestResult converts a BigQuery row into a PRTestResult
func deserializePRTestResult(row []bigquery.Value, schema bigquery.Schema) (PRTestResult, error) {
	if len(row) != len(schema) {
		return PRTestResult{}, fmt.Errorf("row length %d does not match schema length %d", len(row), len(schema))
	}

	result := PRTestResult{}

	for i, field := range schema {
		switch field.Name {
		case "prowjob_build_id":
			if row[i] != nil {
				result.ProwJobBuildID = row[i].(string)
			}
		case "prowjob_name":
			if row[i] != nil {
				result.ProwJobName = row[i].(string)
			}
		case "prowjob_url":
			if row[i] != nil {
				result.ProwJobURL = row[i].(string)
			}
		case "pr_sha":
			if row[i] != nil {
				result.PRSha = row[i].(string)
			}
		case "prowjob_start":
			if row[i] != nil {
				// BigQuery returns civil.DateTime for DATETIME columns
				civilDT := row[i].(civil.DateTime)
				layout := "2006-01-02T15:04:05"
				parsedTime, err := time.Parse(layout, civilDT.String())
				if err != nil {
					return PRTestResult{}, errors.Wrap(err, "failed to parse prowjob_start")
				}
				result.ProwJobStart = parsedTime
			}
		case "test_name":
			if row[i] != nil {
				result.TestName = row[i].(string)
			}
		case "testsuite":
			if row[i] != nil {
				result.TestSuite = row[i].(string)
			}
		case "flaked":
			if row[i] != nil {
				result.Flaked = row[i].(bool)
			}
		case "success":
			if row[i] != nil {
				result.Success = row[i].(bool)
			}
		case "failure_content":
			if row[i] != nil {
				result.FailureContent = row[i].(string)
			}
		}
	}

	return result, nil
}

// PrintPRTestResultsJSON is the HTTP handler for /api/pull_requests/test_results
func PrintPRTestResultsJSON(w http.ResponseWriter, req *http.Request, bqc *bq.Client) {
	// Parse and validate query parameters
	org := param.SafeRead(req, "org")
	if org == "" {
		// Default to openshift.
		org = "openshift"
	}

	repo := param.SafeRead(req, "repo")
	if repo == "" {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": "required parameter 'repo' is missing",
		})
		return
	}

	prNumberStr := param.SafeRead(req, "pr_number")
	if prNumberStr == "" {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": "required parameter 'pr_number' is missing",
		})
		return
	}

	prNumber, err := strconv.Atoi(prNumberStr)
	if err != nil {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": fmt.Sprintf("invalid pr_number: %v", err),
		})
		return
	}

	startDateStr := param.SafeRead(req, "start_date")
	if startDateStr == "" {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": "required parameter 'start_date' is missing (format: YYYY-MM-DD)",
		})
		return
	}

	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": fmt.Sprintf("invalid start_date format (expected YYYY-MM-DD): %v", err),
		})
		return
	}

	endDateStr := param.SafeRead(req, "end_date")
	if endDateStr == "" {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": "required parameter 'end_date' is missing (format: YYYY-MM-DD)",
		})
		return
	}

	endDate, err := time.Parse("2006-01-02", endDateStr)
	if err != nil {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": fmt.Sprintf("invalid end_date format (expected YYYY-MM-DD): %v", err),
		})
		return
	}

	// Validate date range
	if endDate.Before(startDate) {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": "end_date must be after start_date",
		})
		return
	}

	// Add one day to end_date to make it inclusive
	endDate = endDate.AddDate(0, 0, 1)

	// Limit date range to 30 days to prevent expensive queries
	maxDuration := 30 * 24 * time.Hour
	duration := endDate.Sub(startDate)
	if duration > maxDuration {
		RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": fmt.Sprintf("date range too large: %d days (maximum is 30 days)", int(duration.Hours()/24)),
		})
		return
	}

	// Parse optional include_successes parameter (multi-valued)
	includeSuccesses := req.URL.Query()["include_successes"]

	// Execute query
	results, err := GetPRTestResults(req.Context(), bqc, org, repo, prNumber, startDate, endDate, includeSuccesses)
	if err != nil {
		log.WithError(err).Error("error fetching PR test results")
		RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{
			"code":    http.StatusInternalServerError,
			"message": fmt.Sprintf("error fetching test results: %v", err),
		})
		return
	}

	RespondWithJSON(http.StatusOK, w, results)
}

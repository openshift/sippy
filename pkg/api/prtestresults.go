package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/util/param"
)

// PRTestResult represents a test result from a pull request job run
type PRTestResult struct {
	ProwJobRunID uint      `json:"prow_job_run_id"`
	ProwJobName  string    `json:"prow_job_name"`
	ProwJobURL   string    `json:"prow_job_url"`
	PRSha        string    `json:"pr_sha"`
	ProwJobStart time.Time `json:"prow_job_start"`
	TestName     string    `json:"test_name"`
	TestSuite    string    `json:"test_suite"`
	Status       string    `json:"status"`
	Output       string    `json:"output,omitempty"`
}

func testStatusString(status int) string {
	switch sippyprocessingv1.TestStatus(status) {
	case sippyprocessingv1.TestStatusSuccess:
		return "success"
	case sippyprocessingv1.TestStatusFlake:
		return "flake"
	case sippyprocessingv1.TestStatusFailure:
		return "failure"
	default:
		return fmt.Sprintf("unknown(%d)", status)
	}
}

// GetPRTestResults fetches test results for a specific pull request from PostgreSQL.
const defaultPRTestResultsLimit = 10000

func GetPRTestResults(dbc *db.DB, org, repo string, prNumber int, latestSHAOnly bool, startDate, endDate time.Time, includeSuccesses []string, limit int) ([]PRTestResult, error) {
	if limit <= 0 || limit > defaultPRTestResultsLimit {
		limit = defaultPRTestResultsLimit
	}
	log.WithFields(log.Fields{
		"org":             org,
		"repo":            repo,
		"pr_number":       prNumber,
		"latest_sha_only": latestSHAOnly,
		"start_date":      startDate.Format("2006-01-02"),
		"end_date":        endDate.Format("2006-01-02"),
	}).Info("querying test results for pull request")

	// Start from the PR side and use partition keys (release, timestamp) on
	// prow_job_run_tests to allow PostgreSQL to prune partitions and avoid
	// locking every partition in the table.
	query := dbc.DB.Table("prow_pull_requests pp").
		Select(`pjr.id AS prow_job_run_id,
			pj.name AS prow_job_name,
			pjr.url AS prow_job_url,
			pp.sha AS pr_sha,
			pjr.timestamp AS prow_job_start,
			t.name AS test_name,
			COALESCE(s.name, '') AS test_suite,
			pjrt.status,
			COALESCE(pjrto.output, '') AS output`).
		Joins("JOIN prow_job_run_prow_pull_requests jrpr ON jrpr.prow_pull_request_id = pp.id AND jrpr.prow_job_run_release = ? AND jrpr.prow_job_run_timestamp >= ? AND jrpr.prow_job_run_timestamp < ?", models.ReleasePresubmits, startDate, endDate).
		Joins("JOIN prow_job_runs pjr ON pjr.id = jrpr.prow_job_run_id").
		Joins("JOIN prow_jobs pj ON pj.id = pjr.prow_job_id AND pj.release = ?", models.ReleasePresubmits).
		Joins("JOIN prow_job_run_tests pjrt ON pjrt.prow_job_run_id = pjr.id AND pjrt.prow_job_run_release = ? AND pjrt.prow_job_run_timestamp >= ? AND pjrt.prow_job_run_timestamp < ?", models.ReleasePresubmits, startDate, endDate).
		Joins("JOIN tests t ON t.id = pjrt.test_id").
		Joins("LEFT JOIN suites s ON s.id = pjrt.suite_id").
		Joins("LEFT JOIN prow_job_run_test_outputs pjrto ON pjrto.prow_job_run_test_id = pjrt.id AND pjrto.prow_job_run_test_timestamp = pjrt.prow_job_run_timestamp AND pjrto.prow_job_run_test_release = pjrt.prow_job_run_release").
		Where("pp.org = ? AND pp.repo = ? AND pp.number = ?", org, repo, prNumber).
		Where("pp.deleted_at IS NULL AND pjr.deleted_at IS NULL AND pj.deleted_at IS NULL").
		Where("pjr.timestamp >= ? AND pjr.timestamp < ?", startDate, endDate)

	if latestSHAOnly {
		query = query.Where("pp.sha = (SELECT pp2.sha FROM prow_pull_requests pp2 JOIN prow_job_run_prow_pull_requests jrpr2 ON jrpr2.prow_pull_request_id = pp2.id JOIN prow_job_runs pjr2 ON pjr2.id = jrpr2.prow_job_run_id WHERE pp2.org = ? AND pp2.repo = ? AND pp2.number = ? ORDER BY pjr2.timestamp DESC LIMIT 1)", org, repo, prNumber)
	}

	// By default only return failures (no flakes, no successes).
	// include_successes adds successes for matching test names (flakes
	// are excluded to match the previous BigQuery implementation).
	if len(includeSuccesses) == 0 {
		query = query.Where("pjrt.status = ?", int(sippyprocessingv1.TestStatusFailure))
	} else {
		conditions := dbc.DB.Where("pjrt.status = ?", int(sippyprocessingv1.TestStatusFailure))
		for _, pattern := range includeSuccesses {
			conditions = conditions.Or("pjrt.status = ? AND t.name LIKE ?", int(sippyprocessingv1.TestStatusSuccess), "%"+pattern+"%")
		}
		query = query.Where(conditions)
	}

	query = query.Order("pjr.timestamp DESC, t.name ASC").Limit(limit)

	type rawRow struct {
		ProwJobRunID uint      `gorm:"column:prow_job_run_id"`
		ProwJobName  string    `gorm:"column:prow_job_name"`
		ProwJobURL   string    `gorm:"column:prow_job_url"`
		PRSha        string    `gorm:"column:pr_sha"`
		ProwJobStart time.Time `gorm:"column:prow_job_start"`
		TestName     string    `gorm:"column:test_name"`
		TestSuite    string    `gorm:"column:test_suite"`
		Status       int       `gorm:"column:status"`
		Output       string    `gorm:"column:output"`
	}

	var rows []rawRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("error querying PR test results: %w", err)
	}

	results := make([]PRTestResult, 0, len(rows))
	for _, r := range rows {
		results = append(results, PRTestResult{
			ProwJobRunID: r.ProwJobRunID,
			ProwJobName:  r.ProwJobName,
			ProwJobURL:   r.ProwJobURL,
			PRSha:        r.PRSha,
			ProwJobStart: r.ProwJobStart,
			TestName:     r.TestName,
			TestSuite:    r.TestSuite,
			Status:       testStatusString(r.Status),
			Output:       r.Output,
		})
	}

	log.Infof("found %d test results for PR %s/%s#%d", len(results), org, repo, prNumber)
	return results, nil
}

// PrintPRTestResultsJSON is the HTTP handler for /api/pull_requests/test_results
func PrintPRTestResultsJSON(w http.ResponseWriter, req *http.Request, dbc *db.DB) {
	org := param.SafeRead(req, "org")
	if org == "" {
		org = "openshift"
	}

	repo := param.SafeRead(req, "repo")
	if repo == "" {
		RespondWithJSON(http.StatusBadRequest, w, map[string]any{
			"code":    http.StatusBadRequest,
			"message": "required parameter 'repo' is missing",
		})
		return
	}

	prNumberStr := param.SafeRead(req, "pr_number")
	if prNumberStr == "" {
		RespondWithJSON(http.StatusBadRequest, w, map[string]any{
			"code":    http.StatusBadRequest,
			"message": "required parameter 'pr_number' is missing",
		})
		return
	}

	prNumber, err := strconv.Atoi(prNumberStr)
	if err != nil {
		RespondWithJSON(http.StatusBadRequest, w, map[string]any{
			"code":    http.StatusBadRequest,
			"message": fmt.Sprintf("invalid pr_number: %v", err),
		})
		return
	}

	now := time.Now().UTC()
	startDate := now.AddDate(0, 0, -14)
	endDate := now.AddDate(0, 0, 1)

	startDateStr := param.SafeRead(req, "start_date")
	if startDateStr != "" {
		parsed, err := time.Parse("2006-01-02", startDateStr)
		if err != nil {
			RespondWithJSON(http.StatusBadRequest, w, map[string]any{
				"code":    http.StatusBadRequest,
				"message": fmt.Sprintf("invalid start_date format (expected YYYY-MM-DD): %v", err),
			})
			return
		}
		startDate = parsed
	}

	endDateStr := param.SafeRead(req, "end_date")
	if endDateStr != "" {
		parsed, err := time.Parse("2006-01-02", endDateStr)
		if err != nil {
			RespondWithJSON(http.StatusBadRequest, w, map[string]any{
				"code":    http.StatusBadRequest,
				"message": fmt.Sprintf("invalid end_date format (expected YYYY-MM-DD): %v", err),
			})
			return
		}
		// Make end_date inclusive
		endDate = parsed.AddDate(0, 0, 1)
	}

	if endDate.Before(startDate) {
		RespondWithJSON(http.StatusBadRequest, w, map[string]any{
			"code":    http.StatusBadRequest,
			"message": "end_date must be after start_date",
		})
		return
	}

	latestSHAOnly, err := param.ReadBool(req, "latest_sha_only", false)
	if err != nil {
		RespondWithJSON(http.StatusBadRequest, w, map[string]any{
			"code":    http.StatusBadRequest,
			"message": fmt.Sprintf("invalid latest_sha_only: %v", err),
		})
		return
	}
	includeSuccesses := req.URL.Query()["include_successes"]

	limit := defaultPRTestResultsLimit
	if limitStr := param.SafeRead(req, "limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil {
			limit = parsed
		}
	}

	results, err := GetPRTestResults(dbc, org, repo, prNumber, latestSHAOnly, startDate, endDate, includeSuccesses, limit)
	if err != nil {
		log.WithError(err).Error("error fetching PR test results")
		RespondWithJSON(http.StatusInternalServerError, w, map[string]any{
			"code":    http.StatusInternalServerError,
			"message": fmt.Sprintf("error fetching test results: %v", err),
		})
		return
	}

	RespondWithJSON(http.StatusOK, w, results)
}

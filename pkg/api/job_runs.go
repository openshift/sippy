package api

import (
	"fmt"
	"net/http"
	"reflect"
	gosort "sort"
	"strconv"
	"time"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/filter"
	log "github.com/sirupsen/logrus"
)

func (runs apiRunResults) sort(req *http.Request) apiRunResults {
	sortField := req.URL.Query().Get("sortField")
	sort := apitype.Sort(req.URL.Query().Get("sort"))

	if sortField == "" {
		sortField = "test_failures"
	}

	if sort == "" {
		sort = apitype.SortDescending
	}

	gosort.Slice(runs, func(i, j int) bool {
		if sort == apitype.SortAscending {
			return filter.Compare(runs[i], runs[j], sortField)
		}
		return filter.Compare(runs[j], runs[i], sortField)
	})

	return runs
}

func (runs apiRunResults) limit(req *http.Request) apiRunResults {
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	if limit > 0 && len(runs) >= limit {
		return runs[:limit]
	}

	return runs
}

type apiRunResults []apitype.JobRun

// JobsRunsReportFromDB renders a filtered summary of matching jobs.
func JobsRunsReportFromDB(dbc *db.DB, filterOpts *filter.FilterOptions, release string, pagination *apitype.Pagination, reportEnd time.Time) (*apitype.PaginationResult, error) {
	jobsResult := make([]apitype.JobRun, 0)
	table := "prow_job_runs_report_matview"
	q, err := filter.FilterableDBResult(dbc.DB.Table(table), filterOpts, apitype.JobRun{})
	if err != nil {
		return nil, err
	}

	if len(release) > 0 {
		q = q.Where("release = ?", release)
	}

	q = q.Where("timestamp < ?", reportEnd.UnixMilli())

	// Get the row count before pagination
	var rowCount int64
	q.Count(&rowCount)

	// Paginate the results:
	if pagination == nil {
		pagination = &apitype.Pagination{
			PerPage: int(rowCount),
			Page:    0,
		}
	} else {
		q = q.Limit(pagination.PerPage).Offset(pagination.Page * pagination.PerPage)
	}

	res := q.Scan(&jobsResult)
	return &apitype.PaginationResult{
		Rows:      jobsResult,
		TotalRows: rowCount,
		PageSize:  pagination.PerPage,
		Page:      pagination.Page,
	}, res.Error
}

// JobRunRiskAnalysis checks the test failures and linked bugs for a job run, and reports back an estimated
// risk level for each failed test, and the job run overall.
func JobRunRiskAnalysis(dbc *db.DB, jobRunID int64) (apitype.ProwJobRunRiskAnalysis, error) {

	jobRun := &models.ProwJobRun{}
	// Load the ProwJobRun, ProwJob, and failed tests:
	// TODO: we may want to expand to analyzing flakes here in the future
	res := dbc.DB.Joins("ProwJob").
		Preload("ProwJob.Bugs", "status != 'Closed' and status != 'Verified'").
		Preload("Tests", "status = 12").
		Preload("Tests.Test").Preload("Tests.Test.Bugs").
		Preload("Tests.Suite").First(jobRun, jobRunID)
	if res.Error != nil {
		return apitype.ProwJobRunRiskAnalysis{}, res.Error
	}

	// If this job is a Presubmit, compare to test results from master, not presubmits, which may perform
	// worse due to dev code that hasn't merged. We do not presently track presubmits on branches other than
	// master, so it should be safe to assume the latest compareRelease in the db.
	compareRelease := jobRun.ProwJob.Release
	if compareRelease == "Presubmits" {
		// Get latest release from the DB:
		ar, err := query.ReleasesFromDB(dbc)
		if err != nil {
			return apitype.ProwJobRunRiskAnalysis{}, err
		}
		if len(ar) == 0 {
			return apitype.ProwJobRunRiskAnalysis{}, fmt.Errorf("no releases found in db")
		}

		compareRelease = ar[0].Release
	}

	return runJobRunAnalysis(jobRun, compareRelease,
		func(testName, release, suite string, variants []string) (*apitype.Test, error) {

			logger := log.WithFields(log.Fields{
				"func":     "testResultsFunc",
				"jobRunID": jobRun.ID,
			})

			fil := &filter.Filter{
				Items: []filter.FilterItem{
					{
						Field:    "name",
						Not:      false,
						Operator: filter.OperatorEquals,
						Value:    testName,
					},
				},
				LinkOperator: "and",
			}
			trs, _, err := BuildTestsResults(dbc, release, "default", false, false,
				fil)
			if err != nil {
				return nil, err
			}
			logger.Infof("Got test results: %d", len(trs))
			for _, tr := range trs {
				// TODO: this is a weird way to get the variant we want, but it allows re-use
				// of the existing code.
				if reflect.DeepEqual(tr.Variants, variants) && tr.SuiteName == suite {
					return &tr, nil
				}
			}

			return nil, nil
		})
}

// testResultsFunc is used for injecting db responses in unit tests.
type testResultsFunc func(testName string, release, suite string, variants []string) (*apitype.Test, error)

func runJobRunAnalysis(jobRun *models.ProwJobRun, compareRelease string,
	testResultsFunc testResultsFunc) (apitype.ProwJobRunRiskAnalysis, error) {

	logger := log.WithFields(log.Fields{
		"func":     "jobRunAnalysis",
		"jobRunID": jobRun.ID,
	})

	logger.WithField("url", jobRun.URL).Info("loaded prow job run for analysis")
	logger.Infof("this job run has %d failed tests", len(jobRun.Tests))
	logger.WithField("variants", jobRun.ProwJob.Variants).Debug("job variants")

	response := apitype.ProwJobRunRiskAnalysis{
		ProwJobRunID: jobRun.ID,
		ProwJobName:  jobRun.ProwJob.Name,
		ProwJobURL:   jobRun.URL,
		Timestamp:    jobRun.Timestamp,
		Tests:        []apitype.ProwJobRunTestRiskAnalysis{},
		OverallRisk: apitype.FailureRisk{
			Level:   apitype.FailureRiskLevelNone,
			Reasons: []string{},
		},
		OpenBugs: jobRun.ProwJob.Bugs,
	}

	switch {

	// Return early if no tests failed in this run:
	case len(jobRun.Tests) == 0:
		response.OverallRisk.Level = apitype.FailureRiskLevelNone
		response.OverallRisk.Reasons = append(response.OverallRisk.Reasons,
			"No test failures found in this job run.")
		return response, nil

	// Return early if we see mass test failures:
	case len(jobRun.Tests) > 20:
		response.OverallRisk.Level = apitype.FailureRiskLevelHigh
		response.OverallRisk.Reasons = append(response.OverallRisk.Reasons,
			fmt.Sprintf("%d tests failed in this run: High", len(jobRun.Tests)))
		return response, nil
	}

	var maxTestRiskReason string

	// Iterate each failed test, query it's pass rates by NURPs, find a matching variant combo, and
	// see how often we've passed in the last week.
	for _, ft := range jobRun.Tests {
		logger.WithFields(log.Fields{
			"testID": ft.TestID,
			"name":   ft.Test.Name,
		}).Debug("failed test")

		testResult, err := testResultsFunc(
			ft.Test.Name, compareRelease, ft.Suite.Name, jobRun.ProwJob.Variants)
		if err != nil {
			return response, err
		}
		if testResult != nil {
			testRiskLvl := getSeverityLevelForPassRate(testResult.CurrentPassPercentage)
			if testRiskLvl.Level >= response.OverallRisk.Level.Level {
				response.OverallRisk.Level = testRiskLvl
				maxTestRiskReason = fmt.Sprintf("Maximum failed test risk: %s", testRiskLvl.Name)
			}
			response.Tests = append(response.Tests, apitype.ProwJobRunTestRiskAnalysis{
				Name: testResult.Name,
				Risk: apitype.FailureRisk{
					Level: testRiskLvl,
					Reasons: []string{
						fmt.Sprintf("This test has passed %.2f%% of %d runs on release %s %v in the last week.",
							testResult.CurrentPassPercentage, testResult.CurrentRuns, compareRelease, testResult.Variants),
					},
				},
				OpenBugs: ft.Test.Bugs,
			})
		} else {
			testRiskLvl := apitype.FailureRiskLevelUnknown
			if testRiskLvl.Level >= response.OverallRisk.Level.Level {
				response.OverallRisk.Level = testRiskLvl
				maxTestRiskReason = fmt.Sprintf("Maximum failed test risk: %s", testRiskLvl.Name)
			}
			response.Tests = append(response.Tests, apitype.ProwJobRunTestRiskAnalysis{
				Name: ft.Test.Name,
				Risk: apitype.FailureRisk{
					Level: testRiskLvl,
					Reasons: []string{
						fmt.Sprintf("Unable to find matching test results for variants: %v",
							jobRun.ProwJob.Variants),
					},
				},
			})
		}
	}

	response.OverallRisk.Reasons = append(response.OverallRisk.Reasons, maxTestRiskReason)

	return response, nil
}

func getSeverityLevelForPassRate(passPercentage float64) apitype.RiskLevel {
	switch {
	case passPercentage >= 98.0:
		return apitype.FailureRiskLevelHigh
	case passPercentage >= 80:
		return apitype.FailureRiskLevelMedium
	case passPercentage < 80:
		return apitype.FailureRiskLevelLow
	}
	return apitype.FailureRiskLevelUnknown
}

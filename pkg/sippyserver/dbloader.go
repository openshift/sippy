package sippyserver

import (
	"strconv"
	"strings"
	"time"

	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"k8s.io/klog"

	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridconversion"
	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"
)

func (a TestReportGeneratorConfig) PrepareDatabase(
	dbc *db.DB,
	dashboard TestGridDashboardCoordinates,
	variantManager testidentification.VariantManager,
	syntheticTestManager testgridconversion.SyntheticTestManager,
	bugCache buganalysis.BugCache, // required to associate tests with bug
) error {
	testGridJobDetails, _ := a.TestGridLoadingConfig.load(dashboard.TestGridDashboardNames)
	rawJobResultOptions := testgridconversion.ProcessingOptions{
		SyntheticTestManager: syntheticTestManager,
		// Load the last 30 days of data.  Note that we do not prune data today, so we'll be accumulating
		// data over time for now.
		StartDay: 0,
		NumDays:  30,
	}
	rawJobResults, _ := rawJobResultOptions.ProcessTestGridDataIntoRawJobResults(testGridJobDetails)

	testCtr := 0
	for _, rjr := range rawJobResults.JobResults {
		for _, rjrr := range rjr.JobRunResults {
			for _ = range rjrr.TestResults {
				testCtr++
			}
		}
	}
	klog.Infof("total test results: %d", testCtr)

	// allJobResults holds all the job results with all the test results.  It contains complete frequency information and
	/*
		allJobResults := testreportconversion.ConvertRawJobResultsToProcessedJobResults(
			dashboard.ReportName, rawJobResults, bugCache, dashboard.BugzillaRelease, variantManager)
	*/

	// Load all job and test results into database:
	klog.Info("loading ProwJobs into db")

	// Build up a cache of all prow jobs we know about to speedup data entry.
	// Maps job name to db ID.
	prowJobCache := map[string]uint{}
	var idNames []models.IDName
	dbc.DB.Model(&models.ProwJob{}).Find(&idNames)
	for _, idn := range idNames {
		if _, ok := prowJobCache[idn.Name]; !ok {
			prowJobCache[idn.Name] = idn.ID
		}
	}

	// Cache all tests by name to their ID, used for the join object.
	testCache := map[string]uint{}
	dbc.DB.Model(&models.Test{}).Find(&idNames)
	for _, idn := range idNames {
		if _, ok := testCache[idn.Name]; !ok {
			testCache[idn.Name] = idn.ID
		}
	}

	//jobRuns := []models.ProwJobRun{}
	for i := range rawJobResults.JobResults {
		jr := rawJobResults.JobResults[i]
		// Create ProwJob if we don't have one already:
		// TODO: we do not presently update a ProwJob once created, so any change in our variant detection code for ex
		// would not make it to the db.
		if _, ok := prowJobCache[jr.JobName]; !ok {
			dbProwJob := models.ProwJob{
				Name:        jr.JobName,
				Release:     dashboard.ReportName,
				Variants:    variantManager.IdentifyVariants(jr.JobName),
				TestGridURL: jr.TestGridJobURL,
			}
			err := dbc.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&dbProwJob).Error
			if err != nil {
				return errors.Wrapf(err, "error loading prow job into db: %s", jr.JobName)
			}
			prowJobCache[jr.JobName] = dbProwJob.ID
		}

		// CreateJobRuns if we don't have them already:
		for _, jobRun := range jr.JobRunResults {
			tokens := strings.Split(jobRun.JobRunURL, "/")
			prowID, _ := strconv.ParseUint(tokens[len(tokens)-1], 10, 64)

			// TODO: copy whatever's happening in jobresults.go
			//knownFailure := jobRun.Failed && areAllFailuresKnown(jrr, testResults)

			// success - we saw the setup/infra test result, it succeeded (or the whole job succeeeded)
			// failure - we saw the test result, it failed
			// unknown - we know this job doesn't have a setup test, and the job didn't succeed, so we don't know if it
			//           failed due to infra issues or not.  probably not infra.
			// emptystring - we expected to see a test result for a setup test but we didn't and the overall job failed, probably infra
			infraFailure := jobRun.SetupStatus != testgridanalysisapi.Success && jobRun.SetupStatus != testgridanalysisapi.Unknown

			pjr := models.ProwJobRun{
				Model: gorm.Model{
					ID: uint(prowID),
				},
				ProwJobID:             prowJobCache[jr.JobName],
				URL:                   jobRun.JobRunURL,
				TestFailures:          jobRun.TestFailures,
				Failed:                jobRun.Failed,
				InfrastructureFailure: infraFailure,
				KnownFailure:          false, // TODO: see above
				Succeeded:             jobRun.Succeeded,
				Timestamp:             time.Unix(int64(jobRun.Timestamp)/1000, 0), // Timestamp is in millis since epoch
				OverallResult:         jobRun.OverallResult,
			}

			/*
				failedTests := make([]models.Test, len(jobRun.FailedTestNames))
				for i, ftn := range jobRun.FailedTestNames {
					ft := models.Test{}
					r := dbc.DB.Where("name = ?", ftn).First(&ft)
					if errors.Is(r.Error, gorm.ErrRecordNotFound) {
						ft = models.Test{
							Name: ftn,
						}
						err := dbc.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&ft).Error
						if err != nil {
							return errors.Wrapf(err, "error loading test into db: %s", ft.Name)
						}
					}
					failedTests[i] = ft
				}
				pjr.FailedTests = failedTests
			*/

			err := dbc.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&pjr).Error
			if err != nil {
				return errors.Wrap(err, "error loading prow jobs into db")
			}

			testRuns := make([]models.ProwJobRunTest, len(jobRun.TestResults))
			for i, tr := range jobRun.TestResults {
				if _, ok := testCache[tr.Name]; !ok {
					klog.Info("Creating new test row: %s", tr.Name)
					t := models.Test{
						Name: tr.Name,
					}
					err := dbc.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&t).Error
					if err != nil {
						return errors.Wrapf(err, "error loading test into db: %s", t.Name)
					}
					testCache[tr.Name] = t.ID
				}
				testID := testCache[tr.Name]

				testRuns[i] = models.ProwJobRunTest{
					TestID:       testID,
					ProwJobRunID: pjr.ID,
					Status:       int(tr.Status),
				}
			}

			// Update to create test run join table entries now that we know the pjr ID.
			pjr.Tests = testRuns
			err = dbc.DB.Clauses(clause.OnConflict{UpdateAll: true}).Save(&pjr).Error
			if err != nil {
				return errors.Wrap(err, "error updating prow job run with tests")
			}

			//jobRuns = append(jobRuns, pjr)

		}
	}
	/*
		err := dbc.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(jobRuns, 1).Error
		if err != nil {
			return errors.Wrap(err, "error loading prow jobs into db")
		}

	*/

	klog.Info("done loading ProwJobs")

	return nil
}

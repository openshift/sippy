package sippyserver

import (
	"strconv"
	"strings"
	"time"

	v1 "github.com/openshift/sippy/pkg/apis/testgrid/v1"
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

func (a TestReportGeneratorConfig) LoadDatabase(
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

	// TODO: this can probably be removed, just for development purposes to see how many tests we're dealing with
	testCtr := 0
	for _, rjr := range rawJobResults.JobResults {
		for _, rjrr := range rjr.JobRunResults {
			for _ = range rjrr.TestResults {
				testCtr++
			}
		}
	}
	klog.V(4).Infof("total test results from testgrid data: %d", testCtr)

	// allJobResults holds all the job results with all the test results.  It contains complete frequency information and
	/*
		allJobResults := testreportconversion.ConvertRawJobResultsToProcessedJobResults(
			dashboard.ReportName, rawJobResults, bugCache, dashboard.BugzillaRelease, variantManager)
	*/

	// Load all job and test results into database:
	klog.V(4).Info("loading ProwJobs into db")

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
	klog.V(4).Infof("job cache created with %d entries from database", len(prowJobCache))

	// Cache the IDs of all known ProwJobRuns. Will be used to skip job run and test results we've already committed.
	prowJobRunCache := map[uint]bool{} // value is unused, just hashing
	allKnownJobRuns := []models.ProwJobRun{}
	dbc.DB.Select("id").Find(&allKnownJobRuns)
	for _, kjr := range allKnownJobRuns {
		prowJobRunCache[kjr.ID] = true
	}

	// First pass we just create any new ProwJobs we do not already have. This will allow us to run the second pass
	// inserts in parallel without conflicts.
	for i := range rawJobResults.JobResults {
		klog.V(4).Infof("Loading prow job %d of %d", i, len(rawJobResults.JobResults))
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
	}

	// Cache all tests by name to their ID, used for the join object.
	testCache := map[string]uint{}
	dbc.DB.Model(&models.Test{}).Find(&idNames)
	for _, idn := range idNames {
		if _, ok := testCache[idn.Name]; !ok {
			testCache[idn.Name] = idn.ID
		}
	}
	klog.V(4).Infof("test cache created with %d entries from database", len(testCache))

	// Second pass we create all ProwJobRuns we do not already have.
	// ProwJobRuns are created individually in a transaction to ensure we get the job run, and all it's test results
	// committed at the same time.
	//
	// TODO: parallelize with goroutines for faster entry
	for i := range rawJobResults.JobResults {
		jr := rawJobResults.JobResults[i]

		jobRunsToCreate := []models.ProwJobRun{}
		// CreateJobRuns if we don't have them already:
		for _, jobRun := range jr.JobRunResults {
			tokens := strings.Split(jobRun.JobRunURL, "/")
			prowID, _ := strconv.ParseUint(tokens[len(tokens)-1], 10, 64)

			if _, ok := prowJobRunCache[uint(prowID)]; ok {
				// skip job runs we already have:
				continue
			}

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

			// Add all test run results to the ProwJobRun. Due to oddness in the underlying structures, this requires
			// processing both the TestResults and the FailedTestNames, which are not in the TestResults.
			testRuns := make([]models.ProwJobRunTest, 0, len(jobRun.TestResults)+len(jobRun.FailedTestNames))
			for _, tr := range jobRun.TestResults {
				// TODO: Move outside this loop so we can cache faster.
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

				testRuns = append(testRuns, models.ProwJobRunTest{
					TestID:       testID,
					ProwJobRunID: pjr.ID,
					Status:       int(tr.Status),
				})
			}

			for _, ftn := range jobRun.FailedTestNames {
				if _, ok := testCache[ftn]; !ok {
					klog.Info("Creating new test row: %s", ftn)
					t := models.Test{
						Name: ftn,
					}
					err := dbc.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&t).Error
					if err != nil {
						return errors.Wrapf(err, "error loading test into db: %s", t.Name)
					}
					testCache[ftn] = t.ID
				}
				testID := testCache[ftn]

				testRuns = append(testRuns, models.ProwJobRunTest{
					TestID:       testID,
					ProwJobRunID: pjr.ID,
					Status:       int(v1.TestStatusFailure),
				})
			}

			pjr.Tests = testRuns

			//jobRuns = append(jobRuns, pjr)

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

			jobRunsToCreate = append(jobRunsToCreate, pjr)
			err := dbc.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&pjr).Error
			if err != nil {
				return errors.Wrap(err, "error loading prow job runs into db")
			}
		}
		/*
			err := dbc.DB.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(jobRunsToCreate, 50).Error
			if err != nil {
				return errors.Wrap(err, "error loading prow job runs into db")
			}
		*/
	}

	klog.Info("done loading ProwJobRuns")

	return nil
}

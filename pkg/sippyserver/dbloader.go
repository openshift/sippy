package sippyserver

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	bugsv1 "github.com/openshift/sippy/pkg/apis/bugs/v1"
	processingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	v1 "github.com/openshift/sippy/pkg/apis/testgrid/v1"
	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridconversion"
	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"
	"github.com/openshift/sippy/pkg/util/sets"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (a TestReportGeneratorConfig) LoadDatabase(
	dbc *db.DB,
	dashboard TestGridDashboardCoordinates,
	variantManager testidentification.VariantManager,
	syntheticTestManager testgridconversion.SyntheticTestManager) error {

	testGridJobDetails, _ := a.TestGridLoadingConfig.load(dashboard.TestGridDashboardNames)
	rawJobResultOptions := testgridconversion.ProcessingOptions{
		SyntheticTestManager: syntheticTestManager,
		// Load the last 14 days of data.
		StartDay: 0,
		NumDays:  14,
	}

	// Load all job and test results into database:
	log.Info("loading ProwJobs into db")

	// Load cache of all known prow jobs from DB:
	prowJobCache, err := LoadProwJobCache(dbc)
	if err != nil {
		return err
	}
	prowJobCacheLock := &sync.RWMutex{}

	// Load cache of all known tests from db:
	testCache, err := LoadTestCache(dbc)
	if err != nil {
		return err
	}
	testCacheLock := &sync.RWMutex{}

	// Cache all test suites by name to their ID, used for the join object.
	// Unlike other caches used in this area, this one is purely populated from the db.go initialization, we
	// only recognize certain suite names as test authors have used . liberally such that we cannot make any other
	// assumptions about what prefix is a suite name and what isn't.
	suiteCache := map[string]uint{}
	idNames := []models.IDName{}
	dbc.DB.Model(&models.Suite{}).Find(&idNames)
	for _, idn := range idNames {
		if _, ok := suiteCache[idn.Name]; !ok {
			suiteCache[idn.Name] = idn.ID
		}
	}
	log.Infof("test cache created with %d entries from database", len(testCache))

	for i, jobDetails := range testGridJobDetails {
		jobResult, warnings := rawJobResultOptions.ProcessJobDetailsIntoRawJobResult(jobDetails)
		for _, warning := range warnings {
			log.Warningf("warning from testgrid processing: " + warning)
		}

		log.Infof("Loading prow job %d of %d", i+1, len(testGridJobDetails))
		err := createOrUpdateJob(dbc, dashboard.ReportName, variantManager, prowJobCache, jobResult)
		if err != nil {
			return err
		}

		jobStatus := fmt.Sprintf("%d/%d", i, len(testGridJobDetails))

		// ProwJobRuns for a ProwJob are created in a batch transaction to ensure we get the job run and all it's test results
		// committed at the same time.
		err = loadJob(dbc, prowJobCache, prowJobCacheLock, suiteCache, testCache,
			testCacheLock, jobResult, jobStatus)
		if err != nil {
			return err
		}
	}

	log.Info("done loading ProwJobRuns")

	return nil
}

func createOrUpdateJob(dbc *db.DB, reportName string,
	variantManager testidentification.VariantManager, prowJobCache map[string]*models.ProwJob,
	jr *testgridanalysisapi.RawJobResult) error {
	// Create ProwJob if we don't have one already:
	// TODO: we do not presently update a ProwJob once created, so any change in our variant detection code for ex
	// would not make it to the db.
	if _, ok := prowJobCache[jr.JobName]; !ok {
		dbProwJob := &models.ProwJob{
			Name:        jr.JobName,
			Release:     reportName,
			Variants:    variantManager.IdentifyVariants(jr.JobName),
			TestGridURL: jr.TestGridJobURL,
		}
		err := dbc.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(dbProwJob).Error
		if err != nil {
			return errors.Wrapf(err, "error loading prow job into db: %s", jr.JobName)
		}
		prowJobCache[jr.JobName] = dbProwJob
	} else {
		// Ensure the job is up to date, especially for variants.
		dbProwJob := prowJobCache[jr.JobName]
		dbProwJob.Variants = variantManager.IdentifyVariants(jr.JobName)
		dbProwJob.TestGridURL = jr.TestGridJobURL
		dbc.DB.Save(&dbProwJob)
	}
	return nil
}

func LoadTestCache(dbc *db.DB) (map[string]*models.Test, error) {
	// Cache all tests by name to their ID, used for the join object.
	testCache := map[string]*models.Test{}
	allTests := []*models.Test{}
	res := dbc.DB.Model(&models.Test{}).Find(&allTests)
	if res.Error != nil {
		return map[string]*models.Test{}, res.Error
	}
	for _, idn := range allTests {
		if _, ok := testCache[idn.Name]; !ok {
			testCache[idn.Name] = idn
		}
	}
	log.Infof("test cache created with %d entries from database", len(testCache))
	return testCache, nil
}

func LoadProwJobCache(dbc *db.DB) (map[string]*models.ProwJob, error) {
	prowJobCache := map[string]*models.ProwJob{}
	var allJobs []*models.ProwJob
	res := dbc.DB.Model(&models.ProwJob{}).Find(&allJobs)
	if res.Error != nil {
		return map[string]*models.ProwJob{}, res.Error
	}
	for _, j := range allJobs {
		if _, ok := prowJobCache[j.Name]; !ok {
			prowJobCache[j.Name] = j
		}
	}
	log.Infof("job cache created with %d entries from database", len(prowJobCache))
	return prowJobCache, nil
}

func loadJob(
	dbc *db.DB,
	prowJobCache map[string]*models.ProwJob,
	prowJobCacheLock *sync.RWMutex,
	suiteCache map[string]uint,
	testCache map[string]*models.Test,
	testCacheLock *sync.RWMutex,
	jr *testgridanalysisapi.RawJobResult,
	jobStatus string) error {

	// Cache the IDs of all known ProwJobRuns for this job. Will be used to skip job run and test results we've already processed.
	prowJobRunCache := map[uint]bool{} // value is unused, just hashing
	knownJobRuns := []models.ProwJobRun{}
	prowJobCacheLock.RLock()
	prowJob := prowJobCache[jr.JobName]
	prowJobCacheLock.RUnlock()
	dbc.DB.Select("id").Where("prow_job_id = ?", prowJob.ID).Find(&knownJobRuns)
	log.Infof("Found %d known job runs in db for %s", len(knownJobRuns), jr.JobName)
	log.Infof("Loaded %d testgrid job runs for %s", len(jr.JobRunResults), jr.JobName)
	for _, kjr := range knownJobRuns {
		prowJobRunCache[kjr.ID] = true
	}

	// CreateJobRuns if we don't have them already:
	jobRunResultCtr := 0
	for _, jobRun := range jr.JobRunResults {
		log.WithFields(log.Fields{
			"job":   jr.JobName,
			"run":   jobRun.Job,
			"tests": len(jobRun.TestResults),
		}).Debug("loading job run")
		jobRunResultCtr++
		tokens := strings.Split(jobRun.JobRunURL, "/")
		prowID, _ := strconv.ParseUint(tokens[len(tokens)-1], 10, 64)

		if _, ok := prowJobRunCache[uint(prowID)]; ok {
			// skip job runs we already have:
			continue
		}

		if jobRun.OverallResult == processingv1.JobRunning {
			// skip jobs that are still running, we'll record them next time when they're complete
			continue
		}

		// TODO: copy whatever's happening in jobresults.go
		// knownFailure := jobRun.Failed && areAllFailuresKnown(jrr, testResults)

		// success - we saw the setup/infra test result, it succeeded (or the whole job succeeeded)
		// failure - we saw the test result, it failed
		// unknown - we know this job doesn't have a setup test, and the job didn't succeed, so we don't know if it
		//           failed due to infra issues or not.  probably not infra.
		// emptystring - we expected to see a test result for a setup test but we didn't and the overall job failed, probably infra
		infraFailure := jobRun.InstallStatus != testgridanalysisapi.Success && jobRun.InstallStatus != testgridanalysisapi.Unknown

		pjr := models.ProwJobRun{
			Model: gorm.Model{
				ID: uint(prowID),
			},
			ProwJobID:             prowJob.ID,
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
			suiteID, testName := getSuiteIDAndTestName(suiteCache, tr.Name)

			testID, err := getOrCreateTestID(dbc, testName, testCache, testCacheLock)
			if err != nil {
				return err
			}

			pjrt := models.ProwJobRunTest{
				TestID:       testID,
				ProwJobRunID: pjr.ID,
				Status:       int(tr.Status),
			}
			if suiteID > 0 {
				pjrt.SuiteID = &suiteID
			}
			testRuns = append(testRuns, pjrt)
		}

		for _, ftn := range jobRun.FailedTestNames {
			suiteID, testName := getSuiteIDAndTestName(suiteCache, ftn)

			testID, err := getOrCreateTestID(dbc, testName, testCache, testCacheLock)
			if err != nil {
				return err
			}

			pjrt := models.ProwJobRunTest{
				TestID:       testID,
				ProwJobRunID: pjr.ID,
				Status:       int(v1.TestStatusFailure),
			}
			if suiteID > 0 {
				pjrt.SuiteID = &suiteID
			}
			testRuns = append(testRuns, pjrt)
		}

		// Some upstream kube jobs have ~40k tests in each run, which can hit limits in postgres doing
		// a single insert.
		// To work around this we have to create the tests in batches, separately from the job run.
		// We still want to commit all of this as a single transaction however, as this allows us to
		// retry loading the db and not miss job runs. (we check existence of job run IDs and only
		// load those we don't have)
		err := dbc.DB.Transaction(func(tx *gorm.DB) error {
			err := dbc.DB.Create(&pjr).Error
			if err != nil {
				return err
			}

			err = dbc.DB.CreateInBatches(testRuns, 1000).Error
			if err != nil {
				return err
			}

			log.Infof("Created prow job run %d/%d of job %s", jobRunResultCtr, len(jr.JobRunResults), jobStatus)
			return nil
		})
		if err != nil {
			return errors.Wrapf(err, "error loading prow job %s run %d into db",
				pjr.ProwJob.Name, pjr.ID)
		}
	}
	return nil
}

// getSuiteIDAndTestName uses the suiteCache from the db to determine if the testname starts with a known test suite
// prefix. If so we return the suiteID to associate with on the test run row, otherwise 0.
func getSuiteIDAndTestName(suiteCache map[string]uint, origTestName string) (suiteID uint, testName string) {
	for suitePrefix, suiteID := range suiteCache {
		if strings.HasPrefix(origTestName, suitePrefix+".") {
			return suiteID, origTestName[len(suitePrefix)+1:]
		}

	}
	return 0, origTestName
}

func getOrCreateTestID(
	dbc *db.DB,
	testName string,
	testCache map[string]*models.Test,
	testCacheLock *sync.RWMutex) (testID uint, err error) {

	testCacheLock.RLock()
	if _, ok := testCache[testName]; !ok {
		log.Infof("Creating new test row: %s", testName)
		t := &models.Test{
			Name: testName,
		}
		err := dbc.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(t).Error
		if err != nil {
			return 0, errors.Wrapf(err, "error loading test into db: %s", testName)
		}
		testCache[testName] = t
	}
	testID = testCache[testName].ID
	testCacheLock.RUnlock()

	return testID, nil
}

// LoadBugs does a bulk query of all our test names, 50 at a time, to bugzilla and then syncs the associations to the db.
func LoadBugs(dbc *db.DB, bugCache buganalysis.BugCache, testCache map[string]*models.Test, jobCache map[string]*models.ProwJob) error {
	log.Info("querying bugzilla for test/job associations")
	bugCache.Clear()

	if err := bugCache.UpdateForFailedTests(sets.StringKeySet(testCache).List()...); err != nil {
		log.Warningf("Bugzilla Lookup Error: an error was encountered looking up existing bugs for failing tests, some test failures may have associated bugs that are not listed below.  Lookup error: %v", err.Error())
	}
	if err := bugCache.UpdateJobBlockers(sets.StringKeySet(jobCache).List()...); err != nil {
		log.Warningf("Bugzilla Lookup Error: an error was encountered looking up existing bugs for failing tests, some test failures may have associated bugs that are not listed below.  Lookup error: %v", err.Error())
	}

	log.Info("syncing bugzilla test/job associations to db")

	// Merge the test/job bugs into one list, associated with each failing test or job, mapped to our db model for the bug.
	dbExpectedBugs := map[int64]*models.Bug{}

	for testName, apiBugArr := range bugCache.ListAllTestBugs() {
		for _, apiBug := range apiBugArr {
			if _, ok := dbExpectedBugs[apiBug.ID]; !ok {
				newBug := convertAPIBugToDBBug(apiBug)
				dbExpectedBugs[apiBug.ID] = newBug
			}
			if _, ok := testCache[testName]; !ok {
				// Shouldn't be possible, if it is we want to know.
				panic("Test name in bug cache does not exist in db: " + testName)
			}
			dbExpectedBugs[apiBug.ID].Tests = append(dbExpectedBugs[apiBug.ID].Tests, *testCache[testName])
		}
	}

	for jobName, apiBugArr := range bugCache.ListAllJobBlockingBugs() {
		for _, apiBug := range apiBugArr {
			if _, ok := dbExpectedBugs[apiBug.ID]; !ok {
				newBug := convertAPIBugToDBBug(apiBug)
				dbExpectedBugs[apiBug.ID] = newBug
			}
			if _, ok := jobCache[jobName]; !ok {
				// Shouldn't be possible, if it is we want to know.
				panic("Job name in bug cache does not exist in db: " + jobName)
			}
			dbExpectedBugs[apiBug.ID].Jobs = append(dbExpectedBugs[apiBug.ID].Jobs, *jobCache[jobName])
		}
	}

	for _, bug := range dbExpectedBugs {
		res := dbc.DB.Clauses(clause.OnConflict{
			UpdateAll: true,
		}).Create(bug)
		if res.Error != nil {
			log.Errorf("error creating bug: %s %v", res.Error, bug)
			return errors.Wrap(res.Error, "error creating bug")
		}
		// With gorm we need to explicitly replace the associations to tests and jobs to get them to take effect:
		err := dbc.DB.Model(bug).Association("Tests").Replace(bug.Tests)
		if err != nil {
			log.Errorf("error updating bug test associations: %s %v", err, bug)
			return errors.Wrap(res.Error, "error updating bug test assocations")
		}
	}

	return nil
}

func convertAPIBugToDBBug(apiBug bugsv1.Bug) *models.Bug {
	newBug := &models.Bug{
		Model: gorm.Model{
			ID: uint(apiBug.ID),
		},
		Status:         apiBug.Status,
		LastChangeTime: apiBug.LastChangeTime,
		Summary:        apiBug.Summary,
		URL:            apiBug.URL,
		FailureCount:   apiBug.FailureCount,
		FlakeCount:     apiBug.FlakeCount,
		Tests:          []models.Test{},
	}
	// We are assuming single valued for each of these despite the fact the bz models appear to support
	// multi-valued, OpenShift does not use this if so.
	if len(apiBug.TargetRelease) > 0 {
		newBug.TargetRelease = apiBug.TargetRelease[0]
	}
	if len(apiBug.Version) > 0 {
		newBug.Version = apiBug.Version[0]
	}
	if len(apiBug.Component) > 0 {
		newBug.Component = apiBug.Component[0]
	}
	return newBug
}

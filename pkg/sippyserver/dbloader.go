package sippyserver

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/andygrunwald/go-jira"
	"github.com/openshift/sippy/pkg/db/loader"
	"github.com/openshift/sippy/pkg/util/sets"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	processingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	v1 "github.com/openshift/sippy/pkg/apis/testgrid/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/synthetictests"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridconversion"
	"github.com/openshift/sippy/pkg/testidentification"
)

var FindIssuesForVariants = loader.FindIssuesForVariants

func (a TestReportGeneratorConfig) LoadDatabase(
	dbc *db.DB,
	dashboard TestGridDashboardCoordinates,
	variantManager testidentification.VariantManager,
	syntheticTestManager synthetictests.SyntheticTestManager,
	startDay, numDays int, reportEnd time.Time) error {

	testGridJobDetails, _ := a.TestGridLoadingConfig.load(dashboard.TestGridDashboardNames)
	rawJobResultOptions := testgridconversion.ProcessingOptions{
		SyntheticTestManager: syntheticTestManager,
		StartDay:             startDay,
		NumDays:              numDays,
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
	testCache, err := LoadTestCache(dbc, []string{})
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
		jobResult, warnings := rawJobResultOptions.ProcessJobDetailsIntoRawJobResult(jobDetails, reportEnd)
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
	jr *processingv1.RawJobResult) error {
	// Create ProwJob if we don't have one already:
	// TODO: we do not presently update a ProwJob once created, so any change in our variant detection code for ex
	// would not make it to the db.
	if _, ok := prowJobCache[jr.JobName]; !ok {
		dbProwJob := &models.ProwJob{
			Name:        jr.JobName,
			Release:     reportName,
			Variants:    variantManager.IdentifyVariants(jr.JobName, reportName),
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
		dbProwJob.Variants = variantManager.IdentifyVariants(jr.JobName, reportName)
		dbProwJob.TestGridURL = jr.TestGridJobURL
		dbc.DB.Save(&dbProwJob)
	}
	return nil
}

func LoadTestCache(dbc *db.DB, preloads []string) (map[string]*models.Test, error) {
	// Cache all tests by name to their ID, used for the join object.
	testCache := map[string]*models.Test{}
	allTests := []*models.Test{}
	q := dbc.DB.Model(&models.Test{})
	for _, p := range preloads {
		q = q.Preload(p)
	}
	res := q.Find(&allTests)
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
	jr *processingv1.RawJobResult,
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
		infraFailure := jobRun.InstallStatus != testidentification.Success && jobRun.InstallStatus != testidentification.Unknown

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

func variantsKey(variants []string) string {
	v := make([]string, len(variants))
	copy(v, variants)
	sort.Strings(v)
	return strings.Join(v, ",")
}

func appendJobIssuesFromVariants(jobCache map[string]*models.ProwJob, jobIssues map[string][]jira.Issue) error {
	// variantSetMap maps a sorted names of variants to a set of the variants for easy comparison
	variantsSetMap := map[string]sets.String{}
	// variantsIssuesMap maps a sorted names of variants to a slice of issues
	variantsIssuesMap := map[string][]jira.Issue{}

	variantIssues, err := FindIssuesForVariants()
	if err != nil {
		log.Warningf("Issue Lookup Error: an error was encountered looking up existing bugs for variants, some test failures may have associated bugs that are not listed below.  Lookup error: %v", err.Error())
		return err
	}

	variantMatches := regexp.MustCompile(loader.VariantSearchRegex)
	for key, issues := range variantIssues {
		subMatches := variantMatches.FindStringSubmatch(key)
		if len(subMatches) == 2 {
			// Update the variantsSetMap
			variants := strings.Split(subMatches[1], ",")
			variantsKey := variantsKey(variants)
			if _, ok := variantsSetMap[variantsKey]; !ok {
				variantsSetMap[variantsKey] = sets.NewString(variants...)
			}

			// Update the variantsIssuesMap
			if _, ok := variantsIssuesMap[variantsKey]; !ok {
				variantsIssuesMap[variantsKey] = []jira.Issue{}
			}
			variantsIssuesMap[variantsKey] = append(variantsIssuesMap[variantsKey], issues...)
		}
	}
	// Now go through all jobs to append issues
	for _, job := range jobCache {
		if len(job.Variants) > 0 {
			variantsKey := variantsKey(job.Variants)

			// Cache in the map for subsequent jobs
			if _, ok := variantsSetMap[variantsKey]; !ok {
				variantsSetMap[variantsKey] = sets.NewString(job.Variants...)
			}

			for key, issues := range variantsIssuesMap {
				if !variantsSetMap[variantsKey].IsSuperset(variantsSetMap[key]) {
					continue
				}
				candidates := []jira.Issue{}
				for _, issue := range issues {
					if issue.Fields == nil {
						continue
					}
					for _, version := range issue.Fields.AffectsVersions {
						if job.Release == version.Name || strings.HasPrefix(version.Name, job.Release+".") {
							candidates = append(candidates, issue)
							break
						}
					}
				}
				jobSearchStrings := fmt.Sprintf("job=%s=all", job.Name)
				if _, ok := jobIssues[jobSearchStrings]; !ok {
					jobIssues[jobSearchStrings] = []jira.Issue{}
				}
				jobIssues[jobSearchStrings] = append(jobIssues[jobSearchStrings], candidates...)
			}
		}
	}
	return nil
}

func UpdateWatchlist(dbc *db.DB) error {
	// Load the test cache, we'll iterate every test and see if it should be in the watchlist or not:
	testCache, err := LoadTestCache(dbc, []string{"Bugs"})
	if err != nil {
		return err
	}

	for testName, test := range testCache {
		expected := testidentification.IsTestOnWatchlist(test)
		if test.Watchlist != expected {
			log.WithFields(log.Fields{"old": test.Watchlist, "new": expected}).Infof("test watchlist status changed for %s", testName)
			test.Watchlist = expected
			res := dbc.DB.Save(test)
			if res.Error != nil {
				log.WithError(err).Error("error updating test watchlist status")
			}
		}
	}
	return nil
}

// LoadBugs does a bulk query of all our tests and jobs, 50 at a time, to search.ci and then syncs the associations to the db.
func LoadBugs(dbc *db.DB, testCache map[string]*models.Test, jobCache map[string]*models.ProwJob) error {
	log.Info("querying search.ci for test/job associations")
	testIssues, err := loader.FindIssuesForTests(sets.StringKeySet(testCache).List()...)
	if err != nil {
		log.Warningf("Issue Lookup Error: an error was encountered looking up existing bugs for failing tests, some test failures may have associated bugs that are not listed below.  Lookup error: %v", err.Error())
	}

	jobIssues, err := loader.FindIssuesForJobs(sets.StringKeySet(jobCache).List()...)
	if err != nil {
		log.Warningf("Issue Lookup Error: an error was encountered looking up existing bugs for failing tests, some test failures may have associated bugs that are not listed below.  Lookup error: %v", err.Error())
	}

	err = appendJobIssuesFromVariants(jobCache, jobIssues)
	if err != nil {
		log.Warningf("Issue Lookup Error: an error was encountered looking up existing bugs by jobs by variants.  Lookup error: %v", err.Error())
	}

	log.Info("syncing issue test/job associations to db")

	// Merge the test/job bugs into one list, associated with each failing test or job, mapped to our db model for the bug.
	dbExpectedBugs := map[int64]*models.Bug{}

	for testName, apiBugArr := range testIssues {
		for _, apiBug := range apiBugArr {
			issueID, err := strconv.ParseInt(apiBug.ID, 10, 64)
			if err != nil {
				log.WithError(err).Errorf("error parsing issue ID: %+v", apiBug)
				// TODO: start returning errors here? Eris working on a fix for results with context but no issues
				// return errors.Wrap(err, "error parsing issue ID")
				continue
			}
			if _, ok := dbExpectedBugs[issueID]; !ok {
				log.Debugf("converting issue: %+v", apiBug)
				newBug := convertAPIIssueToDBIssue(issueID, apiBug)
				dbExpectedBugs[issueID] = newBug
			}
			if _, ok := testCache[testName]; !ok {
				// Shouldn't be possible, if it is we want to know.
				panic("Test name in bug cache does not exist in db: " + testName)
			}
			dbExpectedBugs[issueID].Tests = append(dbExpectedBugs[issueID].Tests, *testCache[testName])
		}
	}

	log.WithField("jobIssues", len(jobIssues)).Info("found job issues")
	for jobSearchStr, apiBugArr := range jobIssues {
		for _, apiBug := range apiBugArr {
			issueID, err := strconv.ParseInt(apiBug.ID, 10, 64)
			if err != nil {
				log.WithError(err).Errorf("error parsing issue ID: %+v", apiBug)
				// TODO: start returning errors here? Eris working on a fix for results with context but no issues
				// return errors.Wrap(err, "error parsing issue ID")
				continue
			}
			if _, ok := dbExpectedBugs[issueID]; !ok {
				newBug := convertAPIIssueToDBIssue(issueID, apiBug)
				dbExpectedBugs[issueID] = newBug
			}
			// We search for job=[jobname]=all, need to extract the raw job name from that search string
			// which is what appears in our jobIssues map.
			jobName := jobSearchStr[4 : len(jobSearchStr)-4]
			if _, ok := jobCache[jobName]; !ok {
				// Shouldn't be possible, if it is we want to know.
				panic("Job name in bug cache does not exist in db: " + jobName)
			}
			dbExpectedBugs[issueID].Jobs = append(dbExpectedBugs[issueID].Jobs, *jobCache[jobName])
		}
	}

	expectedBugIDs := make([]uint, 0, len(dbExpectedBugs))
	for _, bug := range dbExpectedBugs {
		expectedBugIDs = append(expectedBugIDs, bug.ID)
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
		err = dbc.DB.Model(bug).Association("Jobs").Replace(bug.Jobs)
		if err != nil {
			log.Errorf("error updating bug job associations: %s %v", err, bug)
			return errors.Wrap(res.Error, "error updating bug job assocations")
		}
	}

	// Delete all stale referenced bugs that are no longer in our expected bugs.
	// Unscoped deletes the rows from the db, rather than soft delete.
	res := dbc.DB.Where("id not in ?", expectedBugIDs).Unscoped().Delete(&models.Bug{})
	if res.Error != nil {
		return errors.Wrap(res.Error, "error deleting stale bugs")
	}
	log.Infof("deleted %d stale bugs", res.RowsAffected)

	return nil
}

func convertAPIIssueToDBIssue(issueID int64, apiIssue jira.Issue) *models.Bug {
	newBug := &models.Bug{
		ID:             uint(issueID),
		Key:            apiIssue.Key,
		Status:         apiIssue.Fields.Status.Name,
		LastChangeTime: time.Time(apiIssue.Fields.Updated),
		Summary:        apiIssue.Fields.Summary,
		URL:            fmt.Sprintf("https://issues.redhat.com/browse/%s", apiIssue.Key),
		Tests:          []models.Test{},
	}

	// The version and components fields may typically or always be just one value, but we're told it
	// may not be possible to actually prevent someone adding multiple, so we'll be ready for the possibility.
	components := []string{}
	for _, c := range apiIssue.Fields.Components {
		components = append(components, c.Name)
	}
	sort.Strings(components)
	newBug.Components = components

	affectsVersions := []string{}
	for _, av := range apiIssue.Fields.AffectsVersions {
		affectsVersions = append(affectsVersions, av.Name)
	}
	sort.Strings(affectsVersions)
	newBug.AffectsVersions = affectsVersions

	fixVersions := []string{}
	for _, fv := range apiIssue.Fields.FixVersions {
		fixVersions = append(fixVersions, fv.Name)
	}
	sort.Strings(fixVersions)
	newBug.FixVersions = fixVersions

	labels := apiIssue.Fields.Labels
	labels = append(labels, apiIssue.Fields.Labels...)
	sort.Strings(labels)
	newBug.Labels = labels

	return newBug
}

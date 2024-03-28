package bugloader

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/andygrunwald/go-jira"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/loader"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/testidentification"
	"github.com/openshift/sippy/pkg/util/sets"
)

var FindIssuesForVariants = loader.FindIssuesForVariants

type BugLoader struct {
	dbc    *db.DB
	errors []error
}

func New(dbc *db.DB) *BugLoader {
	return &BugLoader{
		dbc: dbc,
	}
}

func (bl *BugLoader) Name() string {
	return "bugs"
}

func (bl *BugLoader) Errors() []error {
	return bl.errors
}

// LoadBugs does a bulk query of all our tests and jobs, 50 at a time, to search.ci and then syncs the associations to the db.
func (bl *BugLoader) Load() {
	testCache, err := loadTestCache(bl.dbc, []string{})
	if err != nil {
		bl.errors = append(bl.errors, err)
		return
	}

	jobCache, err := loadProwJobCache(bl.dbc)
	if err != nil {
		bl.errors = append(bl.errors, err)
		return
	}

	log.Info("querying search.ci for test/job associations")
	testIssues, err := loader.FindIssuesForTests(sets.StringKeySet(testCache).List()...)
	if err != nil {
		log.WithError(err).Warning("Issue Lookup Error: an error was encountered looking up existing bugs for failing tests, some test failures may have associated bugs that are not listed below.")
		err = errors.Wrap(err, "error querying bugs for tests")
		bl.errors = append(bl.errors, err)
	}

	jobIssues, err := loader.FindIssuesForJobs(sets.StringKeySet(jobCache).List()...)
	if err != nil {
		log.WithError(err).Warning("Issue Lookup Error: an error was encountered looking up existing bugs for failing jobs, some job failures may have associated bugs that are not listed below.")
		err = errors.Wrap(err, "error querying bugs for jobs")
		bl.errors = append(bl.errors, err)
	}

	err = appendJobIssuesFromVariants(jobCache, jobIssues)
	if err != nil {
		log.WithError(err).Warning("Issue Lookup Error: an error was encountered looking up existing bugs by jobs by variants.")
		err = errors.Wrap(err, "error querying bugs for variants")
		bl.errors = append(bl.errors, err)
	}

	log.Info("syncing issue test/job associations to db")

	// Merge the test/job bugs into one list, associated with each failing test or job, mapped to our db model for the bug.
	dbExpectedBugs := map[int64]*models.Bug{}

	for testName, apiBugArr := range testIssues {
		for _, apiBug := range apiBugArr {
			issueID, err := strconv.ParseInt(apiBug.ID, 10, 64)
			if err != nil {
				log.WithError(err).Errorf("error parsing issue ID: %+v", apiBug)
				err = errors.Wrap(err, "error parsing issue ID")
				bl.errors = append(bl.errors, err)
				continue
			}
			if _, ok := dbExpectedBugs[issueID]; !ok {
				log.Debugf("converting issue: %+v", apiBug)
				newBug := convertAPIIssueToDBIssue(issueID, apiBug)
				dbExpectedBugs[issueID] = newBug
			}
			if _, ok := testCache[testName]; !ok {
				// Shouldn't be possible, if it is we want to know.
				err := fmt.Errorf("test name was in bug cache but not in database?: %s", testName)
				log.WithError(err).Error("unexpected error getting test from cache")
				bl.errors = append(bl.errors, err)
				continue
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
				err = errors.Wrap(err, "error parsing issue ID")
				bl.errors = append(bl.errors, err)
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
				err := fmt.Errorf("job name was in bug cache but not in database?: %s", jobName)
				log.WithError(err).Error("unexpected error getting job from cache")
				bl.errors = append(bl.errors, err)
				continue
			}
			dbExpectedBugs[issueID].Jobs = append(dbExpectedBugs[issueID].Jobs, *jobCache[jobName])
		}
	}

	expectedBugIDs := make([]uint, 0, len(dbExpectedBugs))
	for _, bug := range dbExpectedBugs {
		expectedBugIDs = append(expectedBugIDs, bug.ID)
		res := bl.dbc.DB.Clauses(clause.OnConflict{
			UpdateAll: true,
		}).Create(bug)
		if res.Error != nil {
			log.Errorf("error creating bug: %s %v", res.Error, bug)
			err := errors.Wrap(res.Error, "error creating bug")
			bl.errors = append(bl.errors, err)
			continue
		}
		// With gorm we need to explicitly replace the associations to tests and jobs to get them to take effect:
		err := bl.dbc.DB.Model(bug).Association("Tests").Replace(bug.Tests)
		if err != nil {
			log.Errorf("error updating bug test associations: %s %v", err, bug)
			err := errors.Wrap(res.Error, "error updating bug test assocations")
			bl.errors = append(bl.errors, err)
			continue
		}
		err = bl.dbc.DB.Model(bug).Association("Jobs").Replace(bug.Jobs)
		if err != nil {
			log.Errorf("error updating bug job associations: %s %v", err, bug)
			err := errors.Wrap(res.Error, "error updating bug job assocations")
			bl.errors = append(bl.errors, err)
			continue
		}
	}

	// Delete all stale referenced bugs that are no longer in our expected bugs.
	// Unscoped deletes the rows from the db, rather than soft delete.
	res := bl.dbc.DB.Where("id not in ?", expectedBugIDs).Unscoped().Delete(&models.Bug{})
	if res.Error != nil {
		err := errors.Wrap(res.Error, "error deleting stale bugs")
		bl.errors = append(bl.errors, err)
	}
	log.Infof("deleted %d stale bugs", res.RowsAffected)

	// Update watch list
	if err := updateWatchlist(bl.dbc); err != nil {
		bl.errors = append(bl.errors, err...)
	}

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

func loadTestCache(dbc *db.DB, preloads []string) (map[string]*models.Test, error) {
	// Cache all tests by name to their ID, used for the join object.
	testCache := map[string]*models.Test{}
	q := dbc.DB.Model(&models.Test{})
	for _, p := range preloads {
		q = q.Preload(p)
	}

	// Kube exceeds 60000 tests, more than postgres can load at once:
	testsBatch := []*models.Test{}
	res := q.FindInBatches(&testsBatch, 5000, func(tx *gorm.DB, batch int) error {
		for _, idn := range testsBatch {
			if _, ok := testCache[idn.Name]; !ok {
				testCache[idn.Name] = idn
			}
		}
		return nil
	})

	if res.Error != nil {
		return map[string]*models.Test{}, res.Error
	}

	log.Infof("test cache created with %d entries from database", len(testCache))
	return testCache, nil
}

func loadProwJobCache(dbc *db.DB) (map[string]*models.ProwJob, error) {
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

func updateWatchlist(dbc *db.DB) []error {
	// Load the test cache, we'll iterate every test and see if it should be in the watchlist or not:
	testCache, err := loadTestCache(dbc, []string{"Bugs"})
	if err != nil {
		return []error{errors.Wrap(err, "error loading test class for UpdateWatchList")}
	}

	errs := []error{}
	for testName, test := range testCache {
		expected := testidentification.IsTestOnWatchlist(test)
		if test.Watchlist != expected {
			log.WithFields(log.Fields{"old": test.Watchlist, "new": expected}).Infof("test watchlist status changed for %s", testName)
			test.Watchlist = expected
			res := dbc.DB.Save(test)
			if res.Error != nil {
				log.WithError(err).Errorf("error updating test watchlist status for: %s", testName)
				errs = append(errs, errors.Wrapf(err, "error updating test watchlist status for: %s", testName))
			}
		}
	}
	return errs
}

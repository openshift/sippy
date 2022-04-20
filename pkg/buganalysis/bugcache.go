package buganalysis

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"regexp/syntax"
	"strings"
	"sync"
	"time"

	bugsv1 "github.com/openshift/sippy/pkg/apis/bugs/v1"
	"github.com/openshift/sippy/pkg/buganalysis/internal"
	"github.com/openshift/sippy/pkg/util"
	log "github.com/sirupsen/logrus"
)

// BugCache is a thread-safe way to query about bug status.
// It is stateful though, so for a time after clearing the data will not be up to date until the Update is called
type BugCache interface {
	ListJobBlockingBugs(job string) []bugsv1.Bug
	ListBugs(release, variant, testName string) []bugsv1.Bug
	ListAllTestBugs() map[string][]bugsv1.Bug
	ListAllJobBlockingBugs() map[string][]bugsv1.Bug
	// ListAssociatedBugs lists bugs that match the testname or variant, but do not match the specified release
	ListAssociatedBugs(release, variant, testName string) []bugsv1.Bug
	UpdateForFailedTests(failedTestNames ...string) error
	UpdateJobBlockers(jobNames ...string) error
	Clear()
	// LastUpdateError returns the last update error, if one exists
	LastUpdateError() error
}

// noOpBugCache is a no-op implementation of the bug cache/lookup interface
// used to opt-out of bug lookups for faster test analysis/reporting.
type noOpBugCache struct {
}

func (*noOpBugCache) ListJobBlockingBugs(job string) []bugsv1.Bug {
	return []bugsv1.Bug{}
}
func (*noOpBugCache) ListBugs(release, variant, testName string) []bugsv1.Bug {
	return []bugsv1.Bug{}
}
func (*noOpBugCache) ListAllTestBugs() map[string][]bugsv1.Bug {
	return map[string][]bugsv1.Bug{}
}
func (*noOpBugCache) ListAllJobBlockingBugs() map[string][]bugsv1.Bug {
	return map[string][]bugsv1.Bug{}
}
func (*noOpBugCache) ListAssociatedBugs(release, variant, testName string) []bugsv1.Bug {
	return []bugsv1.Bug{}
}
func (*noOpBugCache) UpdateForFailedTests(failedTestNames ...string) error {
	return nil
}
func (*noOpBugCache) UpdateJobBlockers(jobNames ...string) error {
	return nil
}
func (*noOpBugCache) Clear() {}
func (*noOpBugCache) LastUpdateError() error {
	return nil
}

func NewNoOpBugCache() BugCache {
	return &noOpBugCache{}
}

type bugCache struct {
	lock sync.RWMutex
	// maps test name to any bugzilla bug mentioning the test name
	testBugsCache map[string][]bugsv1.Bug
	// jobBlockersBugCache is indexed by getJobKey(jobName) and lists the bugs that are considered to be responsible for all failures on a job.
	jobBlockersBugCache map[string][]bugsv1.Bug
	lastUpdateError     error
}

func NewBugCache() BugCache {
	return &bugCache{
		testBugsCache:       map[string][]bugsv1.Bug{},
		jobBlockersBugCache: map[string][]bugsv1.Bug{},
	}
}

// UpdateForFailedTests updates a global variable with the bug mapping based on current failures.
func (c *bugCache) UpdateForFailedTests(failedTestNames ...string) error {
	c.lock.RLock()
	newFailedTestNames := []string{}
	for _, testName := range failedTestNames {
		if _, found := c.testBugsCache[testName]; !found {
			newFailedTestNames = append(newFailedTestNames, testName)
		}
	}
	c.lock.RUnlock()
	newBugs, lastUpdateError := findBugsForFailedTests(newFailedTestNames...)

	c.lock.Lock()
	defer c.lock.Unlock()

	for testName, bug := range newBugs {
		c.testBugsCache[testName] = bug
	}
	c.lastUpdateError = lastUpdateError
	return lastUpdateError
}

func GetJobKey(jobName string) string {
	return fmt.Sprintf("job=%v=all", jobName)
}

// UpdateJobBlockers updates a global variable with the bug mapping based on current failures.
func (c *bugCache) UpdateJobBlockers(jobNames ...string) error {
	c.lock.RLock()
	jobSearchStrings := []string{}
	for _, jobName := range jobNames {
		jobKey := GetJobKey(jobName)
		if _, found := c.jobBlockersBugCache[jobKey]; !found {
			jobSearchStrings = append(jobSearchStrings, jobKey)
		}
	}
	c.lock.RUnlock()
	newBugs, lastUpdateError := findBugsForFailedTests(jobSearchStrings...)

	c.lock.Lock()
	defer c.lock.Unlock()

	for testName, bug := range newBugs {
		c.jobBlockersBugCache[testName] = bug
	}
	c.lastUpdateError = lastUpdateError
	return lastUpdateError
}

// finds bugs given the test names
func findBugsForFailedTests(failedTestNames ...string) (map[string][]bugsv1.Bug, error) {
	ret := map[string][]bugsv1.Bug{}

	var lastUpdateError error
	batchTestNames := []string{}
	queryCtr := 0
	for i, testName := range failedTestNames {
		if _, found := ret[testName]; found {
			continue
		}
		batchTestNames = append(batchTestNames, testName)
		// we're going to lookup bugs for this test, so put an entry into the map.
		// if we find a bug for this test, the entry will be replaced with the actual
		// array of bugs.  if not, this serves as a placeholder so we know not to look
		// it up again in the future.
		ret[testName] = []bugsv1.Bug{}

		// continue building our batch until we have a largish set to check
		onLastItem := (i + 1) == len(failedTestNames)
		if !onLastItem && len(batchTestNames) <= 50 {
			continue
		}

		r, err := findBugs(batchTestNames)
		queryCtr++
		for k, v := range r {
			ret[k] = v
		}
		if err != nil {
			lastUpdateError = err
		}
		batchTestNames = []string{}
	}
	log.Debugf("findBugsForFailedTests made %d bugzilla requests", queryCtr)

	return ret, lastUpdateError
}

func (c *bugCache) Clear() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.testBugsCache = map[string][]bugsv1.Bug{}
	c.jobBlockersBugCache = map[string][]bugsv1.Bug{}
	c.lastUpdateError = nil
}

func (c *bugCache) LastUpdateError() error {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.lastUpdateError
}

//nolint:revive // flag-parameter: parameter 'invertReleaseQuery' seems to be a control flag, avoid control coupling
func (c *bugCache) listBugsInternal(release, jobName, testName string, invertReleaseQuery bool) []bugsv1.Bug {
	c.lock.RLock()
	defer c.lock.RUnlock()

	ret := []bugsv1.Bug{}

	// first check if this job is covered by a job-blocking bug.  If so, all test
	// failures are attributed to that bug instead of to individual test bugs.
	bugList := c.jobBlockersBugCache[GetJobKey(jobName)]
	for i := range bugList {
		bug := bugList[i]
		// If a target release is set, we prefer that, but if the bug was found in the version we're interested in,
		// we consider that a linked bug and not an associated bug too.
		if len(bug.TargetRelease) == 1 && bug.TargetRelease[0] == "---" {
			for _, r := range bug.Version {
				if (!invertReleaseQuery && strings.HasPrefix(r, release)) || (invertReleaseQuery && !strings.HasPrefix(r, release)) {
					ret = append(ret, bug)
					break
				}
			}
		} else {
			for _, r := range bug.TargetRelease {
				if (!invertReleaseQuery && strings.HasPrefix(r, release)) || (invertReleaseQuery && !strings.HasPrefix(r, release)) {
					ret = append(ret, bug)
					break
				}
			}
		}
	}
	if len(ret) > 0 {
		return ret
	}

	bugList = c.testBugsCache[testName]
	for i := range bugList {
		bug := bugList[i]
		if len(bug.TargetRelease) == 1 && bug.TargetRelease[0] == "---" {
			for _, r := range bug.Version {
				if (!invertReleaseQuery && strings.HasPrefix(r, release)) || (invertReleaseQuery && !strings.HasPrefix(r, release)) {
					ret = append(ret, bug)
					break
				}
			}
		} else {
			for _, r := range bug.TargetRelease {
				if (!invertReleaseQuery && strings.HasPrefix(r, release)) || (invertReleaseQuery && !strings.HasPrefix(r, release)) {
					ret = append(ret, bug)
					break
				}
			}
		}
	}
	return ret
}

func (c *bugCache) ListBugs(release, jobName, testName string) []bugsv1.Bug {
	return c.listBugsInternal(release, jobName, testName, false)
}

func (c *bugCache) ListAssociatedBugs(release, jobName, testName string) []bugsv1.Bug {
	return c.listBugsInternal(release, jobName, testName, true)
}

func (c *bugCache) ListJobBlockingBugs(jobName string) []bugsv1.Bug {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.jobBlockersBugCache[GetJobKey(jobName)]
}

func (c *bugCache) ListAllTestBugs() map[string][]bugsv1.Bug {
	return c.testBugsCache
}

func (c *bugCache) ListAllJobBlockingBugs() map[string][]bugsv1.Bug {
	// Our job blocker cache is of format "job=[jobname]=all". For purposes of this function we need to
	// return just the job name.
	jobNameToBugs := map[string][]bugsv1.Bug{}
	for jobSearchStr, bugs := range c.jobBlockersBugCache {
		trimmed := strings.TrimPrefix(jobSearchStr, "job=")
		trimmed = strings.TrimSuffix(trimmed, "=all")
		jobNameToBugs[trimmed] = bugs
	}
	return jobNameToBugs
}

func findBugs(testNames []string) (map[string][]bugsv1.Bug, error) {
	searchResults := make(map[string][]bugsv1.Bug)

	v := url.Values{}
	v.Set("type", "bug")
	v.Set("context", "-1")
	for _, testName := range testNames {
		testName = regexp.QuoteMeta(testName)
		log.Debugf("Searching bugs for test name: %s\n", testName)
		v.Add("search", testName)
	}

	bzQueryStart := time.Now()
	searchURL := "https://search.ci.openshift.org/v2/search"
	resp, err := http.PostForm(searchURL, v)
	if err != nil {
		e := fmt.Errorf("error during bug search against %s: %w", searchURL, err)
		log.WithError(err).Errorf("error during bug search against %s", searchURL)
		return searchResults, e
	}
	if resp.StatusCode != 200 {
		e := fmt.Errorf("Non-200 response code during bug search against %s: %s", searchURL, resp.Status)
		log.WithError(e).Error("error")
		return searchResults, e
	}

	search := internal.Search{}

	if err := json.NewDecoder(resp.Body).Decode(&search); err != nil {
		e := fmt.Errorf("could not decode bug search results: %w", err)
		log.WithError(err).Errorf("error decoding bug search results")
		return searchResults, e
	}

	for searchString, result := range search.Results {
		// reverse the regex escaping we did earlier, so we get back the pure test name string.
		r, _ := syntax.Parse(searchString, 0)
		searchString = string(r.Rune)
		for _, match := range result.Matches {
			bug := match.Bug
			bug.URL = fmt.Sprintf("https://bugzilla.redhat.com/show_bug.cgi?id=%d", bug.ID)

			// search.ci.openshift.org seems to occasionally return empty BZ results, filter
			// them out.
			if bug.ID == 0 {
				continue
			}

			// ignore any bugs verified over a week ago, they cannot be responsible for test failures
			// (or the bug was incorrectly verified and needs to be revisited)
			if !util.IsActiveBug(bug) {
				if bug.LastChangeTime.Add(time.Hour * 24 * 7).Before(time.Now()) {
					continue
				}
			}
			searchResults[searchString] = append(searchResults[searchString], bug)
		}
	}

	log.Infof("Found bugs: %v", searchResults)
	log.Infof("bugzilla query took: %s", time.Since(bzQueryStart))
	return searchResults, nil
}

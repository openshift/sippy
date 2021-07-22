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
	"k8s.io/klog"
)

// BugCache is a thread-safe way to query about bug status.
// It is stateful though, so for a time after clearing the data will not be up to date until the Update is called
type BugCache interface {
	ListJobBlockingBugs(job string) []bugsv1.Bug
	ListBugs(release, variant, testName string) []bugsv1.Bug
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
	lock  sync.RWMutex
	cache map[string][]bugsv1.Bug
	// jobBlockers is indexed by getJobKey(jobName) and lists the bugs that are considered to be responsible for all failures on a job
	jobBlockers     map[string][]bugsv1.Bug
	lastUpdateError error
}

func NewBugCache() BugCache {
	return &bugCache{
		cache:       map[string][]bugsv1.Bug{},
		jobBlockers: map[string][]bugsv1.Bug{},
	}
}

// updates a global variable with the bug mapping based on current failures.
func (c *bugCache) UpdateForFailedTests(failedTestNames ...string) error {
	c.lock.RLock()
	newFailedTestNames := []string{}
	for _, testName := range failedTestNames {
		if _, found := c.cache[testName]; !found {
			newFailedTestNames = append(newFailedTestNames, testName)
		}
	}
	c.lock.RUnlock()
	newBugs, lastUpdateError := findBugsForFailedTests(newFailedTestNames...)

	c.lock.Lock()
	defer c.lock.Unlock()

	for testName, bug := range newBugs {
		c.cache[testName] = bug
	}
	c.lastUpdateError = lastUpdateError
	return lastUpdateError
}

func GetJobKey(jobName string) string {
	return fmt.Sprintf("job=%v=all", jobName)
}

// updates a global variable with the bug mapping based on current failures.
func (c *bugCache) UpdateJobBlockers(jobNames ...string) error {
	c.lock.RLock()
	jobSearchStrings := []string{}
	for _, jobName := range jobNames {
		jobKey := GetJobKey(jobName)
		if _, found := c.jobBlockers[jobKey]; !found {
			jobSearchStrings = append(jobSearchStrings, jobKey)
		}
	}
	c.lock.RUnlock()
	newBugs, lastUpdateError := findBugsForFailedTests(jobSearchStrings...)

	c.lock.Lock()
	defer c.lock.Unlock()

	for testName, bug := range newBugs {
		c.jobBlockers[testName] = bug
	}
	c.lastUpdateError = lastUpdateError
	return lastUpdateError
}

// finds bugs given the test names
func findBugsForFailedTests(failedTestNames ...string) (map[string][]bugsv1.Bug, error) {
	ret := map[string][]bugsv1.Bug{}

	var lastUpdateError error
	batchTestNames := []string{}
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
		for k, v := range r {
			ret[k] = v
		}
		if err != nil {
			lastUpdateError = err
		}
		batchTestNames = []string{}
	}

	return ret, lastUpdateError
}

func (c *bugCache) Clear() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.cache = map[string][]bugsv1.Bug{}
	c.jobBlockers = map[string][]bugsv1.Bug{}
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
	bugList := c.jobBlockers[GetJobKey(jobName)]
	for i := range bugList {
		bug := bugList[i]
		for _, r := range bug.TargetRelease {
			if (!invertReleaseQuery && strings.HasPrefix(r, release)) || (invertReleaseQuery && !strings.HasPrefix(r, release)) {
				ret = append(ret, bug)
				break
			}
		}
	}
	if len(ret) > 0 {
		return ret
	}

	bugList = c.cache[testName]
	for i := range bugList {
		bug := bugList[i]
		for _, r := range bug.TargetRelease {

			if (!invertReleaseQuery && strings.HasPrefix(r, release)) || (invertReleaseQuery && !strings.HasPrefix(r, release)) {
				ret = append(ret, bug)
				break
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

	return c.jobBlockers[GetJobKey(jobName)]
}

func findBugs(testNames []string) (map[string][]bugsv1.Bug, error) {
	searchResults := make(map[string][]bugsv1.Bug)

	v := url.Values{}
	v.Set("type", "bug")
	v.Set("context", "-1")
	for _, testName := range testNames {
		testName = regexp.QuoteMeta(testName)
		klog.V(4).Infof("Searching bugs for test name: %s\n", testName)
		v.Add("search", testName)
	}

	searchURL := "https://search.ci.openshift.org/v2/search"
	resp, err := http.PostForm(searchURL, v)
	if err != nil {
		e := fmt.Errorf("error during bug search against %s: %w", searchURL, err)
		klog.Errorf(e.Error())
		return searchResults, e
	}
	if resp.StatusCode != 200 {
		e := fmt.Errorf("Non-200 response code during bug search against %s: %s", searchURL, resp.Status)
		klog.Errorf(e.Error())
		return searchResults, e
	}

	search := internal.Search{}

	if err := json.NewDecoder(resp.Body).Decode(&search); err != nil {
		e := fmt.Errorf("could not decode bug search results: %w", err)
		klog.Errorf(e.Error())
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

	klog.V(2).Infof("Found bugs: %v", searchResults)
	return searchResults, nil
}

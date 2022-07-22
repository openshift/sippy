package loader

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"regexp/syntax"
	"time"

	"github.com/andygrunwald/go-jira"
	log "github.com/sirupsen/logrus"
)

// FindIssuesForTests queries search.ci for Jira issues mapping based to the given test names.
func FindIssuesForTests(testNames ...string) (map[string][]jira.Issue, error) {

	issues := map[string][]jira.Issue{}

	newBugs, lastUpdateError := findBugsForSearchStrings(testNames...)

	for testName, bug := range newBugs {
		issues[testName] = bug
	}
	return issues, lastUpdateError
}

// FindIssuesForJobs queries search.ci for Jira issues mapping based to the given job names.
func FindIssuesForJobs(jobNames ...string) (map[string][]jira.Issue, error) {

	issues := map[string][]jira.Issue{}

	newBugs, lastUpdateError := findBugsForSearchStrings(jobNames...)

	for jobKey, bug := range newBugs {
		issues[jobKey] = bug
	}
	return issues, lastUpdateError
}

// findBugsForSearchStrings finds issues in batches based on the given search strings. These can be test names
// or job names.
func findBugsForSearchStrings(failedTestNames ...string) (map[string][]jira.Issue, error) {
	ret := map[string][]jira.Issue{}

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
		ret[testName] = []jira.Issue{}

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
	log.Debugf("findBugsForSearchStrings made %d search.ci requests", queryCtr)

	return ret, lastUpdateError
}

/*
//nolint:revive // flag-parameter: parameter 'invertReleaseQuery' seems to be a control flag, avoid control coupling
func listBugsInternal(release, jobName, testName string, invertReleaseQuery bool) []bugsv1.Bug {
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

*/

func findBugs(testNames []string) (map[string][]jira.Issue, error) {
	searchResults := make(map[string][]jira.Issue)

	v := url.Values{}
	v.Set("type", "issue")
	v.Set("context", "-1")
	for _, testName := range testNames {
		testName = regexp.QuoteMeta(testName)
		//log.Debugf("Searching bugs for test name: %s\n", testName)
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

	search := Search{}

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
			issue := match.Issues
			/*
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

			*/
			searchResults[searchString] = append(searchResults[searchString], issue)
		}
	}

	log.Infof("Found bugs: %v", searchResults)
	log.Infof("bugzilla query took: %s", time.Since(bzQueryStart))
	return searchResults, nil
}

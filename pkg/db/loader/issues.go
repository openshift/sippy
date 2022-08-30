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

	// To prevent jobs matching sub-strings of other jobs, we'll keep the old syntax
	// from the bugzilla code where we search for job=[jobname]=all.
	jobSearchStrings := make([]string, len(jobNames))
	for i, jn := range jobNames {
		jobSearchStrings[i] = fmt.Sprintf("job=%s=all", jn)
	}

	newBugs, lastUpdateError := findBugsForSearchStrings(jobSearchStrings...)

	for jobKey, bug := range newBugs {
		issues[jobKey] = bug
	}
	return issues, lastUpdateError
}

// findBugsForSearchStrings finds issues in batches based on the given search strings. These can be test names
// or job names.
func findBugsForSearchStrings(searchFor ...string) (map[string][]jira.Issue, error) {
	ret := map[string][]jira.Issue{}

	var lastUpdateError error
	batchSearchStrs := []string{}
	queryCtr := 0
	for i, searchStr := range searchFor {
		if _, found := ret[searchStr]; found {
			continue
		}
		batchSearchStrs = append(batchSearchStrs, searchStr)
		// we're going to lookup bugs for this test/job, so put an entry into the map.
		// if we find a bug for this search string, the entry will be replaced with the actual
		// array of bugs.  if not, this serves as a placeholder so we know not to look
		// it up again in the future.
		ret[searchStr] = []jira.Issue{}

		// continue building our batch until we have a largish set to check
		onLastItem := (i + 1) == len(searchFor)
		if !onLastItem && len(batchSearchStrs) <= 50 {
			continue
		}

		r, err := findBugs(batchSearchStrs)
		queryCtr++
		for k, v := range r {
			ret[k] = v
		}
		if err != nil {
			lastUpdateError = err
		}
		batchSearchStrs = []string{}
	}
	log.Debugf("findBugsForSearchStrings made %d search.ci requests", queryCtr)

	return ret, lastUpdateError
}

func findBugs(testNames []string) (map[string][]jira.Issue, error) {
	searchResults := make(map[string][]jira.Issue)

	v := url.Values{}
	v.Set("type", "issue")
	v.Set("context", "-1")
	for _, testName := range testNames {
		testName = regexp.QuoteMeta(testName)
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
			searchResults[searchString] = append(searchResults[searchString], issue)
		}
	}

	log.Debugf("Found bugs: %v", searchResults)
	log.Debugf("bugzilla query took: %s", time.Since(bzQueryStart))
	return searchResults, nil
}

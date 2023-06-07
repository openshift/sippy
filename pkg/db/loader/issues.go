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

const batchSize = 25

var (
	// VariantSearchRegex defines the search regex for search.ci
	VariantSearchRegex = "sippy-link=\\[variants=(\\S+)\\]"
	// by default maxMatches is 1 for search.ci API. Since we are doing regex match, we pick 100 as the default.
	// This should be decided by the number of combination of variants.
	regexMaxMatches = "100"
)

// FindIssuesForTests queries search.ci for Jira issues mapping based to the given test names.
func FindIssuesForTests(testNames ...string) (map[string][]jira.Issue, error) {

	issues := map[string][]jira.Issue{}

	newBugs, lastUpdateError := findBugsForSearchStrings(false, testNames...)

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

	newBugs, lastUpdateError := findBugsForSearchStrings(false, jobSearchStrings...)

	for jobKey, bug := range newBugs {
		issues[jobKey] = bug
	}
	return issues, lastUpdateError
}

// FindIssuesForVariants queries search.ci for Jira issues with incident=variants= annotation.
func FindIssuesForVariants() (map[string][]jira.Issue, error) {
	issues := map[string][]jira.Issue{}
	variantSearchStrings := []string{VariantSearchRegex}
	newBugs, lastUpdateError := findBugsForSearchStrings(true, variantSearchStrings...)

	for jobKey, bug := range newBugs {
		issues[jobKey] = bug
	}
	return issues, lastUpdateError
}

// findBugsForSearchStrings finds issues in batches based on the given search strings. These can be test names,
// job names or job variants.
// isRegex defines whether the search is exact match or match by regex. If match by regex, the matched strings
// in the result context will be used as keys instead of the search string.
func findBugsForSearchStrings(isRegex bool, searchFor ...string) (map[string][]jira.Issue, error) {
	const maxFindBugErrors = 3

	ret := map[string][]jira.Issue{}

	var lastUpdateError error
	batchSearchStrs := []string{}
	queryCtr := 0
	findBugsErrorCount := 0
	for i, searchStr := range searchFor {
		if _, found := ret[searchStr]; found {
			continue
		}
		batchSearchStrs = append(batchSearchStrs, searchStr)
		// we're going to lookup bugs for this test/job, so put an entry into the map.
		// if we find a bug for this search string, the entry will be replaced with the actual
		// array of bugs.  if not, this serves as a placeholder so we know not to look
		// it up again in the future.
		if !isRegex {
			ret[searchStr] = []jira.Issue{}
		}

		// continue building our batch until we have a largish set to check
		onLastItem := (i + 1) == len(searchFor)
		if !onLastItem && len(batchSearchStrs) <= batchSize {
			continue
		}

		r, err := findBugs(isRegex, batchSearchStrs)
		queryCtr++
		for k, v := range r {
			ret[k] = v
		}
		if err != nil {
			lastUpdateError = err
			findBugsErrorCount++
			log.Warnf("findBugsForSearchStrings got error (%d of %d) in findBugs '%v' after %d requests",
				findBugsErrorCount, maxFindBugErrors, err, queryCtr)
			if findBugsErrorCount == maxFindBugErrors {
				// If we exceed what we're willing to tolerate, finish doing search.ci lookups to avoid
				// long delays for fetchdata uploads.
				log.Errorf("findBugs calls with error exceeded max times; search.ci queries aborted")
				break
			}
		}
		batchSearchStrs = []string{}
	}
	log.Infof("findBugsForSearchStrings made %d search.ci requests", queryCtr)

	return ret, lastUpdateError
}

func findBugs(isRegex bool, testNames []string) (map[string][]jira.Issue, error) {
	searchResults := make(map[string][]jira.Issue)

	v := url.Values{}
	v.Set("type", "issue")
	v.Set("context", "1")
	v.Set("maxMatches", regexMaxMatches)
	for _, testName := range testNames {
		if !isRegex {
			testName = regexp.QuoteMeta(testName)
		}
		v.Add("search", testName)
	}

	bzQueryStart := time.Now()
	searchURL := "https://search.ci.openshift.org/v2/search"

	// Set the timeout to something other than 0, which is the default and means no timeout.
	// This prevents waiting for too long in case search.ci is responding slower than usual.
	client := &http.Client{
		Timeout: time.Second * 30,
	}
	resp, err := client.PostForm(searchURL, v)
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

	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&search); err != nil {
		e := fmt.Errorf("could not decode bug search results: %w", err)
		log.WithError(err).Errorf("error decoding bug search results")
		return searchResults, e
	}

	for searchString, result := range search.Results {
		// reverse the regex escaping we did earlier, so we get back the pure test name string.
		if !isRegex {
			r, _ := syntax.Parse(searchString, 0)
			searchString = string(r.Rune)
		}
		for _, match := range result.Matches {
			issue := match.Issues
			if !isRegex {
				searchResults[searchString] = append(searchResults[searchString], issue)
			} else {
				for _, ctx := range match.Context {
					searchResults[ctx] = append(searchResults[ctx], issue)
				}
			}
		}
	}

	log.Debugf("Found bugs: %v", searchResults)
	log.Debugf("bugzilla query took: %s", time.Since(bzQueryStart))
	return searchResults, nil
}

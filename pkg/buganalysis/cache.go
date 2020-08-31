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

	"github.com/openshift/sippy/pkg/buganalysis/internal"

	bugsv1 "github.com/openshift/sippy/pkg/apis/bugs/v1"
	"k8s.io/klog"
)

// BugCache is a thread-safe way to query about bug status.
// It is stateful though, so for a time after clearing the data will not be up to date until the Update is called
type BugCache interface {
	ListBugs(release, platform, testName string) []bugsv1.Bug
	UpdateForFailedTests(failedTestNames ...string)

	Clear()
	// LastUpdateError returns the last update error, if one exists
	LastUpdateError() error
}

type bugCache struct {
	lock            sync.RWMutex
	cache           map[string][]bugsv1.Bug
	lastUpdateError error
}

func NewBugCache() BugCache {
	return &bugCache{
		cache: map[string][]bugsv1.Bug{},
	}
}

// updates a global variable with the bug mapping based on current failures.
func (c *bugCache) UpdateForFailedTests(failedTestNames ...string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	batchCount := 0
	batchNames := []string{}
	for _, testName := range failedTestNames {
		if _, found := c.cache[testName]; found {
			continue
		}
		batchNames = append(batchNames, testName)
		// we're going to lookup bugs for this test, so put an entry into the map.
		// if we find a bug for this test, the entry will be replaced with the actual
		// array of bugs.  if not, this serves as a placeholder so we know not to look
		// it up again in the future.
		c.cache[testName] = []bugsv1.Bug{}
		batchCount++

		if batchCount > 50 {
			r, err := findBugs(batchNames)
			for k, v := range r {
				c.cache[k] = v
			}
			if err != nil {
				c.lastUpdateError = err
			}
			batchNames = []string{}
			batchCount = 0
		}
	}
	if batchCount > 0 {
		r, err := findBugs(batchNames)
		for k, v := range r {
			c.cache[k] = v
		}
		if err != nil {
			c.lastUpdateError = err
		}
	}

	c.lastUpdateError = nil
}

func (c *bugCache) Clear() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.cache = map[string][]bugsv1.Bug{}
	c.lastUpdateError = nil
}

func (c *bugCache) LastUpdateError() error {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.lastUpdateError
}

func (c *bugCache) ListBugs(release, platform, testName string) []bugsv1.Bug {
	c.lock.RLock()
	defer c.lock.RUnlock()

	ret := []bugsv1.Bug{}
	bugList := c.cache[testName]
	for i := range bugList {
		bug := bugList[i]
		for _, r := range bug.TargetRelease {
			if strings.HasPrefix(r, release) {
				ret = append(ret, bug)
				break
			}
		}
	}
	return ret
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

	//searchUrl:="https://search.apps.build01.ci.devcluster.openshift.com/search"
	searchUrl := "https://search.ci.openshift.org/v2/search"
	resp, err := http.PostForm(searchUrl, v)
	if err != nil {
		e := fmt.Errorf("error during bug search against %s: %s", searchUrl, err)
		klog.Errorf(e.Error())
		return searchResults, e
	}
	if resp.StatusCode != 200 {
		e := fmt.Errorf("Non-200 response code during bug search against %s: %s", searchUrl, resp.Status)
		klog.Errorf(e.Error())
		return searchResults, e
	}

	search := internal.Search{}
	err = json.NewDecoder(resp.Body).Decode(&search)

	for searchString, result := range search.Results {
		// reverse the regex escaping we did earlier, so we get back the pure test name string.
		r, _ := syntax.Parse(searchString, 0)
		searchString = string(r.Rune)
		for _, match := range result.Matches {
			bug := match.Bug
			bug.Url = fmt.Sprintf("https://bugzilla.redhat.com/show_bug.cgi?id=%d", bug.ID)

			// ignore any bugs verified over a week ago, they cannot be responsible for test failures
			// (or the bug was incorrectly verified and needs to be revisited)
			if bug.Status == "VERIFIED" {
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

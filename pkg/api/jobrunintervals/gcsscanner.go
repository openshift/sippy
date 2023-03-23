package jobrunintervals

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/openshift/sippy/pkg/apis/api"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/html"
)

// Original code borrowed from https://github.com/vrutkovs/kaas/

// Scanner locates files in the artifacts Google Cloud Storage sub-buckets for a given prow job URL.
type Scanner struct{}

func (g *Scanner) FindMatchingFiles(baseURL string) ([]*url.URL, error) {
	foundFiles := []*url.URL{}
	// Find the link to gcs artifacts on the prow job page:
	gcsURL, err := GetMatchingLinkFromURL(baseURL, regexp.MustCompile(".*gcsweb.*"), false)
	if err != nil {
		return []*url.URL{}, err
	}
	log.WithField("gcsURL", gcsURL).Info("found GCS URL")

	artifactsURL, err := GetMatchingLinkFromURL(gcsURL.String(), regexp.MustCompile("artifacts"), true)
	if err != nil {
		return []*url.URL{}, err
	}
	log.WithField("artifactsURL", artifactsURL).Info("found artifacts URL")

	// Get a list of folders and find those which contain e2e, looking for the top level bucket for the job
	// i.e. e2e-gcp-ovn-upgrade
	e2eURL, err := GetMatchingLinkFromURL(artifactsURL.String(), regexp.MustCompile(".*e2e.*"), true)
	if err != nil {
		return []*url.URL{}, err
	}
	log.WithField("e2eURL", e2eURL).Info("found e2eURL")

	// Locate gather-extra/artifacts/ for some files:
	/*
		gatherExtraURL, err := GetMatchingLinkFromURL(e2eURL.String(), regexp.MustCompile("gather-extra"), true)
		if err != nil {
			return []*url.URL{}, err
		}
		gatherExtraURL, err = GetMatchingLinkFromURL(gatherExtraURL.String(), regexp.MustCompile("artifacts"), true)
		if err != nil {
			return []*url.URL{}, err
		}
		log.WithField("gatherExtraURL", gatherExtraURL).Info("found gatherExtraURL")
		kubeEventsURL, err := GetMatchingLinkFromURL(gatherExtraURL.String(), regexp.MustCompile("events.json"), true)
		if err != nil {
			return []*url.URL{}, err
		}
		foundFiles = append(foundFiles, kubeEventsURL)
	*/

	// Locate openshift-e2e-test:
	// TODO: may need an adjustment for baremetal jobs which have a different name here?
	e2eTestFilesURL, err := GetMatchingLinkFromURL(e2eURL.String(), regexp.MustCompile("openshift-e2e-test$"), true)
	if err != nil {
		return []*url.URL{}, err
	}
	e2eTestFilesURL, err = GetMatchingLinkFromURL(e2eTestFilesURL.String(), regexp.MustCompile("^artifacts$"), true)
	if err != nil {
		return []*url.URL{}, err
	}
	e2eTestFilesURL, err = GetMatchingLinkFromURL(e2eTestFilesURL.String(), regexp.MustCompile("^junit$"), true)
	if err != nil {
		return []*url.URL{}, err
	}
	e2eTestFiles, err := GetMatchingLinksFromURL(e2eTestFilesURL.String(), []*regexp.Regexp{
		regexp.MustCompile("e2e-events_.*\\.json"),
		//regexp.MustCompile("e2e-intervals_everything_.*\\.json"),
	}, true)
	if err != nil {
		return []*url.URL{}, err
	}
	for _, e2ef := range e2eTestFiles {
		foundFiles = append(foundFiles, e2ef)
	}

	return foundFiles, nil
}

func GetLinksFromURL(url string) ([]string, error) {
	links := []string{}

	var netClient = &http.Client{
		Timeout: time.Second * 10,
	}
	resp, err := netClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s: %v", url, err)
	}
	defer resp.Body.Close()

	z := html.NewTokenizer(resp.Body)
	for {
		tt := z.Next()

		switch {
		case tt == html.ErrorToken:
			// End of the document, we're done
			return links, nil
		case tt == html.StartTagToken:
			t := z.Token()

			isAnchor := t.Data == "a"
			if isAnchor {
				for _, a := range t.Attr {
					if a.Key == "href" {
						links = append(links, a.Val)
						break
					}
				}
			}
		}
	}
}

func GetMatchingLinkFromURL(baseURL string, regex *regexp.Regexp, onlyMatchLastSegment bool) (*url.URL, error) {
	urls, err := GetMatchingLinksFromURL(baseURL, []*regexp.Regexp{regex}, onlyMatchLastSegment)
	if err != nil {
		return nil, err
	}
	// We're expecting exactly one in this function call, anything else is an error:
	if len(urls) != 1 {
		return nil, fmt.Errorf("expected 1 matching URL, found %d on: %s", len(urls), baseURL)
	}
	return urls[0], nil
}

// GetMatchingLinksFromURL scans all the links found on the given URL for those matching a regex.
// Used to crawl GCS bucket links from our prow jobs.
func GetMatchingLinksFromURL(baseURL string, regexes []*regexp.Regexp, onlyMatchLastSegment bool) ([]*url.URL, error) {
	allLinks, err := GetLinksFromURL(baseURL)
	if err != nil {
		return []*url.URL{}, fmt.Errorf("failed to fetch links on %s: %v", baseURL, err)
	}
	if len(allLinks) == 0 {
		return []*url.URL{}, fmt.Errorf("no links found on: %s", baseURL)
	}

	matchedLinks := []string{}
	for _, link := range allLinks {
		log.WithField("link", link).Debug("checking link")
		linkSplitBySlash := strings.Split(link, "/")
		lastPathSegment := linkSplitBySlash[len(linkSplitBySlash)-1]
		if len(lastPathSegment) == 0 {
			lastPathSegment = linkSplitBySlash[len(linkSplitBySlash)-2]
		}
		for _, re := range regexes {
			if (onlyMatchLastSegment && re.Match([]byte(lastPathSegment))) ||
				(!onlyMatchLastSegment && re.Match([]byte(link))) {

				log.WithField("link", link).Debug("found link match")
				matchedLinks = append(matchedLinks, link)
			}
		}
	}

	matchedURLs := make([]*url.URL, len(matchedLinks))
	for i, ml := range matchedLinks {
		matchedLink := ml
		if strings.HasPrefix(ml, "/") {
			matchedLink = gcsPrefix + ml
		}
		mURL, err := url.Parse(matchedLink)
		if err != nil {
			return []*url.URL{}, fmt.Errorf("failed to parse URL from link %s: %v", ml, err)
		}
		matchedURLs[i] = mURL

	}
	return matchedURLs, nil
}

func ReadIntervalsFile(url *url.URL) (*api.EventIntervalList, error) {
	log.WithField("url", url).Info("getting intervals file")

	// Get the base filename we'll add to each incoming interval:
	tokens := strings.Split(url.Path, "/")
	baseFile := tokens[len(tokens)-1]

	resp, err := http.Get(url.String())
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.WithError(err).Error("error reading response body fetching intervals file")
		return nil, err
	}
	log.WithField("bytes", len(body)).Info("read response body")

	var intervals api.EventIntervalList
	if err := json.Unmarshal(body, &intervals); err != nil {
		log.WithError(err).Error("error unmarshaling intervals file")
		return nil, err
	}
	for i := range intervals.Items {
		intervals.Items[i].Filename = baseFile
	}

	return &intervals, nil
}

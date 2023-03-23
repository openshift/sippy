package jobrunintervals

import (
	"fmt"
	"net/url"
	"strings"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db/models"
	log "github.com/sirupsen/logrus"
)

const (
	gcsPrefix     = "https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com"
	storagePrefix = "https://storage.googleapis.com"
)

// JobRunIntervals fetches intervals for a given job run.
func JobRunIntervals(jobRun *models.ProwJobRun, logger *log.Entry) (*apitype.EventIntervalList, error) {

	scanner := Scanner{}
	fileURLs, err := scanner.FindMatchingFiles(jobRun.URL)
	if err != nil {
		return nil, err
	}
	logger.WithField("interval_files", fileURLs).Info("found interval files")

	// We will often match multiple files here, one for upgrade phase, one for conformance
	// testing phase. For now, we return them all, and each interval has a filename it
	// originated from.
	intervals := &apitype.EventIntervalList{}
	for i, fu := range fileURLs {
		// Replace our gcs web UI with storage.googleapis.com for better downloading:
		tempURL := strings.Replace(fu.String(), gcsPrefix+"/gcs", storagePrefix, -1)
		newURL, err := url.Parse(tempURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse URL from %s: %v", fu, err)
		}
		fileURLs[i] = newURL
		newIntervals, err := ReadIntervalsFile(fileURLs[i])
		if err != nil {
			return nil, err
		}
		intervals.Items = append(intervals.Items, newIntervals.Items...)
	}

	return intervals, nil
}

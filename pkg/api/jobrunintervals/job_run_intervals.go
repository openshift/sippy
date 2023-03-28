package jobrunintervals

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"cloud.google.com/go/storage"
	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/prowloader/gcs"
	log "github.com/sirupsen/logrus"
)

// JobRunIntervals fetches intervals for a given job run.
func JobRunIntervals(gcsClient *storage.Client, jobRun *models.ProwJobRun, logger *log.Entry) (*apitype.EventIntervalList, error) {

	parts := strings.Split(jobRun.URL, gcs.OpenshiftGCSBucket)
	path := parts[1][1:]
	log.WithField("path", path).Debug("calculated gcs path")
	bkt := gcsClient.Bucket(gcs.OpenshiftGCSBucket)
	gcsJobRun := gcs.NewGCSJobRun(bkt, path)

	intervalFiles := gcsJobRun.FindAllMatches([]*regexp.Regexp{gcs.GetIntervalFile()})

	// We will often match multiple files here, one for upgrade phase, one for conformance
	// testing phase. For now, we return them all, and each interval has a filename it
	// originated from.
	intervals := &apitype.EventIntervalList{}
	if len(intervalFiles) == 0 {
		logger.Info("no interval files found")
		return intervals, nil
	}
	logger.WithField("interval_files", intervalFiles[0]).Info("found interval files")
	for _, fp := range intervalFiles[0] {

		// Get the base filename we'll add to each incoming interval:
		tokens := strings.Split(fp, "/")
		baseFile := tokens[len(tokens)-1]

		content, err := gcsJobRun.GetContent(context.TODO(), fp)
		if err != nil {
			logger.WithError(err).Errorf("error getting content for file: %s", fp)
			return nil, err
		}
		var newIntervals apitype.EventIntervalList
		if err := json.Unmarshal(content, &newIntervals); err != nil {
			log.WithError(err).Error("error unmarshaling intervals file")
			return nil, err
		}
		for i := range newIntervals.Items {
			newIntervals.Items[i].Filename = baseFile
		}

		intervals.Items = append(intervals.Items, newIntervals.Items...)
	}

	return intervals, nil
}

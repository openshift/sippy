package jobrunintervals

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"cloud.google.com/go/storage"
	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/api"
	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/db"
)

// JobRunIntervals fetches intervals for a given job run by fetching from the prow job's GCS bucket path
// contructed using the prow job name and jobID using one of these methods:
// 1) using a GCS path that was calculated and passed in (we can retrieve intervals immediately)
// 2) looking up the url given the jobRunID and extracting the prow job name (we need to wait until the sippyDB is populated)
// If the GCS path could not be calculated, it will be empty.
func JobRunIntervals(gcsClient *storage.Client, dbc *db.DB, jobRunID int64, gcsBucket, gcsPath string, logger *log.Entry) (*apitype.EventIntervalList, error) {

	bkt := gcsClient.Bucket(gcsBucket)

	var gcsJobRun *gcs.GCSJobRun

	if len(gcsPath) > 0 {
		log.WithField("gcsPath", gcsPath).Debug("calculated gcs path from job attributes")
		gcsJobRun = gcs.NewGCSJobRun(bkt, gcsPath)
	} else {
		// Fall back to looking up the job run ID in the DB and extracting the URL that way.
		// This is here to support older prow jobs where only the jobID was passed.  Eventually,
		// we will not have to fallback because we will expect all jobs to pass in enough
		// information to construct a full GCS bucket path.
		jobRun, _, err := api.FetchJobRun(dbc, jobRunID, logger)
		if err != nil {
			logger.WithError(err).Error("error querying job run")
			return nil, err
		}
		parts := strings.Split(jobRun.URL, gcsBucket)
		path := parts[1][1:]
		log.WithField("path", path).Debug("calculated gcs path")
		gcsJobRun = gcs.NewGCSJobRun(bkt, path)
	}
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

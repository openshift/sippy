package jobrunintervals

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/api"
	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/db"
)

// JobRunIntervals fetches intervals for a given job run by fetching from the prow job's GCS bucket path
// constructed using the prow job name and jobID using one of these methods:
// 1) using a GCS path that was calculated and passed in (we can retrieve intervals immediately)
// 2) looking up the url given the jobRunID and extracting the prow job name (we need to wait until the sippyDB is populated)
// If the GCS path could not be calculated, it will be empty.
func JobRunIntervals(gcsClient *storage.Client, dbc *db.DB, jobRunID int64, gcsPath string, intervalFile string, logger *log.Entry) (*apitype.EventIntervalList, error) {

	jobRun, err := api.FetchJobRun(dbc, jobRunID, false, nil, logger)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to fetch job run %d", jobRunID)
	}

	bkt := gcsClient.Bucket(jobRun.GCSBucket)
	gcsJobRun := gcs.NewGCSJobRun(bkt, gcsPath)
	intervals := &apitype.EventIntervalList{}
	intervalFiles, err := gcsJobRun.FindAllMatches([]*regexp.Regexp{gcs.GetIntervalFile()})
	if err != nil {
		return intervals, err
	}

	// We will often match multiple files here, one for upgrade phase, one for conformance
	// testing phase. For now, we return them all, and each interval has a filename it
	// originated from.
	if len(intervalFiles) == 0 {
		logger.Info("no interval files found")
		return intervals, nil
	}
	logger.WithField("interval_files", intervalFiles[0]).Info("found interval files")

	// Now that we have the list of all matching interval files, if the user specified one, return only
	// intervals from that file. If they didn't, make a best guess on the default to load, our minimal
	// "spyglass" file we publish for the prow UI. The user can select other files if they wish.
	var fullGCSIntervalFile string
	intervalFilesAvailable := []string{}

	// Find the full path to the filename specified:
	for _, fp := range intervalFiles[0] {
		// Get the base filename we'll add to each incoming interval:
		tokens := strings.Split(fp, "/")
		baseFile := tokens[len(tokens)-1]
		intervalFilesAvailable = append(intervalFilesAvailable, baseFile)
		if intervalFile == "" && strings.Contains(fp, "e2e-timelines_spyglass_") {
			intervalFile = baseFile
		}
		if baseFile == intervalFile {
			fullGCSIntervalFile = fp
		}
	}

	logger.Infof("Loading interval file: %s", fullGCSIntervalFile)

	// Get the base filename we'll add to each incoming interval:
	tokens := strings.Split(fullGCSIntervalFile, "/")
	baseFile := tokens[len(tokens)-1]

	// Loading full intervals file into memory here briefly, they can be quite large...
	content, err := gcsJobRun.GetContent(context.TODO(), fullGCSIntervalFile)
	if err != nil {
		logger.WithError(err).Errorf("error getting content for file: %s", fullGCSIntervalFile)
		return nil, err
	}
	var newIntervals apitype.EventIntervalList
	var legacyIntervals apitype.LegacyEventIntervalList
	if err := json.Unmarshal(content, &newIntervals); err != nil {
		log.WithError(err).Error("error unmarshaling intervals file, attempting to parse legacy schema instead")
		if err := json.Unmarshal(content, &legacyIntervals); err != nil {
			log.WithError(err).Error("error unmarshaling legacy intervals file, giving up")
			return nil, err
		}
		log.Info("legacy interval files detected, successfully parsed")
	}

	// If legacy intervals is populated, we failed to parse this file with the new schema, so it must be an older
	// intervals file. Translate it to look like a new.
	// Memory use getting even worse in this path through the code but hopefully it won't be around for too long.
	if len(legacyIntervals.Items) > 0 {
		newIntervals = apitype.EventIntervalList{Items: make([]apitype.EventInterval, len(legacyIntervals.Items))}
		for i, li := range legacyIntervals.Items {
			interval := apitype.EventInterval{
				Level:             li.Level,
				Display:           li.Display,
				Source:            li.Source,
				StructuredLocator: li.StructuredLocator,
				StructuredMessage: li.StructuredMessage,
				From:              li.From,
				To:                li.To,
			}
			newIntervals.Items[i] = interval
		}
	}

	for i := range newIntervals.Items {
		newIntervals.Items[i].Filename = baseFile
	}

	newIntervals.IntervalFilesAvailable = intervalFilesAvailable

	return &newIntervals, nil
}

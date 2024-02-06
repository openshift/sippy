package variantregistry

import (
	"context"

	"cloud.google.com/go/bigquery"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
)

type VariantSyncer struct {
	BigQueryClient *bigquery.Client
}

type prowJobName struct {
	JobName string `bigquery:"prowjob_job_name"`
}

func (v *VariantSyncer) Sync() error {
	logrus.Info("Syncing all variants")

	// TODO: pull presubmits for sippy as well

	// For the primary list of all job names, we will query everything that's run in the last 3 months:
	query := v.BigQueryClient.Query(`SELECT distinct(prowjob_job_name) FROM ` +
		"`ci_analysis_us.jobs` " +
		`WHERE 
	  	created BETWEEN DATETIME_SUB(CURRENT_DATETIME(), INTERVAL 90 DAY) AND CURRENT_DATETIME()
		AND (prowjob_job_name LIKE 'periodic-%%' OR prowjob_job_name LIKE 'release-%%' OR prowjob_job_name like 'aggregator-%%')`)
	it, err := query.Read(context.TODO())
	if err != nil {
		return errors.Wrap(err, "error querying primary list of all jobs")
	}

	// Using a set since sometimes bigquery has multiple copies of the same prow job
	//prowJobs := map[string]bool{}
	count := 0
	for {
		jn := prowJobName{}
		err := it.Next(&jn)
		if err == iterator.Done {
			break
		}
		if err != nil {
			logrus.WithError(err).Error("error parsing prowjob name from bigquery")
			return err
		}
		logrus.WithField("job", jn.JobName).Debug("found job")

		count++
	}
	logrus.WithField("count", count).Info("loaded primary job list")
	return nil
}

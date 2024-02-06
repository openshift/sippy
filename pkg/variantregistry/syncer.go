package variantregistry

import (
	"context"

	"cloud.google.com/go/bigquery"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/testidentification"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
)

type VariantSyncer struct {
	BigQueryClient *bigquery.Client
	VariantManager testidentification.VariantManager
}

type prowJobName struct {
	JobName string `bigquery:"prowjob_job_name"`
}

func (v *VariantSyncer) Sync() error {
	logrus.Info("Syncing all variants")

	// TODO: pull presubmits for sippy as well

	// delete everything from the registry for now, we'll rebuild completely until sync logic is implemented.
	// TODO: Remove this, sync the changes we need to only, otherwise the apps will be working incorrectly while this process runs
	query := v.BigQueryClient.Query(`DELETE FROM openshift-ci-data-analysis.sippy.JobVariants WHERE true`)
	_, err := query.Read(context.TODO())
	if err != nil {
		logrus.WithError(err).Error("error clearing current registry job variants")
		return errors.Wrap(err, "error clearing current registry job variants")
	}
	logrus.Warn("deleted all current job variants in the registry")
	query = v.BigQueryClient.Query(`DELETE FROM openshift-ci-data-analysis.sippy.Jobs WHERE true`)
	_, err = query.Read(context.TODO())
	if err != nil {
		logrus.WithError(err).Error("error clearing current registry jobs")
		return errors.Wrap(err, "error clearing current registry jobs")
	}
	logrus.Warn("deleted all current jobs in the registry")

	// For the primary list of all job names, we will query everything that's run in the last 3 months:
	// TODO: for component readiness queries to work in the past, we may need far more than jobs that ran in 3 months
	// since start of 4.14 perhaps?
	query = v.BigQueryClient.Query(`SELECT distinct(prowjob_job_name) FROM ` +
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
		variants := v.GetVariantsForJob(jn.JobName)
		logrus.WithField("variants", variants).Debugf("calculated variants for %s", jn.JobName)

		count++
	}
	logrus.WithField("count", count).Info("loaded primary job list")

	// TODO: load the current registry job to variant mappings. join and then iterate building out go structure.
	// keep variants list sorted for comparisons.

	// build out a data structure mapping job name to variant key/value pairs:

	return nil
}

func (v *VariantSyncer) GetVariantsForJob(jobName string) []string {
	variants := v.VariantManager.IdentifyVariants(jobName, "0.0", models.ClusterData{})
	return variants
}

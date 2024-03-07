package variantregistry

import (
	"context"
	"fmt"

	"cloud.google.com/go/bigquery"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
)

// Syncer is responsible for reconciling a given list of all known jobs and their variant key/values map to BigQuery.
type Syncer struct{}

// SyncJobVariants can be used to reconcile expected job variants with whatever is currently in the bigquery
// tables.
// If a job is missing from the current tables it will be added, of or if missing from expected it will be removed from
// the current tables.
// In the event a jobs variants have changed, they will be fully updated to match the new expected variants.
//
// The expectedVariants passed in is Sippy/Component Readiness deployment specific, users can define how they map
// job to variants, and then use this generic reconcile logic to get it into bigquery.
func SyncJobVariants(bqClient *bigquery.Client, expectedVariants map[string]map[string]string) error {

	currentVariants, err := loadCurrentJobVariants(bqClient)
	if err != nil {
		log.WithError(err).Error("error loading current job variants")
		return errors.Wrap(err, "error loading current job variants")
	}
	log.Infof("loaded %d current jobs with variants", len(currentVariants))

	inserts, updates, deletes, deleteJobs := compareVariants(expectedVariants, currentVariants)

	log.Infof("inserting %d new job variants", len(inserts))
	err = bulkInsertVariants(bqClient, inserts)
	if err != nil {
		log.WithError(err).Error("error syncing job variants to bigquery")
	}

	log.Infof("updating %d job variants", len(updates))
	for i, jv := range updates {
		uLog := log.WithField("progress", fmt.Sprintf("%d/%d", i+1, len(updates)))
		err = updateVariant(uLog, bqClient, jv)
		if err != nil {
			log.WithError(err).Error("error syncing job variants to bigquery")
		}
	}

	// This loop is called for variants being removed from a job that is still in the system.
	log.Infof("deleting %d job variants", len(deletes))
	for i, jv := range deletes {
		uLog := log.WithField("progress", fmt.Sprintf("%d/%d", i+1, len(updates)))
		err = deleteVariant(uLog, bqClient, jv)
		if err != nil {
			log.WithError(err).Error("error syncing job variants to bigquery")
		}
	}

	// Delete jobs entirely, much faster than one variant at a time when jobs have been removed.
	// This should be relatively rare and would require the job to not have run for weeks/months.
	log.Infof("deleting %d jobs", len(deleteJobs))
	for i, job := range deleteJobs {
		uLog := log.WithField("progress", fmt.Sprintf("%d/%d", i+1, len(updates)))
		err = deleteJob(uLog, bqClient, job)
		if err != nil {
			log.WithError(err).Error("error syncing job variants to bigquery")
		}
	}

	return nil
}

// compareVariants compares the list of variants vs expected and returns the variants to be inserted, deleted, and updated.
// Broken out for unit testing purposes.
func compareVariants(expectedVariants, currentVariants map[string]map[string]string) (insertVariants, updateVariants, deleteVariants []jobVariant, deleteJobs []string) {
	insertVariants = []jobVariant{}
	updateVariants = []jobVariant{}
	deleteVariants = []jobVariant{}
	deleteJobs = []string{}

	for expectedJob, expectedVariants := range expectedVariants {
		if _, ok := currentVariants[expectedJob]; !ok {
			// Handle net new jobs:
			for k, v := range expectedVariants {
				insertVariants = append(insertVariants, jobVariant{
					JobName:      expectedJob,
					VariantName:  k,
					VariantValue: v,
				})
			}
			continue
		}

		// Sync variants for an existing job if any have changed:
		for k, v := range expectedVariants {
			currVarVal, ok := currentVariants[expectedJob][k]
			if !ok {
				// New variant added:
				insertVariants = append(insertVariants, jobVariant{
					JobName:      expectedJob,
					VariantName:  k,
					VariantValue: v,
				})
			} else {
				if currVarVal != v {
					updateVariants = append(updateVariants, jobVariant{
						JobName:      expectedJob,
						VariantName:  k,
						VariantValue: v,
					})
				}
			}
		}

		// Look for any variants for this job that should be removed:
		for k, v := range currentVariants[expectedJob] {
			if _, ok := expectedVariants[k]; !ok {
				deleteVariants = append(deleteVariants, jobVariant{
					JobName:      expectedJob,
					VariantName:  k,
					VariantValue: v,
				})
			}
		}
	}

	// Look for any jobs that should be removed:
	for currJobName := range currentVariants {
		if _, ok := expectedVariants[currJobName]; !ok {
			deleteJobs = append(deleteJobs, currJobName)
		}
	}

	return insertVariants, updateVariants, deleteVariants, deleteJobs
}

func loadCurrentJobVariants(bqClient *bigquery.Client) (map[string]map[string]string, error) {
	query := bqClient.Query(`SELECT * FROM openshift-ci-data-analysis.sippy.JobVariants ORDER BY JobName, VariantName`)
	it, err := query.Read(context.TODO())
	if err != nil {
		return nil, errors.Wrap(err, "error querying current job variants")
	}

	currentVariants := map[string]map[string]string{}

	for {
		jv := jobVariant{}
		err := it.Next(&jv)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.WithError(err).Error("error parsing job variant from bigquery")
			return nil, err
		}
		if _, ok := currentVariants[jv.JobName]; !ok {
			currentVariants[jv.JobName] = map[string]string{}
		}
		currentVariants[jv.JobName][jv.VariantName] = jv.VariantValue
	}

	return currentVariants, nil
}

type jobVariant struct {
	JobName      string `bigquery:"JobName"`
	VariantName  string `bigquery:"VariantName"`
	VariantValue string `bigquery:"VariantValue"`
}

// bulkInsertVariants inserts all new job variants in batches.
func bulkInsertVariants(bqClient *bigquery.Client, inserts []jobVariant) error {
	var batchSize = 500

	table := bqClient.Dataset("sippy").Table("JobVariants")
	inserter := table.Inserter()
	for i := 0; i < len(inserts); i += batchSize {
		end := i + batchSize
		if end > len(inserts) {
			end = len(inserts)
		}

		if err := inserter.Put(context.TODO(), inserts[i:end]); err != nil {
			return err
		}
		log.Infof("added %d new job variant rows", end-i)
	}

	return nil
}

// updateVariant updates a job variant in the registry.
func updateVariant(logger log.FieldLogger, bqClient *bigquery.Client, jv jobVariant) error {
	queryStr := fmt.Sprintf("UPDATE `openshift-ci-data-analysis.sippy.JobVariants` SET VariantValue = '%s' WHERE JobName = '%s' and VariantName = '%s'",
		jv.VariantValue, jv.JobName, jv.VariantName)
	insertQuery := bqClient.Query(queryStr)
	_, err := insertQuery.Read(context.TODO())
	if err != nil {
		return errors.Wrapf(err, "error updating variants: %s", queryStr)
	}
	logger.Infof("successful query: %s", queryStr)
	return nil
}

// deleteVariant deletes a job variant in the registry.
func deleteVariant(logger log.FieldLogger, bqClient *bigquery.Client, jv jobVariant) error {
	queryStr := fmt.Sprintf("DELETE FROM `openshift-ci-data-analysis.sippy.JobVariants` WHERE JobName = '%s' and VariantName = '%s' and VariantValue = '%s'",
		jv.JobName, jv.VariantName, jv.VariantValue)
	insertQuery := bqClient.Query(queryStr)
	_, err := insertQuery.Read(context.TODO())
	if err != nil {
		return errors.Wrapf(err, "error deleting variant: %s", queryStr)
	}
	logger.Infof("successful query: %s", queryStr)
	return nil
}

// deleteJob deletes all variants for a given job in the registry.
func deleteJob(logger log.FieldLogger, bqClient *bigquery.Client, job string) error {
	queryStr := fmt.Sprintf("DELETE FROM `openshift-ci-data-analysis.sippy.JobVariants` WHERE JobName = '%s'",
		job)
	insertQuery := bqClient.Query(queryStr)
	_, err := insertQuery.Read(context.TODO())
	if err != nil {
		return errors.Wrapf(err, "error deleting job: %s", queryStr)
	}
	logger.Infof("successful query: %s", queryStr)
	return nil
}

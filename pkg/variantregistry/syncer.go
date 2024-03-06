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

	currentVariants, err := LoadCurrentJobVariants(bqClient)
	if err != nil {
		log.WithError(err).Error("error loading current job variants")
		return errors.Wrap(err, "error loading current job variants")
	}
	log.Infof("loaded %d current jobs with variants", len(currentVariants))

	syncErrs := []error{}
	inserts, updates, deletes := compareVariants(expectedVariants, currentVariants)

	err = bulkInsertVariants(bqClient, inserts)
	if err != nil {
		syncErrs = append(syncErrs, err)
	}

	for _, jv := range updates {
		err = updateVariant(bqClient, jv)
		if err != nil {
			syncErrs = append(syncErrs, err)
		}
	}
	for _, jv := range deletes {
		err = deleteVariant(bqClient, jv)
		if err != nil {
			syncErrs = append(syncErrs, err)
		}
	}

	for _, se := range syncErrs {
		log.WithError(se).Error("error syncing job variants to bigquery")
	}

	return nil
}

// compareVariants compares the list of variants vs expected and returns the variants to be inserted, deleted, and updated.
// Broken out for unit testing purposes.
func compareVariants(expectedVariants, currentVariants map[string]map[string]string) (insertVariants, updateVariants, deleteVariants []jobVariant) {
	insertVariants = []jobVariant{}
	updateVariants = []jobVariant{}
	deleteVariants = []jobVariant{}

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
	for currJobName, currJobVariants := range currentVariants {
		if _, ok := expectedVariants[currJobName]; !ok {
			for k, v := range currJobVariants {
				deleteVariants = append(deleteVariants, jobVariant{
					JobName:      currJobName,
					VariantName:  k,
					VariantValue: v,
				})
			}
		}
	}

	return insertVariants, updateVariants, deleteVariants
}

func LoadCurrentJobVariants(bqClient *bigquery.Client) (map[string]map[string]string, error) {
	// There is technically a Jobs table, however it's just a name right now, so little point in joining on it.
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
func updateVariant(bqClient *bigquery.Client, jv jobVariant) error {
	queryStr := fmt.Sprintf("UPDATE `openshift-ci-data-analysis.sippy.JobVariants` SET VariantValue = '%s' WHERE JobName = '%s' and VariantName = '%s'",
		jv.VariantValue, jv.JobName, jv.VariantName)
	log.Info(queryStr)
	insertQuery := bqClient.Query(queryStr)
	_, err := insertQuery.Read(context.TODO())
	if err != nil {
		return errors.Wrapf(err, "error updating varia%s", queryStr)
	}
	return nil
}

// deleteVariant deletes a job variant in the registry.
func deleteVariant(bqClient *bigquery.Client, jv jobVariant) error {
	queryStr := fmt.Sprintf("DELETE FROM `openshift-ci-data-analysis.sippy.JobVariants` WHERE JobName = '%s' and VariantName = '%s' and VariantValue = '%s'",
		jv.JobName, jv.VariantName, jv.VariantValue)
	log.Info(queryStr)
	insertQuery := bqClient.Query(queryStr)
	_, err := insertQuery.Read(context.TODO())
	if err != nil {
		return errors.Wrapf(err, "error deleting variant: %s", queryStr)
	}
	return nil
}

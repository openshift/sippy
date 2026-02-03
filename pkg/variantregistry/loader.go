package variantregistry

import (
	"context"
	"fmt"
	"strings"

	"cloud.google.com/go/bigquery"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
)

const invalidCharacters = ",:"

// JobVariantsLoader can be used to reconcile expected job variants with whatever is currently in the bigquery
// tables.
// If a job is missing from the current tables it will be added, of or if missing from expected it will be removed from
// the current tables.
// In the event a jobs variants have changed, they will be fully updated to match the new expected variants.
//
// The expectedVariants passed in is Sippy/Component Readiness deployment specific, users can define how they map
// job to variants, and then use this generic reconcile logic to get it into bigquery.
type JobVariantsLoader struct {
	bqClient         *bigquery.Client
	bqOpContext      bqlabel.OperationalContext
	bigQueryProject  string
	bigQueryDataSet  string
	bigQueryTable    string
	expectedVariants map[string]map[string]string
	errors           []error
}

func NewJobVariantsLoader(
	bigQueryClient *bigquery.Client,
	opCtx bqlabel.OperationalContext,
	bigQueryProject, bigQueryDataSet, bigQueryTable string,
	expectedVariants map[string]map[string]string,
) *JobVariantsLoader {

	return &JobVariantsLoader{
		bqClient:         bigQueryClient,
		bqOpContext:      opCtx,
		bigQueryProject:  bigQueryProject,
		bigQueryDataSet:  bigQueryDataSet,
		bigQueryTable:    bigQueryTable,
		expectedVariants: expectedVariants,
		errors:           []error{},
	}
}

func (s *JobVariantsLoader) Name() string {
	return "job-variants"
}

func (s *JobVariantsLoader) Errors() []error {
	return s.errors
}

func (s *JobVariantsLoader) Load() {
	currentVariants, err := s.loadCurrentJobVariants()
	if err != nil {
		log.WithError(err).Error("error loading current job variants")
		s.errors = append(s.errors, errors.Wrap(err, "error loading current job variants"))
		return
	}
	log.Infof("loaded %d current jobs with variants", len(currentVariants))

	inserts, updates, deletes, deleteJobs := compareVariants(s.expectedVariants, currentVariants)

	if err := verifyVariants(inserts, updates); err != nil {
		s.errors = append(s.errors, err)
		return
	}

	log.Infof("inserting %d new job variants", len(inserts))
	err = s.bulkInsertVariants(inserts)
	if err != nil {
		log.WithError(err).Error("error syncing job variants to bigquery")
		s.errors = append(s.errors, err)
	}

	log.Infof("updating %d job variants", len(updates))
	for i, jv := range updates {
		uLog := log.WithField("progress", fmt.Sprintf("%d/%d", i+1, len(updates)))
		err = s.updateVariant(uLog, jv)
		if err != nil {
			log.WithError(err).Error("error syncing job variants to bigquery")
			s.errors = append(s.errors, err)
		}
	}

	// This loop is called for variants being removed from a job that is still in the system.
	log.Infof("deleting %d job variants", len(deletes))
	for i, jv := range deletes {
		uLog := log.WithField("progress", fmt.Sprintf("%d/%d", i+1, len(deletes)))
		err = s.deleteVariant(uLog, jv)
		if err != nil {
			log.WithError(err).Error("error syncing job variants to bigquery")
			s.errors = append(s.errors, err)
		}
	}

	// Delete jobs entirely, much faster than one variant at a time when jobs have been removed.
	// This should be relatively rare and would require the job to not have run for weeks/months.
	log.Infof("deleting %d jobs", len(deleteJobs))
	err = s.deleteJobsInBatches(deleteJobs, 500)
	if err != nil {
		log.WithError(err).Error("error deleting jobs from registry")
		s.errors = append(s.errors, err)
	}
}

func verifyVariants(variants ...[]jobVariant) error {
	for _, variantGroup := range variants {
		for _, variant := range variantGroup {
			if strings.ContainsAny(variant.VariantName, invalidCharacters) {
				return fmt.Errorf("invalid character in VariantName: %q", variant.VariantName)
			}
			if strings.ContainsAny(variant.VariantValue, invalidCharacters) {
				return fmt.Errorf("invalid character in VariantValue: %q", variant.VariantValue)
			}
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
			} else if currVarVal != v {
				updateVariants = append(updateVariants, jobVariant{
					JobName:      expectedJob,
					VariantName:  k,
					VariantValue: v,
				})
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

// applyQueryLabels applies query labels manually since this loader does not use the client that would do it for us.
func (s *JobVariantsLoader) applyQueryLabels(queryLabel bqlabel.QueryValue, q *bigquery.Query) {
	bqlabel.Context{
		OperationalContext: s.bqOpContext,
		RequestContext:     bqlabel.RequestContext{Query: queryLabel},
	}.ApplyLabels(q)
}

func (s *JobVariantsLoader) loadCurrentJobVariants() (map[string]map[string]string, error) {
	sql := `SELECT * FROM ` +
		fmt.Sprintf("%s.%s.%s", s.bigQueryProject, s.bigQueryDataSet, s.bigQueryTable) +
		` ORDER BY job_name, variant_name`
	query := s.bqClient.Query(sql)
	s.applyQueryLabels(bqlabel.VariantRegistryLoadCurrentVariants, query)
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
	JobName      string `bigquery:"job_name"`
	VariantName  string `bigquery:"variant_name"`
	VariantValue string `bigquery:"variant_value"`
}

// bulkInsertVariants inserts all new job variants in batches.
func (s *JobVariantsLoader) bulkInsertVariants(inserts []jobVariant) error {
	var batchSize = 500

	table := s.bqClient.Dataset(s.bigQueryDataSet).Table(s.bigQueryTable)
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
func (s *JobVariantsLoader) updateVariant(logger log.FieldLogger, jv jobVariant) error {
	queryStr := fmt.Sprintf("UPDATE `%s.%s.%s` SET variant_value = '%s' WHERE job_name = '%s' and variant_name = '%s'",
		s.bigQueryProject, s.bigQueryDataSet, s.bigQueryTable, jv.VariantValue, jv.JobName, jv.VariantName)
	insertQuery := s.bqClient.Query(queryStr)
	s.applyQueryLabels(bqlabel.VariantRegistryUpdateVariant, insertQuery)
	_, err := insertQuery.Read(context.TODO())
	if err != nil {
		return errors.Wrapf(err, "error updating variants: %s", queryStr)
	}
	logger.Infof("successful query: %s", queryStr)
	return nil
}

// deleteVariant deletes a job variant in the registry.
func (s *JobVariantsLoader) deleteVariant(logger log.FieldLogger, jv jobVariant) error {
	queryStr := fmt.Sprintf("DELETE FROM `%s.%s.%s` WHERE job_name = '%s' and variant_name = '%s' and variant_value = '%s'",
		s.bigQueryProject, s.bigQueryDataSet, s.bigQueryTable, jv.JobName, jv.VariantName, jv.VariantValue)
	insertQuery := s.bqClient.Query(queryStr)
	s.applyQueryLabels(bqlabel.VariantRegistryDeleteVariant, insertQuery)
	_, err := insertQuery.Read(context.TODO())
	if err != nil {
		return errors.Wrapf(err, "error deleting variant: %s", queryStr)
	}
	logger.Infof("successful query: %s", queryStr)
	return nil
}

// deleteJobsInBatches deletes jobs that should no longer be in the registry in batches, as one at a time can be
// very slow.
func (s *JobVariantsLoader) deleteJobsInBatches(deleteJobs []string, batchSize int) error {
	numBatches := (len(deleteJobs) + batchSize - 1) / batchSize
	for batchNum := 0; batchNum < numBatches; batchNum++ {
		start := batchNum * batchSize
		end := (batchNum + 1) * batchSize
		if end > len(deleteJobs) {
			end = len(deleteJobs)
		}

		batch := deleteJobs[start:end]

		err := s.deleteJobsBatch(batch)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *JobVariantsLoader) deleteJobsBatch(batch []string) error {
	log.Infof("deleting batch of %d jobs", len(batch))

	queryStr := fmt.Sprintf("DELETE FROM `%s.%s.%s` WHERE job_name IN ('%s')",
		s.bigQueryProject, s.bigQueryDataSet, s.bigQueryTable, strings.Join(batch, "','"))

	insertQuery := s.bqClient.Query(queryStr)
	s.applyQueryLabels(bqlabel.VariantRegistryDeleteJobBatch, insertQuery)
	_, err := insertQuery.Read(context.TODO())
	if err != nil {
		return errors.Wrapf(err, "error deleting batch of jobs: %s", queryStr)
	}

	log.Infof("successful batch job delete query: %s", queryStr)
	return nil
}

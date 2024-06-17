package variantsyncer

import (
	"context"
	"reflect"

	log "github.com/sirupsen/logrus"

	bqcached "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/testidentification"
)

type VariantSyncer struct {
	dbc    *db.DB
	mgr    testidentification.VariantManager
	errors []error
}

func New(dbc *db.DB, bqc *bqcached.Client) (*VariantSyncer, error) {
	mgr, err := testidentification.NewOpenshiftVariantManager(context.TODO(), bqc)
	if err != nil {
		return nil, err
	}

	return &VariantSyncer{
		dbc: dbc,
		mgr: mgr,
	}, nil
}

func (vl *VariantSyncer) Name() string {
	return "sync-variants"
}

func (vl *VariantSyncer) Errors() []error {
	return vl.errors
}

func (vl *VariantSyncer) Load() {
	allJobs := loadAllProwJobs(vl.dbc)
	for _, j := range allJobs {
		newVariants := vl.mgr.IdentifyVariants(j.Name)
		if !reflect.DeepEqual(newVariants, []string(j.Variants)) {
			j.Variants = newVariants
			if res := vl.dbc.DB.WithContext(context.TODO()).Save(j); res.Error != nil {
				vl.errors = append(vl.errors, res.Error)
			}
		}
	}
}

func loadAllProwJobs(dbc *db.DB) map[string]*models.ProwJob {
	results := map[string]*models.ProwJob{}
	var allJobs []*models.ProwJob
	dbc.DB.Model(&models.ProwJob{}).Find(&allJobs)
	for _, j := range allJobs {
		if _, ok := results[j.Name]; !ok {
			results[j.Name] = j
		}
	}
	log.Infof("jobs fetched with %d entries from database", len(results))
	return results
}

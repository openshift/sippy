package testownershiploader

import (
	"context"
	"fmt"

	"github.com/openshift-eng/ci-test-mapping/pkg/bigquery"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
)

// TestOwnershipLoader loads test ownership information from BigQuery. This data is generated and
// pushed to BigQuery from https://github.com/openshift-eng/ci-test-mapping
type TestOwnershipLoader struct {
	dbc              *db.DB
	mappingTableMgr  *bigquery.MappingTableManager
	errors           []error
	jiraComponentIDs map[string]uint
	suiteIDs         map[string]uint
}

func New(ctx context.Context, dbc *db.DB, googleServiceAccountCredentialFile, googleOAuthClientCredentialFile string) (*TestOwnershipLoader, error) {
	client, err := bigquery.NewClient(ctx, googleServiceAccountCredentialFile, googleOAuthClientCredentialFile)
	if err != nil {
		return nil, err
	}
	mappingTableMgr := bigquery.NewMappingTableManager(ctx, client)

	return &TestOwnershipLoader{
		dbc:              dbc,
		mappingTableMgr:  mappingTableMgr,
		jiraComponentIDs: make(map[string]uint),
		suiteIDs:         make(map[string]uint),
	}, nil
}

func (tol *TestOwnershipLoader) Name() string {
	return "test ownership"
}

func (tol *TestOwnershipLoader) Load() {
	mappings, err := tol.mappingTableMgr.ListMappings()
	if err != nil {
		tol.errors = append(tol.errors, err)
		return
	}

	// Link up the ci-test-mapping records to Sippy's test_ids
	unknown := 0
	known := 0
	var ids []uint
	for _, m := range mappings {
		// Find the test in Sippy DB:
		var test models.Test
		res := tol.dbc.DB.Table("tests").First(&test, "name = ?", m.Name)
		if res.Error == gorm.ErrRecordNotFound {
			log.WithFields(log.Fields{
				"testname": m.Name,
			}).Debugf("sippy doesn't know about this test")
			unknown++
			continue
		}
		if res.Error != nil {
			tol.errors = append(tol.errors, res.Error)
			return
		}
		if test.ID == 0 {
			log.Warningf("test %q has id 0", m.Name)
			continue
		}

		var suiteID *uint
		if id, ok := tol.suiteIDs[m.Suite]; ok {
			suiteID = &id
		} else {
			var suite models.Suite
			res = tol.dbc.DB.Model(&models.Suite{}).First(&suite, "name = ?", m.Suite)
			if res.Error != nil && res.Error != gorm.ErrRecordNotFound {
				tol.errors = append(tol.errors, errors.Wrap(res.Error, "couldn't find suite "+m.Suite))
				continue
			}
			if res.Error == nil {
				suiteIDVal := suite.ID
				suiteID = &suiteIDVal
				tol.suiteIDs[m.Suite] = suiteIDVal
			}
		}

		var jiraComponentID *uint
		if id, ok := tol.jiraComponentIDs[m.JIRAComponent]; ok {
			jiraComponentID = &id
		} else {
			var jiraComponent models.JiraComponent
			res = tol.dbc.DB.Model(models.JiraComponent{}).First(&jiraComponent, "name = ?", m.JIRAComponent)
			if res.Error != nil {
				msg := fmt.Sprintf("error with jira component %q", m.JIRAComponent)
				tol.errors = append(tol.errors, errors.WithMessage(res.Error, msg))
				log.WithError(err).Warning(msg)
				continue
			}
			id := jiraComponent.ID
			jiraComponentID = &id
			tol.jiraComponentIDs[m.JIRAComponent] = id
		}

		tom := &models.TestOwnership{
			APIVersion:            m.APIVersion,
			Name:                  m.Name,
			Suite:                 m.Suite,
			UniqueID:              m.ID,
			Product:               m.Product,
			Component:             m.Component,
			JiraComponent:         m.JIRAComponent,
			Capabilities:          m.Capabilities,
			Priority:              m.Priority,
			StaffApprovedObsolete: m.StaffApprovedObsolete,
			TestID:                test.ID,
			SuiteID:               suiteID,
			JiraComponentID:       jiraComponentID,
		}
		known++
		res = tol.dbc.DB.Model(&models.TestOwnership{}).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "name"}, {Name: "suite"}},
			UpdateAll: true,
		}).Save(tom)
		if res.Error != nil {
			tol.errors = append(tol.errors, res.Error)
			log.WithError(err).Warningf("error saving test ownership record for %q", m.Name)
			return
		}
		ids = append(ids, tom.ID)
	}

	log.Infof("deleting old records...")
	oldRecords := tol.dbc.DB.Where("id NOT IN ?", ids).Unscoped().Delete(&models.TestOwnership{})
	if oldRecords.Error != nil {
		log.WithError(oldRecords.Error).Warningf("couldn't delete old records")
		tol.errors = append(tol.errors, oldRecords.Error)
	}

	log.WithFields(log.Fields{
		"known":    known,
		"unknown":  unknown,
		"obsolete": oldRecords.RowsAffected,
	}).Infof("component loading complete")
}

func (tol *TestOwnershipLoader) Errors() []error {
	return tol.errors
}

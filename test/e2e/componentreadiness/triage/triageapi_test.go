package triage

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/openshift/sippy/pkg/api/componentreadiness"
	"github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/test/e2e/util"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var view = crview.View{
	Name: "4.19-main",
	SampleRelease: reqopts.RelativeRelease{
		Release: reqopts.Release{
			Name: "4.19",
		},
	},
}

func cleanupAllTriages(dbc *db.DB) {
	// Delete all triage and test regressions in the e2e postgres db.
	dbc.DB.Exec("DELETE FROM triage_regressions WHERE 1=1")
	res := dbc.DB.Where("1 = 1").Delete(&models.Triage{})
	if res.Error != nil {
		log.Errorf("error deleting triage records: %v", res.Error)
	}
}

func Test_TriageAPI(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)
	tracker := componentreadiness.NewPostgresRegressionStore(dbc)

	jiraBug := createBug(t, dbc.DB)
	defer dbc.DB.Delete(jiraBug)

	testRegression1 := createTestRegression(t, tracker, view, "faketestid")
	defer dbc.DB.Delete(testRegression1)
	testRegression2 := createTestRegression(t, tracker, view, "faketestid2")
	defer dbc.DB.Delete(testRegression2)

	t.Run("create requires a valid triage type", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		triage1 := models.Triage{
			URL: jiraBug.URL,
			Regressions: []models.TestRegression{
				{
					ID: testRegression1.ID, // test just setting the ID to link up
				},
			},
		}

		var triageResponse models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage1, &triageResponse)
		require.Error(t, err)

		triage1.Type = "fake"
		err = util.SippyPost("/api/component_readiness/triages", &triage1, &triageResponse)
		require.Error(t, err)
	})

	t.Run("create generates audit_log record", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		triage1 := models.Triage{
			URL: jiraBug.URL,
			Regressions: []models.TestRegression{
				{
					ID: testRegression1.ID,
				},
			},
			Type: "test",
		}

		var triageResponse models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage1, &triageResponse)
		require.NoError(t, err)

		var auditLog models.AuditLog
		res := dbc.DB.
			Where("table_name = ?", "triage").
			Where("row_id = ?", triageResponse.ID).
			First(&auditLog)
		require.NoError(t, res.Error)

		assert.Equal(t, models.Create, models.OperationType(auditLog.Operation))
		assert.Equal(t, "developer", auditLog.User)
		assert.NotEmpty(t, auditLog.NewData, "NewData should contain the created triage record")

		var auditedTriage models.Triage
		err = json.Unmarshal(auditLog.NewData, &auditedTriage)
		require.NoError(t, err, "NewData should be valid JSON")

		assertTriageDataMatches(t, triageResponse, auditedTriage, "NewData")
	})

	t.Run("get", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)

		// ensure hateoas links are present
		assert.Equal(t, fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse.ID),
			triageResponse.Links["self"])
	})
	t.Run("list", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)

		var allTriages []models.Triage
		err := util.SippyGet("/api/component_readiness/triages", &allTriages)
		require.NoError(t, err)
		var foundTriage *models.Triage
		for i, triage := range allTriages {
			if triage.ID == triageResponse.ID {
				foundTriage = &allTriages[i]
				break
			}
		}
		require.NotNil(t, foundTriage, "expected triage was not found in list")
		assert.Equal(t, testRegression1.TestName, foundTriage.Regressions[0].TestName,
			"list triage records should include regression details")

		// ensure hateoas links are present
		for _, triage := range allTriages {
			assert.Equal(t, fmt.Sprintf("/api/component_readiness/triages/%d", triage.ID),
				triage.Links["self"])
		}
	})
	t.Run("update to add regression", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)

		// Update with a new regression:
		var triageResponse2 models.Triage
		triageResponse.Regressions = append(triageResponse.Regressions, models.TestRegression{ID: testRegression2.ID})
		err := util.SippyPut(fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse.ID), &triageResponse, &triageResponse2)
		require.NoError(t, err)
		assert.Equal(t, 2, len(triageResponse2.Regressions))
		assert.Equal(t, triageResponse.CreatedAt, triageResponse2.CreatedAt)
		assert.NotEqual(t, triageResponse.UpdatedAt, triageResponse2.UpdatedAt)

		// ensure hateoas links are present
		assert.Equal(t, fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse2.ID),
			triageResponse2.Links["self"])
	})
	t.Run("update to remove a regression", func(t *testing.T) {
		defer cleanupAllTriages(dbc)

		triage := models.Triage{
			URL:  jiraBug.URL,
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				{ID: testRegression1.ID},
				{ID: testRegression2.ID},
			},
		}

		var triageResponse models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage, &triageResponse)
		require.NoError(t, err)
		assert.Equal(t, 2, len(triageResponse.Regressions))

		// Update to remove one regression - keep only testRegression1
		triageResponse.Regressions = []models.TestRegression{{ID: testRegression1.ID}}
		var triageResponse2 models.Triage
		err = util.SippyPut(fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse.ID), &triageResponse, &triageResponse2)
		require.NoError(t, err)
		assert.Equal(t, 1, len(triageResponse2.Regressions))
		assert.Equal(t, testRegression1.ID, triageResponse2.Regressions[0].ID, "should keep testRegression1")
		assert.WithinDuration(t, triageResponse.CreatedAt, triageResponse2.CreatedAt, time.Second)
		assert.NotEqual(t, triageResponse.UpdatedAt, triageResponse2.UpdatedAt)
	})
	t.Run("update to remove all regressions", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)

		var triageResponse2 models.Triage
		triageResponse.Regressions = []models.TestRegression{}
		err := util.SippyPut(fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse.ID), &triageResponse, &triageResponse2)
		require.NoError(t, err)
		assert.Equal(t, 0, len(triageResponse2.Regressions))
	})
	t.Run("update fails if resource has no ID", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)

		var triageResponse2 models.Triage
		triageResponse.ID = 0
		err := util.SippyPut(fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse.ID), &triageResponse, &triageResponse2)
		require.Error(t, err)
	})
	t.Run("update fails if URL has no ID", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)

		var triageResponse2 models.Triage
		// No ID specified in URL should not work for an update
		err := util.SippyPut("/api/component_readiness/triages", &triageResponse, &triageResponse2)
		require.Error(t, err)
	})
	t.Run("update fails if URL ID and resource ID do not match", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)

		var triageResponse2 models.Triage
		err := util.SippyPut("/api/component_readiness/triages/128736182736128736", &triageResponse, &triageResponse2)
		require.Error(t, err)
	})
	t.Run("update generates audit_log record", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)
		originalTriage := deepCopyTriage(t, triageResponse)

		// Update with a new regression, and a changed description:
		triageResponse.Regressions = append(triageResponse.Regressions, models.TestRegression{ID: testRegression2.ID})
		triageResponse.Description = "updated description"
		var triageResponse2 models.Triage
		err := util.SippyPut(fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse.ID), &triageResponse, &triageResponse2)
		require.NoError(t, err)

		var auditLog models.AuditLog
		res := dbc.DB.
			Where("table_name = ?", "triage").
			Where("operation = ?", models.Update).
			Where("row_id = ?", triageResponse.ID).
			First(&auditLog)
		require.NoError(t, res.Error)

		assert.Equal(t, "developer", auditLog.User)
		assert.NotEmpty(t, auditLog.NewData, "NewData should contain the updated triage record")
		assert.NotEmpty(t, auditLog.OldData, "OldData should contain the original triage record")

		var newTriageData models.Triage
		err = json.Unmarshal(auditLog.NewData, &newTriageData)
		require.NoError(t, err, "NewData should be valid JSON")
		assertTriageDataMatches(t, triageResponse2, newTriageData, "NewData")

		var oldTriageData models.Triage
		err = json.Unmarshal(auditLog.OldData, &oldTriageData)
		require.NoError(t, err, "OldData should be valid JSON")
		assertTriageDataMatches(t, originalTriage, oldTriageData, "OldData")
	})
	t.Run("delete generates audit_log record", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)
		originalTriage := deepCopyTriage(t, triageResponse)

		// Delete the triage record
		err := util.SippyDelete(fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse.ID))
		require.NoError(t, err)

		var auditLog models.AuditLog
		res := dbc.DB.
			Where("table_name = ?", "triage").
			Where("operation = ?", models.Delete).
			Where("row_id = ?", triageResponse.ID).
			First(&auditLog)
		require.NoError(t, res.Error)

		assert.Equal(t, "developer", auditLog.User)
		assert.NotEmpty(t, auditLog.OldData, "OldData should contain the deleted triage record")
		assert.Empty(t, auditLog.NewData, "NewData should be empty for delete operations")

		var oldTriageData models.Triage
		err = json.Unmarshal(auditLog.OldData, &oldTriageData)
		require.NoError(t, err, "OldData should be valid JSON")
		assertTriageDataMatches(t, originalTriage, oldTriageData, "OldData")
	})
}

func createAndValidateTriageRecord(t *testing.T, bugURL string, testRegression1 *models.TestRegression) models.Triage {
	triage1 := models.Triage{
		URL:  bugURL,
		Type: models.TriageTypeProduct,
		Regressions: []models.TestRegression{
			{
				ID: testRegression1.ID, // test just setting the ID to link up
			},
		},
	}

	var triageResponse models.Triage
	err := util.SippyPost("/api/component_readiness/triages", &triage1, &triageResponse)
	require.NoError(t, err)
	assert.True(t, triageResponse.ID > 0)
	assert.Equal(t, 1, len(triageResponse.Regressions))

	// Use the API get to ensure we get a clean object
	var lookupTriage models.Triage
	err = util.SippyGet(fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse.ID), &lookupTriage)
	require.NoError(t, err)
	assert.Equal(t, models.TriageTypeProduct, lookupTriage.Type)
	return lookupTriage
}

func createBug(t *testing.T, dbc *gorm.DB) *models.Bug {
	jiraBug := models.Bug{
		Key:        "MYBUGS-100",
		Status:     "New",
		Summary:    "foo bar",
		Components: pq.StringArray{"component1", "component2"},
		Labels:     pq.StringArray{"label1", "label2"},
		URL:        "https://issues.redhat.com/browse/MYBUGS-100",
	}
	res := dbc.Create(&jiraBug)
	require.NoError(t, res.Error)
	return &jiraBug
}

// Test_TriageRawDB ensures our gorm postgresql mappings are working as we'd expect.
func Test_TriageRawDB(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)
	dbWithContext := dbc.DB.WithContext(context.WithValue(context.TODO(), models.CurrentUserKey, "developer"))
	tracker := componentreadiness.NewPostgresRegressionStore(dbc)

	testRegression := createTestRegression(t, tracker, view, "faketestid")
	defer dbc.DB.Delete(testRegression)

	t.Run("test Triage model in postgres", func(t *testing.T) {
		defer cleanupAllTriages(dbc)

		triage1 := models.Triage{
			URL: "http://myjira",
			Regressions: []models.TestRegression{
				*testRegression,
			},
		}
		res := dbWithContext.Create(&triage1)
		require.NoError(t, res.Error)
		testRegression.Triages = append(testRegression.Triages, triage1)
		res = dbWithContext.Save(&testRegression)
		require.NoError(t, res.Error)

		// Lookup the Triage again to ensure we persisted what we expect:
		res = dbWithContext.First(&triage1, triage1.ID)
		require.NoError(t, res.Error)
		assert.Equal(t, 1, len(triage1.Regressions))

		// Ensure loading a regression can load the triage records for it:
		var lookupRegression models.TestRegression
		res = dbWithContext.First(&lookupRegression, testRegression.ID).Preload("Triages")
		require.NoError(t, res.Error)
		assert.Equal(t, 1, len(testRegression.Triages))

		openRegressions := make([]*models.TestRegression, 0)

		res = dbWithContext.
			Model(&models.TestRegression{}).
			Preload("Triages").
			Where("test_regressions.release = ?", view.SampleRelease.Name).
			Where("test_regressions.id = ?", testRegression.ID).
			Where("test_regressions.closed IS NULL").
			Find(&openRegressions)
		require.NoError(t, res.Error)
		assert.Equal(t, 1, len(openRegressions))
		assert.Equal(t, 1, len(openRegressions[0].Triages))

		// Make a second Triage for the same regression:
		triage2 := models.Triage{
			URL: "http://myjira2",
			Regressions: []models.TestRegression{
				*testRegression,
			},
		}
		res = dbWithContext.Create(&triage2)
		require.NoError(t, res.Error)
		testRegression.Triages = append(testRegression.Triages, triage2)
		res = dbWithContext.Save(&testRegression)
		require.NoError(t, res.Error)

		// Query for triages for a specific regression:
		res = dbWithContext.First(&testRegression, testRegression.ID).Preload("Triages")
		require.NoError(t, res.Error)
		assert.Equal(t, 2, len(testRegression.Triages))

		// Delete the association:
		triage1.Regressions = []models.TestRegression{}
		res = dbWithContext.Save(&triage1)
		require.NoError(t, res.Error)
		res = dbWithContext.First(&triage1, triage1.ID)
		require.Nil(t, res.Error)
		assert.Equal(t, 0, len(triage1.Regressions))
		// Make sure we didn't wipe out the regression itself:
		res = dbWithContext.First(&lookupRegression, testRegression.ID)
		require.NoError(t, res.Error)
	})

	t.Run("test Triage model Bug relationship", func(t *testing.T) {
		defer cleanupAllTriages(dbc)

		jiraBug := createBug(t, dbWithContext)
		defer dbWithContext.Delete(jiraBug)

		triage1 := models.Triage{
			URL: "http://myjira",
			Bug: jiraBug,
		}
		res := dbWithContext.Create(&triage1)
		require.NoError(t, res.Error)

		// Lookup the Triage again to ensure we persisted what we expect:
		res = dbWithContext.First(&triage1, triage1.ID)
		require.NoError(t, res.Error)
		assert.Equal(t, "MYBUGS-100", triage1.Bug.Key)

	})
}

func createTestRegression(t *testing.T, tracker componentreadiness.RegressionStore, view crview.View, testID string) *models.TestRegression {
	newRegression := componentreport.ReportTestSummary{
		Identification: crtest.Identification{
			RowIdentification: crtest.RowIdentification{
				Component:  "comp",
				Capability: "cap",
				TestName:   "fake test",
				TestSuite:  "fakesuite",
				TestID:     testID,
			},
			ColumnIdentification: crtest.ColumnIdentification{
				Variants: map[string]string{
					"a": "b",
					"c": "d",
				},
			},
		},
	}
	testRegression, err := tracker.OpenRegression(view, newRegression)
	require.NoError(t, err)
	return testRegression
}

// deepCopyTriage creates a deep copy of a Triage struct using JSON marshal/unmarshal
func deepCopyTriage(t *testing.T, original models.Triage) models.Triage {
	data, err := json.Marshal(original)
	require.NoError(t, err, "Failed to marshal triage for deep copy")

	var triageCopy models.Triage
	err = json.Unmarshal(data, &triageCopy)
	require.NoError(t, err, "Failed to unmarshal triage for deep copy")

	return triageCopy
}

func assertTriageDataMatches(t *testing.T, expectedTriage, actualTriage models.Triage, field string) {
	assert.Equal(t, expectedTriage.ID, actualTriage.ID, "%s ID should match the expected triage ID", field)
	assert.Equal(t, expectedTriage.URL, actualTriage.URL, "%s URL should match the expected triage URL", field)
	assert.Len(t, actualTriage.Regressions, len(expectedTriage.Regressions), "%s regressions count should match", field)

	if len(actualTriage.Regressions) > 0 && len(expectedTriage.Regressions) > 0 {
		assert.Equal(t, expectedTriage.Regressions[0].ID, actualTriage.Regressions[0].ID, "%s regression ID should match", field)
	}
}

package triage

import (
	"fmt"
	"testing"

	"github.com/lib/pq"
	"github.com/openshift/sippy/pkg/api/componentreadiness"
	"github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/test/e2e/util"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var view = componentreport.View{
	Name: "4.19-main",
	SampleRelease: componentreport.RequestRelativeReleaseOptions{
		RequestReleaseOptions: componentreport.RequestReleaseOptions{
			Release: "4.19",
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

	jiraBug := createBug(t, dbc)
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

}

func Test_RegressionAPI(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)
	tracker := componentreadiness.NewPostgresRegressionStore(dbc)

	testRegression1 := createTestRegression(t, tracker, view, "faketestid1")
	defer dbc.DB.Delete(testRegression1)

	testRegression2 := createTestRegression(t, tracker, view, "faketestid2")
	defer dbc.DB.Delete(testRegression2)

	jiraBug := createBug(t, dbc)
	defer dbc.DB.Delete(jiraBug)

	t.Run("list regressions", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		_ = createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)

		// Test listing all regressions
		var allRegressions []models.TestRegression
		err := util.SippyGet("/api/component_readiness/regressions", &allRegressions)
		require.NoError(t, err)

		// Should find at least our test regression
		var foundRegression *models.TestRegression
		for i, regression := range allRegressions {
			if regression.ID == testRegression1.ID {
				foundRegression = &allRegressions[i]
				break
			}
		}
		require.NotNil(t, foundRegression, "expected regression was not found in list")
		assert.Equal(t, testRegression1.TestName, foundRegression.TestName)
		assert.Equal(t, testRegression1.View, foundRegression.View)
		assert.Equal(t, testRegression1.Release, foundRegression.Release)

		// Verify HATEOAS links are present
		assert.NotNil(t, foundRegression.Links, "regression should have HATEOAS links")
		assert.Contains(t, foundRegression.Links, "test_details", "regression should have test_details link")
		testDetailsLink := foundRegression.Links["test_details"]
		assert.Contains(t, testDetailsLink, "/api/component_readiness/test_details", "test_details link should point to correct endpoint")
		assert.Contains(t, testDetailsLink, "testId="+testRegression1.TestID, "test_details link should contain testId")
	})
	t.Run("list regressions with view filter", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		_ = createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)

		// Test listing regressions filtered by view
		var filteredRegressions []models.TestRegression
		err := util.SippyGet("/api/component_readiness/regressions?view="+view.Name, &filteredRegressions)
		require.NoError(t, err)

		// Should find our test regression
		var foundRegression *models.TestRegression
		for i, regression := range filteredRegressions {
			if regression.ID == testRegression1.ID {
				foundRegression = &filteredRegressions[i]
				break
			}
		}
		require.NotNil(t, foundRegression, "expected regression was not found in filtered list")
		assert.Equal(t, view.Name, foundRegression.View)
	})
	t.Run("list regressions with release filter", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		_ = createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)

		// Test listing regressions filtered by release
		var filteredRegressions []models.TestRegression
		err := util.SippyGet("/api/component_readiness/regressions?release="+view.SampleRelease.Release, &filteredRegressions)
		require.NoError(t, err)

		// Should find our test regression
		var foundRegression *models.TestRegression
		for i, regression := range filteredRegressions {
			if regression.ID == testRegression1.ID {
				foundRegression = &filteredRegressions[i]
				break
			}
		}
		require.NotNil(t, foundRegression, "expected regression was not found in release filtered list")
		assert.Equal(t, view.SampleRelease.Release, foundRegression.Release)
	})
	t.Run("error when both view and release are specified", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		_ = createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)

		// Test that specifying both view and release parameters returns an error
		var regressions []models.TestRegression
		err := util.SippyGet("/api/component_readiness/regressions?view="+view.Name+"&release="+view.SampleRelease.Release, &regressions)
		require.Error(t, err, "Expected error when both view and release are specified")
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

func createBug(t *testing.T, dbc *db.DB) *models.Bug {
	jiraBug := models.Bug{
		Key:        "MYBUGS-100",
		Status:     "New",
		Summary:    "foo bar",
		Components: pq.StringArray{"component1", "component2"},
		Labels:     pq.StringArray{"label1", "label2"},
		URL:        "https://issues.redhat.com/browse/MYBUGS-100",
	}
	res := dbc.DB.Create(&jiraBug)
	require.NoError(t, res.Error)
	return &jiraBug
}

// Test_TriageRawDB ensures our gorm postgresql mappings are working as we'd expect.
func Test_TriageRawDB(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)
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
		res := dbc.DB.Create(&triage1)
		require.NoError(t, res.Error)
		testRegression.Triages = append(testRegression.Triages, triage1)
		res = dbc.DB.Save(&testRegression)
		require.NoError(t, res.Error)

		// Lookup the Triage again to ensure we persisted what we expect:
		res = dbc.DB.First(&triage1, triage1.ID)
		require.NoError(t, res.Error)
		assert.Equal(t, 1, len(triage1.Regressions))

		// Ensure loading a regression can load the triage records for it:
		var lookupRegression models.TestRegression
		res = dbc.DB.First(&lookupRegression, testRegression.ID).Preload("Triages")
		require.NoError(t, res.Error)
		assert.Equal(t, 1, len(testRegression.Triages))

		openRegressions := make([]*models.TestRegression, 0)

		res = dbc.DB.
			Model(&models.TestRegression{}).
			Preload("Triages").
			Where("test_regressions.release = ?", view.SampleRelease.Release).
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
		res = dbc.DB.Create(&triage2)
		require.NoError(t, res.Error)
		testRegression.Triages = append(testRegression.Triages, triage2)
		res = dbc.DB.Save(&testRegression)
		require.NoError(t, res.Error)

		// Query for triages for a specific regression:
		res = dbc.DB.First(&testRegression, testRegression.ID).Preload("Triages")
		require.NoError(t, res.Error)
		assert.Equal(t, 2, len(testRegression.Triages))

		// Delete the association:
		triage1.Regressions = []models.TestRegression{}
		res = dbc.DB.Save(&triage1)
		require.NoError(t, res.Error)
		res = dbc.DB.First(&triage1, triage1.ID)
		require.Nil(t, res.Error)
		assert.Equal(t, 0, len(triage1.Regressions))
		// Make sure we didn't wipe out the regression itself:
		res = dbc.DB.First(&lookupRegression, testRegression.ID)
		require.NoError(t, res.Error)
	})

	t.Run("test Triage model Bug relationship", func(t *testing.T) {
		defer cleanupAllTriages(dbc)

		jiraBug := createBug(t, dbc)
		defer dbc.DB.Delete(jiraBug)

		triage1 := models.Triage{
			URL: "http://myjira",
			Bug: jiraBug,
		}
		res := dbc.DB.Create(&triage1)
		require.NoError(t, res.Error)

		// Lookup the Triage again to ensure we persisted what we expect:
		res = dbc.DB.First(&triage1, triage1.ID)
		require.NoError(t, res.Error)
		assert.Equal(t, "MYBUGS-100", triage1.Bug.Key)

	})
}

func createTestRegression(t *testing.T, tracker componentreadiness.RegressionStore, view componentreport.View, testID string) *models.TestRegression {
	newRegression := componentreport.ReportTestSummary{
		ReportTestIdentification: componentreport.ReportTestIdentification{
			RowIdentification: componentreport.RowIdentification{
				Component:  "comp",
				Capability: "cap",
				TestName:   "fake test",
				TestSuite:  "fakesuite",
				TestID:     testID,
			},
			ColumnIdentification: componentreport.ColumnIdentification{
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

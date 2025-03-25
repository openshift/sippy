package triage

import (
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

func cleanupAllRegressions(dbc *db.DB) {
	// Delete all triage and test regressions in the e2e postgres db.
	dbc.DB.Exec("DELETE FROM triage_regressions WHERE 1=1")
	res := dbc.DB.Where("1 = 1").Delete(&models.Triage{})
	if res.Error != nil {
		log.Errorf("error deleting triage records: %v", res.Error)
	}
	res = dbc.DB.Where("1 = 1").Delete(&models.TestRegression{})
	if res.Error != nil {
		log.Errorf("error deleting test regressions: %v", res.Error)
	}
}

func Test_TriageAPI(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)
	tracker := componentreadiness.NewPostgresRegressionStore(dbc)

	jiraBug := createBug(t, dbc)
	defer dbc.DB.Delete(jiraBug)

	testRegression := createTestRegression(t, tracker, view)
	defer dbc.DB.Delete(testRegression)

	t.Run("create and list", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)
		triage1 := models.Triage{
			URL: "http://myjira",
			Regressions: []models.TestRegression{
				{
					ID: testRegression.ID, // test just setting the ID to link up
				},
			},
		}

		var triageResponse models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage1, &triageResponse)
		require.NoError(t, err)
		assert.True(t, triageResponse.ID > 0)
		assert.Equal(t, 1, len(triageResponse.Regressions))
		log.Infof("triage response: %+v", triageResponse)

		// List
		var allTriages []models.Triage
		err = util.SippyGet("/api/component_readiness/triages", &allTriages)
		require.NoError(t, err)
		var foundTriage *models.Triage
		for i, triage := range allTriages {
			if triage.ID == triageResponse.ID {
				foundTriage = &allTriages[i]
				break
			}
		}
		require.NotNil(t, foundTriage, "expected triage was not found in list")
		assert.Equal(t, testRegression.TestName, foundTriage.Regressions[0].TestName,
			"list triage records should include regression details")

	})
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

	testRegression := createTestRegression(t, tracker, view)
	defer dbc.DB.Delete(testRegression)

	t.Run("test Triage model in postgres", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)

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

		// Lookup the Triage again to ensure we persisted what we expect:
		res = dbc.DB.First(&triage1, triage1.ID)
		require.NoError(t, res.Error)
		assert.Equal(t, 1, len(triage1.Regressions))

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
	})

	t.Run("test Triage model Bug relationship", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)

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

func createTestRegression(t *testing.T, tracker componentreadiness.RegressionStore, view componentreport.View) *models.TestRegression {
	newRegression := componentreport.ReportTestSummary{
		ReportTestIdentification: componentreport.ReportTestIdentification{
			RowIdentification: componentreport.RowIdentification{
				Component:  "comp",
				Capability: "cap",
				TestName:   "fake test",
				TestSuite:  "fakesuite",
				TestID:     "faketestid",
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

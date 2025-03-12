package db

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

func Test_Triage(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)
	tracker := componentreadiness.NewPostgresRegressionStore(dbc)
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
	view := componentreport.View{
		Name: "4.19-main",
		SampleRelease: componentreport.RequestRelativeReleaseOptions{
			RequestReleaseOptions: componentreport.RequestReleaseOptions{
				Release: "4.19",
			},
		},
	}

	t.Run("test Triage model in postgres", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)
		testRegression, err := tracker.OpenRegression(view, newRegression)
		require.NoError(t, err)

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

		triage1 := models.Triage{
			URL: "http://myjira",
			Bug: jiraBug,
		}
		res = dbc.DB.Create(&triage1)
		require.NoError(t, res.Error)

		// Lookup the Triage again to ensure we persisted what we expect:
		res = dbc.DB.First(&triage1, triage1.ID)
		require.NoError(t, res.Error)
		assert.Equal(t, "MYBUGS-100", triage1.Bug.Key)

	})
}

package db

import (
	"testing"

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
	res = dbc.DB.Where("1 = 1").Delete(&componentreport.TestRegression{})
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

	t.Run("triage a test regression", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)
		tr, err := tracker.OpenRegression(view, newRegression)
		require.NoError(t, err)

		triageRecord := models.Triage{
			URL: "http://myjira",
			Regressions: []componentreport.TestRegression{
				*tr,
			},
		}
		dbc.DB.Create(&triageRecord)
		res := dbc.DB.First(&triageRecord, triageRecord.ID)
		require.Nil(t, res.Error)
		assert.Equal(t, 1, len(triageRecord.Regressions))

		// Delete the association:
		triageRecord.Regressions = []componentreport.TestRegression{}
		res = dbc.DB.Save(&triageRecord)
		require.NoError(t, res.Error)
		res = dbc.DB.First(&triageRecord, triageRecord.ID)
		require.Nil(t, res.Error)
		assert.Equal(t, 0, len(triageRecord.Regressions))
	})
}

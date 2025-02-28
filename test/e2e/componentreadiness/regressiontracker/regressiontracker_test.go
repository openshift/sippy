package regressiontracker

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/openshift/sippy/pkg/api/componentreadiness"
	"github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/flags"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cleanupAllRegressions(dbc *db.DB) {
	// Delete all test regressions in the e2e postgres db.
	res := dbc.DB.Where("1 = 1").Delete(&componentreport.TestRegression{})
	if res.Error != nil {
		log.Fatalf("error deleting test regressions: %v", res.Error)
	}
}

func Test_RegressionTracker(t *testing.T) {
	require.NotEqual(t, "", os.Getenv("SIPPY_E2E_DSN"),
		"SIPPY_E2E_DSN environment variable not set")

	psqlFlags := flags.NewPostgresDatabaseFlags(os.Getenv("SIPPY_E2E_DSN"))
	dbc, err := psqlFlags.GetDBClient()
	require.NoError(t, err, "error connecting to db")

	// Simple check that someone doesn't accidentally run the e2es against the prod db:
	var totalRegressions int64
	dbc.DB.Model(&componentreport.TestRegression{}).Count(&totalRegressions)
	require.Less(t, int(totalRegressions), 100, "found too many test regressions in db, possible indicator someone is running e2e against prod, please clean out test_regressions if this is not the case")

	log.Info("got db connection", dbc)
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
	t.Run("open a new regression", func(t *testing.T) {
		tr, err := tracker.OpenRegression(context.Background(), view, newRegression)
		require.NoError(t, err)
		assert.Equal(t, "4.19", tr.Release)
		assert.Equal(t, "4.19-main", tr.View)
		assert.ElementsMatch(t, pq.StringArray([]string{"a:b", "c:d"}), tr.Variants)
		assert.True(t, tr.ID > 0)
		defer cleanupAllRegressions(dbc)
	})

	t.Run("close and reopen a regression", func(t *testing.T) {
		tr, err := tracker.OpenRegression(context.Background(), view, newRegression)
		require.NoError(t, err)

		// look it up just to be sure:
		lookup := &componentreport.TestRegression{
			ID: tr.ID,
		}
		dbc.DB.First(&lookup)
		assert.Equal(t, tr.ID, lookup.ID)
		assert.Equal(t, tr.TestName, lookup.TestName)

		// Now close it:
		assert.False(t, lookup.Closed.Valid)
		err = tracker.CloseRegression(context.Background(), lookup, time.Now())
		assert.NoError(t, err)
		// look it up again because we're being ridiculous:
		dbc.DB.First(&lookup)

		assert.True(t, lookup.Closed.Valid)

		err = tracker.ReOpenRegression(context.Background(), lookup)
		assert.NoError(t, err)
		dbc.DB.First(&lookup)
		assert.False(t, lookup.Closed.Valid)

		defer cleanupAllRegressions(dbc)

	})

	t.Run("reopen a regression", func(t *testing.T) {
		tr, err := tracker.OpenRegression(context.Background(), view, newRegression)
		require.NoError(t, err)

		// look it up just to be sure:
		lookup := &componentreport.TestRegression{
			ID: tr.ID,
		}
		dbc.DB.First(&lookup)
		assert.Equal(t, tr.ID, lookup.ID)
		assert.Equal(t, tr.TestName, lookup.TestName)

		// Now close it:
		assert.False(t, lookup.Closed.Valid)
		err = tracker.CloseRegression(context.Background(), lookup, time.Now())
		assert.NoError(t, err)
		// look it up again because we're being ridiculous:
		lookup = &componentreport.TestRegression{
			ID: tr.ID,
		}
		dbc.DB.First(&lookup)

		assert.True(t, lookup.Closed.Valid)

		defer cleanupAllRegressions(dbc)

	})

}

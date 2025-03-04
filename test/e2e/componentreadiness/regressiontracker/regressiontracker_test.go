package regressiontracker

import (
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/openshift/sippy/pkg/api/componentreadiness"
	"github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/db"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/logger"
)

func cleanupAllRegressions(dbc *db.DB) {
	// Delete all test regressions in the e2e postgres db.
	res := dbc.DB.Where("1 = 1").Delete(&componentreport.TestRegression{})
	if res.Error != nil {
		log.Errorf("error deleting test regressions: %v", res.Error)
	}
}

func Test_RegressionTracker(t *testing.T) {
	require.NotEqual(t, "", os.Getenv("SIPPY_E2E_DSN"),
		"SIPPY_E2E_DSN environment variable not set")

	dbc, err := db.New(os.Getenv("SIPPY_E2E_DSN"), logger.Info)
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
		defer cleanupAllRegressions(dbc)
		tr, err := tracker.OpenRegression(view, newRegression)
		require.NoError(t, err)
		assert.Equal(t, "4.19", tr.Release)
		assert.Equal(t, "4.19-main", tr.View)
		assert.ElementsMatch(t, pq.StringArray([]string{"a:b", "c:d"}), tr.Variants)
		assert.True(t, tr.ID > 0)
	})

	t.Run("close and reopen a regression", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)
		tr, err := tracker.OpenRegression(view, newRegression)
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
		err = tracker.CloseRegression(lookup, time.Now())
		assert.NoError(t, err)
		// look it up again because we're being ridiculous:
		dbc.DB.First(&lookup)

		assert.True(t, lookup.Closed.Valid)

		err = tracker.ReOpenRegression(lookup)
		assert.NoError(t, err)
		dbc.DB.First(&lookup)
		assert.False(t, lookup.Closed.Valid)
	})

	t.Run("list current regressions for release", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)
		var err error
		open419, err := rawCreateRegression(dbc, "4.19-main", "4.19",
			"test1ID", "test 1",
			uuid.New().String(),
			[]string{"a:b", "c:d"},
			time.Now().Add(-77*24*time.Hour), time.Time{})
		require.NoError(t, err)
		recentlyClosed419, err := rawCreateRegression(dbc, "4.19-main", "4.19",
			"test2ID", "test 2",
			uuid.New().String(),
			[]string{"a:b", "c:d"},
			time.Now().Add(-77*24*time.Hour), time.Now().Add(-2*24*time.Hour))
		require.NoError(t, err)
		_, err = rawCreateRegression(dbc, "4.19-main", "4.19",
			"test3ID", "test 3",
			uuid.New().String(),
			[]string{"a:b", "c:d"},
			time.Now().Add(-77*24*time.Hour), time.Now().Add(-70*24*time.Hour))
		require.NoError(t, err)
		_, err = rawCreateRegression(dbc, "4.18-main", "4.18",
			"test1ID", "test 1",
			uuid.New().String(),
			[]string{"a:b", "c:d"},
			time.Now().Add(-77*24*time.Hour), time.Time{})
		require.NoError(t, err)

		// List all regressions for 4.19, should exclude 4.18, include recently closed regressions,
		// and exclude older closed regressions.
		relRegressions, err := tracker.ListCurrentRegressionsForRelease("4.19")
		require.NoError(t, err)
		assert.Equal(t, 2, len(relRegressions))
		for _, rel := range relRegressions {
			assert.True(t, rel.ID == open419.ID || rel.ID == recentlyClosed419.ID,
				"unexpected regression was returned: %+v", *rel)
		}
	})

}

func rawCreateRegression(
	dbc *db.DB,
	view,
	release,
	testID,
	testName,
	regID string,
	variants []string,
	opened, closed time.Time) (*componentreport.TestRegression, error) {
	newRegression := &componentreport.TestRegression{
		View:         view,
		Release:      release,
		TestID:       testID,
		TestName:     testName,
		RegressionID: regID,
		Opened:       opened,
		Variants:     variants,
	}
	if closed.IsZero() {
		newRegression.Closed = sql.NullTime{
			Valid: false,
		}
	} else {
		newRegression.Closed = sql.NullTime{
			Valid: true,
			Time:  closed,
		}
	}
	res := dbc.DB.Create(newRegression)
	return newRegression, res.Error
}

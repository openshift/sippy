package regressiontracker

import (
	"database/sql"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/openshift/sippy/pkg/api/componentreadiness"
	"github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/test/e2e/util"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cleanupAllRegressions(dbc *db.DB) {
	// Delete all test regressions in the e2e postgres db.
	res := dbc.DB.Where("1 = 1").Delete(&models.TestRegression{})
	if res.Error != nil {
		log.Errorf("error deleting test regressions: %v", res.Error)
	}
}

func Test_RegressionTracker(t *testing.T) {
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
		SampleRelease: reqopts.RequestRelativeReleaseOptions{
			RequestReleaseOptions: reqopts.RequestReleaseOptions{
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
		lookup := &models.TestRegression{
			ID: tr.ID,
		}
		dbc.DB.First(&lookup)
		assert.Equal(t, tr.ID, lookup.ID)
		assert.Equal(t, tr.TestName, lookup.TestName)

		// Now close it:
		assert.False(t, lookup.Closed.Valid)
		lookup.Closed = sql.NullTime{Valid: true, Time: time.Now()}
		err = tracker.UpdateRegression(lookup)
		assert.NoError(t, err)
		// look it up again because we're being ridiculous:
		dbc.DB.First(&lookup)

		assert.True(t, lookup.Closed.Valid)

		lookup.Closed = sql.NullTime{Valid: false}
		err = tracker.UpdateRegression(lookup)
		assert.NoError(t, err)
		dbc.DB.First(&lookup)
		assert.False(t, lookup.Closed.Valid)
	})

	t.Run("list current regressions for release", func(t *testing.T) {
		defer cleanupAllRegressions(dbc)
		var err error
		open419, err := rawCreateRegression(dbc, "4.19-main", "4.19",
			"test1ID", "test 1",
			[]string{"a:b", "c:d"},
			time.Now().Add(-77*24*time.Hour), time.Time{})
		require.NoError(t, err)
		recentlyClosed419, err := rawCreateRegression(dbc, "4.19-main", "4.19",
			"test2ID", "test 2",
			[]string{"a:b", "c:d"},
			time.Now().Add(-77*24*time.Hour), time.Now().Add(-2*24*time.Hour))
		require.NoError(t, err)
		_, err = rawCreateRegression(dbc, "4.19-main", "4.19",
			"test3ID", "test 3",
			[]string{"a:b", "c:d"},
			time.Now().Add(-77*24*time.Hour), time.Now().Add(-70*24*time.Hour))
		require.NoError(t, err)
		_, err = rawCreateRegression(dbc, "4.18-main", "4.18",
			"test1ID", "test 1",
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
	view string,
	release string,
	testID string,
	testName string,
	variants []string,
	opened, closed time.Time) (*models.TestRegression, error) {
	newRegression := &models.TestRegression{
		View:     view,
		Release:  release,
		TestID:   testID,
		TestName: testName,
		Opened:   opened,
		Variants: variants,
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

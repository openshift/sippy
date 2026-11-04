package componentreadiness

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/api/componentreadiness"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/test/e2e/util"
)

// seededSampleRelease is the sample release of the seed views with regression tracking enabled.
const seededSampleRelease = "4.22"

// TestSeededRegressionsForceClose verifies seed-data opens regressions at a historical time and that
// a seeded regression can be force closed with a resolution date between opened and today. It must
// run before packages that clear test_regressions (e.g. regressiontracker).
func TestSeededRegressionsForceClose(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)

	var seeded []models.TestRegression
	require.NoError(t, dbc.DB.Where("release = ? AND closed IS NULL", seededSampleRelease).Order("id").Find(&seeded).Error)
	require.NotEmpty(t, seeded, "seed-data should create open regressions for %s", seededSampleRelease)

	opened := seeded[0].Opened.UTC()
	for _, reg := range seeded {
		assert.True(t, opened.Equal(reg.Opened), "regression %d opened %v, want the shared seeded time %v", reg.ID, reg.Opened, opened)
	}
	assert.Equal(t, opened.Truncate(24*time.Hour), opened, "seeded opened time should be the UTC midnight starting the sample window")

	todayStart := time.Now().UTC().Truncate(24 * time.Hour)
	resolved := opened.Add(24 * time.Hour)
	require.True(t, resolved.Before(todayStart), "resolution %v should be selectable before today %v", resolved, todayStart)

	reg := seeded[0]
	var triage models.Triage
	require.NoError(t, util.SippyPost("/api/component_readiness/triages", &models.Triage{
		URL:         "https://issues.example.com/browse/SEED-FORCE-CLOSE",
		Type:        models.TriageTypeProduct,
		Regressions: []models.TestRegression{{ID: reg.ID}},
	}, &triage))
	defer restoreSeededRegression(dbc, triage.ID, reg)

	triage.Resolved = sql.NullTime{Valid: true, Time: resolved}
	var updated models.Triage
	require.NoError(t, util.SippyPut(fmt.Sprintf("/api/component_readiness/triages/%d", triage.ID), &triage, &updated))

	var result componentreadiness.ForceCloseResult
	require.NoError(t, util.SippyPost(fmt.Sprintf("/api/component_readiness/triages/%d/force_close_regressions", triage.ID),
		&map[string]string{"reason": "seeded force close e2e"}, &result))
	assert.ElementsMatch(t, []uint{reg.ID}, result.ClosedRegressionIDs)

	var got models.TestRegression
	require.NoError(t, dbc.DB.First(&got, reg.ID).Error)
	require.True(t, got.Closed.Valid, "force close should persist a closed time")
	assert.WithinDuration(t, resolved, got.Closed.Time, time.Second)
	assert.True(t, got.ForceClosed)
	require.NotNil(t, got.ForceClosedByTriageID)
	assert.Equal(t, triage.ID, *got.ForceClosedByTriageID)
}

// restoreSeededRegression deletes the test triage and reopens the seeded regression so later tests
// see the seeded state.
func restoreSeededRegression(dbc *db.DB, triageID uint, reg models.TestRegression) {
	if err := dbc.DB.Exec("DELETE FROM triage_regressions WHERE triage_id = ?", triageID).Error; err != nil {
		log.WithError(err).WithField("triageID", triageID).Error("error deleting triage associations")
	}
	if err := dbc.DB.Unscoped().Delete(&models.Triage{}, triageID).Error; err != nil {
		log.WithError(err).WithField("triageID", triageID).Error("error deleting triage")
	}
	if err := dbc.DB.Model(&models.TestRegression{}).Where("id = ?", reg.ID).Updates(map[string]any{
		"closed":                    reg.Closed,
		"force_closed":              reg.ForceClosed,
		"force_closed_by":           reg.ForceClosedBy,
		"force_closed_reason":       reg.ForceClosedReason,
		"force_closed_by_triage_id": reg.ForceClosedByTriageID,
	}).Error; err != nil {
		log.WithError(err).WithField("regressionID", reg.ID).Error("error restoring seeded regression")
	}
}

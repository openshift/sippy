package componentreadiness

import (
	"testing"

	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/test/e2e/util"
	"github.com/stretchr/testify/require"
)

// TestSeedGASnapshots checks that available raw data builds a matching GA
// snapshot while older releases retain their synthetic aggregate fixtures.
func TestSeedGASnapshots(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)
	for _, release := range []string{"4.21", "4.20", "4.19"} {
		t.Run(release, func(t *testing.T) {
			var def models.ReleaseDefinition
			require.NoError(t, dbc.DB.Where("release = ?", release).First(&def).Error)
			require.NotNil(t, def.GADate)
			require.Equal(t, def.GADate, def.LoadedGADate)
			for _, windowDays := range []int{1, 30, 90} {
				var count int64
				require.NoError(t, dbc.DB.Model(&models.ProwGARawTestDatum{}).Where("release = ? AND window_days = ?", release, windowDays).Count(&count).Error)
				require.Positive(t, count, "release %s must have %d-day snapshot rows", release, windowDays)
			}
		})
	}
	var raw, snapshot int64
	require.NoError(t, dbc.DB.Raw(`SELECT COUNT(*) FROM prow_job_run_tests t
        JOIN prow_job_runs r ON r.id = t.prow_job_run_id AND r.prow_job_release = t.prow_job_run_release AND r.timestamp = t.prow_job_run_timestamp
        JOIN release_definitions d ON d.release = t.prow_job_run_release
        WHERE d.release = '4.21' AND t.prow_job_run_timestamp >= ((d.ga_date - 30)::timestamp AT TIME ZONE 'UTC')
          AND t.prow_job_run_timestamp < ((d.ga_date + 1)::timestamp AT TIME ZONE 'UTC')
          AND t.deleted_at IS NULL AND r.deleted_at IS NULL AND (r.labels IS NULL OR NOT r.labels @> ARRAY['InfraFailure'])`).Scan(&raw).Error)
	require.Positive(t, raw)
	require.NoError(t, dbc.DB.Raw("SELECT COALESCE(SUM(runs), 0) FROM prow_ga_raw_test_data WHERE release = '4.21' AND window_days = 30").Scan(&snapshot).Error)
	require.Equal(t, raw, snapshot)
}

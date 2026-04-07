package datasync

import (
	"os"
	"os/exec"
	"testing"

	"github.com/openshift/sippy/test/e2e/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataSync(t *testing.T) {
	if os.Getenv("GCS_SA_JSON_PATH") == "" {
		t.Skip("GCS_SA_JSON_PATH not set, skipping data sync test")
	}

	dbc := util.CreateE2EPostgresConnection(t)

	// Count prow_job_runs before sync to compare after
	var countBefore int64
	dbc.DB.Table("prow_job_runs").Count(&countBefore)
	t.Logf("prow_job_runs before sync: %d", countBefore)

	// SIPPY_E2E_REPO_ROOT is set by e2e.sh to the repo root where the sippy
	// binary and config files live.
	repoRoot := os.Getenv("SIPPY_E2E_REPO_ROOT")
	require.NotEmpty(t, repoRoot, "SIPPY_E2E_REPO_ROOT must be set")

	// Run sippy load with minimal scope: just prow loader, single release,
	// last 2 hours of data only
	cmd := exec.Command(repoRoot+"/sippy", "load", // #nosec G204
		"--loader", "prow",
		"--release", util.Release,
		"--prow-load-since", "2h",
		"--config", "config/e2e-openshift.yaml",
		"--google-service-account-credential-file", os.Getenv("GCS_SA_JSON_PATH"),
		"--database-dsn", os.Getenv("SIPPY_E2E_DSN"),
		"--skip-matview-refresh",
		"--log-level", "debug",
	)
	cmd.Dir = repoRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	require.NoError(t, err, "sippy load command should complete without error")

	// Verify some real prow job runs were loaded (with real job names from e2e-openshift.yaml)
	var countAfter int64
	dbc.DB.Table("prow_job_runs").Count(&countAfter)
	t.Logf("prow_job_runs after sync: %d (loaded %d new)", countAfter, countAfter-countBefore)
	assert.Greater(t, countAfter, countBefore, "sync should have loaded new prow job runs")
}

package datasync

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/openshift/sippy/test/e2e/util"
	"github.com/stretchr/testify/require"
)

func TestDataSync(t *testing.T) {
	if os.Getenv("GCS_SA_JSON_PATH") == "" {
		t.Skip("GCS_SA_JSON_PATH not set, skipping data sync test")
	}

	dbc := util.CreateE2EPostgresConnection(t)

	var countBefore int64
	require.NoError(t, dbc.DB.Table("prow_job_runs").Count(&countBefore).Error)
	t.Logf("prow_job_runs before sync: %d", countBefore)

	repoRoot := os.Getenv("SIPPY_E2E_REPO_ROOT")
	require.NotEmpty(t, repoRoot, "SIPPY_E2E_REPO_ROOT must be set")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, repoRoot+"/sippy", "load", // #nosec G204
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

	var countAfter int64
	require.NoError(t, dbc.DB.Table("prow_job_runs").Count(&countAfter).Error)
	t.Logf("prow_job_runs after sync: %d (loaded %d new)", countAfter, countAfter-countBefore)
}

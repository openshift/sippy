package datasync

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/openshift/sippy/test/e2e/util"
	"github.com/stretchr/testify/require"
)

func TestDataSync(t *testing.T) {
	sippyImage := os.Getenv("SIPPY_E2E_SIPPY_IMAGE")
	if sippyImage == "" {
		t.Skip("SIPPY_E2E_SIPPY_IMAGE not set, skipping data sync test")
	}

	dbc := util.CreateE2EPostgresConnection(t)

	var countBefore int64
	require.NoError(t, dbc.DB.Table("prow_job_runs").Count(&countBefore).Error)
	t.Logf("prow_job_runs before sync: %d", countBefore)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	kubectl := os.Getenv("KUBECTL_CMD")
	if kubectl == "" {
		kubectl = "oc"
	}

	// Create a Job that runs sippy load as a pod on the cluster, with GCS
	// credentials and coverage instrumentation.
	jobManifest := fmt.Sprintf(`apiVersion: batch/v1
kind: Job
metadata:
  name: sippy-datasync-job
  namespace: sippy-e2e
spec:
  template:
    spec:
      containers:
      - name: sippy
        image: %s
        resources:
          limits:
            memory: 8Gi
        command: ["/bin/sippy-cover"]
        args:
        - load
        - --loader
        - prow
        - --release
        - "%s"
        - --prow-load-since
        - 2h
        - --config
        - config/e2e-openshift.yaml
        - --google-service-account-credential-file
        - /tmp/secrets/gcs-cred
        - --database-dsn
        - postgresql://postgres:password@postgres.sippy-e2e.svc.cluster.local:5432/postgres
        - --skip-matview-refresh
        - --log-level
        - debug
        env:
        - name: GOCOVERDIR
          value: /tmp/coverage
        volumeMounts:
        - mountPath: /tmp/secrets
          name: gcs-cred
          readOnly: true
        - mountPath: /tmp/coverage
          name: coverage
      imagePullSecrets:
      - name: regcred
      volumes:
      - name: gcs-cred
        secret:
          secretName: gcs-cred
      - name: coverage
        persistentVolumeClaim:
          claimName: sippy-coverage
      restartPolicy: Never
  backoffLimit: 0`, sippyImage, util.Release)

	// Apply the job manifest
	applyCmd := exec.CommandContext(ctx, kubectl, "apply", "-f", "-") // #nosec G204
	applyCmd.Stdin = strings.NewReader(jobManifest)
	applyCmd.Stdout = os.Stdout
	applyCmd.Stderr = os.Stderr
	require.NoError(t, applyCmd.Run(), "failed to create datasync job")

	// Wait for the job to complete
	waitCmd := exec.CommandContext(ctx, kubectl, "-n", "sippy-e2e", "wait", // #nosec G204
		"--for=condition=complete", "job/sippy-datasync-job", "--timeout=600s")
	waitCmd.Stdout = os.Stdout
	waitCmd.Stderr = os.Stderr
	waitErr := waitCmd.Run()

	// Collect logs regardless of success/failure
	logCmd := exec.CommandContext(ctx, kubectl, "-n", "sippy-e2e", "logs", // #nosec G204
		"--selector=job-name=sippy-datasync-job")
	logCmd.Stdout = os.Stdout
	logCmd.Stderr = os.Stderr
	_ = logCmd.Run()

	require.NoError(t, waitErr, "sippy load job should complete successfully")

	var countAfter int64
	require.NoError(t, dbc.DB.Table("prow_job_runs").Count(&countAfter).Error)
	t.Logf("prow_job_runs after sync: %d (loaded %d new)", countAfter, countAfter-countBefore)
}

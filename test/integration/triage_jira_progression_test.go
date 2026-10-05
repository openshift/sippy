package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v4/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/dataloader/bugloader"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	intutil "github.com/openshift/sippy/test/integration/util"
)

func TestReconcileTriagesJiraProgression(t *testing.T) {
	t.Run("auto-resolves single-release triage when bug reaches ON_QA", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		ctx := context.Background()

		bugURL := "https://issues.example.com/RESOLVE-1"
		createBug(t, dbc, "RESOLVE-1", "ON_QA", "fix landed", bugURL)
		reg := intutil.CreateTestRegression(t, dbc, "test-resolve-1", "4.19")
		triage := intutil.CreateTriage(t, dbc, bugURL, intutil.WithRegressions(reg))

		runReconcileTriages(ctx, t, dbc)

		reloadTriage(t, dbc, &triage)
		assert.True(t, triage.Resolved.Valid, "triage should be resolved")
		assert.Equal(t, string(models.JiraProgression), string(triage.ResolutionReason))
	})

	t.Run("auto-resolves for all progressed statuses", func(t *testing.T) {
		for _, status := range []string{"ON_QA", "Verified", "Release Pending", "Closed"} {
			t.Run(status, func(t *testing.T) {
				dbc := intutil.NewTestDB(t, pgContainer)
				ctx := context.Background()

				bugURL := "https://issues.example.com/STATUS-" + status
				createBug(t, dbc, "STATUS-"+status, status, "bug summary", bugURL)
				reg := intutil.CreateTestRegression(t, dbc, "test-status-"+status, "4.19")
				triage := intutil.CreateTriage(t, dbc, bugURL, intutil.WithRegressions(reg))

				runReconcileTriages(ctx, t, dbc)

				reloadTriage(t, dbc, &triage)
				assert.True(t, triage.Resolved.Valid, "triage should be resolved for status %s", status)
				assert.Equal(t, string(models.JiraProgression), string(triage.ResolutionReason))
			})
		}
	})

	t.Run("does not auto-resolve multi-release triage", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		ctx := context.Background()

		bugURL := "https://issues.example.com/MULTI-1"
		createBug(t, dbc, "MULTI-1", "ON_QA", "multi-release bug", bugURL)
		reg1 := intutil.CreateTestRegression(t, dbc, "test-multi-1a", "4.18")
		reg2 := intutil.CreateTestRegression(t, dbc, "test-multi-1b", "4.19")
		triage := intutil.CreateTriage(t, dbc, bugURL, intutil.WithRegressions(reg1, reg2))

		runReconcileTriages(ctx, t, dbc)

		reloadTriage(t, dbc, &triage)
		assert.False(t, triage.Resolved.Valid, "multi-release triage should not be auto-resolved")
		assert.Empty(t, string(triage.ResolutionReason))
	})

	t.Run("clears jira-progression resolution when bug reopens", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		ctx := context.Background()

		bugURL := "https://issues.example.com/REOPEN-1"
		bug := createBug(t, dbc, "REOPEN-1", "In Progress", "bug reopened", bugURL)
		reg := intutil.CreateTestRegression(t, dbc, "test-reopen-1", "4.19")
		triage := intutil.CreateTriage(t, dbc, bugURL,
			intutil.WithResolved(time.Now()), intutil.WithRegressions(reg))
		setResolutionReason(t, dbc, triage.ID, "jira-progression")

		runReconcileTriages(ctx, t, dbc)

		reloadTriage(t, dbc, &triage)
		assert.False(t, triage.Resolved.Valid, "triage should be unresolved after bug reopened")
		assert.Empty(t, string(triage.ResolutionReason))

		_ = bug
	})

	t.Run("does not clear user resolution when bug reopens", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		ctx := context.Background()

		bugURL := "https://issues.example.com/USER-1"
		createBug(t, dbc, "USER-1", "In Progress", "user resolved", bugURL)
		reg := intutil.CreateTestRegression(t, dbc, "test-user-1", "4.19")
		resolvedAt := time.Now().Truncate(time.Second)
		triage := intutil.CreateTriage(t, dbc, bugURL,
			intutil.WithResolved(resolvedAt), intutil.WithRegressions(reg))
		setResolutionReason(t, dbc, triage.ID, "user")

		runReconcileTriages(ctx, t, dbc)

		reloadTriage(t, dbc, &triage)
		assert.True(t, triage.Resolved.Valid, "user-resolved triage should stay resolved")
		assert.Equal(t, string(models.User), string(triage.ResolutionReason))
	})

	t.Run("does not clear regressions-rolled-off resolution when bug reopens", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		ctx := context.Background()

		bugURL := "https://issues.example.com/ROLLED-1"
		createBug(t, dbc, "ROLLED-1", "In Progress", "rolled off", bugURL)
		reg := intutil.CreateTestRegression(t, dbc, "test-rolled-1", "4.19")
		triage := intutil.CreateTriage(t, dbc, bugURL,
			intutil.WithResolved(time.Now()), intutil.WithRegressions(reg))
		setResolutionReason(t, dbc, triage.ID, "regressions-rolled-off")

		runReconcileTriages(ctx, t, dbc)

		reloadTriage(t, dbc, &triage)
		assert.True(t, triage.Resolved.Valid, "regressions-rolled-off triage should stay resolved")
		assert.Equal(t, string(models.RegressionsRolledOff), string(triage.ResolutionReason))
	})

	t.Run("full lifecycle resolve then unresolve", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		ctx := context.Background()

		bugURL := "https://issues.example.com/LIFECYCLE-1"
		bug := createBug(t, dbc, "LIFECYCLE-1", "ON_QA", "lifecycle bug", bugURL)
		reg := intutil.CreateTestRegression(t, dbc, "test-lifecycle-1", "4.19")
		triage := intutil.CreateTriage(t, dbc, bugURL, intutil.WithRegressions(reg))

		// Step 1: bug progresses, triage gets auto-resolved
		runReconcileTriages(ctx, t, dbc)

		reloadTriage(t, dbc, &triage)
		require.True(t, triage.Resolved.Valid, "triage should be resolved after bug progressed")
		assert.Equal(t, string(models.JiraProgression), string(triage.ResolutionReason))

		// Step 2: bug reopens, triage resolution should be cleared
		require.NoError(t, dbc.DB.Model(&bug).Update("status", "In Progress").Error)

		runReconcileTriages(ctx, t, dbc)

		reloadTriage(t, dbc, &triage)
		assert.False(t, triage.Resolved.Valid, "triage should be unresolved after bug reopened")
		assert.Empty(t, string(triage.ResolutionReason))
	})

	t.Run("idempotent auto-resolve", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		ctx := context.Background()

		bugURL := "https://issues.example.com/IDEM-1"
		createBug(t, dbc, "IDEM-1", "Verified", "idempotent", bugURL)
		reg := intutil.CreateTestRegression(t, dbc, "test-idem-1", "4.19")
		triage := intutil.CreateTriage(t, dbc, bugURL, intutil.WithRegressions(reg))

		runReconcileTriages(ctx, t, dbc)

		reloadTriage(t, dbc, &triage)
		firstResolved := triage.Resolved.Time

		runReconcileTriages(ctx, t, dbc)

		reloadTriage(t, dbc, &triage)
		assert.True(t, triage.Resolved.Valid, "triage should still be resolved")
		assert.Equal(t, firstResolved.Unix(), triage.Resolved.Time.Unix(), "resolved time should not change on second run")
	})

	t.Run("idempotent unresolve", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		ctx := context.Background()

		bugURL := "https://issues.example.com/IDEM-2"
		createBug(t, dbc, "IDEM-2", "New", "reopened", bugURL)
		reg := intutil.CreateTestRegression(t, dbc, "test-idem-2", "4.19")
		triage := intutil.CreateTriage(t, dbc, bugURL,
			intutil.WithResolved(time.Now()), intutil.WithRegressions(reg))
		setResolutionReason(t, dbc, triage.ID, "jira-progression")

		runReconcileTriages(ctx, t, dbc)

		reloadTriage(t, dbc, &triage)
		require.False(t, triage.Resolved.Valid, "triage should be unresolved")

		// Second run should be a no-op
		runReconcileTriages(ctx, t, dbc)

		reloadTriage(t, dbc, &triage)
		assert.False(t, triage.Resolved.Valid, "triage should still be unresolved")
	})

	t.Run("updates triage description from bug summary", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		ctx := context.Background()

		bugURL := "https://issues.example.com/DESC-1"
		createBug(t, dbc, "DESC-1", "New", "updated summary", bugURL)
		reg := intutil.CreateTestRegression(t, dbc, "test-desc-1", "4.19")
		triage := intutil.CreateTriage(t, dbc, bugURL, intutil.WithRegressions(reg))

		runReconcileTriages(ctx, t, dbc)

		reloadTriage(t, dbc, &triage)
		assert.Equal(t, "updated summary", triage.Description)
	})

	t.Run("links triage to bug record", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		ctx := context.Background()

		bugURL := "https://issues.example.com/LINK-1"
		bug := createBug(t, dbc, "LINK-1", "New", "linkable", bugURL)
		reg := intutil.CreateTestRegression(t, dbc, "test-link-1", "4.19")
		triage := intutil.CreateTriage(t, dbc, bugURL, intutil.WithRegressions(reg))
		require.Nil(t, triage.BugID, "bug_id should be nil before reconciliation")

		runReconcileTriages(ctx, t, dbc)

		reloadTriage(t, dbc, &triage)
		require.NotNil(t, triage.BugID, "bug_id should be set after reconciliation")
		assert.Equal(t, bug.ID, *triage.BugID)
	})
}

func createBug(t *testing.T, dbc *db.DB, key, status, summary, url string) models.Bug {
	t.Helper()
	bug := models.Bug{
		Key:            key,
		Status:         status,
		Summary:        summary,
		URL:            url,
		LastChangeTime: time.Now(),
	}
	require.NoError(t, dbc.DB.Create(&bug).Error, "creating Bug %q", key)
	return bug
}

func setResolutionReason(t *testing.T, dbc *db.DB, triageID uint, reason string) {
	t.Helper()
	require.NoError(t, dbc.DB.Exec(
		"UPDATE triages SET resolution_reason = ? WHERE id = ?", reason, triageID).Error)
}

func reloadTriage(t *testing.T, dbc *db.DB, triage *models.Triage) {
	t.Helper()
	require.NoError(t, dbc.DB.First(triage, triage.ID).Error, "reloading triage %d", triage.ID)
}

func runReconcileTriages(ctx context.Context, t *testing.T, dbc *db.DB) {
	t.Helper()
	sqlDB, err := dbc.DB.DB()
	require.NoError(t, err)
	conn, err := stdlib.AcquireConn(sqlDB)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, stdlib.ReleaseConn(sqlDB, conn))
	}()
	require.NoError(t, bugloader.ReconcileTriages(ctx, conn))
}

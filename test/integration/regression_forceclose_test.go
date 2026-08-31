package integration

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/api/componentreadiness"
	"github.com/openshift/sippy/pkg/db/models"
	intutil "github.com/openshift/sippy/test/integration/util"
)

// These tests exercise the force close regression store methods against a real PostgreSQL clone.
// pgContainer and TestMain live in jobs_test.go (same package), so each test just calls
// intutil.NewTestDB(t, pgContainer) for an isolated database.

func TestForceCloseRegressions(t *testing.T) {
	t.Run("basic close records force close metadata on the regression", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		store := componentreadiness.NewPostgresRegressionStore(dbc, nil)

		resolved := time.Now().Truncate(time.Second)
		reg := intutil.CreateTestRegression(t, dbc, "basic-close", "4.19",
			intutil.WithOpened(resolved.Add(-10*24*time.Hour)))
		triage := intutil.CreateTriage(t, dbc, "https://issues.example.com/BASIC-1",
			intutil.WithResolved(resolved), intutil.WithRegressions(reg))

		result, err := store.ForceCloseRegressions(triage.ID, "developer", "unrelated failures")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.ElementsMatch(t, []uint{reg.ID}, result.ClosedRegressionIDs)
		assert.WithinDuration(t, resolved, result.Timestamp, time.Second, "close time should be the resolution time")

		var got models.TestRegression
		require.NoError(t, dbc.DB.First(&got, reg.ID).Error)
		assert.True(t, got.Closed.Valid, "regression should be closed")
		assert.WithinDuration(t, resolved, got.Closed.Time, time.Second, "regression should close at the resolution time")
		assert.True(t, got.ForceClosed)
		require.NotNil(t, got.ForceClosedBy)
		assert.Equal(t, "developer", *got.ForceClosedBy)
		require.NotNil(t, got.ForceClosedReason)
		assert.Equal(t, "unrelated failures", *got.ForceClosedReason)
		require.NotNil(t, got.ForceClosedByTriageID)
		assert.Equal(t, triage.ID, *got.ForceClosedByTriageID)
	})

	t.Run("time scoping closes only regressions opened before the resolution time", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		store := componentreadiness.NewPostgresRegressionStore(dbc, nil)

		resolved := time.Now().Add(-5 * 24 * time.Hour).Truncate(time.Second)
		before := intutil.CreateTestRegression(t, dbc, "scope-before", "4.19",
			intutil.WithOpened(resolved.Add(-24*time.Hour)))
		// Opened at the exact resolution instant: the exclusive boundary (opened < resolved) means it
		// must NOT be closed.
		atBoundary := intutil.CreateTestRegression(t, dbc, "scope-boundary", "4.19",
			intutil.WithOpened(resolved))
		after := intutil.CreateTestRegression(t, dbc, "scope-after", "4.19",
			intutil.WithOpened(resolved.Add(24*time.Hour)))
		triage := intutil.CreateTriage(t, dbc, "https://issues.example.com/SCOPE-1",
			intutil.WithResolved(resolved), intutil.WithRegressions(before, atBoundary, after))

		result, err := store.ForceCloseRegressions(triage.ID, "developer", "scoped close")
		require.NoError(t, err)
		assert.ElementsMatch(t, []uint{before.ID}, result.ClosedRegressionIDs,
			"only the regression opened before the resolution time should be closed")

		var gotBefore, gotBoundary, gotAfter models.TestRegression
		require.NoError(t, dbc.DB.First(&gotBefore, before.ID).Error)
		require.NoError(t, dbc.DB.First(&gotBoundary, atBoundary.ID).Error)
		require.NoError(t, dbc.DB.First(&gotAfter, after.ID).Error)

		assert.True(t, gotBefore.ForceClosed, "regression opened before resolution should be force closed")
		assert.WithinDuration(t, resolved, gotBefore.Closed.Time, time.Second)

		assert.False(t, gotBoundary.Closed.Valid, "regression opened exactly at resolution should remain open")
		assert.False(t, gotBoundary.ForceClosed, "boundary regression should not be force closed")

		assert.False(t, gotAfter.Closed.Valid, "regression opened after resolution should remain open")
		assert.False(t, gotAfter.ForceClosed)
	})

	t.Run("unresolved triage returns ErrTriageNotResolved and touches nothing", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		store := componentreadiness.NewPostgresRegressionStore(dbc, nil)

		reg := intutil.CreateTestRegression(t, dbc, "guard-open", "4.19",
			intutil.WithOpened(time.Now().Add(-10*24*time.Hour)))
		triage := intutil.CreateTriage(t, dbc, "https://issues.example.com/GUARD-1", intutil.WithRegressions(reg))

		_, err := store.ForceCloseRegressions(triage.ID, "developer", "should fail")
		require.Error(t, err)
		assert.ErrorIs(t, err, componentreadiness.ErrTriageNotResolved)

		var got models.TestRegression
		require.NoError(t, dbc.DB.First(&got, reg.ID).Error)
		assert.False(t, got.Closed.Valid, "regression should remain open")
		assert.False(t, got.ForceClosed, "regression should not be force closed")
	})

	t.Run("idempotent repeat call closes nothing new and leaves the close time unchanged", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		store := componentreadiness.NewPostgresRegressionStore(dbc, nil)

		resolved := time.Now().Truncate(time.Second)
		reg := intutil.CreateTestRegression(t, dbc, "idem", "4.19",
			intutil.WithOpened(resolved.Add(-10*24*time.Hour)))
		triage := intutil.CreateTriage(t, dbc, "https://issues.example.com/IDEM-1",
			intutil.WithResolved(resolved), intutil.WithRegressions(reg))

		first, err := store.ForceCloseRegressions(triage.ID, "developer", "first call")
		require.NoError(t, err)
		require.Len(t, first.ClosedRegressionIDs, 1)

		var afterFirst models.TestRegression
		require.NoError(t, dbc.DB.First(&afterFirst, reg.ID).Error)
		originalClose := afterFirst.Closed.Time

		second, err := store.ForceCloseRegressions(triage.ID, "developer", "second call")
		require.NoError(t, err)
		assert.Empty(t, second.ClosedRegressionIDs, "repeat call should close no additional regressions")

		var afterSecond models.TestRegression
		require.NoError(t, dbc.DB.First(&afterSecond, reg.ID).Error)
		assert.WithinDuration(t, originalClose, afterSecond.Closed.Time, time.Second,
			"closed time should not change on a repeat call")
	})
}

func TestForceClosePreview(t *testing.T) {
	t.Run("classifies would-close and would-not-close with failure gap data", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		store := componentreadiness.NewPostgresRegressionStore(dbc, nil)

		resolved := time.Now().Add(-5 * 24 * time.Hour).Truncate(time.Second)

		wouldClose := intutil.CreateTestRegression(t, dbc, "prev-close", "4.19",
			intutil.WithOpened(resolved.Add(-10*24*time.Hour)))
		// Opened exactly at the resolution instant belongs in would_not_close (exclusive boundary).
		atBoundary := intutil.CreateTestRegression(t, dbc, "prev-boundary", "4.19",
			intutil.WithOpened(resolved))
		wouldNotClose := intutil.CreateTestRegression(t, dbc, "prev-open", "4.19",
			intutil.WithOpened(resolved.Add(24*time.Hour)))
		triage := intutil.CreateTriage(t, dbc, "https://issues.example.com/PREV-1",
			intutil.WithResolved(resolved), intutil.WithRegressions(wouldClose, atBoundary, wouldNotClose))

		// Failing runs before and after the resolution time drive the gap indicator on the closing regression.
		lastBefore := resolved.Add(-2 * 24 * time.Hour)
		firstAfter := resolved.Add(2 * 24 * time.Hour)
		require.NoError(t, dbc.DB.Create(&models.RegressionJobRun{
			RegressionID: wouldClose.ID, ProwJobRunID: "gap-before", ProwJobName: "job-1", StartTime: lastBefore, TestFailed: true, TestFailures: 1,
		}).Error)
		require.NoError(t, dbc.DB.Create(&models.RegressionJobRun{
			RegressionID: wouldClose.ID, ProwJobRunID: "gap-after", ProwJobName: "job-1", StartTime: firstAfter, TestFailed: true, TestFailures: 1,
		}).Error)

		preview, err := store.ForceClosePreview(triage.ID)
		require.NoError(t, err)
		assert.WithinDuration(t, resolved, preview.Resolved, time.Second)

		require.Len(t, preview.WouldClose, 1, "only the regression opened before resolution should be in would_close")
		assert.Equal(t, wouldClose.ID, preview.WouldClose[0].RegressionID)
		require.NotNil(t, preview.WouldClose[0].LastFailureBeforeResolution, "should report the last failure before resolution")
		assert.WithinDuration(t, lastBefore, *preview.WouldClose[0].LastFailureBeforeResolution, time.Second)
		require.NotNil(t, preview.WouldClose[0].FirstFailureAfterResolution, "should report the first failure after resolution")
		assert.WithinDuration(t, firstAfter, *preview.WouldClose[0].FirstFailureAfterResolution, time.Second)

		notCloseIDs := make([]uint, 0, len(preview.WouldNotClose))
		for _, r := range preview.WouldNotClose {
			notCloseIDs = append(notCloseIDs, r.RegressionID)
		}
		assert.ElementsMatch(t, []uint{atBoundary.ID, wouldNotClose.ID}, notCloseIDs,
			"regressions opened at or after resolution should be in would_not_close")
	})

	t.Run("unresolved triage returns ErrTriageNotResolved", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		store := componentreadiness.NewPostgresRegressionStore(dbc, nil)

		reg := intutil.CreateTestRegression(t, dbc, "prev-guard", "4.19")
		triage := intutil.CreateTriage(t, dbc, "https://issues.example.com/PREV-GUARD", intutil.WithRegressions(reg))

		_, err := store.ForceClosePreview(triage.ID)
		assert.ErrorIs(t, err, componentreadiness.ErrTriageNotResolved)
	})
}

func TestListCurrentRegressionsForReleaseForceClose(t *testing.T) {
	t.Run("force closed regressions are excluded from the reuse window", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		store := componentreadiness.NewPostgresRegressionStore(dbc, nil)

		now := time.Now()
		recentlyClosed := now.Add(-24 * time.Hour) // within the 5-day hysteresis reuse window
		by := "developer"
		reason := "excluded from reuse"

		open := intutil.CreateTestRegression(t, dbc, "list-open", "4.19",
			intutil.WithOpened(now.Add(-10*24*time.Hour)))
		recentNormal := intutil.CreateTestRegression(t, dbc, "list-recent-normal", "4.19",
			intutil.WithOpened(now.Add(-10*24*time.Hour)), intutil.WithClosed(recentlyClosed))
		forceClosed := intutil.CreateTestRegression(t, dbc, "list-forceclosed", "4.19",
			intutil.WithOpened(now.Add(-10*24*time.Hour)), intutil.WithClosed(recentlyClosed),
			intutil.WithForceClosed(true), intutil.WithForceClosedBy(&by), intutil.WithForceClosedReason(&reason))
		oldClosed := intutil.CreateTestRegression(t, dbc, "list-old-closed", "4.19",
			intutil.WithOpened(now.Add(-30*24*time.Hour)), intutil.WithClosed(now.Add(-10*24*time.Hour)))
		otherRelease := intutil.CreateTestRegression(t, dbc, "list-other-release", "4.18",
			intutil.WithOpened(now.Add(-24*time.Hour)))

		got, err := store.ListCurrentRegressionsForRelease("4.19")
		require.NoError(t, err)

		gotIDs := make([]uint, 0, len(got))
		for _, r := range got {
			gotIDs = append(gotIDs, r.ID)
		}
		assert.Contains(t, gotIDs, open.ID, "open regression should be listed")
		assert.Contains(t, gotIDs, recentNormal.ID, "recently closed (non-force) regression should be within the reuse window")
		assert.NotContains(t, gotIDs, forceClosed.ID, "force closed regression should be excluded even though it closed recently")
		assert.NotContains(t, gotIDs, oldClosed.ID, "regression closed beyond the hysteresis window should be excluded")
		assert.NotContains(t, gotIDs, otherRelease.ID, "regression from another release should be excluded")
	})
}

func TestResolveTriagesForceClose(t *testing.T) {
	t.Run("force closed regressions do not block triage auto-resolution", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		store := componentreadiness.NewPostgresRegressionStore(dbc, nil)

		// Closed recently, well within the 5-day hysteresis window: a non-force close would still block
		// auto-resolution, but a force close should not.
		recentlyClosed := time.Now().Add(-1 * time.Hour).Truncate(time.Second)

		// Triage A: its only regression was force closed. Force closed regressions no longer count as
		// active, so the triage should auto-resolve despite the recent close.
		forceClosedReg := intutil.CreateTestRegression(t, dbc, "resolve-forceclosed", "4.19",
			intutil.WithOpened(time.Now().Add(-10*24*time.Hour)))
		triageA := intutil.CreateTriage(t, dbc, "https://issues.example.com/RESOLVE-A",
			intutil.WithRegressions(forceClosedReg))
		require.NoError(t, dbc.DB.Model(&models.TestRegression{}).Where("id = ?", forceClosedReg.ID).
			Updates(map[string]any{
				"closed":              sql.NullTime{Valid: true, Time: recentlyClosed},
				"force_closed":        true,
				"force_closed_by":     "developer",
				"force_closed_reason": "force closed",
			}).Error)

		// Triage B: its only regression was closed recently but NOT force closed, so it still blocks
		// auto-resolution until the hysteresis window passes.
		normalReg := intutil.CreateTestRegression(t, dbc, "resolve-normal", "4.19",
			intutil.WithOpened(time.Now().Add(-10*24*time.Hour)))
		triageB := intutil.CreateTriage(t, dbc, "https://issues.example.com/RESOLVE-B",
			intutil.WithRegressions(normalReg))
		require.NoError(t, dbc.DB.Model(&models.TestRegression{}).Where("id = ?", normalReg.ID).
			Update("closed", sql.NullTime{Valid: true, Time: recentlyClosed}).Error)

		require.NoError(t, store.ResolveTriages())

		var gotA, gotB models.Triage
		require.NoError(t, dbc.DB.First(&gotA, triageA.ID).Error)
		require.NoError(t, dbc.DB.First(&gotB, triageB.ID).Error)

		assert.True(t, gotA.Resolved.Valid, "triage with only a force closed regression should be auto-resolved")
		assert.WithinDuration(t, recentlyClosed, gotA.Resolved.Time, time.Second,
			"resolution time should match the regression close time")
		assert.False(t, gotB.Resolved.Valid,
			"triage with a recently, non-force closed regression should still be blocked")
	})
}

func TestForceCloseRegressionsErrorPaths(t *testing.T) {
	t.Run("nonexistent triage returns a not found error", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		store := componentreadiness.NewPostgresRegressionStore(dbc, nil)

		_, err := store.ForceCloseRegressions(999999, "developer", "valid reason")
		require.Error(t, err)
		assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	})

	t.Run("empty reason is rejected and closes nothing", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		store := componentreadiness.NewPostgresRegressionStore(dbc, nil)

		resolved := time.Now().Truncate(time.Second)
		reg := intutil.CreateTestRegression(t, dbc, "empty-reason", "4.19",
			intutil.WithOpened(resolved.Add(-24*time.Hour)))
		triage := intutil.CreateTriage(t, dbc, "https://issues.example.com/REASON-EMPTY",
			intutil.WithResolved(resolved), intutil.WithRegressions(reg))

		_, err := store.ForceCloseRegressions(triage.ID, "developer", "")
		assert.ErrorIs(t, err, componentreadiness.ErrForceCloseReasonRequired)

		var got models.TestRegression
		require.NoError(t, dbc.DB.First(&got, reg.ID).Error)
		assert.False(t, got.Closed.Valid, "regression must not be closed when the reason is rejected")
		assert.False(t, got.ForceClosed)
	})

	t.Run("whitespace only reason is rejected", func(t *testing.T) {
		dbc := intutil.NewTestDB(t, pgContainer)
		store := componentreadiness.NewPostgresRegressionStore(dbc, nil)

		resolved := time.Now().Truncate(time.Second)
		reg := intutil.CreateTestRegression(t, dbc, "ws-reason", "4.19",
			intutil.WithOpened(resolved.Add(-24*time.Hour)))
		triage := intutil.CreateTriage(t, dbc, "https://issues.example.com/REASON-WS",
			intutil.WithResolved(resolved), intutil.WithRegressions(reg))

		_, err := store.ForceCloseRegressions(triage.ID, "developer", "   ")
		assert.ErrorIs(t, err, componentreadiness.ErrForceCloseReasonRequired)
	})
}

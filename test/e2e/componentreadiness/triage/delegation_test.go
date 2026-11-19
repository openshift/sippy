package triage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/api/componentreadiness"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/test/e2e/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	triagesPath       = "/api/component_readiness/triages"
	delegationTestURL = "https://redhat.atlassian.net/browse/MYBUGS-3105"
)

func delegatedHeaders(principal, actingFor string) map[string]string {
	headers := map[string]string{}
	if principal != "" {
		headers["X-Forwarded-User"] = principal
	}
	if actingFor != "" {
		headers["X-Acting-For"] = actingFor
	}
	return headers
}

func triagePath(id uint) string {
	return fmt.Sprintf("%s/%d", triagesPath, id)
}

// latestAuditLog returns the newest audit record for a triage.
func latestAuditLog(t *testing.T, dbc *db.DB, triageID uint) models.AuditLog {
	var auditLog models.AuditLog
	res := dbc.DB.Where("table_name = ?", "triage").Where("row_id = ?", triageID).Order("created_at DESC").First(&auditLog)
	require.NoError(t, res.Error)
	return auditLog
}

func countTriages(t *testing.T, dbc *db.DB) int64 {
	var count int64
	require.NoError(t, dbc.DB.Model(&models.Triage{}).Count(&count).Error)
	return count
}

func countAuditLogs(t *testing.T, dbc *db.DB) int64 {
	var count int64
	require.NoError(t, dbc.DB.Model(&models.AuditLog{}).Where("table_name = ?", "triage").Count(&count).Error)
	return count
}

// createTriageAs creates a triage through the API with explicit identity headers.
func createTriageAs(t *testing.T, headers map[string]string, regression *models.TestRegression) models.Triage {
	body := models.Triage{
		URL:         delegationTestURL,
		Type:        models.TriageTypeProduct,
		Description: "delegation e2e",
		Regressions: []models.TestRegression{{ID: regression.ID}},
	}
	status, respBody, err := util.SippyRequestWithHeaders(http.MethodPost, triagesPath, headers, body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, string(respBody))
	var created models.Triage
	require.NoError(t, json.Unmarshal(respBody, &created))
	return created
}

// Test_TriageDelegation verifies X-Acting-For delegation from configured trusted delegators, rejection
// of incomplete delegated requests without changes, and persisted audit attribution.
func Test_TriageDelegation(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)
	tracker := componentreadiness.NewPostgresRegressionStore(dbc, nil)
	regression := createTestRegression(t, tracker, view, "delegation-testid")
	defer dbc.DB.Delete(regression)

	t.Run("each configured delegator attributes create, update, and delete to the delegated user", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		for _, tc := range []struct{ principal, user string }{
			{principal: util.TrustedDelegator, user: "alice@example.com"},
			{principal: util.SecondTrustedDelegator, user: "bob@example.com"},
		} {
			headers := delegatedHeaders(tc.principal, tc.user)
			created := createTriageAs(t, headers, regression)
			createLog := latestAuditLog(t, dbc, created.ID)
			assert.Equal(t, string(models.Create), createLog.Operation)
			assert.Equal(t, tc.user, createLog.User)

			created.Description = "updated by " + tc.user
			status, body, err := util.SippyRequestWithHeaders(http.MethodPut, triagePath(created.ID), headers, created)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, status, string(body))
			updateLog := latestAuditLog(t, dbc, created.ID)
			assert.Equal(t, string(models.Update), updateLog.Operation)
			assert.Equal(t, tc.user, updateLog.User)

			status, body, err = util.SippyRequestWithHeaders(http.MethodDelete, triagePath(created.ID), headers, nil)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, status, string(body))
			deleteLog := latestAuditLog(t, dbc, created.ID)
			assert.Equal(t, string(models.Delete), deleteLog.Operation)
			assert.Equal(t, tc.user, deleteLog.User)

			// The audit API reports the same attribution.
			var auditLogs []componentreadiness.TriageAuditLog
			require.NoError(t, util.SippyGet(fmt.Sprintf("%s/audit", triagePath(created.ID)), &auditLogs))
			require.Len(t, auditLogs, 3)
			for _, auditLog := range auditLogs {
				assert.Equal(t, tc.user, auditLog.User)
			}
		}
	})

	t.Run("delegated reads require a delegated identity", func(t *testing.T) {
		status, body, err := util.SippyRequestWithHeaders(http.MethodGet, triagesPath, delegatedHeaders(util.TrustedDelegator, ""), nil)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, status, string(body))

		status, body, err = util.SippyRequestWithHeaders(http.MethodGet, triagesPath, delegatedHeaders(util.TrustedDelegator, "alice@example.com"), nil)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, status, string(body))
	})

	t.Run("delegator without a delegated identity cannot create", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		triagesBefore, auditsBefore := countTriages(t, dbc), countAuditLogs(t, dbc)
		for _, actingFor := range []string{"", "   "} {
			body := models.Triage{URL: delegationTestURL, Type: models.TriageTypeProduct}
			status, respBody, err := util.SippyRequestWithHeaders(http.MethodPost, triagesPath, delegatedHeaders(util.SecondTrustedDelegator, actingFor), body)
			require.NoError(t, err)
			assert.Equal(t, http.StatusBadRequest, status, string(respBody))
			assert.Contains(t, string(respBody), "X-Acting-For")
		}
		assert.Equal(t, triagesBefore, countTriages(t, dbc), "rejected requests must not create triages")
		assert.Equal(t, auditsBefore, countAuditLogs(t, dbc), "rejected requests must not write audit records")
	})

	t.Run("delegator without a delegated identity cannot update or delete", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		created := createTriageAs(t, delegatedHeaders(util.TrustedDelegator, "alice@example.com"), regression)
		auditsBefore := countAuditLogs(t, dbc)

		changed := created
		changed.Description = "should not persist"
		status, body, err := util.SippyRequestWithHeaders(http.MethodPut, triagePath(created.ID), delegatedHeaders(util.TrustedDelegator, ""), changed)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, status, string(body))

		status, body, err = util.SippyRequestWithHeaders(http.MethodDelete, triagePath(created.ID), delegatedHeaders(util.TrustedDelegator, " "), nil)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, status, string(body))

		var current models.Triage
		require.NoError(t, util.SippyGet(triagePath(created.ID), &current))
		assert.Equal(t, created.Description, current.Description)
		assert.Equal(t, auditsBefore, countAuditLogs(t, dbc))
	})

	t.Run("principal not in the allowlist acts as itself", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		created := createTriageAs(t, delegatedHeaders("chai-bot", "mallory@example.com"), regression)
		assert.Equal(t, "chai-bot", latestAuditLog(t, dbc, created.ID).User)
	})

	t.Run("direct user ignores X-Acting-For", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		created := createTriageAs(t, delegatedHeaders("developer", "mallory@example.com"), regression)
		assert.Equal(t, "developer", latestAuditLog(t, dbc, created.ID).User)
	})

	t.Run("unauthenticated request cannot acquire an identity", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		created := createTriageAs(t, delegatedHeaders("developer", ""), regression)
		triagesBefore, auditsBefore := countTriages(t, dbc), countAuditLogs(t, dbc)

		body := models.Triage{URL: delegationTestURL, Type: models.TriageTypeProduct}
		status, respBody, err := util.SippyRequestWithHeaders(http.MethodPost, triagesPath, delegatedHeaders("", "alice@example.com"), body)
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, status, string(respBody))

		changed := created
		changed.Description = "should not persist"
		status, respBody, err = util.SippyRequestWithHeaders(http.MethodPut, triagePath(created.ID), delegatedHeaders("", "alice@example.com"), changed)
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, status, string(respBody))

		status, respBody, err = util.SippyRequestWithHeaders(http.MethodDelete, triagePath(created.ID), delegatedHeaders("", "alice@example.com"), nil)
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, status, string(respBody))

		assert.Equal(t, triagesBefore, countTriages(t, dbc))
		assert.Equal(t, auditsBefore, countAuditLogs(t, dbc))
	})

	t.Run("update and delete of a nonexistent triage return not found", func(t *testing.T) {
		headers := delegatedHeaders(util.TrustedDelegator, "alice@example.com")
		missing := models.Triage{ID: 999999999, URL: delegationTestURL, Type: models.TriageTypeProduct}
		status, body, err := util.SippyRequestWithHeaders(http.MethodPut, triagePath(missing.ID), headers, missing)
		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, status, string(body))

		status, body, err = util.SippyRequestWithHeaders(http.MethodDelete, triagePath(missing.ID), headers, nil)
		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, status, string(body))
	})
}

// Test_TriageUpdatePreservesResolutionReason verifies a full-record PUT that leaves the resolution
// unchanged keeps a system-assigned resolution reason instead of reattributing it to the user.
func Test_TriageUpdatePreservesResolutionReason(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)
	tracker := componentreadiness.NewPostgresRegressionStore(dbc, nil)
	regression := createTestRegression(t, tracker, view, "resolution-reason-testid")
	defer dbc.DB.Delete(regression)
	defer cleanupAllTriages(dbc)

	created := createTriageAs(t, delegatedHeaders("developer", ""), regression)
	resolvedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	res := dbc.DB.Model(&models.Triage{}).Where("id = ?", created.ID).UpdateColumns(map[string]interface{}{
		models.ColResolved:         sql.NullTime{Time: resolvedAt, Valid: true},
		models.ColResolutionReason: models.JiraProgression,
	})
	require.NoError(t, res.Error)

	var current models.Triage
	require.NoError(t, util.SippyGet(triagePath(created.ID), &current))
	require.Equal(t, models.JiraProgression, current.ResolutionReason)
	current.Description = "edited without touching the resolution"

	var updated models.Triage
	require.NoError(t, util.SippyPut(triagePath(created.ID), &current, &updated))
	assert.Equal(t, models.JiraProgression, updated.ResolutionReason)
	assert.True(t, updated.Resolved.Time.Equal(resolvedAt))
}

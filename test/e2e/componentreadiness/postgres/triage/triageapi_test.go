package triage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/openshift/sippy/pkg/api/componentreadiness"
	"github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/sippyserver"
	"github.com/openshift/sippy/test/e2e/util"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var view = crview.View{
	Name: fmt.Sprintf("%s-main", util.Release),
	BaseRelease: reqopts.RelativeRelease{
		Release: reqopts.Release{
			Name: util.BaseRelease,
		},
	},
	SampleRelease: reqopts.RelativeRelease{
		Release: reqopts.Release{
			Name: util.Release,
		},
	},
}

// cleanupTriages deletes only the specified triages and their associated
// regression links. Tests should clean up only what they create to avoid
// destroying seed data.
func cleanupTriages(dbc *db.DB, triages ...*models.Triage) {
	for _, tr := range triages {
		if tr == nil || tr.ID == 0 {
			continue
		}
		dbc.DB.Exec("DELETE FROM triage_regressions WHERE triage_id = ?", tr.ID)
		res := dbc.DB.Delete(tr)
		if res.Error != nil {
			log.Errorf("error deleting triage %d: %v", tr.ID, res.Error)
		}
	}
}

func Test_TriageAPI(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)
	// jiraClient is intentionally nil to prevent commenting on jiras
	tracker := componentreadiness.NewPostgresRegressionStore(dbc, nil)

	jiraBug := createBug(t, dbc.DB)
	defer dbc.DB.Delete(jiraBug)

	testRegression1 := createTestRegression(t, tracker, view, "faketestid")
	defer dbc.DB.Delete(testRegression1)
	testRegression2 := createTestRegression(t, tracker, view, "faketestid2")
	defer dbc.DB.Delete(testRegression2)

	t.Run("create requires a valid triage type", func(t *testing.T) {
		triage1 := models.Triage{
			URL: jiraBug.URL,
			Regressions: []models.TestRegression{
				{
					ID: testRegression1.ID, // test just setting the ID to link up
				},
			},
		}

		var triageResponse models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage1, &triageResponse)
		require.Error(t, err)

		triage1.Type = "fake"
		err = util.SippyPost("/api/component_readiness/triages", &triage1, &triageResponse)
		require.Error(t, err)
	})

	t.Run("create fails with non-existent regression ID", func(t *testing.T) {
		triage := models.Triage{
			URL:  jiraBug.URL,
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				{ID: testRegression1.ID},
				{ID: 999999}, // non-existent
			},
		}

		var triageResponse models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage, &triageResponse)
		require.Error(t, err, "should fail when a regression ID does not exist")
	})

	t.Run("create generates audit_log record", func(t *testing.T) {
		triage1 := models.Triage{
			URL: jiraBug.URL,
			Regressions: []models.TestRegression{
				{
					ID: testRegression1.ID,
				},
			},
			Type: "test",
		}

		var triageResponse models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage1, &triageResponse)
		require.NoError(t, err)
		defer cleanupTriages(dbc, &triageResponse)

		var auditLog models.AuditLog
		res := dbc.DB.
			Where("table_name = ?", "triage").
			Where("row_id = ?", triageResponse.ID).
			First(&auditLog)
		require.NoError(t, res.Error)

		assert.Equal(t, models.Create, models.OperationType(auditLog.Operation))
		assert.Equal(t, "developer", auditLog.User)
		assert.NotEmpty(t, auditLog.NewData, "NewData should contain the created triage record")

		var auditedTriage models.Triage
		err = json.Unmarshal(auditLog.NewData, &auditedTriage)
		require.NoError(t, err, "NewData should be valid JSON")

		assertTriageDataMatches(t, triageResponse, auditedTriage, "NewData")
	})

	t.Run("get", func(t *testing.T) {
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)
		defer cleanupTriages(dbc, &triageResponse)

		// ensure hateoas links are present
		require.NotEmpty(t, triageResponse.Links["self"])
		assert.Equal(t, fmt.Sprintf("http://%s:%s/api/component_readiness/triages/%d", os.Getenv("SIPPY_ENDPOINT"), os.Getenv("SIPPY_API_PORT"), triageResponse.ID),
			triageResponse.Links["self"])
		require.NotEmpty(t, triageResponse.Links["potential_matches"])
		assert.Equal(t, fmt.Sprintf("http://%s:%s/api/component_readiness/triages/%d/matches", os.Getenv("SIPPY_ENDPOINT"), os.Getenv("SIPPY_API_PORT"), triageResponse.ID),
			triageResponse.Links["potential_matches"])
		require.NotEmpty(t, triageResponse.Links["audit_logs"])
		assert.Equal(t, fmt.Sprintf("http://%s:%s/api/component_readiness/triages/%d/audit", os.Getenv("SIPPY_ENDPOINT"), os.Getenv("SIPPY_API_PORT"), triageResponse.ID),
			triageResponse.Links["audit_logs"])
	})
	t.Run("get with expanded regressions", func(t *testing.T) {
		// Use real regressions from seed data instead of injecting fake data into cache.
		// We filter to seed data regressions (test IDs starting with "test-") because
		// other subtests in this function create synthetic regressions that won't appear
		// in the component report.
		realRegressions := getSeedDataRegressions(t)
		require.GreaterOrEqual(t, len(realRegressions), 2, "seed data should produce at least 2 regressions")

		// Create a triage with real regressions
		triage := models.Triage{
			URL:  jiraBug.URL,
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				{ID: realRegressions[0].ID},
				{ID: realRegressions[1].ID},
			},
		}

		var triageResponse models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage, &triageResponse)
		require.NoError(t, err)
		defer cleanupTriages(dbc, &triageResponse)
		require.Equal(t, 2, len(triageResponse.Regressions))

		// Validate that the expanded regressions are present
		var expandedTriage sippyserver.ExpandedTriage
		err = util.SippyGet(fmt.Sprintf("/api/component_readiness/triages/%d?view=%s-main&expand=regressions", triageResponse.ID, util.Release), &expandedTriage)
		require.NoError(t, err)

		// Verify the expanded triage contains the regressed tests with correct status values
		require.NotNil(t, expandedTriage.Triage, "ExpandedTriage should contain a Triage")
		assert.Equal(t, triageResponse.ID, expandedTriage.Triage.ID, "ExpandedTriage should have the same ID as the created triage")
		expectedViewKey := view.Name
		require.Contains(t, expandedTriage.RegressedTests, expectedViewKey, "ExpandedTriage should contain regressed tests for view %q", expectedViewKey)
		regressedTestsForView := expandedTriage.RegressedTests[expectedViewKey]
		assert.Len(t, regressedTestsForView, 2, "ExpandedTriage should contain 2 regressed tests for view %q", expectedViewKey)

		// Verify status values are marked as triaged (the triage causes status transformation)
		for _, regressedTest := range regressedTestsForView {
			require.NotNil(t, regressedTest, "regressed test should not be nil")
			require.NotNil(t, regressedTest.Regression, "regressed test should have regression data")
			assert.True(t,
				regressedTest.TestComparison.ReportStatus == crtest.ExtremeTriagedRegression ||
					regressedTest.TestComparison.ReportStatus == crtest.SignificantTriagedRegression,
				"regressed test %s should have a triaged status, got %d",
				regressedTest.Regression.TestID, regressedTest.TestComparison.ReportStatus)
		}
	})
	t.Run("list", func(t *testing.T) {
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)
		defer cleanupTriages(dbc, &triageResponse)

		var allTriages []models.Triage
		err := util.SippyGet("/api/component_readiness/triages", &allTriages)
		require.NoError(t, err)
		var foundTriage *models.Triage
		for i, triage := range allTriages {
			if triage.ID == triageResponse.ID {
				foundTriage = &allTriages[i]
				break
			}
		}
		require.NotNil(t, foundTriage, "expected triage was not found in list")
		assert.Equal(t, testRegression1.TestName, foundTriage.Regressions[0].TestName,
			"list triage records should include regression details")

		// ensure hateoas links are present
		for _, triage := range allTriages {
			require.NotEmpty(t, triage.Links["self"])
			assert.Equal(t, fmt.Sprintf("http://%s:%s/api/component_readiness/triages/%d", os.Getenv("SIPPY_ENDPOINT"), os.Getenv("SIPPY_API_PORT"), triage.ID),
				triage.Links["self"])
			require.NotEmpty(t, triage.Links["potential_matches"])
			assert.Equal(t, fmt.Sprintf("http://%s:%s/api/component_readiness/triages/%d/matches", os.Getenv("SIPPY_ENDPOINT"), os.Getenv("SIPPY_API_PORT"), triage.ID),
				triage.Links["potential_matches"])
			require.NotEmpty(t, triage.Links["audit_logs"])
			assert.Equal(t, fmt.Sprintf("http://%s:%s/api/component_readiness/triages/%d/audit", os.Getenv("SIPPY_ENDPOINT"), os.Getenv("SIPPY_API_PORT"), triage.ID),
				triage.Links["audit_logs"])
		}
	})
	t.Run("update to add regression", func(t *testing.T) {
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)
		defer cleanupTriages(dbc, &triageResponse)

		// Update with a new regression:
		var triageResponse2 models.Triage
		triageResponse.Regressions = append(triageResponse.Regressions, models.TestRegression{ID: testRegression2.ID})
		err := util.SippyPut(fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse.ID), &triageResponse, &triageResponse2)
		require.NoError(t, err)
		assert.Equal(t, 2, len(triageResponse2.Regressions))
		assert.Equal(t, triageResponse.CreatedAt, triageResponse2.CreatedAt)
		assert.NotEqual(t, triageResponse.UpdatedAt, triageResponse2.UpdatedAt)

		// ensure hateoas links are present
		require.NotEmpty(t, triageResponse2.Links["self"])
		assert.Equal(t, fmt.Sprintf("http://%s:%s/api/component_readiness/triages/%d", os.Getenv("SIPPY_ENDPOINT"), os.Getenv("SIPPY_API_PORT"), triageResponse2.ID),
			triageResponse2.Links["self"])
		require.NotEmpty(t, triageResponse2.Links["potential_matches"])
		assert.Equal(t, fmt.Sprintf("http://%s:%s/api/component_readiness/triages/%d/matches", os.Getenv("SIPPY_ENDPOINT"), os.Getenv("SIPPY_API_PORT"), triageResponse2.ID),
			triageResponse2.Links["potential_matches"])
		require.NotEmpty(t, triageResponse2.Links["audit_logs"])
		assert.Equal(t, fmt.Sprintf("http://%s:%s/api/component_readiness/triages/%d/audit", os.Getenv("SIPPY_ENDPOINT"), os.Getenv("SIPPY_API_PORT"), triageResponse2.ID),
			triageResponse2.Links["audit_logs"])
	})
	t.Run("update to remove a regression", func(t *testing.T) {
		triage := models.Triage{
			URL:  jiraBug.URL,
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				{ID: testRegression1.ID},
				{ID: testRegression2.ID},
			},
		}

		var triageResponse models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage, &triageResponse)
		require.NoError(t, err)
		defer cleanupTriages(dbc, &triageResponse)
		assert.Equal(t, 2, len(triageResponse.Regressions))

		// Update to remove one regression - keep only testRegression1
		triageResponse.Regressions = []models.TestRegression{{ID: testRegression1.ID}}
		var triageResponse2 models.Triage
		err = util.SippyPut(fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse.ID), &triageResponse, &triageResponse2)
		require.NoError(t, err)
		assert.Equal(t, 1, len(triageResponse2.Regressions))
		assert.Equal(t, testRegression1.ID, triageResponse2.Regressions[0].ID, "should keep testRegression1")
		assert.WithinDuration(t, triageResponse.CreatedAt, triageResponse2.CreatedAt, time.Second)
		assert.NotEqual(t, triageResponse.UpdatedAt, triageResponse2.UpdatedAt)
	})
	t.Run("update to remove all regressions", func(t *testing.T) {
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)
		defer cleanupTriages(dbc, &triageResponse)

		var triageResponse2 models.Triage
		triageResponse.Regressions = []models.TestRegression{}
		err := util.SippyPut(fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse.ID), &triageResponse, &triageResponse2)
		require.NoError(t, err)
		assert.Equal(t, 0, len(triageResponse2.Regressions))
	})
	t.Run("update to resolve triage sets resolution reason to user", func(t *testing.T) {
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)
		defer cleanupTriages(dbc, &triageResponse)

		// Resolve the triage by setting the Resolved timestamp
		resolvedTime := time.Now()
		triageResponse.Resolved = sql.NullTime{Time: resolvedTime, Valid: true}

		var updateResponse models.Triage
		err := util.SippyPut(fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse.ID), &triageResponse, &updateResponse)
		require.NoError(t, err)

		// Verify the triage was resolved and resolution reason was set correctly
		assert.True(t, updateResponse.Resolved.Valid, "Triage should be marked as resolved")
		assert.WithinDuration(t, resolvedTime, updateResponse.Resolved.Time, time.Second, "Resolved time should match")
		assert.Equal(t, models.User, updateResponse.ResolutionReason, "Resolution reason should be set to 'user'")
	})
	t.Run("update fails if resource has no ID", func(t *testing.T) {
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)
		defer cleanupTriages(dbc, &triageResponse)

		var triageResponse2 models.Triage
		triageResponse.ID = 0
		err := util.SippyPut(fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse.ID), &triageResponse, &triageResponse2)
		require.Error(t, err)
	})
	t.Run("update fails if URL has no ID", func(t *testing.T) {
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)
		defer cleanupTriages(dbc, &triageResponse)

		var triageResponse2 models.Triage
		// No ID specified in URL should not work for an update
		err := util.SippyPut("/api/component_readiness/triages", &triageResponse, &triageResponse2)
		require.Error(t, err)
	})
	t.Run("update fails if URL ID and resource ID do not match", func(t *testing.T) {
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)
		defer cleanupTriages(dbc, &triageResponse)

		var triageResponse2 models.Triage
		err := util.SippyPut("/api/component_readiness/triages/128736182736128736", &triageResponse, &triageResponse2)
		require.Error(t, err)
	})
	t.Run("update generates audit_log record", func(t *testing.T) {
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)
		defer cleanupTriages(dbc, &triageResponse)
		originalTriage := deepCopyTriage(t, triageResponse)

		// Update with a new regression, and a changed description:
		triageResponse.Regressions = append(triageResponse.Regressions, models.TestRegression{ID: testRegression2.ID})
		triageResponse.Description = "updated description"
		var triageResponse2 models.Triage
		err := util.SippyPut(fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse.ID), &triageResponse, &triageResponse2)
		require.NoError(t, err)

		var auditLog models.AuditLog
		res := dbc.DB.
			Where("table_name = ?", "triage").
			Where("operation = ?", models.Update).
			Where("row_id = ?", triageResponse.ID).
			First(&auditLog)
		require.NoError(t, res.Error)

		assert.Equal(t, "developer", auditLog.User)
		assert.NotEmpty(t, auditLog.NewData, "NewData should contain the updated triage record")
		assert.NotEmpty(t, auditLog.OldData, "OldData should contain the original triage record")

		var newTriageData models.Triage
		err = json.Unmarshal(auditLog.NewData, &newTriageData)
		require.NoError(t, err, "NewData should be valid JSON")
		assertTriageDataMatches(t, triageResponse2, newTriageData, "NewData")

		var oldTriageData models.Triage
		err = json.Unmarshal(auditLog.OldData, &oldTriageData)
		require.NoError(t, err, "OldData should be valid JSON")
		assertTriageDataMatches(t, originalTriage, oldTriageData, "OldData")
	})
	t.Run("delete generates audit_log record", func(t *testing.T) {
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)
		defer cleanupTriages(dbc, &triageResponse)
		originalTriage := deepCopyTriage(t, triageResponse)

		// Delete the triage record
		err := util.SippyDelete(fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse.ID))
		require.NoError(t, err)

		var auditLog models.AuditLog
		res := dbc.DB.
			Where("table_name = ?", "triage").
			Where("operation = ?", models.Delete).
			Where("row_id = ?", triageResponse.ID).
			First(&auditLog)
		require.NoError(t, res.Error)

		assert.Equal(t, "developer", auditLog.User)
		assert.NotEmpty(t, auditLog.OldData, "OldData should contain the deleted triage record")
		assert.Empty(t, auditLog.NewData, "NewData should be empty for delete operations")

		var oldTriageData models.Triage
		err = json.Unmarshal(auditLog.OldData, &oldTriageData)
		require.NoError(t, err, "OldData should be valid JSON")
		assertTriageDataMatches(t, originalTriage, oldTriageData, "OldData")
	})

	t.Run("audit endpoint returns full lifecycle operations", func(t *testing.T) {
		// Create a triage
		triage := models.Triage{
			URL:         "https://issues.redhat.com/browse/OCPBUGS-8888",
			Description: "Initial description for audit test",
			Type:        models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				{ID: testRegression1.ID},
			},
		}

		var triageResponse models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage, &triageResponse)
		require.NoError(t, err)
		defer cleanupTriages(dbc, &triageResponse)
		require.True(t, triageResponse.ID > 0)

		// Small delay to ensure different timestamps
		time.Sleep(10 * time.Millisecond)

		// Update the triage
		triageResponse.Description = "Updated description for audit test"
		triageResponse.Type = models.TriageTypeCIInfra
		triageResponse.Regressions = append(triageResponse.Regressions, models.TestRegression{ID: testRegression2.ID})

		var updatedTriageResponse models.Triage
		err = util.SippyPut(fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse.ID), &triageResponse, &updatedTriageResponse)
		require.NoError(t, err)

		// Small delay to ensure different timestamps
		time.Sleep(10 * time.Millisecond)

		// Delete the triage
		err = util.SippyDelete(fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse.ID))
		require.NoError(t, err)

		// Call the audit endpoint
		var auditLogs []componentreadiness.TriageAuditLog
		err = util.SippyGet(fmt.Sprintf("/api/component_readiness/triages/%d/audit", triageResponse.ID), &auditLogs)
		require.NoError(t, err)

		// Verify we have exactly 3 audit log entries (create, update, delete)
		require.Len(t, auditLogs, 3, "Should have exactly 3 audit log entries")

		// Ensure most recent first
		require.True(t, auditLogs[0].CreatedAt.After(auditLogs[1].CreatedAt), "Audit logs should be ordered by creation time (newest first)")
		require.True(t, auditLogs[1].CreatedAt.After(auditLogs[2].CreatedAt), "Audit logs should be ordered by creation time (newest first)")

		// Verify DELETE operation (most recent)
		deleteLog := auditLogs[0]
		assert.Equal(t, "DELETE", deleteLog.Operation)
		assert.Equal(t, "developer", deleteLog.User)
		assert.NotEmpty(t, deleteLog.Changes, "Delete operation should have changes")

		// Verify DELETE changes show fields going from values to empty
		deleteChangesByField := make(map[string]componentreadiness.FieldChange)
		for _, change := range deleteLog.Changes {
			deleteChangesByField[change.FieldName] = change
		}

		assert.Contains(t, deleteChangesByField, "url")
		assert.Equal(t, "https://issues.redhat.com/browse/OCPBUGS-8888", deleteChangesByField["url"].Original)
		assert.Equal(t, "", deleteChangesByField["url"].Modified)

		assert.Contains(t, deleteChangesByField, "description")
		assert.Equal(t, "Updated description for audit test", deleteChangesByField["description"].Original)
		assert.Equal(t, "", deleteChangesByField["description"].Modified)

		assert.Contains(t, deleteChangesByField, "type")
		assert.Equal(t, "ci-infra", deleteChangesByField["type"].Original)
		assert.Equal(t, "", deleteChangesByField["type"].Modified)

		// Verify UPDATE operation (middle)
		updateLog := auditLogs[1]
		assert.Equal(t, "UPDATE", updateLog.Operation)
		assert.Equal(t, "developer", updateLog.User)
		assert.NotEmpty(t, updateLog.Changes, "Update operation should have changes")

		// Verify UPDATE changes show field transitions
		updateChangesByField := make(map[string]componentreadiness.FieldChange)
		for _, change := range updateLog.Changes {
			updateChangesByField[change.FieldName] = change
		}

		assert.Contains(t, updateChangesByField, "description")
		assert.Equal(t, "Initial description for audit test", updateChangesByField["description"].Original)
		assert.Equal(t, "Updated description for audit test", updateChangesByField["description"].Modified)

		assert.Contains(t, updateChangesByField, "type")
		assert.Equal(t, "product", updateChangesByField["type"].Original)
		assert.Equal(t, "ci-infra", updateChangesByField["type"].Modified)

		assert.Contains(t, updateChangesByField, "regressions")
		assert.NotEmpty(t, updateChangesByField["regressions"].Original, "Should show original regression IDs")
		assert.NotEmpty(t, updateChangesByField["regressions"].Modified, "Should show updated regression IDs")

		// Verify CREATE operation (oldest)
		createLog := auditLogs[2]
		assert.Equal(t, "CREATE", createLog.Operation)
		assert.Equal(t, "developer", createLog.User)
		assert.NotEmpty(t, createLog.Changes, "Create operation should have changes")

		// Verify CREATE changes show fields going from empty to values
		createChangesByField := make(map[string]componentreadiness.FieldChange)
		for _, change := range createLog.Changes {
			createChangesByField[change.FieldName] = change
		}

		assert.Contains(t, createChangesByField, "url")
		assert.Equal(t, "", createChangesByField["url"].Original)
		assert.Equal(t, "https://issues.redhat.com/browse/OCPBUGS-8888", createChangesByField["url"].Modified)

		assert.Contains(t, createChangesByField, "description")
		assert.Equal(t, "", createChangesByField["description"].Original)
		assert.Equal(t, "Initial description for audit test", createChangesByField["description"].Modified)

		assert.Contains(t, createChangesByField, "type")
		assert.Equal(t, "", createChangesByField["type"].Original)
		assert.Equal(t, "product", createChangesByField["type"].Modified)

		assert.Contains(t, createChangesByField, "regressions")
		assert.Equal(t, "", createChangesByField["regressions"].Original)
		assert.NotEmpty(t, createChangesByField["regressions"].Modified, "Should show created regression IDs")

		// Verify timestamps are in chronological order
		assert.True(t, createLog.CreatedAt.Before(updateLog.CreatedAt), "Create should be before update")
		assert.True(t, updateLog.CreatedAt.Before(deleteLog.CreatedAt), "Update should be before delete")

		// Verify HATEOAS links are present in audit log responses
		baseURL := fmt.Sprintf("http://%s:%s", os.Getenv("SIPPY_ENDPOINT"), os.Getenv("SIPPY_API_PORT"))
		for _, auditLog := range auditLogs {
			assert.Equal(t, fmt.Sprintf("%s/api/component_readiness/triages/%d/audit", baseURL, triageResponse.ID),
				auditLog.Links["self"], "Audit log should have self link")
			assert.Equal(t, fmt.Sprintf("%s/api/component_readiness/triages/%d", baseURL, triageResponse.ID),
				auditLog.Links["triage"], "Audit log should have triage link")
		}
	})

}

func Test_RegressionAPI(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)
	// jiraClient is intentionally nil to prevent commenting on jiras
	tracker := componentreadiness.NewPostgresRegressionStore(dbc, nil)

	testRegression1 := createTestRegression(t, tracker, view, "faketestid1")
	defer dbc.DB.Delete(testRegression1)

	testRegression2 := createTestRegression(t, tracker, view, "faketestid2")
	defer dbc.DB.Delete(testRegression2)

	jiraBug := createBug(t, dbc.DB)
	defer dbc.DB.Delete(jiraBug)

	release := view.SampleRelease.Release.Name

	t.Run("list regressions", func(t *testing.T) {
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)
		defer cleanupTriages(dbc, &triageResponse)

		// Test listing regressions by release (release is required)
		var allRegressions []models.TestRegression
		err := util.SippyGet("/api/component_readiness/regressions?release="+release, &allRegressions)
		require.NoError(t, err)

		// Should find at least our test regression
		var foundRegression *models.TestRegression
		for i, regression := range allRegressions {
			if regression.ID == testRegression1.ID {
				foundRegression = &allRegressions[i]
				break
			}
		}
		require.NotNil(t, foundRegression, "expected regression was not found in list")
		assert.Equal(t, testRegression1.TestName, foundRegression.TestName)
		assert.Equal(t, testRegression1.Release, foundRegression.Release)

		// Verify HATEOAS links are present
		assert.NotNil(t, foundRegression.Links, "regression should have HATEOAS links")
		assert.Contains(t, foundRegression.Links, "test_details", "regression should have test_details link")
		require.NotEmpty(t, foundRegression.Links["test_details"], "test_details link should not be empty")
		testDetailsHREF := foundRegression.Links["test_details"]
		assert.Contains(t, testDetailsHREF, fmt.Sprintf("http://%s:%s/api/component_readiness/test_details", os.Getenv("SIPPY_ENDPOINT"), os.Getenv("SIPPY_API_PORT")), "test_details link should point to correct endpoint")
		// Note: testId will be URL encoded, so we check for the encoded version
		assert.Contains(t, testDetailsHREF, "testId=", "test_details link should contain testId parameter")
	})
	t.Run("error when both view and release are specified", func(t *testing.T) {
		var regressions []models.TestRegression
		err := util.SippyGet(fmt.Sprintf("/api/component_readiness/regressions?view=%s-main&release=%s", util.Release, util.Release), &regressions)
		require.Error(t, err, "Expected error when both view and release are provided")
	})
}

// Test_RegressionPotentialMatchingTriages tests the /api/component_readiness/regressions/{id}/matches endpoint
func Test_RegressionPotentialMatchingTriages(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)
	// jiraClient is intentionally nil to prevent commenting on jiras
	tracker := componentreadiness.NewPostgresRegressionStore(dbc, nil)

	jiraBug := createBug(t, dbc.DB)
	defer dbc.DB.Delete(jiraBug)

	// Create test regressions with specific characteristics for matching
	commonFailureTime := time.Now().Add(-24 * time.Hour)
	differentFailureTime := time.Now().Add(-12 * time.Hour)

	// Regression 1: Will be the target regression for matching
	targetRegression := createTestRegressionWithDetails(t, tracker, view, "target-test", "component-a", "capability-x", "TestTargetFunction", &commonFailureTime, crtest.ExtremeRegression)
	defer dbc.DB.Delete(targetRegression.Regression)

	// Regression 2: Will match by similar test name (edit distance <= 5)
	matchByNameRegression := createTestRegressionWithDetails(t, tracker, view, "match-name", "component-b", "capability-y", "TestTargetFunctin", &differentFailureTime, crtest.SignificantRegression) // missing 'o' from "TestTargetFunction"
	defer dbc.DB.Delete(matchByNameRegression.Regression)

	// Regression 3: Will match by same last failure time
	matchByTimeRegression := createTestRegressionWithDetails(t, tracker, view, "match-time", "component-c", "capability-z", "TestDifferentName", &commonFailureTime, crtest.ExtremeTriagedRegression)
	defer dbc.DB.Delete(matchByTimeRegression.Regression)

	// Regression 4: No match - different name and different failure time
	noMatchRegression := createTestRegressionWithDetails(t, tracker, view, "no-match", "component-d", "capability-w", "CompletelyDifferentTest", &differentFailureTime, crtest.NotSignificant)
	defer dbc.DB.Delete(noMatchRegression.Regression)

	t.Run("find potential matching triages", func(t *testing.T) {
		// Create triages with the matching regressions
		triage1 := models.Triage{
			URL:  jiraBug.URL,
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				{ID: matchByNameRegression.Regression.ID},
			},
		}
		var triageResponse1 models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage1, &triageResponse1)
		require.NoError(t, err)
		defer cleanupTriages(dbc, &triageResponse1)

		triage2 := models.Triage{
			URL:  jiraBug.URL,
			Type: models.TriageTypeCIInfra,
			Regressions: []models.TestRegression{
				{ID: matchByTimeRegression.Regression.ID},
			},
		}
		var triageResponse2 models.Triage
		err = util.SippyPost("/api/component_readiness/triages", &triage2, &triageResponse2)
		require.NoError(t, err)
		defer cleanupTriages(dbc, &triageResponse2)

		// Create a triage with the no-match regression (should not appear in results)
		triageNoMatch := models.Triage{
			URL:  jiraBug.URL,
			Type: models.TriageTypeTest,
			Regressions: []models.TestRegression{
				{ID: noMatchRegression.Regression.ID},
			},
		}
		var triageResponseNoMatch models.Triage
		err = util.SippyPost("/api/component_readiness/triages", &triageNoMatch, &triageResponseNoMatch)
		require.NoError(t, err)
		defer cleanupTriages(dbc, &triageResponseNoMatch)

		// Query for potential matches for the target regression
		var potentialMatches []componentreadiness.PotentialMatchingTriage
		endpoint := fmt.Sprintf("/api/component_readiness/regressions/%d/matches", targetRegression.Regression.ID)
		err = util.SippyGet(endpoint, &potentialMatches)
		require.NoError(t, err)

		// Verify we found 2 potential matches
		assert.Len(t, potentialMatches, 2, "Should find 2 potential matching triages")

		// Verify HATEOAS links are present
		for _, match := range potentialMatches {
			assert.Contains(t, match.Links, "self", "Potential match should have self link")
		}

		// Build map for easier verification
		triagesByID := make(map[uint]componentreadiness.PotentialMatchingTriage)
		for _, match := range potentialMatches {
			triagesByID[match.Triage.ID] = match
		}

		// Verify match by similar name
		nameMatch, found := triagesByID[triageResponse1.ID]
		assert.True(t, found, "Should find triage with similar named test")
		assert.Len(t, nameMatch.SimilarlyNamedTests, 1, "Should have one similarly named test")
		assert.Equal(t, 1, nameMatch.SimilarlyNamedTests[0].EditDistance, "Edit distance should be 1")
		assert.Equal(t, 5, nameMatch.ConfidenceLevel, "Confidence should be 5 (6-1)")

		// Verify match by same failure time
		timeMatch, found := triagesByID[triageResponse2.ID]
		assert.True(t, found, "Should find triage with same failure time")
		assert.Len(t, timeMatch.SameLastFailures, 1, "Should have one same failure time match")
		assert.Equal(t, 1, timeMatch.ConfidenceLevel, "Confidence should be 1")

		// Verify non-matching triage is not included
		_, found = triagesByID[triageResponseNoMatch.ID]
		assert.False(t, found, "Should not find triage with no matching criteria")
	})

	t.Run("no potential matches found", func(t *testing.T) {
		// Create a triage with the no-match regression (different name and time)
		triage := models.Triage{
			URL:  jiraBug.URL,
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				{ID: noMatchRegression.Regression.ID},
			},
		}
		var triageResponse models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage, &triageResponse)
		require.NoError(t, err)
		defer cleanupTriages(dbc, &triageResponse)

		// Query for potential matches for the target regression
		var potentialMatches []componentreadiness.PotentialMatchingTriage
		endpoint := fmt.Sprintf("/api/component_readiness/regressions/%d/matches", targetRegression.Regression.ID)
		err = util.SippyGet(endpoint, &potentialMatches)
		require.NoError(t, err)

		// Should find no matches since the test name and failure time are too different
		assert.Len(t, potentialMatches, 0, "Should find no potential matching triages")
	})

	t.Run("resolved triage confidence level capped at 5", func(t *testing.T) {
		// Create a regression with the exact same test name (edit distance 0, would normally give confidence 6)
		exactMatchRegression := createTestRegressionWithDetails(t, tracker, view, "exact-match", "component-e", "capability-v", "TestTargetFunction", &differentFailureTime, crtest.ExtremeRegression)
		defer dbc.DB.Delete(exactMatchRegression.Regression)

		// Create a triage with the exact match regression
		triageExactMatch := models.Triage{
			URL:  jiraBug.URL,
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				{ID: exactMatchRegression.Regression.ID},
			},
		}
		var triageResponseExactMatch models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triageExactMatch, &triageResponseExactMatch)
		require.NoError(t, err)
		defer cleanupTriages(dbc, &triageResponseExactMatch)

		// Resolve the triage
		resolvedTime := time.Now()
		triageResponseExactMatch.Resolved = sql.NullTime{Time: resolvedTime, Valid: true}
		var updateResponse models.Triage
		err = util.SippyPut(fmt.Sprintf("/api/component_readiness/triages/%d", triageResponseExactMatch.ID), &triageResponseExactMatch, &updateResponse)
		require.NoError(t, err)
		assert.True(t, updateResponse.Resolved.Valid, "Triage should be marked as resolved")

		// Query for potential matches for the target regression
		var potentialMatches []componentreadiness.PotentialMatchingTriage
		endpoint := fmt.Sprintf("/api/component_readiness/regressions/%d/matches", targetRegression.Regression.ID)
		err = util.SippyGet(endpoint, &potentialMatches)
		require.NoError(t, err)

		// Verify we found the resolved triage
		assert.Len(t, potentialMatches, 1, "Should find 1 potential matching triage")

		// Build map for easier verification
		triagesByID := make(map[uint]componentreadiness.PotentialMatchingTriage)
		for _, match := range potentialMatches {
			triagesByID[match.Triage.ID] = match
		}

		// Verify the resolved triage match
		resolvedMatch, found := triagesByID[triageResponseExactMatch.ID]
		assert.True(t, found, "Should find resolved triage with exact test name match")
		assert.Len(t, resolvedMatch.SimilarlyNamedTests, 1, "Should have one similarly named test")
		assert.Equal(t, 0, resolvedMatch.SimilarlyNamedTests[0].EditDistance, "Edit distance should be 0")
		// Confidence should be capped at 5 even though edit distance 0 would normally give confidence 6
		assert.Equal(t, 5, resolvedMatch.ConfidenceLevel, "Confidence should be capped at 5 for resolved triage")
	})
}

func createAndValidateTriageRecord(t *testing.T, bugURL string, testRegression1 *models.TestRegression) models.Triage {
	triage1 := models.Triage{
		URL:  bugURL,
		Type: models.TriageTypeProduct,
		Regressions: []models.TestRegression{
			{
				ID: testRegression1.ID, // test just setting the ID to link up
			},
		},
	}

	var triageResponse models.Triage
	err := util.SippyPost("/api/component_readiness/triages", &triage1, &triageResponse)
	require.NoError(t, err)
	assert.True(t, triageResponse.ID > 0)
	assert.Equal(t, 1, len(triageResponse.Regressions))

	// Use the API get to ensure we get a clean object
	var lookupTriage models.Triage
	err = util.SippyGet(fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse.ID), &lookupTriage)
	require.NoError(t, err)
	assert.Equal(t, models.TriageTypeProduct, lookupTriage.Type)
	return lookupTriage
}

func createBug(t *testing.T, dbc *gorm.DB) *models.Bug {
	jiraBug := models.Bug{
		Key:        "MYBUGS-100",
		Status:     "New",
		Summary:    "foo bar",
		Components: pq.StringArray{"component1", "component2"},
		Labels:     pq.StringArray{"label1", "label2"},
		URL:        "https://issues.redhat.com/browse/MYBUGS-100",
	}
	res := dbc.Create(&jiraBug)
	require.NoError(t, res.Error)
	return &jiraBug
}

// Test_TriageRawDB ensures our gorm postgresql mappings are working as we'd expect.
func Test_TriageRawDB(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)
	dbWithContext := dbc.DB.WithContext(context.WithValue(context.TODO(), models.CurrentUserKey, "developer"))
	// jiraClient is intentionally nil to prevent commenting on jiras
	tracker := componentreadiness.NewPostgresRegressionStore(dbc, nil)

	testRegression := createTestRegression(t, tracker, view, "faketestid")
	defer dbc.DB.Delete(testRegression)

	t.Run("test Triage model in postgres", func(t *testing.T) {
		triage1 := models.Triage{
			URL: "http://myjira",
			Regressions: []models.TestRegression{
				*testRegression,
			},
		}
		res := dbWithContext.Create(&triage1)
		require.NoError(t, res.Error)
		defer cleanupTriages(dbc, &triage1)
		testRegression.Triages = append(testRegression.Triages, triage1)
		res = dbWithContext.Save(&testRegression)
		require.NoError(t, res.Error)

		// Lookup the Triage again to ensure we persisted what we expect:
		res = dbWithContext.First(&triage1, triage1.ID)
		require.NoError(t, res.Error)
		assert.Equal(t, 1, len(triage1.Regressions))

		// Ensure loading a regression can load the triage records for it:
		var lookupRegression models.TestRegression
		res = dbWithContext.First(&lookupRegression, testRegression.ID).Preload("Triages")
		require.NoError(t, res.Error)
		assert.Equal(t, 1, len(testRegression.Triages))

		openRegressions := make([]*models.TestRegression, 0)

		res = dbWithContext.
			Model(&models.TestRegression{}).
			Preload("Triages").
			Where("test_regressions.release = ?", view.SampleRelease.Name).
			Where("test_regressions.id = ?", testRegression.ID).
			Where("test_regressions.closed IS NULL").
			Find(&openRegressions)
		require.NoError(t, res.Error)
		assert.Equal(t, 1, len(openRegressions))
		assert.Equal(t, 1, len(openRegressions[0].Triages))

		// Make a second Triage for the same regression:
		triage2 := models.Triage{
			URL: "http://myjira2",
			Regressions: []models.TestRegression{
				*testRegression,
			},
		}
		res = dbWithContext.Create(&triage2)
		require.NoError(t, res.Error)
		defer cleanupTriages(dbc, &triage2)
		testRegression.Triages = append(testRegression.Triages, triage2)
		res = dbWithContext.Save(&testRegression)
		require.NoError(t, res.Error)

		// Query for triages for a specific regression:
		res = dbWithContext.First(&testRegression, testRegression.ID).Preload("Triages")
		require.NoError(t, res.Error)
		assert.Equal(t, 2, len(testRegression.Triages))

		// Delete the association:
		triage1.Regressions = []models.TestRegression{}
		res = dbWithContext.Save(&triage1)
		require.NoError(t, res.Error)
		res = dbWithContext.First(&triage1, triage1.ID)
		require.Nil(t, res.Error)
		assert.Equal(t, 0, len(triage1.Regressions))
		// Make sure we didn't wipe out the regression itself:
		res = dbWithContext.First(&lookupRegression, testRegression.ID)
		require.NoError(t, res.Error)
	})

	t.Run("test Triage model Bug relationship", func(t *testing.T) {
		jiraBug := createBug(t, dbWithContext)
		defer dbWithContext.Delete(jiraBug)

		triage1 := models.Triage{
			URL: "http://myjira",
			Bug: jiraBug,
		}
		res := dbWithContext.Create(&triage1)
		require.NoError(t, res.Error)
		defer cleanupTriages(dbc, &triage1)

		// Lookup the Triage again to ensure we persisted what we expect:
		res = dbWithContext.First(&triage1, triage1.ID)
		require.NoError(t, res.Error)
		assert.Equal(t, "MYBUGS-100", triage1.Bug.Key)

	})
}

func createTestRegression(t *testing.T, tracker componentreadiness.RegressionStore, view crview.View, testID string) *models.TestRegression {
	newRegression := componentreport.ReportTestSummary{
		TestComparison: testdetails.TestComparison{
			BaseStats: &testdetails.ReleaseStats{
				Release: util.BaseRelease,
			},
		},
		Identification: crtest.Identification{
			RowIdentification: crtest.RowIdentification{
				Component:  "comp",
				Capability: "cap",
				TestName:   "fake test",
				TestSuite:  "fakesuite",
				TestID:     testID,
			},
			ColumnIdentification: crtest.ColumnIdentification{
				Variants: map[string]string{
					"a": "b",
					"c": "d",
				},
			},
		},
	}
	testRegression, err := tracker.OpenRegression(view, newRegression)
	t.Logf("created testRegression: %+v", testRegression)
	require.NoError(t, err)
	return testRegression
}

// deepCopyTriage creates a deep copy of a Triage struct using JSON marshal/unmarshal
func deepCopyTriage(t *testing.T, original models.Triage) models.Triage {
	data, err := json.Marshal(original)
	require.NoError(t, err, "Failed to marshal triage for deep copy")

	var triageCopy models.Triage
	err = json.Unmarshal(data, &triageCopy)
	require.NoError(t, err, "Failed to unmarshal triage for deep copy")

	return triageCopy
}

// getSeedDataRegressions fetches regressions from the API and filters to only those
// from the seed data (test IDs starting with "test-"), excluding any synthetic regressions
// created by other test functions in this package.
func getSeedDataRegressions(t *testing.T) []models.TestRegression {
	var allRegressions []models.TestRegression
	err := util.SippyGet(fmt.Sprintf("/api/component_readiness/regressions?release=%s", util.Release), &allRegressions)
	require.NoError(t, err)

	var seedRegressions []models.TestRegression
	for _, r := range allRegressions {
		if strings.HasPrefix(r.TestID, "test-") {
			seedRegressions = append(seedRegressions, r)
		}
	}
	sort.Slice(seedRegressions, func(i, j int) bool {
		return seedRegressions[i].TestID < seedRegressions[j].TestID
	})
	return seedRegressions
}

func assertTriageDataMatches(t *testing.T, expectedTriage, actualTriage models.Triage, field string) {
	assert.Equal(t, expectedTriage.ID, actualTriage.ID, "%s ID should match the expected triage ID", field)
	assert.Equal(t, expectedTriage.URL, actualTriage.URL, "%s URL should match the expected triage URL", field)
	assert.Len(t, actualTriage.Regressions, len(expectedTriage.Regressions), "%s regressions count should match", field)

	if len(actualTriage.Regressions) > 0 && len(expectedTriage.Regressions) > 0 {
		assert.Equal(t, expectedTriage.Regressions[0].ID, actualTriage.Regressions[0].ID, "%s regression ID should match", field)
	}
}

// Test_TriagePotentialMatchingRegressions tests the /api/component_readiness/triages/{id}/matches endpoint
func Test_TriagePotentialMatchingRegressions(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)
	// jiraClient is intentionally nil to prevent commenting on jiras
	tracker := componentreadiness.NewPostgresRegressionStore(dbc, nil)

	// Create a common failure time for some regressions to match on
	commonFailureTime := time.Now().Add(-24 * time.Hour)
	differentFailureTime := time.Now().Add(-12 * time.Hour)

	// Create 10 test regressions with various characteristics for matching
	testRegressions := make([]componentreport.ReportTestSummary, 10)

	// Regression 1: Will be linked to the triage
	testRegressions[0] = createTestRegressionWithDetails(t, tracker, view, "linked-test-1", "component-a", "capability-x", "TestSomething", &commonFailureTime, crtest.ExtremeRegression)
	defer dbc.DB.Delete(testRegressions[0].Regression)

	// Regression 2: Will be linked to the triage
	uniqueFailureTime := time.Now().Add(-36 * time.Hour)
	testRegressions[1] = createTestRegressionWithDetails(t, tracker, view, "linked-test-2", "component-b", "capability-y", "TestAnotherOne", &uniqueFailureTime, crtest.SignificantRegression)
	defer dbc.DB.Delete(testRegressions[1].Regression)

	// Regression 3: Should match by similar test name (edit distance <= 5)
	testRegressions[2] = createTestRegressionWithDetails(t, tracker, view, "match-similar-name", "component-c", "capability-z", "TestSomthng", &differentFailureTime, crtest.ExtremeTriagedRegression) // missing 'e' and 'i' from "TestSomething"
	defer dbc.DB.Delete(testRegressions[2].Regression)

	// Regression 4: Should match by same last failure time
	testRegressions[3] = createTestRegressionWithDetails(t, tracker, view, "match-same-failure", "component-d", "capability-w", "TestDifferent", &commonFailureTime, crtest.SignificantTriagedRegression)
	defer dbc.DB.Delete(testRegressions[3].Regression)

	// Regression 5: Should match both similar name AND same failure time
	testRegressions[4] = createTestRegressionWithDetails(t, tracker, view, "match-both", "component-e", "capability-v", "TestAnoterOne", &commonFailureTime, crtest.FixedRegression) // missing 'h' from "TestAnotherOne"
	defer dbc.DB.Delete(testRegressions[4].Regression)

	// Regression 6: Similar name to regression 1 but different failure time
	testRegressions[5] = createTestRegressionWithDetails(t, tracker, view, "match-name-only", "component-f", "capability-u", "TestSomthing", &differentFailureTime, crtest.MissingSample) // missing 'e' from "TestSomething"
	defer dbc.DB.Delete(testRegressions[5].Regression)

	// Regression 7: No match - different name, different failure time
	testRegressions[6] = createTestRegressionWithDetails(t, tracker, view, "no-match-1", "component-g", "capability-t", "CompletelyDifferentTest", &differentFailureTime, crtest.NotSignificant)
	defer dbc.DB.Delete(testRegressions[6].Regression)

	// Regression 8: No match - name too different (edit distance > 5)
	testRegressions[7] = createTestRegressionWithDetails(t, tracker, view, "no-match-2", "component-h", "capability-s", "VeryDifferentTestName", &differentFailureTime, crtest.MissingBasis)
	defer dbc.DB.Delete(testRegressions[7].Regression)

	// Regression 9: Same failure time as linked regression but different name
	testRegressions[8] = createTestRegressionWithDetails(t, tracker, view, "match-failure-time", "component-i", "capability-r", "TestUnrelated", &commonFailureTime, crtest.MissingBasisAndSample)
	defer dbc.DB.Delete(testRegressions[8].Regression)

	// Regression 10: Another potential match with similar name to regression 2
	testRegressions[9] = createTestRegressionWithDetails(t, tracker, view, "match-similar-2", "component-j", "capability-q", "TestAnotheOne", &differentFailureTime, crtest.SignificantImprovement) // missing 'r' from "TestAnotherOne"
	defer dbc.DB.Delete(testRegressions[9].Regression)

	t.Run("find potential matching regressions", func(t *testing.T) {
		// Use real regressions from seed data that appear in the component report.
		// Synthetic regressions with fake test IDs won't appear in the report, so
		// GetTriagePotentialMatches would skip them entirely.
		realRegressions := getSeedDataRegressions(t)
		require.GreaterOrEqual(t, len(realRegressions), 3, "seed data should produce at least 3 regressions")

		// Create a triage linked to the first real regression
		triage := models.Triage{
			URL:  "https://issues.redhat.com/OCPBUGS-1234",
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				{ID: realRegressions[0].ID},
			},
		}

		var triageResponse models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage, &triageResponse)
		require.NoError(t, err)
		defer cleanupTriages(dbc, &triageResponse)
		require.Equal(t, 1, len(triageResponse.Regressions))

		// Query for potential matches — the other real regressions should appear
		var potentialMatches []componentreadiness.PotentialMatchingRegression
		endpoint := fmt.Sprintf("/api/component_readiness/triages/%d/matches?baseRelease=%s&sampleRelease=%s", triageResponse.ID, view.BaseRelease.Release.Name, view.SampleRelease.Release.Name)
		err = util.SippyGet(endpoint, &potentialMatches)
		require.NoError(t, err)

		// Should find some potential matches from the other real regressions
		assert.True(t, len(potentialMatches) > 0, "Should find potential matches from real seed data regressions")

		// Verify HATEOAS links are present on matches
		baseURL := fmt.Sprintf("http://%s:%s", os.Getenv("SIPPY_ENDPOINT"), os.Getenv("SIPPY_API_PORT"))
		for _, match := range potentialMatches {
			assert.Equal(t, fmt.Sprintf("%s/api/component_readiness/triages/%d/matches", baseURL, triageResponse.ID),
				match.Links["self"], "Potential match should have self link")
			assert.Equal(t, fmt.Sprintf("%s/api/component_readiness/triages/%d", baseURL, triageResponse.ID),
				match.Links["triage"], "Potential match should have triage link")
		}

		// Verify that the linked regression is NOT in the potential matches
		foundRegressionIDs := make(map[uint]bool)
		for _, match := range potentialMatches {
			if match.RegressedTest.Regression != nil {
				foundRegressionIDs[match.RegressedTest.Regression.ID] = true
			}
		}
		assert.False(t, foundRegressionIDs[realRegressions[0].ID], "Linked regression should not appear in potential matches")

		// Verify each match has valid regression data and a confidence level
		for _, match := range potentialMatches {
			require.NotNil(t, match.RegressedTest.Regression, "matched regression should not be nil")
			assert.Greater(t, match.ConfidenceLevel, 0, "confidence level should be positive")
			// Each match should have at least one reason (similar name or same failure time)
			assert.True(t,
				len(match.SimilarlyNamedTests) > 0 || len(match.SameLastFailures) > 0,
				"match for regression %d should have at least one match reason", match.RegressedTest.Regression.ID)
		}
	})

	t.Run("empty potential matches when no regressions exist", func(t *testing.T) {
		// Create a triage with one linked regression
		triage := models.Triage{
			URL:  "https://issues.redhat.com/OCPBUGS-1234",
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				{ID: testRegressions[6].Regression.ID}, // CompletelyDifferentTest - won't match anything
			},
		}

		var triageResponse models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage, &triageResponse)
		require.NoError(t, err)
		defer cleanupTriages(dbc, &triageResponse)

		// Query for potential matches
		var potentialMatches []componentreadiness.PotentialMatchingRegression

		endpoint := fmt.Sprintf("/api/component_readiness/triages/%d/matches?baseRelease=%s&sampleRelease=%s", triageResponse.ID, view.BaseRelease.Release.Name, view.SampleRelease.Release.Name)
		err = util.SippyGet(endpoint, &potentialMatches)
		require.NoError(t, err)

		// Should still find some matches since other regressions might have similar names or failure times
		// but the linked regression itself should not appear
		foundRegressionIDs := make(map[uint]bool)
		for _, match := range potentialMatches {
			if match.RegressedTest.Regression != nil {
				foundRegressionIDs[match.RegressedTest.Regression.ID] = true
			}
		}

		assert.False(t, foundRegressionIDs[testRegressions[6].Regression.ID], "Linked regression should not appear in potential matches")
	})

	t.Run("empty potential matches when release pair does not match any view", func(t *testing.T) {
		triage := models.Triage{
			URL:  "https://issues.redhat.com/OCPBUGS-9999",
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				{ID: testRegressions[0].Regression.ID},
			},
		}

		var triageResponse models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage, &triageResponse)
		require.NoError(t, err)
		defer cleanupTriages(dbc, &triageResponse)

		var potentialMatches []componentreadiness.PotentialMatchingRegression
		endpoint := fmt.Sprintf("/api/component_readiness/triages/%d/matches?baseRelease=no-such-base&sampleRelease=no-such-sample", triageResponse.ID)
		err = util.SippyGet(endpoint, &potentialMatches)
		require.NoError(t, err)
		assert.Empty(t, potentialMatches, "Non-matching release pair should return no potential matches")
	})

	t.Run("error when triage not found", func(t *testing.T) {
		var potentialMatches []any

		endpoint := "/api/component_readiness/triages/999999/matches"
		err := util.SippyGet(endpoint, &potentialMatches)
		require.Error(t, err, "Should return error for non-existent triage")
	})

	t.Run("verify status values in triage responses", func(t *testing.T) {

		// Use real regressions that appear in the component report so we can verify
		// status transformation via the expand endpoint
		realRegressions := getSeedDataRegressions(t)
		require.GreaterOrEqual(t, len(realRegressions), 2, "seed data should produce at least 2 regressions")

		triage := models.Triage{
			URL:  "https://issues.redhat.com/OCPBUGS-5678",
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				{ID: realRegressions[0].ID},
				{ID: realRegressions[1].ID},
			},
		}

		var triageResponse models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage, &triageResponse)
		require.NoError(t, err)
		defer cleanupTriages(dbc, &triageResponse)
		require.Equal(t, 2, len(triageResponse.Regressions))

		// Use the expand endpoint to get status values from the component report
		var expandedTriage sippyserver.ExpandedTriage
		err = util.SippyGet(fmt.Sprintf("/api/component_readiness/triages/%d?view=%s-main&expand=regressions", triageResponse.ID, util.Release), &expandedTriage)
		require.NoError(t, err)

		regressedTests := expandedTriage.RegressedTests[view.Name]
		require.NotEmpty(t, regressedTests, "expanded triage should contain regressed tests")

		// Verify each regressed test has a triaged status (the triage transforms the status)
		for _, rt := range regressedTests {
			require.NotNil(t, rt, "regressed test should not be nil")
			require.NotNil(t, rt.Regression, "regressed test should have regression data")
			status := rt.TestComparison.ReportStatus
			assert.True(t,
				status == crtest.ExtremeTriagedRegression || status == crtest.SignificantTriagedRegression,
				"regression %s status should be triaged, got %d", rt.Regression.TestID, status)
		}
	})
}

// Helper function to create test regressions with specific details
func createTestRegressionWithDetails(t *testing.T, tracker componentreadiness.RegressionStore, view crview.View, testID, component, capability, testName string, lastFailure *time.Time, status crtest.Status) componentreport.ReportTestSummary {
	newRegression := componentreport.ReportTestSummary{
		TestComparison: testdetails.TestComparison{
			ReportStatus: status,
			BaseStats: &testdetails.ReleaseStats{
				Release: util.BaseRelease,
			},
			LastFailure: lastFailure,
		},
		Identification: crtest.Identification{
			RowIdentification: crtest.RowIdentification{
				Component:  component,
				Capability: capability,
				TestName:   testName,
				TestSuite:  "fakesuite",
				TestID:     testID,
			},
			ColumnIdentification: crtest.ColumnIdentification{
				Variants: map[string]string{
					"a": "b",
					"c": "d",
				},
			},
		},
	}
	regression, err := tracker.OpenRegression(view, newRegression)
	require.NoError(t, err)
	newRegression.Regression = regression
	return newRegression
}

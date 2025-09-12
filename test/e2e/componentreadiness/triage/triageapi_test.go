package triage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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
	SampleRelease: reqopts.RelativeRelease{
		Release: reqopts.Release{
			Name: util.Release,
		},
	},
}

func cleanupAllTriages(dbc *db.DB) {
	// Delete all triage and test regressions in the e2e postgres db.
	dbc.DB.Exec("DELETE FROM triage_regressions WHERE 1=1")
	res := dbc.DB.Where("1 = 1").Delete(&models.Triage{})
	if res.Error != nil {
		log.Errorf("error deleting triage records: %v", res.Error)
	}
}

func Test_TriageAPI(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)
	tracker := componentreadiness.NewPostgresRegressionStore(dbc)

	jiraBug := createBug(t, dbc.DB)
	defer dbc.DB.Delete(jiraBug)

	testRegression1 := createTestRegression(t, tracker, view, "faketestid")
	defer dbc.DB.Delete(testRegression1)
	testRegression2 := createTestRegression(t, tracker, view, "faketestid2")
	defer dbc.DB.Delete(testRegression2)

	t.Run("create requires a valid triage type", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
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

	t.Run("create generates audit_log record", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
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
		defer cleanupAllTriages(dbc)
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)

		// ensure hateoas links are present
		assert.Equal(t, fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse.ID),
			triageResponse.Links["self"])
		assert.Equal(t, fmt.Sprintf("/api/component_readiness/triages/%d/matches", triageResponse.ID),
			triageResponse.Links["potential_matches"])
		assert.Equal(t, fmt.Sprintf("/api/component_readiness/triages/%d/audit", triageResponse.ID),
			triageResponse.Links["audit_logs"])
	})
	t.Run("get with expanded regressions", func(t *testing.T) {
		defer cleanupAllTriages(dbc)

		r := createTestRegressionWithDetails(t, tracker, view, "expanded-test-1", "component-expand", "capability-expand", "TestExpanded1", nil, crtest.ExtremeRegression)
		defer dbc.DB.Delete(r.Regression)

		r2 := createTestRegressionWithDetails(t, tracker, view, "expanded-test-2", "component-expand", "capability-expand", "TestExpanded2", nil, crtest.SignificantRegression)
		defer dbc.DB.Delete(r2.Regression)

		// Create a triage with the test regressions
		triage := models.Triage{
			URL:  jiraBug.URL,
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				{ID: r.Regression.ID},
				{ID: r2.Regression.ID},
			},
		}

		var triageResponse models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage, &triageResponse)
		require.NoError(t, err)
		require.Equal(t, 2, len(triageResponse.Regressions))

		// Add the test regressions to the component report cache so they can be found by the expanded endpoint
		cache, err := util.NewE2ECacheManipulator(util.Release)
		if err != nil {
			t.Fatalf("Failed to create component report cache: %v", err)
		}
		defer cache.Close()

		err = cache.AddTestRegressionsToReport([]componentreport.ReportTestSummary{r, r2})
		if err != nil {
			t.Fatalf("Failed to add test regressions to component report: %v", err)
		}

		// Validate that the expanded regressions are present
		var expandedTriage sippyserver.ExpandedTriage
		err = util.SippyGet(fmt.Sprintf("/api/component_readiness/triages/%d?view=%s-main&expand=regressions", triageResponse.ID, util.Release), &expandedTriage)
		require.NoError(t, err)

		// Verify the expanded triage contains the regressed tests with correct status values
		require.NotNil(t, expandedTriage.Triage, "ExpandedTriage should contain a Triage")
		assert.Equal(t, triageResponse.ID, expandedTriage.Triage.ID, "ExpandedTriage should have the same ID as the created triage")
		assert.Len(t, expandedTriage.RegressedTests, 2, "ExpandedTriage should contain 2 regressed tests")

		// Verify status values are marked as their respective triaged values in the expanded response
		statusMap := make(map[uint]crtest.Status)
		for _, regressedTest := range expandedTriage.RegressedTests {
			if regressedTest != nil && regressedTest.Regression != nil {
				statusMap[regressedTest.Regression.ID] = regressedTest.TestComparison.ReportStatus
			}
		}

		assert.Equal(t, crtest.ExtremeTriagedRegression, statusMap[r.Regression.ID], "First regressed test should have ExtremeTriagedRegression status")
		assert.Equal(t, crtest.SignificantTriagedRegression, statusMap[r2.Regression.ID], "Second regressed test should have SignificantTriagedRegression status")
	})
	t.Run("list", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)

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
			assert.Equal(t, fmt.Sprintf("/api/component_readiness/triages/%d", triage.ID),
				triage.Links["self"])
			assert.Equal(t, fmt.Sprintf("/api/component_readiness/triages/%d/matches", triage.ID),
				triage.Links["potential_matches"])
			assert.Equal(t, fmt.Sprintf("/api/component_readiness/triages/%d/audit", triage.ID),
				triage.Links["audit_logs"])
		}
	})
	t.Run("update to add regression", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)

		// Update with a new regression:
		var triageResponse2 models.Triage
		triageResponse.Regressions = append(triageResponse.Regressions, models.TestRegression{ID: testRegression2.ID})
		err := util.SippyPut(fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse.ID), &triageResponse, &triageResponse2)
		require.NoError(t, err)
		assert.Equal(t, 2, len(triageResponse2.Regressions))
		assert.Equal(t, triageResponse.CreatedAt, triageResponse2.CreatedAt)
		assert.NotEqual(t, triageResponse.UpdatedAt, triageResponse2.UpdatedAt)

		// ensure hateoas links are present
		assert.Equal(t, fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse2.ID),
			triageResponse2.Links["self"])
		assert.Equal(t, fmt.Sprintf("/api/component_readiness/triages/%d/matches", triageResponse2.ID),
			triageResponse2.Links["potential_matches"])
		assert.Equal(t, fmt.Sprintf("/api/component_readiness/triages/%d/audit", triageResponse2.ID),
			triageResponse2.Links["audit_logs"])
	})
	t.Run("update to remove a regression", func(t *testing.T) {
		defer cleanupAllTriages(dbc)

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
		defer cleanupAllTriages(dbc)
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)

		var triageResponse2 models.Triage
		triageResponse.Regressions = []models.TestRegression{}
		err := util.SippyPut(fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse.ID), &triageResponse, &triageResponse2)
		require.NoError(t, err)
		assert.Equal(t, 0, len(triageResponse2.Regressions))
	})
	t.Run("update to resolve triage sets resolution reason to user", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)

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
		defer cleanupAllTriages(dbc)
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)

		var triageResponse2 models.Triage
		triageResponse.ID = 0
		err := util.SippyPut(fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse.ID), &triageResponse, &triageResponse2)
		require.Error(t, err)
	})
	t.Run("update fails if URL has no ID", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)

		var triageResponse2 models.Triage
		// No ID specified in URL should not work for an update
		err := util.SippyPut("/api/component_readiness/triages", &triageResponse, &triageResponse2)
		require.Error(t, err)
	})
	t.Run("update fails if URL ID and resource ID do not match", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)

		var triageResponse2 models.Triage
		err := util.SippyPut("/api/component_readiness/triages/128736182736128736", &triageResponse, &triageResponse2)
		require.Error(t, err)
	})
	t.Run("update generates audit_log record", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)
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
		defer cleanupAllTriages(dbc)
		triageResponse := createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)
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
		defer cleanupAllTriages(dbc)

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
		for _, auditLog := range auditLogs {
			assert.Equal(t, fmt.Sprintf("/api/component_readiness/triages/%d/audit", triageResponse.ID),
				auditLog.Links["self"], "Audit log should have self link")
			assert.Equal(t, fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse.ID),
				auditLog.Links["triage"], "Audit log should have triage link")
		}
	})
}

// Test_RegressionPotentialMatchingTriages tests the /api/component_readiness/regressions/{id}/matches endpoint
func Test_RegressionPotentialMatchingTriages(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)
	tracker := componentreadiness.NewPostgresRegressionStore(dbc)

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
		defer cleanupAllTriages(dbc)

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
		defer cleanupAllTriages(dbc)

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

		// Query for potential matches for the target regression
		var potentialMatches []componentreadiness.PotentialMatchingTriage
		endpoint := fmt.Sprintf("/api/component_readiness/regressions/%d/matches", targetRegression.Regression.ID)
		err = util.SippyGet(endpoint, &potentialMatches)
		require.NoError(t, err)

		// Should find no matches since the test name and failure time are too different
		assert.Len(t, potentialMatches, 0, "Should find no potential matching triages")
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
	tracker := componentreadiness.NewPostgresRegressionStore(dbc)

	testRegression := createTestRegression(t, tracker, view, "faketestid")
	defer dbc.DB.Delete(testRegression)

	t.Run("test Triage model in postgres", func(t *testing.T) {
		defer cleanupAllTriages(dbc)

		triage1 := models.Triage{
			URL: "http://myjira",
			Regressions: []models.TestRegression{
				*testRegression,
			},
		}
		res := dbWithContext.Create(&triage1)
		require.NoError(t, res.Error)
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
		defer cleanupAllTriages(dbc)

		jiraBug := createBug(t, dbWithContext)
		defer dbWithContext.Delete(jiraBug)

		triage1 := models.Triage{
			URL: "http://myjira",
			Bug: jiraBug,
		}
		res := dbWithContext.Create(&triage1)
		require.NoError(t, res.Error)

		// Lookup the Triage again to ensure we persisted what we expect:
		res = dbWithContext.First(&triage1, triage1.ID)
		require.NoError(t, res.Error)
		assert.Equal(t, "MYBUGS-100", triage1.Bug.Key)

	})
}

func createTestRegression(t *testing.T, tracker componentreadiness.RegressionStore, view crview.View, testID string) *models.TestRegression {
	newRegression := componentreport.ReportTestSummary{
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
	tracker := componentreadiness.NewPostgresRegressionStore(dbc)

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

	// Add all test regressions to the component report so they can be found by GetTriagePotentialMatches
	cache, err := util.NewE2ECacheManipulator(util.Release)
	if err != nil {
		t.Fatalf("Failed to create component report cache: %v", err)
	}
	defer cache.Close()

	err = cache.AddTestRegressionsToReport(testRegressions)
	if err != nil {
		t.Fatalf("Failed to add test regressions to component report: %v", err)
	}

	t.Run("find potential matching regressions", func(t *testing.T) {
		defer cleanupAllTriages(dbc)

		// Create a triage with two linked regressions
		triage := models.Triage{
			URL:  "https://issues.redhat.com/OCPBUGS-1234",
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				{ID: testRegressions[0].Regression.ID}, // TestSomething with commonFailureTime
				{ID: testRegressions[1].Regression.ID}, // TestAnother with unique failure time
			},
		}

		var triageResponse models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage, &triageResponse)
		require.NoError(t, err)
		require.Equal(t, 2, len(triageResponse.Regressions))

		// Query for potential matches
		var potentialMatches []componentreadiness.PotentialMatchingRegression

		endpoint := fmt.Sprintf("/api/component_readiness/triages/%d/matches?view=%s", triageResponse.ID, view.Name)
		err = util.SippyGet(endpoint, &potentialMatches)
		require.NoError(t, err)

		// Verify the results
		assert.True(t, len(potentialMatches) > 0, "Should find some potential matches")

		// Verify HATEOAS links are present in potential match responses
		for _, match := range potentialMatches {
			assert.Equal(t, fmt.Sprintf("/api/component_readiness/triages/%d/matches", triageResponse.ID),
				match.Links["self"], "Potential match should have self link")
			assert.Equal(t, fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse.ID),
				match.Links["triage"], "Potential match should have triage link")
		}

		// Verify status values are correctly returned for the potential matches
		statusMap := make(map[uint]crtest.Status)
		for _, match := range potentialMatches {
			if match.RegressedTest.Regression != nil {
				statusMap[match.RegressedTest.Regression.ID] = match.RegressedTest.TestComparison.ReportStatus
			}
		}

		// Build maps for easier verification
		foundRegressionIDs := make(map[uint]bool)
		matchesBySimilarName := make(map[uint][]componentreadiness.SimilarlyNamedTest)
		matchesBySameFailure := make(map[uint][]models.TestRegression)
		confidenceLevels := make(map[uint]int)

		for _, match := range potentialMatches {
			if match.RegressedTest.Regression == nil {
				continue // Skip if no regression data
			}
			regressionID := match.RegressedTest.Regression.ID
			foundRegressionIDs[regressionID] = true
			confidenceLevels[regressionID] = match.ConfidenceLevel
			if len(match.SimilarlyNamedTests) > 0 {
				matchesBySimilarName[regressionID] = match.SimilarlyNamedTests
			}
			if len(match.SameLastFailures) > 0 {
				matchesBySameFailure[regressionID] = match.SameLastFailures
			}
		}

		// Verify that linked regressions are NOT in the potential matches
		assert.False(t, foundRegressionIDs[testRegressions[0].Regression.ID], "Linked regression 0 should not appear in potential matches")
		assert.False(t, foundRegressionIDs[testRegressions[1].Regression.ID], "Linked regression 1 should not appear in potential matches")

		// Verify expected matches are found
		assert.True(t, foundRegressionIDs[testRegressions[2].Regression.ID], "Should find regression 2 (similar name to TestSomething)")
		assert.True(t, foundRegressionIDs[testRegressions[3].Regression.ID], "Should find regression 3 (same failure time)")
		assert.True(t, foundRegressionIDs[testRegressions[4].Regression.ID], "Should find regression 4 (both similar name and same failure)")
		assert.True(t, foundRegressionIDs[testRegressions[5].Regression.ID], "Should find regression 5 (similar name)")
		assert.True(t, foundRegressionIDs[testRegressions[8].Regression.ID], "Should find regression 8 (same failure time)")
		assert.True(t, foundRegressionIDs[testRegressions[9].Regression.ID], "Should find regression 9 (similar name to TestAnother)")

		// Verify the status values are correctly returned
		assert.Equal(t, crtest.ExtremeTriagedRegression, statusMap[testRegressions[2].Regression.ID], "Regression 2 should have ExtremeTriagedRegression status")
		assert.Equal(t, crtest.SignificantTriagedRegression, statusMap[testRegressions[3].Regression.ID], "Regression 3 should have SignificantTriagedRegression status")
		assert.Equal(t, crtest.FixedRegression, statusMap[testRegressions[4].Regression.ID], "Regression 4 should have FixedRegression status")
		assert.Equal(t, crtest.MissingSample, statusMap[testRegressions[5].Regression.ID], "Regression 5 should have MissingSample status")
		assert.Equal(t, crtest.MissingBasisAndSample, statusMap[testRegressions[8].Regression.ID], "Regression 8 should have MissingBasisAndSample status")
		assert.Equal(t, crtest.SignificantImprovement, statusMap[testRegressions[9].Regression.ID], "Regression 9 should have SignificantImprovement status")

		// Verify non-matches are not found
		assert.False(t, foundRegressionIDs[testRegressions[6].Regression.ID], "Should not find regression 6 (no match)")
		assert.False(t, foundRegressionIDs[testRegressions[7].Regression.ID], "Should not find regression 7 (name too different)")

		// Verify match reasons are correct

		// Regression 2: Should match by similar name to "TestSomething"
		if assert.Contains(t, matchesBySimilarName, testRegressions[2].Regression.ID) {
			matches := matchesBySimilarName[testRegressions[2].Regression.ID]
			assert.Equal(t, 1, len(matches), "Should match exactly one similar name")
			assert.Equal(t, testRegressions[0].Regression.ID, matches[0].Regression.ID, "Should match against TestSomething regression")
			// TestSomthng vs TestSomething = edit distance 2, so score = 6-2 = 4
			assert.Equal(t, 4, confidenceLevels[testRegressions[2].Regression.ID], "Confidence should be 4 (edit distance 2: 6-2)")
		}

		// Regression 3: Should match by same failure time
		if assert.Contains(t, matchesBySameFailure, testRegressions[3].Regression.ID) {
			matches := matchesBySameFailure[testRegressions[3].Regression.ID]
			assert.Equal(t, 1, len(matches), "Should match exactly one same failure time")
			assert.Equal(t, testRegressions[0].Regression.ID, matches[0].ID, "Should match against commonFailureTime regression")
			assert.Equal(t, 1, confidenceLevels[testRegressions[3].Regression.ID], "Confidence should be 1 (1 failure match * 1)")
		}

		// Regression 4: Should match both similar name AND same failure time
		if assert.Contains(t, matchesBySimilarName, testRegressions[4].Regression.ID) {
			nameMatches := matchesBySimilarName[testRegressions[4].Regression.ID]
			assert.Equal(t, 1, len(nameMatches), "Should match exactly one similar name")
			assert.Equal(t, testRegressions[1].Regression.ID, nameMatches[0].Regression.ID, "Should match against TestAnotherOne regression")
		}
		if assert.Contains(t, matchesBySameFailure, testRegressions[4].Regression.ID) {
			failureMatches := matchesBySameFailure[testRegressions[4].Regression.ID]
			assert.Equal(t, 1, len(failureMatches), "Should match exactly one same failure time")
			assert.Equal(t, testRegressions[0].Regression.ID, failureMatches[0].ID, "Should match against commonFailureTime regression")
			// TestAnoterOne vs TestAnotherOne = edit distance 1, so name score = 6-1 = 5, failure = 1, total = 6
			assert.Equal(t, 6, confidenceLevels[testRegressions[4].Regression.ID], "Confidence should be 6 (name edit distance 1: 6-1=5, plus 1 failure match)")
		}

		// Regression 5: Should match by similar name only
		if assert.Contains(t, matchesBySimilarName, testRegressions[5].Regression.ID) {
			matches := matchesBySimilarName[testRegressions[5].Regression.ID]
			assert.Equal(t, 1, len(matches), "Should match exactly one similar name")
			assert.Equal(t, testRegressions[0].Regression.ID, matches[0].Regression.ID, "Should match against TestSomething regression")
			// TestSomthing vs TestSomething = edit distance 1, so score = 6-1 = 5
			assert.Equal(t, 5, confidenceLevels[testRegressions[5].Regression.ID], "Confidence should be 5 (edit distance 1: 6-1)")
		}
		assert.NotContains(t, matchesBySameFailure, testRegressions[5].Regression.ID, "Should not match by failure time")

		// Regression 8: Should match by same failure time only
		if assert.Contains(t, matchesBySameFailure, testRegressions[8].Regression.ID) {
			matches := matchesBySameFailure[testRegressions[8].Regression.ID]
			assert.Equal(t, 1, len(matches), "Should match exactly one same failure time")
			assert.Equal(t, testRegressions[0].Regression.ID, matches[0].ID, "Should match against commonFailureTime regression")
			assert.Equal(t, 1, confidenceLevels[testRegressions[8].Regression.ID], "Confidence should be 1 (1 failure match * 1)")
		}
		assert.NotContains(t, matchesBySimilarName, testRegressions[8].Regression.ID, "Should not match by similar name")

		// Regression 9: Should match by similar name to "TestAnotherOne"
		if assert.Contains(t, matchesBySimilarName, testRegressions[9].Regression.ID) {
			matches := matchesBySimilarName[testRegressions[9].Regression.ID]
			assert.Equal(t, 1, len(matches), "Should match exactly one similar name")
			assert.Equal(t, testRegressions[1].Regression.ID, matches[0].Regression.ID, "Should match against TestAnotherOne regression")
			// TestAnotheOne vs TestAnotherOne = edit distance 1, so score = 6-1 = 5
			assert.Equal(t, 5, confidenceLevels[testRegressions[9].Regression.ID], "Confidence should be 5 (edit distance 1: 6-1)")
		}
	})

	t.Run("empty potential matches when no regressions exist", func(t *testing.T) {
		defer cleanupAllTriages(dbc)

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

		// Query for potential matches
		var potentialMatches []componentreadiness.PotentialMatchingRegression

		endpoint := fmt.Sprintf("/api/component_readiness/triages/%d/matches?view=%s", triageResponse.ID, view.Name)
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

	t.Run("error when triage not found", func(t *testing.T) {
		var potentialMatches []interface{}

		endpoint := "/api/component_readiness/triages/999999/matches"
		err := util.SippyGet(endpoint, &potentialMatches)
		require.Error(t, err, "Should return error for non-existent triage")
	})

	t.Run("verify status values in triage responses", func(t *testing.T) {
		defer cleanupAllTriages(dbc)

		// Create a triage with regressions that have different status values
		triage := models.Triage{
			URL:  "https://issues.redhat.com/OCPBUGS-5678",
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				{ID: testRegressions[0].Regression.ID}, // ExtremeRegression
				{ID: testRegressions[1].Regression.ID}, // SignificantRegression
				{ID: testRegressions[4].Regression.ID}, // FixedRegression
			},
		}

		var triageResponse models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage, &triageResponse)
		require.NoError(t, err)
		require.Equal(t, 3, len(triageResponse.Regressions))

		// Note: TestComparison (including status) is not available on the basic TestRegression model
		// returned by the triage API. Status is only available in the potential matches endpoint
		// where regressions are represented as ReportTestSummary with full component report data.
		//
		// However, we can verify that our test setup correctly created regressions with different IDs
		regressionIDs := make(map[uint]bool)
		for _, regression := range triageResponse.Regressions {
			regressionIDs[regression.ID] = true
		}

		assert.True(t, regressionIDs[testRegressions[0].Regression.ID], "Should find first regression")
		assert.True(t, regressionIDs[testRegressions[1].Regression.ID], "Should find second regression")
		assert.True(t, regressionIDs[testRegressions[4].Regression.ID], "Should find third regression")
	})
}

// Helper function to create test regressions with specific details
func createTestRegressionWithDetails(t *testing.T, tracker componentreadiness.RegressionStore, view crview.View, testID, component, capability, testName string, lastFailure *time.Time, status crtest.Status) componentreport.ReportTestSummary {
	newRegression := componentreport.ReportTestSummary{
		TestComparison: testdetails.TestComparison{
			ReportStatus: status,
			BaseStats: &testdetails.ReleaseStats{
				Release: util.Release,
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

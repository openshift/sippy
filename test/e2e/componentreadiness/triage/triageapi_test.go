package triage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
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
	// jiraClient is intentionally nil to prevent commenting on jiras
	tracker := componentreadiness.NewPostgresRegressionStore(dbc, nil)

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
		defer cleanupAllTriages(dbc)

		r := createTestRegressionWithDetails(t, tracker, view, "expanded-test-1", "component-expand", "capability-expand", "TestExpanded1", crtest.ExtremeRegression)
		defer dbc.DB.Delete(r.Regression)
		require.NoError(t, tracker.UpsertRegressionView(r.Regression.ID, view.Name))

		r2 := createTestRegressionWithDetails(t, tracker, view, "expanded-test-2", "component-expand", "capability-expand", "TestExpanded2", crtest.SignificantRegression)
		defer dbc.DB.Delete(r2.Regression)
		require.NoError(t, tracker.UpsertRegressionView(r2.Regression.ID, view.Name))

		// TODO(sgoeddel): If we ever have a need for another view available within e2e tests we could verify that we could get regressed_tests
		// for multiple views at once here, but it isn't worth the overhead now.

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
		assert.Equal(t, triageResponse.ID, expandedTriage.ID, "ExpandedTriage should have the same ID as the created triage")
		expectedViewKey := view.Name
		require.Contains(t, expandedTriage.RegressedTests, expectedViewKey, "ExpandedTriage should contain regressed tests for view %q", expectedViewKey)
		regressedTestsForView := expandedTriage.RegressedTests[expectedViewKey]
		assert.Len(t, regressedTestsForView, 2, "ExpandedTriage should contain 2 regressed tests for view %q", expectedViewKey)

		// Verify status values are marked as their respective triaged values in the expanded response
		statusMap := make(map[uint]crtest.Status)
		for _, regressedTest := range regressedTestsForView {
			if regressedTest != nil && regressedTest.Regression != nil {
				statusMap[regressedTest.Regression.ID] = regressedTest.ReportStatus
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

	t.Run("expanded triage includes symptom summaries", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		defer util.CleanupTriageSymptoms(dbc)

		symA := util.SeedSymptom(t, dbc, "e2e-sym-a", "E2E Symptom A")
		defer dbc.DB.Delete(symA)
		symB := util.SeedSymptom(t, dbc, "e2e-sym-b", "E2E Symptom B")
		defer dbc.DB.Delete(symB)

		reg := createTestRegression(t, tracker, view, "sym-expand-test-1")
		defer dbc.DB.Delete(reg)

		triage := models.Triage{
			URL:  jiraBug.URL,
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				{ID: reg.ID},
			},
		}
		var triageResp models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage, &triageResp)
		require.NoError(t, err)

		err = tracker.MergeJobRuns(reg.ID, []models.RegressionJobRun{
			{ProwJobRunID: "sym-run-1", ProwJobName: "job-1", TestFailed: true, JobSymptoms: pq.StringArray{"e2e-sym-a", "e2e-sym-b"}},
			{ProwJobRunID: "sym-run-2", ProwJobName: "job-1", TestFailed: true, JobSymptoms: pq.StringArray{"e2e-sym-a"}},
		})
		require.NoError(t, err)

		err = tracker.SyncTriageSymptoms([]*models.TestRegression{{ID: reg.ID}})
		require.NoError(t, err)

		var et sippyserver.ExpandedTriage
		err = util.SippyGet(fmt.Sprintf("/api/component_readiness/triages/%d?expand=regressions,symptoms", triageResp.ID), &et)
		require.NoError(t, err)
		require.NotNil(t, et.SymptomSummaries)
		require.Len(t, et.SymptomSummaries, 2, "should have 2 symptom summaries")

		symMap := make(map[string]componentreadiness.TriageSymptomSummary)
		for _, ss := range et.SymptomSummaries {
			symMap[ss.Symptom.ID] = ss
		}
		require.Contains(t, symMap, "e2e-sym-a")
		assert.Equal(t, 1, symMap["e2e-sym-a"].RegressionCount)
		assert.Equal(t, 2, symMap["e2e-sym-a"].JobRunCount)
		assert.Contains(t, symMap["e2e-sym-a"].RegressionIDs, reg.ID)

		require.Contains(t, symMap, "e2e-sym-b")
		assert.Equal(t, 1, symMap["e2e-sym-b"].RegressionCount)
		assert.Equal(t, 1, symMap["e2e-sym-b"].JobRunCount)
		assert.Contains(t, symMap["e2e-sym-b"].RegressionIDs, reg.ID)
	})

	t.Run("expand=symptoms only returns symptoms without regressed_tests", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		defer util.CleanupTriageSymptoms(dbc)

		sym := util.SeedSymptom(t, dbc, "e2e-sym-only", "E2E Symptom Only")
		defer dbc.DB.Delete(sym)

		reg := createTestRegression(t, tracker, view, "sym-only-test-1")
		defer dbc.DB.Delete(reg)

		triage := models.Triage{
			URL:  jiraBug.URL,
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				{ID: reg.ID},
			},
		}
		var triageResp models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage, &triageResp)
		require.NoError(t, err)

		err = tracker.MergeJobRuns(reg.ID, []models.RegressionJobRun{
			{ProwJobRunID: "sym-only-run-1", ProwJobName: "job-1", TestFailed: true, JobSymptoms: pq.StringArray{"e2e-sym-only"}},
		})
		require.NoError(t, err)

		err = tracker.SyncTriageSymptoms([]*models.TestRegression{{ID: reg.ID}})
		require.NoError(t, err)

		var et sippyserver.ExpandedTriage
		err = util.SippyGet(fmt.Sprintf("/api/component_readiness/triages/%d?expand=symptoms", triageResp.ID), &et)
		require.NoError(t, err)
		require.NotNil(t, et.SymptomSummaries)
		assert.Len(t, et.SymptomSummaries, 1)
		assert.Nil(t, et.RegressedTests, "regressed_tests should be nil when only symptoms is expanded")
	})

	t.Run("delete triage cascades to triage_symptoms", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		defer util.CleanupTriageSymptoms(dbc)

		sym := util.SeedSymptom(t, dbc, "e2e-sym-cascade", "E2E Symptom Cascade")
		defer dbc.DB.Delete(sym)

		reg := createTestRegression(t, tracker, view, "sym-cascade-test-1")
		defer dbc.DB.Delete(reg)

		triage := models.Triage{
			URL:  jiraBug.URL,
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				{ID: reg.ID},
			},
		}
		var triageResp models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage, &triageResp)
		require.NoError(t, err)

		err = tracker.MergeJobRuns(reg.ID, []models.RegressionJobRun{
			{ProwJobRunID: "cascade-run-1", ProwJobName: "job-1", TestFailed: true, JobSymptoms: pq.StringArray{"e2e-sym-cascade"}},
		})
		require.NoError(t, err)

		err = tracker.SyncTriageSymptoms([]*models.TestRegression{{ID: reg.ID}})
		require.NoError(t, err)

		// Verify junction row exists
		var count int64
		dbc.DB.Model(&models.TriageSymptom{}).Where("triage_id = ?", triageResp.ID).Count(&count)
		require.Equal(t, int64(1), count, "should have 1 junction row before delete")

		// Delete via API
		err = util.SippyDelete(fmt.Sprintf("/api/component_readiness/triages/%d", triageResp.ID))
		require.NoError(t, err)

		// Verify junction rows are gone
		dbc.DB.Model(&models.TriageSymptom{}).Where("triage_id = ?", triageResp.ID).Count(&count)
		assert.Equal(t, int64(0), count, "triage_symptoms should be cascade deleted with triage")
	})

	t.Run("expanded triage with no symptoms returns nil symptom summaries", func(t *testing.T) {
		defer cleanupAllTriages(dbc)

		reg := createTestRegression(t, tracker, view, "no-sym-expand-test")
		defer dbc.DB.Delete(reg)

		triage := models.Triage{
			URL:  jiraBug.URL,
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				{ID: reg.ID},
			},
		}
		var triageResp models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage, &triageResp)
		require.NoError(t, err)

		// Merge job runs without any symptoms
		err = tracker.MergeJobRuns(reg.ID, []models.RegressionJobRun{
			{ProwJobRunID: "no-sym-run-1", ProwJobName: "job-1", TestFailed: true},
		})
		require.NoError(t, err)

		err = tracker.SyncTriageSymptoms([]*models.TestRegression{{ID: reg.ID}})
		require.NoError(t, err)

		var et sippyserver.ExpandedTriage
		err = util.SippyGet(fmt.Sprintf("/api/component_readiness/triages/%d?expand=symptoms", triageResp.ID), &et)
		require.NoError(t, err)
		assert.Nil(t, et.SymptomSummaries, "symptom summaries should be nil when no symptoms exist")
	})

	t.Run("audit endpoint returns full lifecycle operations", func(t *testing.T) {
		defer cleanupAllTriages(dbc)

		// Create a triage
		triage := models.Triage{
			URL:         "https://redhat.atlassian.net/browse/OCPBUGS-8888",
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
		assert.Equal(t, "https://redhat.atlassian.net/browse/OCPBUGS-8888", deleteChangesByField["url"].Original)
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
		assert.Equal(t, "https://redhat.atlassian.net/browse/OCPBUGS-8888", createChangesByField["url"].Modified)

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
	// Associate regression with view so HATEOAS links are generated
	require.NoError(t, tracker.UpsertRegressionView(testRegression1.ID, view.Name))

	testRegression2 := createTestRegression(t, tracker, view, "faketestid2")
	defer dbc.DB.Delete(testRegression2)

	jiraBug := createBug(t, dbc.DB)
	defer dbc.DB.Delete(jiraBug)

	release := view.SampleRelease.Name

	t.Run("list regressions", func(t *testing.T) {
		defer cleanupAllTriages(dbc)
		_ = createAndValidateTriageRecord(t, jiraBug.URL, testRegression1)

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

		// Verify HATEOAS links are present - test_details links now use composite keys test_details:<view_name>
		assert.NotNil(t, foundRegression.Links, "regression should have HATEOAS links")
		assert.Contains(t, foundRegression.Links, "self", "regression should have self link")

		// Find any test_details link (format is test_details:<view_name>)
		var testDetailsHREF string
		for key, href := range foundRegression.Links {
			if len(key) > len("test_details:") && key[:len("test_details:")] == "test_details:" {
				testDetailsHREF = href
				break
			}
		}
		require.NotEmpty(t, testDetailsHREF, "regression should have at least one test_details:<view> link")
		assert.Contains(t, testDetailsHREF, fmt.Sprintf("http://%s:%s/api/component_readiness/test_details", os.Getenv("SIPPY_ENDPOINT"), os.Getenv("SIPPY_API_PORT")), "test_details link should point to correct endpoint")
		assert.Contains(t, testDetailsHREF, "testId=", "test_details link should contain testId parameter")
	})
	t.Run("error when both view and release are specified", func(t *testing.T) {
		defer cleanupAllTriages(dbc)

		var regressions []models.TestRegression
		err := util.SippyGet(fmt.Sprintf("/api/component_readiness/regressions?view=%s-main&release=%s", util.Release, util.Release), &regressions)
		require.Error(t, err, "Expected error when both view and release are provided")
	})
	t.Run("filter regressions by test name", func(t *testing.T) {
		defer cleanupAllTriages(dbc)

		// Create a regression with a unique test name
		uniqueTestName := "TestUniqueFilterByName"
		uniqueRegression := createTestRegressionWithDetails(t, tracker, view, "filter-test-id", "comp-filter", "cap-filter", uniqueTestName, crtest.ExtremeRegression)
		defer dbc.DB.Delete(uniqueRegression.Regression)

		// Create additional regressions with different test names to verify they are excluded
		otherRegression1 := createTestRegressionWithDetails(t, tracker, view, "filter-other-id-1", "comp-other-1", "cap-other-1", "TestOtherName1", crtest.SignificantRegression)
		defer dbc.DB.Delete(otherRegression1.Regression)
		otherRegression2 := createTestRegressionWithDetails(t, tracker, view, "filter-other-id-2", "comp-other-2", "cap-other-2", "TestOtherName2", crtest.ExtremeRegression)
		defer dbc.DB.Delete(otherRegression2.Regression)

		// Verify filtering by the unique test name returns only that regression
		var filtered []models.TestRegression
		err := util.SippyGet(fmt.Sprintf("/api/component_readiness/regressions?release=%s&test=%s", release, uniqueTestName), &filtered)
		require.NoError(t, err)
		require.Len(t, filtered, 1, "expected exactly one regression matching the test name")
		assert.Equal(t, uniqueRegression.Regression.ID, filtered[0].ID)
		assert.Equal(t, uniqueTestName, filtered[0].TestName)

		// Verify non-matching regressions are not included
		for _, r := range filtered {
			assert.NotEqual(t, otherRegression1.Regression.ID, r.ID, "otherRegression1 should not be in filtered results")
			assert.NotEqual(t, otherRegression2.Regression.ID, r.ID, "otherRegression2 should not be in filtered results")
		}

		// Verify filtering by a non-existent test name returns empty
		var empty []models.TestRegression
		err = util.SippyGet(fmt.Sprintf("/api/component_readiness/regressions?release=%s&test=%s", release, "NonExistentTestName"), &empty)
		require.NoError(t, err)
		assert.Empty(t, empty, "expected no regressions for a non-existent test name")
	})
	t.Run("filter regressions by test name without release", func(t *testing.T) {
		defer cleanupAllTriages(dbc)

		uniqueTestName := "TestFilterNoRelease"
		uniqueRegression := createTestRegressionWithDetails(t, tracker, view, "filter-norel-id", "comp-norel", "cap-norel", uniqueTestName, crtest.SignificantRegression)
		defer dbc.DB.Delete(uniqueRegression.Regression)

		// Create another regression with a different test name to verify it is excluded
		otherRegression := createTestRegressionWithDetails(t, tracker, view, "filter-norel-other-id", "comp-norel-other", "cap-norel-other", "TestDifferentNoRelease", crtest.ExtremeRegression)
		defer dbc.DB.Delete(otherRegression.Regression)

		// Filtering by test name alone (no release) should also work
		var filtered []models.TestRegression
		err := util.SippyGet(fmt.Sprintf("/api/component_readiness/regressions?test=%s", uniqueTestName), &filtered)
		require.NoError(t, err)
		require.Len(t, filtered, 1, "expected exactly one regression matching the test name")
		assert.Equal(t, uniqueRegression.Regression.ID, filtered[0].ID)

		// Verify the other regression is not included
		for _, r := range filtered {
			assert.NotEqual(t, otherRegression.Regression.ID, r.ID, "otherRegression should not be in filtered results")
		}
	})
}

// Test_RegressionPotentialMatchingTriages tests the /api/component_readiness/regressions/{id}/matches endpoint
func Test_RegressionPotentialMatchingTriages(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)
	tracker := componentreadiness.NewPostgresRegressionStore(dbc, nil)

	jiraBug := createBug(t, dbc.DB)
	defer dbc.DB.Delete(jiraBug)

	// Target regression: the one we'll query matches for
	targetRegression := createTestRegressionWithDetails(t, tracker, view, "target-test", "component-a", "capability-x", "TestTargetFunction", crtest.ExtremeRegression)
	defer dbc.DB.Delete(targetRegression.Regression)
	// Give it some job runs
	mergeJobRunsForRegression(t, tracker, targetRegression.Regression.ID, "run-1", "run-2", "run-3", "run-4")

	// Regression that matches by similar test name only (no overlapping job runs)
	matchByNameRegression := createTestRegressionWithDetails(t, tracker, view, "match-name", "component-b", "capability-y", "TestTargetFunctin", crtest.SignificantRegression) // missing 'o'
	defer dbc.DB.Delete(matchByNameRegression.Regression)
	mergeJobRunsForRegression(t, tracker, matchByNameRegression.Regression.ID, "run-99")

	// Regression that matches by job run overlap (different name, shared job runs)
	matchByOverlapRegression := createTestRegressionWithDetails(t, tracker, view, "match-overlap", "component-c", "capability-z", "TestDifferentName", crtest.ExtremeTriagedRegression)
	defer dbc.DB.Delete(matchByOverlapRegression.Regression)
	mergeJobRunsForRegression(t, tracker, matchByOverlapRegression.Regression.ID, "run-1", "run-2", "run-50") // 2 shared with target out of 3

	// Regression with no match: different name, no overlapping job runs
	noMatchRegression := createTestRegressionWithDetails(t, tracker, view, "no-match", "component-d", "capability-w", "CompletelyDifferentTest", crtest.NotSignificant)
	defer dbc.DB.Delete(noMatchRegression.Regression)
	mergeJobRunsForRegression(t, tracker, noMatchRegression.Regression.ID, "run-90", "run-91")

	t.Run("find potential matching triages by name and job run overlap", func(t *testing.T) {
		defer cleanupAllTriages(dbc)

		// Triage with name-matching regression
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

		// Triage with overlap-matching regression
		triage2 := models.Triage{
			URL:  jiraBug.URL,
			Type: models.TriageTypeCIInfra,
			Regressions: []models.TestRegression{
				{ID: matchByOverlapRegression.Regression.ID},
			},
		}
		var triageResponse2 models.Triage
		err = util.SippyPost("/api/component_readiness/triages", &triage2, &triageResponse2)
		require.NoError(t, err)

		// Triage with no-match regression
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

		var potentialMatches []componentreadiness.PotentialMatchingTriage
		endpoint := fmt.Sprintf("/api/component_readiness/regressions/%d/matches", targetRegression.Regression.ID)
		err = util.SippyGet(endpoint, &potentialMatches)
		require.NoError(t, err)

		assert.Len(t, potentialMatches, 2, "Should find 2 potential matching triages (name match + overlap match)")

		// Verify HATEOAS links
		for _, match := range potentialMatches {
			assert.Contains(t, match.Links, "self", "Potential match should have self link")
		}

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

		// Verify match by job run overlap
		overlapMatch, found := triagesByID[triageResponse2.ID]
		assert.True(t, found, "Should find triage with overlapping job runs")
		assert.Len(t, overlapMatch.OverlappingJobRuns, 1, "Should have one overlapping job run entry")
		assert.ElementsMatch(t, []string{"run-1", "run-2"}, overlapMatch.OverlappingJobRuns[0].SharedJobRunIDs, "Should share run-1 and run-2")
		// 2 shared / 3 (smaller set = overlap regression's 3 runs) = 66.7%
		assert.InDelta(t, 66.7, overlapMatch.OverlappingJobRuns[0].OverlapPercent, 1.0)
		assert.Equal(t, 7, overlapMatch.ConfidenceLevel, "Confidence should be 7 for ~67% overlap")

		// Verify non-matching triage is not included
		_, found = triagesByID[triageResponseNoMatch.ID]
		assert.False(t, found, "Should not find triage with no matching criteria")
	})

	t.Run("no potential matches found", func(t *testing.T) {
		defer cleanupAllTriages(dbc)

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

		var potentialMatches []componentreadiness.PotentialMatchingTriage
		endpoint := fmt.Sprintf("/api/component_readiness/regressions/%d/matches", targetRegression.Regression.ID)
		err = util.SippyGet(endpoint, &potentialMatches)
		require.NoError(t, err)

		assert.Len(t, potentialMatches, 0, "Should find no potential matching triages")
	})

	t.Run("high overlap gives high confidence", func(t *testing.T) {
		defer cleanupAllTriages(dbc)

		// Create a regression with near-full overlap with target
		highOverlapRegression := createTestRegressionWithDetails(t, tracker, view, "high-overlap", "component-e", "capability-v", "TestUnrelatedName", crtest.ExtremeRegression)
		defer dbc.DB.Delete(highOverlapRegression.Regression)
		mergeJobRunsForRegression(t, tracker, highOverlapRegression.Regression.ID, "run-1", "run-2", "run-3", "run-4") // 100% overlap

		triageHighOverlap := models.Triage{
			URL:  jiraBug.URL,
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				{ID: highOverlapRegression.Regression.ID},
			},
		}
		var triageResponse models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triageHighOverlap, &triageResponse)
		require.NoError(t, err)

		var potentialMatches []componentreadiness.PotentialMatchingTriage
		endpoint := fmt.Sprintf("/api/component_readiness/regressions/%d/matches", targetRegression.Regression.ID)
		err = util.SippyGet(endpoint, &potentialMatches)
		require.NoError(t, err)

		require.Len(t, potentialMatches, 1, "Should find 1 potential matching triage")
		assert.Equal(t, 10, potentialMatches[0].ConfidenceLevel, "100% overlap should give confidence 10 (capped)")
	})

	t.Run("resolved triage confidence level capped at 5", func(t *testing.T) {
		defer cleanupAllTriages(dbc)

		// Create a regression with exact same test name
		exactMatchRegression := createTestRegressionWithDetails(t, tracker, view, "exact-match", "component-e", "capability-v", "TestTargetFunction", crtest.ExtremeRegression)
		defer dbc.DB.Delete(exactMatchRegression.Regression)

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

		// Resolve the triage
		triageResponseExactMatch.Resolved = sql.NullTime{Time: time.Now(), Valid: true}
		var updateResponse models.Triage
		err = util.SippyPut(fmt.Sprintf("/api/component_readiness/triages/%d", triageResponseExactMatch.ID), &triageResponseExactMatch, &updateResponse)
		require.NoError(t, err)
		assert.True(t, updateResponse.Resolved.Valid, "Triage should be marked as resolved")

		var potentialMatches []componentreadiness.PotentialMatchingTriage
		endpoint := fmt.Sprintf("/api/component_readiness/regressions/%d/matches", targetRegression.Regression.ID)
		err = util.SippyGet(endpoint, &potentialMatches)
		require.NoError(t, err)

		require.Len(t, potentialMatches, 1, "Should find 1 potential matching triage")
		assert.Equal(t, 5, potentialMatches[0].ConfidenceLevel, "Confidence should be capped at 5 for resolved triage")
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

func Test_GetTriageSymptomSummaries(t *testing.T) {
	dbc := util.CreateE2EPostgresConnection(t)
	tracker := componentreadiness.NewPostgresRegressionStore(dbc, nil)
	dbCtx := dbc.DB.WithContext(context.WithValue(context.Background(), models.CurrentUserKey, "e2e-test"))

	cleanup := func() {
		util.CleanupTriageSymptoms(dbc)
		dbc.DB.Exec("DELETE FROM regression_job_runs WHERE 1=1")
		dbc.DB.Exec("DELETE FROM triage_regressions WHERE 1=1")
		dbc.DB.Where("1 = 1").Delete(&models.Triage{})
		dbc.DB.Where("1 = 1").Delete(&models.TestRegression{})
	}

	t.Run("returns nil when totalRegressions is zero", func(t *testing.T) {
		result, err := componentreadiness.GetTriageSymptomSummaries(dbc, 999, 0)
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("returns nil when no triage_symptoms exist", func(t *testing.T) {
		defer cleanup()

		reg := createTestRegression(t, tracker, view, "no-syms-1")
		triage := models.Triage{
			URL:  "https://redhat.atlassian.net/browse/TEST-NOSYM-1",
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				*reg,
			},
		}
		require.NoError(t, dbCtx.Create(&triage).Error)

		result, err := componentreadiness.GetTriageSymptomSummaries(dbc, triage.ID, 1)
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("percentage calculation", func(t *testing.T) {
		defer cleanup()

		symA := util.SeedSymptom(t, dbc, "pct-sym-a", "Pct Symptom A")
		symB := util.SeedSymptom(t, dbc, "pct-sym-b", "Pct Symptom B")
		defer util.CleanupSymptoms(dbc, symA.ID, symB.ID)

		var regs []*models.TestRegression
		for i := 1; i <= 3; i++ {
			reg := createTestRegression(t, tracker, view, fmt.Sprintf("pct-test-%d", i))
			regs = append(regs, reg)
		}

		triage := models.Triage{
			URL:  "https://redhat.atlassian.net/browse/TEST-PCT-1",
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				*regs[0], *regs[1], *regs[2],
			},
		}
		require.NoError(t, dbCtx.Create(&triage).Error)

		// SymA on all 3 regressions, SymB on 1
		for _, reg := range regs {
			err := tracker.MergeJobRuns(reg.ID, []models.RegressionJobRun{
				{ProwJobRunID: fmt.Sprintf("pct-run-%d", reg.ID), ProwJobName: "job-1", TestFailed: true, JobSymptoms: pq.StringArray{"pct-sym-a"}},
			})
			require.NoError(t, err)
		}
		err := tracker.MergeJobRuns(regs[0].ID, []models.RegressionJobRun{
			{ProwJobRunID: "pct-run-b", ProwJobName: "job-1", TestFailed: true, JobSymptoms: pq.StringArray{"pct-sym-b"}},
		})
		require.NoError(t, err)

		syncRegs := make([]*models.TestRegression, len(regs))
		for i, r := range regs {
			syncRegs[i] = &models.TestRegression{ID: r.ID}
		}
		err = tracker.SyncTriageSymptoms(syncRegs)
		require.NoError(t, err)

		result, err := componentreadiness.GetTriageSymptomSummaries(dbc, triage.ID, 3)
		require.NoError(t, err)
		require.Len(t, result, 2)

		symMap := make(map[string]componentreadiness.TriageSymptomSummary)
		for _, s := range result {
			symMap[s.Symptom.ID] = s
		}

		require.Contains(t, symMap, "pct-sym-a")
		assert.Equal(t, 3, symMap["pct-sym-a"].RegressionCount)
		assert.Equal(t, 3, symMap["pct-sym-a"].TotalCount)
		assert.InDelta(t, 100.0, symMap["pct-sym-a"].Percentage, 0.01)

		require.Contains(t, symMap, "pct-sym-b")
		assert.Equal(t, 1, symMap["pct-sym-b"].RegressionCount)
		assert.Equal(t, 3, symMap["pct-sym-b"].TotalCount)
		assert.InDelta(t, 33.33, symMap["pct-sym-b"].Percentage, 0.01)
	})

	t.Run("sorts by regression_count descending", func(t *testing.T) {
		defer cleanup()

		symA := util.SeedSymptom(t, dbc, "sort-sym-a", "Sort Symptom A")
		symB := util.SeedSymptom(t, dbc, "sort-sym-b", "Sort Symptom B")
		defer util.CleanupSymptoms(dbc, symA.ID, symB.ID)

		reg1 := createTestRegression(t, tracker, view, "sort-test-1")
		reg2 := createTestRegression(t, tracker, view, "sort-test-2")

		triage := models.Triage{
			URL:  "https://redhat.atlassian.net/browse/TEST-SORT-1",
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				*reg1, *reg2,
			},
		}
		require.NoError(t, dbCtx.Create(&triage).Error)

		// SymB on both regressions, SymA on only one
		err := tracker.MergeJobRuns(reg1.ID, []models.RegressionJobRun{
			{ProwJobRunID: "sort-run-1", ProwJobName: "job-1", TestFailed: true, JobSymptoms: pq.StringArray{"sort-sym-a", "sort-sym-b"}},
		})
		require.NoError(t, err)
		err = tracker.MergeJobRuns(reg2.ID, []models.RegressionJobRun{
			{ProwJobRunID: "sort-run-2", ProwJobName: "job-1", TestFailed: true, JobSymptoms: pq.StringArray{"sort-sym-b"}},
		})
		require.NoError(t, err)

		err = tracker.SyncTriageSymptoms([]*models.TestRegression{{ID: reg1.ID}, {ID: reg2.ID}})
		require.NoError(t, err)

		result, err := componentreadiness.GetTriageSymptomSummaries(dbc, triage.ID, 2)
		require.NoError(t, err)
		require.Len(t, result, 2)

		assert.Equal(t, "sort-sym-b", result[0].Symptom.ID, "symptom with higher regression_count should come first")
		assert.Equal(t, 2, result[0].RegressionCount)
		assert.Equal(t, "sort-sym-a", result[1].Symptom.ID)
		assert.Equal(t, 1, result[1].RegressionCount)
	})

	t.Run("multiple regressions with shared symptom counts distinct regressions", func(t *testing.T) {
		defer cleanup()

		sym := util.SeedSymptom(t, dbc, "shared-sym", "Shared Symptom")
		defer util.CleanupSymptoms(dbc, sym.ID)

		reg1 := createTestRegression(t, tracker, view, "shared-test-1")
		reg2 := createTestRegression(t, tracker, view, "shared-test-2")

		triage := models.Triage{
			URL:  "https://redhat.atlassian.net/browse/TEST-SHARED-1",
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				*reg1, *reg2,
			},
		}
		require.NoError(t, dbCtx.Create(&triage).Error)

		// Same symptom on both regressions, multiple job runs each
		err := tracker.MergeJobRuns(reg1.ID, []models.RegressionJobRun{
			{ProwJobRunID: "shared-run-1a", ProwJobName: "job-1", TestFailed: true, JobSymptoms: pq.StringArray{"shared-sym"}},
			{ProwJobRunID: "shared-run-1b", ProwJobName: "job-1", TestFailed: true, JobSymptoms: pq.StringArray{"shared-sym"}},
		})
		require.NoError(t, err)
		err = tracker.MergeJobRuns(reg2.ID, []models.RegressionJobRun{
			{ProwJobRunID: "shared-run-2a", ProwJobName: "job-1", TestFailed: true, JobSymptoms: pq.StringArray{"shared-sym"}},
		})
		require.NoError(t, err)

		err = tracker.SyncTriageSymptoms([]*models.TestRegression{{ID: reg1.ID}, {ID: reg2.ID}})
		require.NoError(t, err)

		result, err := componentreadiness.GetTriageSymptomSummaries(dbc, triage.ID, 2)
		require.NoError(t, err)
		require.Len(t, result, 1)

		assert.Equal(t, "shared-sym", result[0].Symptom.ID)
		assert.Equal(t, 2, result[0].RegressionCount, "regression_count should be distinct regressions, not job runs")
		assert.Equal(t, 3, result[0].JobRunCount, "job_run_count should be total across all regressions")
		assert.InDelta(t, 100.0, result[0].Percentage, 0.01)
	})

	t.Run("collects regression IDs per symptom", func(t *testing.T) {
		defer cleanup()

		symA := util.SeedSymptom(t, dbc, "regid-sym-a", "RegID Symptom A")
		symB := util.SeedSymptom(t, dbc, "regid-sym-b", "RegID Symptom B")
		defer util.CleanupSymptoms(dbc, symA.ID, symB.ID)

		reg1 := createTestRegression(t, tracker, view, "regid-test-1")
		reg2 := createTestRegression(t, tracker, view, "regid-test-2")
		reg3 := createTestRegression(t, tracker, view, "regid-test-3")

		triage := models.Triage{
			URL:  "https://redhat.atlassian.net/browse/TEST-REGID-1",
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				*reg1, *reg2, *reg3,
			},
		}
		require.NoError(t, dbCtx.Create(&triage).Error)

		// SymA on reg1 and reg2, SymB on reg2 and reg3
		err := tracker.MergeJobRuns(reg1.ID, []models.RegressionJobRun{
			{ProwJobRunID: "regid-run-1", ProwJobName: "job-1", TestFailed: true, JobSymptoms: pq.StringArray{"regid-sym-a"}},
		})
		require.NoError(t, err)
		err = tracker.MergeJobRuns(reg2.ID, []models.RegressionJobRun{
			{ProwJobRunID: "regid-run-2", ProwJobName: "job-1", TestFailed: true, JobSymptoms: pq.StringArray{"regid-sym-a", "regid-sym-b"}},
		})
		require.NoError(t, err)
		err = tracker.MergeJobRuns(reg3.ID, []models.RegressionJobRun{
			{ProwJobRunID: "regid-run-3", ProwJobName: "job-1", TestFailed: true, JobSymptoms: pq.StringArray{"regid-sym-b"}},
		})
		require.NoError(t, err)

		err = tracker.SyncTriageSymptoms([]*models.TestRegression{{ID: reg1.ID}, {ID: reg2.ID}, {ID: reg3.ID}})
		require.NoError(t, err)

		result, err := componentreadiness.GetTriageSymptomSummaries(dbc, triage.ID, 3)
		require.NoError(t, err)
		require.Len(t, result, 2)

		symMap := make(map[string]componentreadiness.TriageSymptomSummary)
		for _, s := range result {
			symMap[s.Symptom.ID] = s
		}

		require.Contains(t, symMap, "regid-sym-a")
		assert.ElementsMatch(t, []uint{reg1.ID, reg2.ID}, symMap["regid-sym-a"].RegressionIDs)

		require.Contains(t, symMap, "regid-sym-b")
		assert.ElementsMatch(t, []uint{reg2.ID, reg3.ID}, symMap["regid-sym-b"].RegressionIDs)
	})
}

func createBug(t *testing.T, dbc *gorm.DB) *models.Bug {
	jiraBug := models.Bug{
		Key:        "MYBUGS-100",
		Status:     "New",
		Summary:    "foo bar",
		Components: pq.StringArray{"component1", "component2"},
		Labels:     pq.StringArray{"label1", "label2"},
		URL:        "https://redhat.atlassian.net/browse/MYBUGS-100",
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
	tracker := componentreadiness.NewPostgresRegressionStore(dbc, nil)

	// Create test regressions with various characteristics for matching.
	// We use job run overlap and name similarity as matching signals.
	testRegressions := make([]componentreport.ReportTestSummary, 10)

	// Regression 0: Linked to triage, has job runs run-1..run-4
	testRegressions[0] = createTestRegressionWithDetails(t, tracker, view, "linked-test-1", "component-a", "capability-x", "TestSomething", crtest.ExtremeRegression)
	defer dbc.DB.Delete(testRegressions[0].Regression)
	require.NoError(t, tracker.UpsertRegressionView(testRegressions[0].Regression.ID, view.Name))
	mergeJobRunsForRegression(t, tracker, testRegressions[0].Regression.ID, "run-1", "run-2", "run-3", "run-4")

	// Regression 1: Linked to triage, has job runs run-10..run-13
	testRegressions[1] = createTestRegressionWithDetails(t, tracker, view, "linked-test-2", "component-b", "capability-y", "TestAnotherOne", crtest.SignificantRegression)
	defer dbc.DB.Delete(testRegressions[1].Regression)
	require.NoError(t, tracker.UpsertRegressionView(testRegressions[1].Regression.ID, view.Name))
	mergeJobRunsForRegression(t, tracker, testRegressions[1].Regression.ID, "run-10", "run-11", "run-12", "run-13")

	// Regression 2: Match by similar name to "TestSomething" (edit distance 2)
	testRegressions[2] = createTestRegressionWithDetails(t, tracker, view, "match-similar-name", "component-c", "capability-z", "TestSomthng", crtest.ExtremeTriagedRegression) // missing 'e' and 'i'
	defer dbc.DB.Delete(testRegressions[2].Regression)
	require.NoError(t, tracker.UpsertRegressionView(testRegressions[2].Regression.ID, view.Name))
	mergeJobRunsForRegression(t, tracker, testRegressions[2].Regression.ID, "run-90")

	// Regression 3: Match by job run overlap with regression 0 (shares run-1, run-2)
	testRegressions[3] = createTestRegressionWithDetails(t, tracker, view, "match-overlap", "component-d", "capability-w", "TestDifferent", crtest.SignificantTriagedRegression)
	defer dbc.DB.Delete(testRegressions[3].Regression)
	require.NoError(t, tracker.UpsertRegressionView(testRegressions[3].Regression.ID, view.Name))
	mergeJobRunsForRegression(t, tracker, testRegressions[3].Regression.ID, "run-1", "run-2", "run-50")

	// Regression 4: Match by both similar name AND job run overlap
	testRegressions[4] = createTestRegressionWithDetails(t, tracker, view, "match-both", "component-e", "capability-v", "TestAnoterOne", crtest.FixedRegression) // missing 'h' from "TestAnotherOne"
	defer dbc.DB.Delete(testRegressions[4].Regression)
	require.NoError(t, tracker.UpsertRegressionView(testRegressions[4].Regression.ID, view.Name))
	mergeJobRunsForRegression(t, tracker, testRegressions[4].Regression.ID, "run-10", "run-11", "run-60") // overlap with regression 1

	// Regression 5: Similar name to regression 0, no job run overlap
	testRegressions[5] = createTestRegressionWithDetails(t, tracker, view, "match-name-only", "component-f", "capability-u", "TestSomthing", crtest.MissingSample) // edit distance 1
	defer dbc.DB.Delete(testRegressions[5].Regression)
	require.NoError(t, tracker.UpsertRegressionView(testRegressions[5].Regression.ID, view.Name))
	mergeJobRunsForRegression(t, tracker, testRegressions[5].Regression.ID, "run-70")

	// Regression 6: No match - different name, no overlapping job runs
	testRegressions[6] = createTestRegressionWithDetails(t, tracker, view, "no-match-1", "component-g", "capability-t", "CompletelyDifferentTest", crtest.NotSignificant)
	defer dbc.DB.Delete(testRegressions[6].Regression)
	require.NoError(t, tracker.UpsertRegressionView(testRegressions[6].Regression.ID, view.Name))
	mergeJobRunsForRegression(t, tracker, testRegressions[6].Regression.ID, "run-80", "run-81")

	// Regression 7: No match - name too different (edit distance > 5), no overlap
	testRegressions[7] = createTestRegressionWithDetails(t, tracker, view, "no-match-2", "component-h", "capability-s", "VeryDifferentTestName", crtest.MissingBasis)
	defer dbc.DB.Delete(testRegressions[7].Regression)
	require.NoError(t, tracker.UpsertRegressionView(testRegressions[7].Regression.ID, view.Name))
	mergeJobRunsForRegression(t, tracker, testRegressions[7].Regression.ID, "run-82")

	// Regression 8: Match by job run overlap only (shares run-3, run-4 with regression 0)
	testRegressions[8] = createTestRegressionWithDetails(t, tracker, view, "match-overlap-only", "component-i", "capability-r", "TestUnrelated", crtest.MissingBasisAndSample)
	defer dbc.DB.Delete(testRegressions[8].Regression)
	require.NoError(t, tracker.UpsertRegressionView(testRegressions[8].Regression.ID, view.Name))
	mergeJobRunsForRegression(t, tracker, testRegressions[8].Regression.ID, "run-3", "run-4")

	// Regression 9: Similar name to "TestAnotherOne" (edit distance 1)
	testRegressions[9] = createTestRegressionWithDetails(t, tracker, view, "match-similar-2", "component-j", "capability-q", "TestAnotheOne", crtest.SignificantImprovement) // missing 'r'
	defer dbc.DB.Delete(testRegressions[9].Regression)
	require.NoError(t, tracker.UpsertRegressionView(testRegressions[9].Regression.ID, view.Name))
	mergeJobRunsForRegression(t, tracker, testRegressions[9].Regression.ID, "run-91")

	// Add all test regressions to the component report cache
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

		triage := models.Triage{
			URL:  "https://redhat.atlassian.net/browse/OCPBUGS-1234",
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				{ID: testRegressions[0].Regression.ID}, // TestSomething with run-1..run-4
				{ID: testRegressions[1].Regression.ID}, // TestAnotherOne with run-10..run-13
			},
		}

		var triageResponse models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage, &triageResponse)
		require.NoError(t, err)
		require.Equal(t, 2, len(triageResponse.Regressions))

		var potentialMatches []componentreadiness.PotentialMatchingRegression
		endpoint := fmt.Sprintf("/api/component_readiness/triages/%d/matches?view=%s", triageResponse.ID, view.Name)
		err = util.SippyGet(endpoint, &potentialMatches)
		require.NoError(t, err)

		assert.True(t, len(potentialMatches) > 0, "Should find some potential matches")

		// Verify HATEOAS links
		baseURL := fmt.Sprintf("http://%s:%s", os.Getenv("SIPPY_ENDPOINT"), os.Getenv("SIPPY_API_PORT"))
		for _, match := range potentialMatches {
			assert.Equal(t, fmt.Sprintf("%s/api/component_readiness/triages/%d/matches", baseURL, triageResponse.ID),
				match.Links["self"], "Potential match should have self link")
			assert.Equal(t, fmt.Sprintf("%s/api/component_readiness/triages/%d", baseURL, triageResponse.ID),
				match.Links["triage"], "Potential match should have triage link")
		}

		// Build maps for verification
		foundRegressionIDs := make(map[uint]bool)
		matchesBySimilarName := make(map[uint][]componentreadiness.SimilarlyNamedTest)
		matchesByOverlap := make(map[uint][]componentreadiness.JobRunOverlap)
		confidenceLevels := make(map[uint]int)
		statusMap := make(map[uint]crtest.Status)

		for _, match := range potentialMatches {
			if match.RegressedTest.Regression == nil {
				continue
			}
			regressionID := match.RegressedTest.Regression.ID
			foundRegressionIDs[regressionID] = true
			confidenceLevels[regressionID] = match.ConfidenceLevel
			statusMap[regressionID] = match.RegressedTest.ReportStatus
			if len(match.SimilarlyNamedTests) > 0 {
				matchesBySimilarName[regressionID] = match.SimilarlyNamedTests
			}
			if len(match.OverlappingJobRuns) > 0 {
				matchesByOverlap[regressionID] = match.OverlappingJobRuns
			}
		}

		// Linked regressions should NOT appear
		assert.False(t, foundRegressionIDs[testRegressions[0].Regression.ID], "Linked regression 0 should not appear in potential matches")
		assert.False(t, foundRegressionIDs[testRegressions[1].Regression.ID], "Linked regression 1 should not appear in potential matches")

		// Expected matches
		assert.True(t, foundRegressionIDs[testRegressions[2].Regression.ID], "Should find regression 2 (similar name)")
		assert.True(t, foundRegressionIDs[testRegressions[3].Regression.ID], "Should find regression 3 (job run overlap)")
		assert.True(t, foundRegressionIDs[testRegressions[4].Regression.ID], "Should find regression 4 (name + overlap)")
		assert.True(t, foundRegressionIDs[testRegressions[5].Regression.ID], "Should find regression 5 (similar name)")
		assert.True(t, foundRegressionIDs[testRegressions[8].Regression.ID], "Should find regression 8 (job run overlap)")
		assert.True(t, foundRegressionIDs[testRegressions[9].Regression.ID], "Should find regression 9 (similar name)")

		// Non-matches
		assert.False(t, foundRegressionIDs[testRegressions[6].Regression.ID], "Should not find regression 6 (no match)")
		assert.False(t, foundRegressionIDs[testRegressions[7].Regression.ID], "Should not find regression 7 (name too different)")

		// Verify status values
		assert.Equal(t, crtest.ExtremeTriagedRegression, statusMap[testRegressions[2].Regression.ID])
		assert.Equal(t, crtest.SignificantTriagedRegression, statusMap[testRegressions[3].Regression.ID])
		assert.Equal(t, crtest.FixedRegression, statusMap[testRegressions[4].Regression.ID])
		assert.Equal(t, crtest.MissingSample, statusMap[testRegressions[5].Regression.ID])
		assert.Equal(t, crtest.MissingBasisAndSample, statusMap[testRegressions[8].Regression.ID])
		assert.Equal(t, crtest.SignificantImprovement, statusMap[testRegressions[9].Regression.ID])

		// Regression 2: Match by similar name only (TestSomthng vs TestSomething, edit distance 2)
		if assert.Contains(t, matchesBySimilarName, testRegressions[2].Regression.ID) {
			matches := matchesBySimilarName[testRegressions[2].Regression.ID]
			assert.Equal(t, 1, len(matches))
			assert.Equal(t, testRegressions[0].Regression.ID, matches[0].Regression.ID)
			// score = 6 - 2 = 4
			assert.Equal(t, 4, confidenceLevels[testRegressions[2].Regression.ID])
		}
		assert.NotContains(t, matchesByOverlap, testRegressions[2].Regression.ID, "Regression 2 should not match by overlap")

		// Regression 3: Match by job run overlap (shares run-1, run-2 with linked regression 0)
		if assert.Contains(t, matchesByOverlap, testRegressions[3].Regression.ID) {
			overlaps := matchesByOverlap[testRegressions[3].Regression.ID]
			assert.Equal(t, 1, len(overlaps))
			assert.ElementsMatch(t, []string{"run-1", "run-2"}, overlaps[0].SharedJobRunIDs)
			// 2 shared / 3 (smaller set) = 66.7%, score = int(66.7/10) + 1 = 7
			assert.InDelta(t, 66.7, overlaps[0].OverlapPercent, 1.0)
			assert.Equal(t, 7, confidenceLevels[testRegressions[3].Regression.ID])
		}
		assert.NotContains(t, matchesBySimilarName, testRegressions[3].Regression.ID, "Regression 3 should not match by name")

		// Regression 4: Match by both name AND job run overlap
		if assert.Contains(t, matchesBySimilarName, testRegressions[4].Regression.ID) {
			nameMatches := matchesBySimilarName[testRegressions[4].Regression.ID]
			assert.Equal(t, 1, len(nameMatches))
			assert.Equal(t, testRegressions[1].Regression.ID, nameMatches[0].Regression.ID)
		}
		if assert.Contains(t, matchesByOverlap, testRegressions[4].Regression.ID) {
			overlaps := matchesByOverlap[testRegressions[4].Regression.ID]
			assert.Equal(t, 1, len(overlaps))
			assert.ElementsMatch(t, []string{"run-10", "run-11"}, overlaps[0].SharedJobRunIDs)
			// 2 shared / 3 (smaller set) = 66.7%, overlap score = 7
			// name score: TestAnoterOne vs TestAnotherOne = edit distance 1, 6-1 = 5
			// total = 7 + 5 = 12, capped at 10
			assert.Equal(t, 10, confidenceLevels[testRegressions[4].Regression.ID])
		}

		// Regression 5: Match by similar name only (TestSomthing vs TestSomething, edit distance 1)
		if assert.Contains(t, matchesBySimilarName, testRegressions[5].Regression.ID) {
			matches := matchesBySimilarName[testRegressions[5].Regression.ID]
			assert.Equal(t, 1, len(matches))
			assert.Equal(t, testRegressions[0].Regression.ID, matches[0].Regression.ID)
			assert.Equal(t, 5, confidenceLevels[testRegressions[5].Regression.ID]) // 6 - 1 = 5
		}
		assert.NotContains(t, matchesByOverlap, testRegressions[5].Regression.ID)

		// Regression 8: Match by job run overlap only (shares run-3, run-4 with linked regression 0)
		if assert.Contains(t, matchesByOverlap, testRegressions[8].Regression.ID) {
			overlaps := matchesByOverlap[testRegressions[8].Regression.ID]
			assert.Equal(t, 1, len(overlaps))
			assert.ElementsMatch(t, []string{"run-3", "run-4"}, overlaps[0].SharedJobRunIDs)
			// 2 shared / 2 (smaller set = regression 8's 2 runs) = 100%, score = 10 (capped)
			assert.InDelta(t, 100.0, overlaps[0].OverlapPercent, 0.1)
			assert.Equal(t, 10, confidenceLevels[testRegressions[8].Regression.ID])
		}
		assert.NotContains(t, matchesBySimilarName, testRegressions[8].Regression.ID)

		// Regression 9: Similar name to "TestAnotherOne" (edit distance 1)
		if assert.Contains(t, matchesBySimilarName, testRegressions[9].Regression.ID) {
			matches := matchesBySimilarName[testRegressions[9].Regression.ID]
			assert.Equal(t, 1, len(matches))
			assert.Equal(t, testRegressions[1].Regression.ID, matches[0].Regression.ID)
			assert.Equal(t, 5, confidenceLevels[testRegressions[9].Regression.ID]) // 6 - 1 = 5
		}
	})

	t.Run("empty potential matches when no regressions exist", func(t *testing.T) {
		defer cleanupAllTriages(dbc)

		triage := models.Triage{
			URL:  "https://redhat.atlassian.net/browse/OCPBUGS-1234",
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				{ID: testRegressions[6].Regression.ID}, // CompletelyDifferentTest
			},
		}

		var triageResponse models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage, &triageResponse)
		require.NoError(t, err)

		var potentialMatches []componentreadiness.PotentialMatchingRegression
		endpoint := fmt.Sprintf("/api/component_readiness/triages/%d/matches?view=%s", triageResponse.ID, view.Name)
		err = util.SippyGet(endpoint, &potentialMatches)
		require.NoError(t, err)

		foundRegressionIDs := make(map[uint]bool)
		for _, match := range potentialMatches {
			if match.RegressedTest.Regression != nil {
				foundRegressionIDs[match.RegressedTest.Regression.ID] = true
			}
		}

		assert.False(t, foundRegressionIDs[testRegressions[6].Regression.ID], "Linked regression should not appear in potential matches")
	})

	t.Run("error when view does not exist", func(t *testing.T) {
		defer cleanupAllTriages(dbc)

		triage := models.Triage{
			URL:  "https://redhat.atlassian.net/browse/OCPBUGS-9999",
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				{ID: testRegressions[0].Regression.ID},
			},
		}

		var triageResponse models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage, &triageResponse)
		require.NoError(t, err)

		var potentialMatches []componentreadiness.PotentialMatchingRegression
		endpoint := fmt.Sprintf("/api/component_readiness/triages/%d/matches?view=no-such-view", triageResponse.ID)
		err = util.SippyGet(endpoint, &potentialMatches)
		require.Error(t, err, "Non-existent view should return an error")
	})

	t.Run("error when triage not found", func(t *testing.T) {
		var potentialMatches []interface{}

		endpoint := "/api/component_readiness/triages/999999/matches"
		err := util.SippyGet(endpoint, &potentialMatches)
		require.Error(t, err, "Should return error for non-existent triage")
	})

	t.Run("verify status values in triage responses", func(t *testing.T) {
		defer cleanupAllTriages(dbc)

		triage := models.Triage{
			URL:  "https://redhat.atlassian.net/browse/OCPBUGS-5678",
			Type: models.TriageTypeProduct,
			Regressions: []models.TestRegression{
				{ID: testRegressions[0].Regression.ID},
				{ID: testRegressions[1].Regression.ID},
				{ID: testRegressions[4].Regression.ID},
			},
		}

		var triageResponse models.Triage
		err := util.SippyPost("/api/component_readiness/triages", &triage, &triageResponse)
		require.NoError(t, err)
		require.Equal(t, 3, len(triageResponse.Regressions))

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
func createTestRegressionWithDetails(t *testing.T, tracker componentreadiness.RegressionStore, view crview.View, testID, component, capability, testName string, status crtest.Status) componentreport.ReportTestSummary {
	newRegression := componentreport.ReportTestSummary{
		TestComparison: testdetails.TestComparison{
			ReportStatus: status,
			BaseStats: &testdetails.ReleaseStats{
				Release: util.BaseRelease,
			},
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

// mergeJobRunsForRegression is a helper that adds job runs with the given prow job run IDs to a regression.
func mergeJobRunsForRegression(t *testing.T, tracker componentreadiness.RegressionStore, regressionID uint, runIDs ...string) {
	var jobRuns []models.RegressionJobRun
	for _, id := range runIDs {
		jobRuns = append(jobRuns, models.RegressionJobRun{
			ProwJobRunID: id,
			ProwJobName:  "periodic-ci-test-job",
			TestFailed:   true,
			TestFailures: 1,
		})
	}
	err := tracker.MergeJobRuns(regressionID, jobRuns)
	require.NoError(t, err)
}

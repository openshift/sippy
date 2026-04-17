package postgres

import (
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/test/e2e/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComponentReadinessViews(t *testing.T) {
	var views []crview.View
	err := util.SippyGet("/api/component_readiness/views", &views)
	require.NoError(t, err, "error making http request")
	t.Logf("found %d views", len(views))
	require.Greater(t, len(views), 0, "no views returned, check server cli params")

	// Make a basic request for the first view and ensure we get some data back
	var report componentreport.ComponentReport
	err = util.SippyGet(fmt.Sprintf("/api/component_readiness?view=%s", views[0].Name), &report)
	require.NoError(t, err, "error making http request")
	assert.Greater(t, len(report.Rows), 10, "component report does not have rows we would expect")
}

func TestTestDetails(t *testing.T) {
	viewName := fmt.Sprintf("%s-main", util.Release)

	// First get a report to find a regressed test
	var report componentreport.ComponentReport
	err := util.SippyGet(fmt.Sprintf("/api/component_readiness?view=%s", viewName), &report)
	require.NoError(t, err, "error getting component report")
	require.NotEmpty(t, report.Rows, "report should have rows")

	// Find a regressed test to query details for
	var testID, component, capability string
	variants := map[string]string{}
	found := false
	for _, row := range report.Rows {
		for _, col := range row.Columns {
			for _, rt := range col.RegressedTests {
				testID = rt.TestID
				component = rt.Component
				capability = rt.Capability
				variants = rt.ColumnIdentification.Variants
				found = true
				break
			}
			if found {
				break
			}
		}
		if found {
			break
		}
	}
	require.True(t, found, "should find at least one regressed test in the report")

	// Build query params for test_details
	params := url.Values{}
	params.Set("view", viewName)
	params.Set("testId", testID)
	params.Set("component", component)
	params.Set("capability", capability)
	for k, v := range variants {
		params.Set(k, v)
	}

	var details testdetails.Report
	err = util.SippyGet(fmt.Sprintf("/api/component_readiness/test_details?%s", params.Encode()), &details)
	require.NoError(t, err, "error getting test details")

	assert.Equal(t, testID, details.TestID, "response should match requested test ID")
	assert.NotEmpty(t, details.Analyses, "details should have analyses")

	for _, analysis := range details.Analyses {
		total := analysis.SampleStats.SuccessCount + analysis.SampleStats.FailureCount + analysis.SampleStats.FlakeCount
		assert.Greater(t, total, 0, "sample stats should have run data")
	}
}

func TestTestDetailsWithFallback(t *testing.T) {
	viewName := fmt.Sprintf("%s-main", util.Release)

	// Request test details for test-fallback-improves with testBasisRelease=4.20.
	// This exercises the releasefallback middleware's QueryTestDetails path which
	// queries base job run status for the override release.
	params := url.Values{}
	params.Set("view", viewName)
	params.Set("testId", "test-fallback-improves")
	params.Set("component", "comp-FallbackImproves")
	params.Set("capability", "cap1")
	params.Set("testBasisRelease", "4.20")
	// All DBGroupBy variants must be specified for test details
	params.Set("Architecture", "amd64")
	params.Set("Platform", "aws")
	params.Set("Network", "ovn")
	params.Set("Topology", "ha")
	params.Set("Installer", "ipi")
	params.Set("FeatureSet", "default")
	params.Set("Suite", "parallel")
	params.Set("Upgrade", "none")
	params.Set("LayeredProduct", "none")

	var details testdetails.Report
	err := util.SippyGet(fmt.Sprintf("/api/component_readiness/test_details?%s", params.Encode()), &details)
	require.NoError(t, err, "error getting fallback test details")

	assert.Equal(t, "test-fallback-improves", details.TestID)
	assert.NotEmpty(t, details.Analyses, "details should have analyses")

	// The fallback path should produce analyses with sample data
	for _, analysis := range details.Analyses {
		sampleTotal := analysis.SampleStats.SuccessCount + analysis.SampleStats.FailureCount + analysis.SampleStats.FlakeCount
		assert.Greater(t, sampleTotal, 0, "sample stats should have run data")
	}
}

func TestTestDetailsForNewTestAPI(t *testing.T) {
	viewName := fmt.Sprintf("%s-main", util.Release)

	// Request test details for test-new-test-pass-rate-fail which has no base data.
	// This exercises the GenerateDetailsReportForTest path where BaseStats is nil.
	params := url.Values{}
	params.Set("view", viewName)
	params.Set("testId", "test-new-test-pass-rate-fail")
	params.Set("component", "comp-NewTestPassRate")
	params.Set("capability", "cap1")
	params.Set("Architecture", "amd64")
	params.Set("Platform", "aws")
	params.Set("Network", "ovn")
	params.Set("Topology", "ha")
	params.Set("Installer", "ipi")
	params.Set("FeatureSet", "default")
	params.Set("Suite", "parallel")
	params.Set("Upgrade", "none")
	params.Set("LayeredProduct", "none")

	var details testdetails.Report
	err := util.SippyGet(fmt.Sprintf("/api/component_readiness/test_details?%s", params.Encode()), &details)
	require.NoError(t, err, "error getting new test details")

	assert.Equal(t, "test-new-test-pass-rate-fail", details.TestID)
	assert.NotEmpty(t, details.Analyses, "details should have analyses")

	for _, analysis := range details.Analyses {
		sampleTotal := analysis.SampleStats.SuccessCount + analysis.SampleStats.FailureCount + analysis.SampleStats.FlakeCount
		assert.Greater(t, sampleTotal, 0, "sample stats should have run data")
	}
}

func TestReportWithIncludeVariantsAPI(t *testing.T) {
	viewName := fmt.Sprintf("%s-main", util.Release)

	var report componentreport.ComponentReport
	err := util.SippyGet(fmt.Sprintf("/api/component_readiness?view=%s&includeVariant=Platform:aws", viewName), &report)
	require.NoError(t, err, "error getting filtered report")
	require.NotEmpty(t, report.Rows, "filtered report should have rows")

	for _, row := range report.Rows {
		for _, col := range row.Columns {
			assert.Equal(t, "aws", col.ColumnIdentification.Variants["Platform"],
				"all columns should be aws when filtering by Platform:aws")
		}
	}
}

func TestVariants(t *testing.T) {
	// /api/component_readiness/variants returns CacheVariants (legacy BQ column names)
	var variants map[string][]string
	err := util.SippyGet("/api/component_readiness/variants", &variants)
	require.NoError(t, err, "error getting variants")
	assert.Contains(t, variants, "platform", "should have platform variant")
	assert.Contains(t, variants, "network", "should have network variant")
	assert.Contains(t, variants, "arch", "should have arch variant")

	// /api/job_variants returns JobVariants with all variant groups
	var jobVariants crtest.JobVariants
	err = util.SippyGet("/api/job_variants", &jobVariants)
	require.NoError(t, err, "error getting job variants")
	require.NotEmpty(t, jobVariants.Variants, "should have variants")

	expectedVariants := []string{"Platform", "Architecture", "Network", "Topology", "FeatureSet"}
	for _, v := range expectedVariants {
		assert.Contains(t, jobVariants.Variants, v, "should have %s variant", v)
		assert.NotEmpty(t, jobVariants.Variants[v], "%s variant should have values", v)
	}
}

func TestRegressionByID(t *testing.T) {
	// List regressions to find one we can query by ID
	var regressions []models.TestRegression
	err := util.SippyGet(fmt.Sprintf("/api/component_readiness/regressions?release=%s", util.Release), &regressions)
	require.NoError(t, err, "error listing regressions")
	require.NotEmpty(t, regressions, "should have regressions from seed data")

	// Fetch one by ID
	regression := regressions[0]
	var fetched models.TestRegression
	err = util.SippyGet(fmt.Sprintf("/api/component_readiness/regressions/%d", regression.ID), &fetched)
	require.NoError(t, err, "error getting regression by ID")

	assert.Equal(t, regression.ID, fetched.ID)
	assert.Equal(t, regression.TestID, fetched.TestID)
	assert.Equal(t, regression.TestName, fetched.TestName)
	assert.Equal(t, regression.Release, fetched.Release)

	// HATEOAS links
	assert.NotNil(t, fetched.Links, "regression should have HATEOAS links")
	assert.Contains(t, fetched.Links, "test_details", "should have test_details link")
}

func TestRegressionByIDNotFound(t *testing.T) {
	var fetched models.TestRegression
	err := util.SippyGet("/api/component_readiness/regressions/999999", &fetched)
	require.Error(t, err, "should return error for non-existent regression")
}

func TestColumnGroupByAndDBGroupBy(t *testing.T) {
	viewName := fmt.Sprintf("%s-main", util.Release)

	t.Run("default grouping from view", func(t *testing.T) {
		var report componentreport.ComponentReport
		err := util.SippyGet(fmt.Sprintf("/api/component_readiness?view=%s", viewName), &report)
		require.NoError(t, err)
		require.NotEmpty(t, report.Rows)

		// Default view has columnGroupBy: Network, Platform, Topology
		for _, row := range report.Rows {
			for _, col := range row.Columns {
				assert.Contains(t, col.ColumnIdentification.Variants, "Platform")
				assert.Contains(t, col.ColumnIdentification.Variants, "Network")
				assert.Contains(t, col.ColumnIdentification.Variants, "Topology")
			}
		}
	})

	t.Run("override columnGroupBy to Platform only", func(t *testing.T) {
		var report componentreport.ComponentReport
		err := util.SippyGet(fmt.Sprintf("/api/component_readiness?view=%s&columnGroupBy=Platform", viewName), &report)
		require.NoError(t, err)
		require.NotEmpty(t, report.Rows)

		for _, row := range report.Rows {
			for _, col := range row.Columns {
				assert.Contains(t, col.ColumnIdentification.Variants, "Platform",
					"columns should still have Platform")
				assert.Equal(t, 1, len(col.ColumnIdentification.Variants),
					"columns should only have Platform when columnGroupBy is overridden to Platform")
			}
		}
	})

	t.Run("override columnGroupBy to Platform,Network", func(t *testing.T) {
		var report componentreport.ComponentReport
		err := util.SippyGet(fmt.Sprintf("/api/component_readiness?view=%s&columnGroupBy=Platform,Network", viewName), &report)
		require.NoError(t, err)
		require.NotEmpty(t, report.Rows)

		for _, row := range report.Rows {
			for _, col := range row.Columns {
				assert.Contains(t, col.ColumnIdentification.Variants, "Platform")
				assert.Contains(t, col.ColumnIdentification.Variants, "Network")
				assert.Equal(t, 2, len(col.ColumnIdentification.Variants),
					"columns should have exactly Platform and Network")
			}
		}
	})

	t.Run("override dbGroupBy reduces aggregation", func(t *testing.T) {
		// With fewer dbGroupBy dimensions, we should get fewer rows because tests
		// are aggregated at a coarser level
		var defaultReport componentreport.ComponentReport
		err := util.SippyGet(fmt.Sprintf("/api/component_readiness?view=%s", viewName), &defaultReport)
		require.NoError(t, err)

		// Use a minimal dbGroupBy — just Platform, Network, Topology (must include columnGroupBy)
		var reducedReport componentreport.ComponentReport
		err = util.SippyGet(fmt.Sprintf("/api/component_readiness?view=%s&dbGroupBy=Platform,Network,Topology", viewName), &reducedReport)
		require.NoError(t, err)
		require.NotEmpty(t, reducedReport.Rows)

		// The reduced dbGroupBy should produce a valid report. We can't strictly assert
		// fewer rows since it depends on the data, but it should still work without error.
		t.Logf("default report: %d rows, reduced dbGroupBy: %d rows", len(defaultReport.Rows), len(reducedReport.Rows))
	})

	t.Run("override both columnGroupBy and dbGroupBy together", func(t *testing.T) {
		var report componentreport.ComponentReport
		err := util.SippyGet(fmt.Sprintf("/api/component_readiness?view=%s&columnGroupBy=Platform&dbGroupBy=Platform,Architecture,Network,Topology", viewName), &report)
		require.NoError(t, err)
		require.NotEmpty(t, report.Rows)

		for _, row := range report.Rows {
			for _, col := range row.Columns {
				assert.Equal(t, 1, len(col.ColumnIdentification.Variants),
					"columns should only have Platform when columnGroupBy is Platform")
				assert.Contains(t, col.ColumnIdentification.Variants, "Platform")
			}
		}
	})
}

func TestComponentReadinessWithExplicitParams(t *testing.T) {
	// Test using explicit date/release params instead of a view.
	// This exercises parseDateRange and parseAdvancedOptions.
	now := time.Now().UTC().Truncate(time.Hour)
	params := url.Values{}
	params.Set("baseRelease", "4.21")
	params.Set("sampleRelease", "4.22")
	params.Set("baseStartTime", now.Add(-60*24*time.Hour).Format(time.RFC3339))
	params.Set("baseEndTime", now.Add(-30*24*time.Hour).Format(time.RFC3339))
	params.Set("sampleStartTime", now.Add(-3*24*time.Hour).Format(time.RFC3339))
	params.Set("sampleEndTime", now.Format(time.RFC3339))
	params.Set("confidence", "95")
	params.Set("minFail", "3")
	params.Set("pity", "5")
	params.Set("passRateNewTests", "90")
	params.Set("columnGroupBy", "Network,Platform,Topology")
	params.Set("dbGroupBy", "Architecture,FeatureSet,Installer,Network,Platform,Suite,Topology,Upgrade,LayeredProduct")

	var report componentreport.ComponentReport
	err := util.SippyGet(fmt.Sprintf("/api/component_readiness?%s", params.Encode()), &report)
	require.NoError(t, err, "explicit params request should succeed")
	assert.NotEmpty(t, report.Rows, "report with explicit params should have rows")

	for _, row := range report.Rows {
		for _, col := range row.Columns {
			assert.Contains(t, col.ColumnIdentification.Variants, "Platform")
			assert.Contains(t, col.ColumnIdentification.Variants, "Network")
			assert.Contains(t, col.ColumnIdentification.Variants, "Topology")
		}
	}
}

func TestVariantCrossCompare(t *testing.T) {
	viewName := fmt.Sprintf("%s-main", util.Release)

	t.Run("cross-compare Platform with specific value", func(t *testing.T) {
		// The default view includes Platform:aws,gcp. Cross-comparing Platform
		// with a specific value means the sample uses only that Platform value.
		params := url.Values{}
		params.Set("view", viewName)
		params.Add("variantCrossCompare", "Platform")
		params.Add("compareVariant", "Platform:gcp")

		var report componentreport.ComponentReport
		err := util.SippyGet(fmt.Sprintf("/api/component_readiness?%s", params.Encode()), &report)
		require.NoError(t, err, "cross-compare request should succeed")
		t.Logf("cross-compare Platform:gcp report: %d rows", len(report.Rows))

		// Columns should still respect the columnGroupBy from the view
		for _, row := range report.Rows {
			for _, col := range row.Columns {
				assert.Contains(t, col.ColumnIdentification.Variants, "Platform")
			}
		}
	})

	t.Run("cross-compare without compareVariant values", func(t *testing.T) {
		// Cross-compare a group without specifying compareVariant values means
		// no restriction on that variant in the sample
		params := url.Values{}
		params.Set("view", viewName)
		params.Add("variantCrossCompare", "Platform")

		var report componentreport.ComponentReport
		err := util.SippyGet(fmt.Sprintf("/api/component_readiness?%s", params.Encode()), &report)
		require.NoError(t, err, "cross-compare without compareVariant should succeed")
		t.Logf("cross-compare (no compareVariant) report: %d rows", len(report.Rows))
	})
}

func TestTriageAffectsReportStatus(t *testing.T) {
	viewName := fmt.Sprintf("%s-main", util.Release)

	// Get a report and find a regressed test
	var report componentreport.ComponentReport
	err := util.SippyGet(fmt.Sprintf("/api/component_readiness?view=%s", viewName), &report)
	require.NoError(t, err)

	var regressionID uint
	var originalStatus crtest.Status
	found := false
	for _, row := range report.Rows {
		for _, col := range row.Columns {
			for _, rt := range col.RegressedTests {
				if rt.ReportStatus == crtest.SignificantRegression || rt.ReportStatus == crtest.ExtremeRegression {
					if rt.Regression != nil {
						regressionID = rt.Regression.ID
						originalStatus = rt.ReportStatus
						found = true
					}
				}
				if found {
					break
				}
			}
			if found {
				break
			}
		}
		if found {
			break
		}
	}
	require.True(t, found, "should find a regressed test with a regression ID")
	t.Logf("found regression ID %d with status %d", regressionID, originalStatus)

	// Create a triage for this regression
	triage := models.Triage{
		URL:  "https://issues.redhat.com/browse/OCPBUGS-99999",
		Type: models.TriageTypeProduct,
		Regressions: []models.TestRegression{
			{ID: regressionID},
		},
	}
	var triageResponse models.Triage
	err = util.SippyPost("/api/component_readiness/triages", &triage, &triageResponse)
	require.NoError(t, err, "creating triage should succeed")
	t.Logf("created triage ID %d", triageResponse.ID)

	// Clean up triage when done
	defer func() {
		_ = util.SippyDelete(fmt.Sprintf("/api/component_readiness/triages/%d", triageResponse.ID))
	}()

	// Re-fetch the report — PostAnalysis runs outside the cache, so triaged status should be reflected
	var updatedReport componentreport.ComponentReport
	err = util.SippyGet(fmt.Sprintf("/api/component_readiness?view=%s", viewName), &updatedReport)
	require.NoError(t, err)

	// Find the same regression and verify its status changed to triaged
	foundTriaged := false
	for _, row := range updatedReport.Rows {
		for _, col := range row.Columns {
			for _, rt := range col.RegressedTests {
				if rt.Regression == nil || rt.Regression.ID != regressionID {
					continue
				}
				expectedStatus := crtest.SignificantTriagedRegression
				if originalStatus == crtest.ExtremeRegression {
					expectedStatus = crtest.ExtremeTriagedRegression
				}
				assert.Equal(t, expectedStatus, rt.ReportStatus,
					"regression should have triaged status after triage creation")
				assert.NotEmpty(t, rt.Explanations, "triaged test should have explanations")
				foundTriaged = true
			}
		}
	}
	assert.True(t, foundTriaged, "should find the triaged regression in the updated report")
}

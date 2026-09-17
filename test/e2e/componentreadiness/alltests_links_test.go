package componentreadiness

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	"github.com/openshift/sippy/test/e2e/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllTestsExcludedFromTopLevel(t *testing.T) {
	var views []crview.View
	err := util.SippyGet("/api/component_readiness/views", &views)
	require.NoError(t, err, "error fetching views")
	require.Greater(t, len(views), 0, "no views returned")

	var report componentreport.ComponentReport
	err = util.SippyGet(fmt.Sprintf("/api/component_readiness?view=%s", url.QueryEscape(views[0].Name)), &report)
	require.NoError(t, err, "error fetching component report")
	require.Greater(t, len(report.Rows), 0, "component report has no rows")

	t.Run("AllTests is empty at top level", func(t *testing.T) {
		for _, row := range report.Rows {
			for _, col := range row.Columns {
				assert.Empty(t, col.AllTests,
					"AllTests should not be populated at top level (component=%s)", row.Component)
			}
		}
	})

	t.Run("RegressedTests still has links at top level", func(t *testing.T) {
		var foundRegression bool
		for _, row := range report.Rows {
			for _, col := range row.Columns {
				for _, rt := range col.RegressedTests {
					foundRegression = true
					assert.NotNil(t, rt.Links, "regressed test %s should have links", rt.TestName)
					assert.Contains(t, rt.Links, "test_details",
						"regressed test %s should have a test_details link", rt.TestName)
				}
			}
		}
		if !foundRegression {
			t.Skip("no regressed tests in report to verify")
		}
	})

	t.Run("RegressedTests unchanged by AllTests addition", func(t *testing.T) {
		for _, row := range report.Rows {
			for _, col := range row.Columns {
				for _, rt := range col.RegressedTests {
					assert.Less(t, int(rt.ReportStatus), int(crtest.MissingSample),
						"RegressedTests should only contain tests with regression status, got %d for %s",
						rt.ReportStatus, rt.TestName)
				}
			}
		}
	})
}

// findRowWithMixedStatuses returns a row that has at least one regressed column
// and at least one non-regressed column, so the Level 4 report will contain both
// regressed and non-regressed cells for subtest coverage.
func findRowWithMixedStatuses(rows []componentreport.ReportRow) *componentreport.ReportRow {
	for i, row := range rows {
		hasRegressed := false
		hasNonRegressed := false
		for _, col := range row.Columns {
			if col.Status <= crtest.SignificantTriagedRegression {
				hasRegressed = true
			}
			if col.Status == crtest.NotSignificant ||
				col.Status == crtest.SignificantImprovement ||
				col.Status == crtest.MissingBasis {
				hasNonRegressed = true
			}
		}
		if hasRegressed && hasNonRegressed {
			return &rows[i]
		}
	}
	return nil
}

func TestAllTestsLinksPresent(t *testing.T) {
	// Drill down from Level 1 to Level 4 to find a report with AllTests populated.
	// At each level, prefer rows with mixed statuses (both regressed and non-regressed
	// columns) so the Level 4 report exercises both link subtests.
	var views []crview.View
	err := util.SippyGet("/api/component_readiness/views", &views)
	require.NoError(t, err, "error fetching views")
	require.Greater(t, len(views), 0, "no views returned")
	viewName := views[0].Name

	// Level 1: find a component with both regressed and non-regressed columns
	var l1Report componentreport.ComponentReport
	err = util.SippyGet(fmt.Sprintf("/api/component_readiness?view=%s", url.QueryEscape(viewName)), &l1Report)
	require.NoError(t, err, "error fetching level 1 report")
	require.Greater(t, len(l1Report.Rows), 0, "level 1 report has no rows")

	l1Row := findRowWithMixedStatuses(l1Report.Rows)
	if l1Row == nil {
		l1Row = &l1Report.Rows[0]
	}
	component := l1Row.Component
	require.NotEmpty(t, component, "level 1 row has empty component")

	// Level 2: get a capability, preferring one with mixed statuses
	var l2Report componentreport.ComponentReport
	err = util.SippyGet(fmt.Sprintf("/api/component_readiness?view=%s&component=%s",
		url.QueryEscape(viewName), url.QueryEscape(component)), &l2Report)
	require.NoError(t, err, "error fetching level 2 report")
	require.Greater(t, len(l2Report.Rows), 0, "level 2 report has no rows for component=%s", component)
	l2Row := findRowWithMixedStatuses(l2Report.Rows)
	if l2Row == nil {
		l2Row = &l2Report.Rows[0]
	}
	capability := l2Row.Capability
	require.NotEmpty(t, capability, "level 2 row has empty capability")

	// Level 3: find a test with mixed variant statuses
	var l3Report componentreport.ComponentReport
	err = util.SippyGet(fmt.Sprintf("/api/component_readiness?view=%s&component=%s&capability=%s",
		url.QueryEscape(viewName), url.QueryEscape(component), url.QueryEscape(capability)), &l3Report)
	require.NoError(t, err, "error fetching level 3 report")
	require.Greater(t, len(l3Report.Rows), 0, "level 3 report has no rows for component=%s capability=%s", component, capability)

	l3Row := findRowWithMixedStatuses(l3Report.Rows)
	if l3Row == nil {
		l3Row = &l3Report.Rows[0]
	}
	testID := l3Row.TestID
	require.NotEmpty(t, testID, "level 3 row has empty testId")

	// Level 4: fetch the test variant report where AllTests should be populated
	var report componentreport.ComponentReport
	err = util.SippyGet(fmt.Sprintf("/api/component_readiness?view=%s&component=%s&capability=%s&testId=%s&includeAllTests=true",
		url.QueryEscape(viewName), url.QueryEscape(component), url.QueryEscape(capability), url.QueryEscape(testID)), &report)
	require.NoError(t, err, "error fetching level 4 report")
	require.Greater(t, len(report.Rows), 0, "level 4 report has no rows")

	t.Run("regressed tests have test_details links in AllTests", func(t *testing.T) {
		var foundRegression bool
		for _, row := range report.Rows {
			for _, col := range row.Columns {
				if len(col.RegressedTests) == 0 {
					continue
				}
				foundRegression = true

				for _, rt := range col.RegressedTests {
					assert.NotNil(t, rt.Links, "regressed test %s should have links", rt.TestName)
					assert.Contains(t, rt.Links, "test_details",
						"regressed test %s should have a test_details link", rt.TestName)
				}

				assert.GreaterOrEqual(t, len(col.AllTests), len(col.RegressedTests),
					"AllTests should contain at least as many entries as RegressedTests")

				for _, at := range col.AllTests {
					assert.NotNil(t, at.Links, "AllTests entry %s should have links", at.TestName)
					assert.Contains(t, at.Links, "test_details",
						"AllTests entry %s should have a test_details link", at.TestName)
				}
			}
		}
		if !foundRegression {
			t.Skip("no regressed tests in report to verify")
		}
	})

	t.Run("non-regressed tests have test_details links in AllTests", func(t *testing.T) {
		var foundNonRegressed bool
		for _, row := range report.Rows {
			for _, col := range row.Columns {
				if len(col.AllTests) == 0 || len(col.RegressedTests) > 0 {
					continue
				}
				if col.Status == crtest.NotSignificant ||
					col.Status == crtest.SignificantImprovement ||
					col.Status == crtest.MissingBasis {
					foundNonRegressed = true

					for _, at := range col.AllTests {
						assert.NotNil(t, at.Links,
							"non-regressed test %s (status %d) should have links", at.TestName, at.ReportStatus)
						assert.Contains(t, at.Links, "test_details",
							"non-regressed test %s (status %d) should have a test_details link", at.TestName, at.ReportStatus)
						assert.Contains(t, at.Links["test_details"], "testId=",
							"test_details link for %s should contain testId parameter", at.TestName)
					}
				}
			}
		}
		if !foundNonRegressed {
			t.Skip("no non-regressed cells with tests found in report")
		}
	})

	t.Run("AllTests is empty only for cells with no test data", func(t *testing.T) {
		for _, row := range report.Rows {
			for _, col := range row.Columns {
				if col.Status == crtest.MissingBasisAndSample {
					assert.Empty(t, col.AllTests,
						"MissingBasisAndSample cell should have no AllTests entries")
				}
			}
		}
	})

	t.Run("RegressedTests unchanged by AllTests addition", func(t *testing.T) {
		for _, row := range report.Rows {
			for _, col := range row.Columns {
				for _, rt := range col.RegressedTests {
					assert.Less(t, int(rt.ReportStatus), int(crtest.MissingSample),
						"RegressedTests should only contain tests with regression status, got %d for %s",
						rt.ReportStatus, rt.TestName)
				}
			}
		}
	})
}

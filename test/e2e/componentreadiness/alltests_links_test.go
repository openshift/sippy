package componentreadiness

import (
	"fmt"
	"testing"

	"github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	"github.com/openshift/sippy/test/e2e/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllTestsLinksPresent(t *testing.T) {
	var views []crview.View
	err := util.SippyGet("/api/component_readiness/views", &views)
	require.NoError(t, err, "error fetching views")
	require.Greater(t, len(views), 0, "no views returned")

	var report componentreport.ComponentReport
	err = util.SippyGet(fmt.Sprintf("/api/component_readiness?view=%s", views[0].Name), &report)
	require.NoError(t, err, "error fetching component report")
	require.Greater(t, len(report.Rows), 0, "component report has no rows")

	t.Run("regressed tests have test_details links in AllTests", func(t *testing.T) {
		var foundRegression bool
		for _, row := range report.Rows {
			for _, col := range row.Columns {
				if len(col.RegressedTests) == 0 {
					continue
				}
				foundRegression = true

				// Every regressed test should appear in AllTests with a test_details link
				for _, rt := range col.RegressedTests {
					assert.NotNil(t, rt.Links, "regressed test %s should have links", rt.TestName)
					assert.Contains(t, rt.Links, "test_details",
						"regressed test %s should have a test_details link", rt.TestName)
				}

				// AllTests should be a superset of RegressedTests
				assert.GreaterOrEqual(t, len(col.AllTests), len(col.RegressedTests),
					"AllTests should contain at least as many entries as RegressedTests")

				// Every entry in AllTests should have a test_details link
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
				// This cell has tests analyzed but none regressed
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

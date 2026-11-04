package componentreadiness

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
	"github.com/openshift/sippy/test/e2e/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSeededRegressionReportMatchesTestDetails checks that the seeded extreme
// regression test is reported identically by the component report and by the
// test details reached through the report's own test_details link. The report
// aggregates whole days while test details filters exact run timestamps, so the
// seed data must keep every sample run inside the rounded sample window.
func TestSeededRegressionReportMatchesTestDetails(t *testing.T) {
	const (
		viewName = "4.22-main"
		testID   = "test-extreme-regression"
	)

	var report componentreport.ComponentReport
	err := util.SippyGet(fmt.Sprintf("/api/component_readiness?view=%s", url.QueryEscape(viewName)), &report)
	require.NoError(t, err, "error fetching component report")

	var regressed []componentreport.ReportTestSummary
	for _, row := range report.Rows {
		for _, col := range row.Columns {
			for _, rt := range col.RegressedTests {
				if rt.TestID == testID {
					regressed = append(regressed, rt)
				}
			}
		}
	}
	// The seed regresses this test on aws/amd64, aws/arm64, and gcp/amd64.
	require.Len(t, regressed, 3, "expected the seeded regression in each seeded default-view variant")

	for _, rt := range regressed {
		name := fmt.Sprintf("%s/%s", rt.Variants["Platform"], rt.Variants["Architecture"])
		t.Run(name, func(t *testing.T) {
			link := rt.Links["test_details"]
			require.NotEmpty(t, link, "regressed test should have a test_details link")

			var details testdetails.Report
			if strings.HasPrefix(link, "http") {
				err = util.SippyGetAbsolute(link, &details)
			} else {
				err = util.SippyGet(link, &details)
			}
			require.NoError(t, err, "error following test_details link")
			require.NotEmpty(t, details.Analyses, "test details returned no analyses")
			analysis := details.Analyses[0]

			assert.Equal(t, rt.SampleStats.SuccessCount, analysis.SampleStats.SuccessCount, "sample successes")
			assert.Equal(t, rt.SampleStats.FailureCount, analysis.SampleStats.FailureCount, "sample failures")
			assert.Equal(t, rt.SampleStats.FlakeCount, analysis.SampleStats.FlakeCount, "sample flakes")
			assert.Equal(t, rt.ReportStatus, analysis.ReportStatus, "status")
			assert.LessOrEqual(t, int(analysis.ReportStatus), int(crtest.SignificantTriagedRegression),
				"test details should classify the seeded test as regressed, got %d", analysis.ReportStatus)
			assert.Equal(t, 200, analysis.SampleStats.SuccessCount+analysis.SampleStats.FailureCount+analysis.SampleStats.FlakeCount,
				"test details should include all seeded sample runs")
		})
	}
}

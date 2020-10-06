package installhtml

import (
	"fmt"
	"strings"

	"github.com/openshift/sippy/pkg/html/generichtml"

	"github.com/openshift/sippy/pkg/util"

	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"

	"github.com/openshift/sippy/pkg/util/sets"

	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

var individualInstallUpgradeColor = generichtml.ColorizationCriteria{
	MinRedPercent:    0,  // failure.  In this range, there is a systemic failure so severe that a reliable signal isn't available.
	MinYellowPercent: 90, // at risk.  In this range, there is a systemic problem that needs to be addressed.
	MinGreenPercent:  95, // no action required.
}

type currPrevTestResult struct {
	curr sippyprocessingv1.TestResult
	prev *sippyprocessingv1.TestResult
}

type currPrevFailedTestResult struct {
	curr sippyprocessingv1.FailingTestResult
	prev *sippyprocessingv1.FailingTestResult
}

func installOperatorTests(curr, prev sippyprocessingv1.TestReport) string {
	// test name | bug | pass rate | higher/lower | pass rate
	s := fmt.Sprintf(`
	<table class="table">
		<tr>
			<th colspan=%d class="text-center"><a class="text-dark" title="Operator installation, sorted by passing rate.  The link will prepopulate a BZ template to be filled out and submitted to report a bug against the test." id="InstallRatesByOperator" href="#InstallRatesByOperator">Install Rates by Operator</a></th>
		</tr>
	`,
		len(curr.ByPlatform)+2)

	operatorTests := map[string]*currPrevFailedTestResult{}
	for _, test := range curr.ByTest {
		if strings.HasPrefix(test.TestName, testgridanalysisapi.OperatorInstallPrefix) && test.TestResultAcrossAllJobs.Failures > 0 {
			operatorTests[test.TestName] = &currPrevFailedTestResult{curr: test}
			if prevTestResult := util.FindFailedTestResult(test.TestName, prev.ByTest); prevTestResult != nil {
				operatorTests[test.TestName].prev = prevTestResult
			}
		}
	}

	// now that we have the tests, let's run through all the platforms to pull the platform aggregation for each of the tests in question
	operatorTestsToPlatformToTestResult := map[string]map[string]*currPrevTestResult{}
	for testName := range operatorTests {
		if _, ok := operatorTestsToPlatformToTestResult[testName]; !ok {
			operatorTestsToPlatformToTestResult[testName] = map[string]*currPrevTestResult{}
		}
		for _, platform := range curr.ByPlatform {
			for _, test := range platform.AllTestResults {
				if test.Name != testName {
					continue
				}

				operatorTestsToPlatformToTestResult[testName][platform.PlatformName] = &currPrevTestResult{curr: test}
				if prevPlatform := util.FindPlatformResultsForName(platform.PlatformName, prev.ByPlatform); prevPlatform != nil {
					if prevTestResult := util.FindTestResult(test.Name, prevPlatform.AllTestResults); prevTestResult != nil {
						operatorTestsToPlatformToTestResult[testName][platform.PlatformName].prev = prevTestResult
					}
				}
				break
			}
		}
	}

	platformToInstallTestResult := map[string]*currPrevTestResult{}
	for _, platform := range curr.ByPlatform {
		for _, test := range platform.AllTestResults {
			if test.Name == testgridanalysisapi.InstallTestName {
				platformToInstallTestResult[platform.PlatformName] = &currPrevTestResult{curr: test}

				if prevPlatform := util.FindPlatformResultsForName(platform.PlatformName, prev.ByPlatform); prevPlatform != nil {
					if prevTestResult := util.FindTestResult(test.Name, prevPlatform.AllTestResults); prevTestResult != nil {
						platformToInstallTestResult[platform.PlatformName].prev = prevTestResult
					}
				}
				break
			}
		}
	}
	// print platform column headers
	s += "    <tr>"
	s += "      <td nowrap=\"nowrap\"></td>\n"
	s += "      <th nowrap=\"nowrap\" class=\"text-center\">All</th>\n"
	for _, platformName := range testidentification.AllPlatforms.List() {
		s += "      <th class=\"text-center\"><nobr>" + platformName + "</nobr></th>\n"
	}
	s += "		</tr>\n"

	// now the overall install results by platform
	s += "    <tr>"
	s += "      <td>Overall</td>\n"
	s += installCellHTMLFromFailedTestResult(&currPrevFailedTestResult{
		curr: *util.FindFailedTestResult(testgridanalysisapi.InstallTestName, curr.ByTest),
		prev: util.FindFailedTestResult(testgridanalysisapi.InstallTestName, prev.ByTest),
	}, generichtml.OverallInstallUpgradeColors)
	for _, platformName := range testidentification.AllPlatforms.List() {
		s += installCellHTMLFromTestResult(platformToInstallTestResult[platformName], generichtml.OverallInstallUpgradeColors)
	}
	s += "		</tr>"

	// now the main results by operator, by platform
	for _, testName := range sets.StringKeySet(operatorTests).List() {
		operatorName := strings.SplitN(testName, " ", 3)[2] // We happen to know the shape of this test name
		s += "    <tr>"
		s += "      <td class=\"\"><nobr>" + operatorName + "</nobr></td>\n"
		s += installCellHTMLFromFailedTestResult(operatorTests[testName], individualInstallUpgradeColor)
		platformResults := operatorTestsToPlatformToTestResult[testName]
		for _, platformName := range testidentification.AllPlatforms.List() {
			s += installCellHTMLFromTestResult(platformResults[platformName], individualInstallUpgradeColor)
		}
		s += "		</tr>"
	}

	s = s + "</table>"

	return s
}

func installCellHTMLFromTestResult(cellResult *currPrevTestResult, colors generichtml.ColorizationCriteria) string {
	if cellResult == nil {
		// we filter out 100% passing results, so this almost certainly means we always pass.  We default to 100
		passPercentage := 100.0
		arrow := generichtml.Flat
		color := colors.GetColor(passPercentage)
		return fmt.Sprintf("      <td class=\"text-center %v\"><nobr>%0.2f%% %v NA</nobr></td>", color, passPercentage, arrow)
	}

	// we filter out 100% passing results, so this almost certainly means we always pass.  We default to 100
	passPercentage := cellResult.curr.PassPercentage
	arrow := generichtml.GetArrowForTestResult(cellResult.curr, cellResult.prev)
	color := colors.GetColor(passPercentage)
	if cellResult.prev == nil {
		return fmt.Sprintf("      <td class=\"text-center %v\"><nobr>%0.2f%% %v NA</nobr></td>", color, passPercentage, arrow)
	}

	return fmt.Sprintf("      <td class=\"text-center %v\"><nobr>%0.2f%% %v %0.2f%% </nobr></td>", color, passPercentage, arrow, cellResult.prev.PassPercentage)
}

func installCellHTMLFromFailedTestResult(cellResult *currPrevFailedTestResult, colors generichtml.ColorizationCriteria) string {
	var prevTestResult *sippyprocessingv1.TestResult
	if cellResult.prev != nil {
		prevTestResult = &cellResult.prev.TestResultAcrossAllJobs
	}
	testResultCell := currPrevTestResult{
		curr: cellResult.curr.TestResultAcrossAllJobs,
		prev: prevTestResult,
	}
	return installCellHTMLFromTestResult(&testResultCell, colors)
}

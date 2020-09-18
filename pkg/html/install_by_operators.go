package html

import (
	"fmt"
	"strings"

	"github.com/openshift/sippy/pkg/util"

	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"

	"github.com/openshift/sippy/pkg/util/sets"

	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

var individualInstallUpgradeColor = colorizationCriteria{
	minRedPercent:    0,  // failure.  In this range, there is a systemic failure so severe that a reliable signal isn't available.
	minYellowPercent: 90, // at risk.  In this range, there is a systemic problem that needs to be addressed.
	minGreenPercent:  95, // no action required.
}

type currPrevTestResult struct {
	curr sippyprocessingv1.TestResult
	prev *sippyprocessingv1.TestResult
}

type currPrevFailedTestResult struct {
	curr sippyprocessingv1.FailingTestResult
	prev *sippyprocessingv1.FailingTestResult
}

func installOperatorTests(curr, prev sippyprocessingv1.TestReport, endDay int, release string) string {
	// test name | bug | pass rate | higher/lower | pass rate
	s := fmt.Sprintf(`
	<table class="table">
		<tr>
			<th colspan=5 class="text-center"><a class="text-dark" title="Operator installation, sorted by passing rate.  The link will prepopulate a BZ template to be filled out and submitted to report a bug against the test." id="OperatorInstall" href="#OperatorInstall">Operator Install</a></th>
		</tr>
	`)

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
	overallInstallTestResult := util.FindFailedTestResult(testgridanalysisapi.InstallTestName, curr.ByTest)
	prevOverallInstallTestResult := util.FindFailedTestResult(testgridanalysisapi.InstallTestName, prev.ByTest)
	arrow := getArrowForFailedTestResult(*overallInstallTestResult, prevOverallInstallTestResult)
	color := overallInstallUpgradeColors.getColor(overallInstallTestResult.TestResultAcrossAllJobs.PassPercentage)
	s += fmt.Sprintf("      <td class=\"text-center %v\"><nobr>%0.2f%% %v</nobr></td>", color, overallInstallTestResult.TestResultAcrossAllJobs.PassPercentage, arrow)
	for _, platformName := range testidentification.AllPlatforms.List() {
		platformTestResult, ok := platformToInstallTestResult[platformName]
		// we filter out 100% passing results, so this almost certainly means we always pass.  We default to 100
		passPercentage := 100.0
		arrow := flatdown
		if ok {
			passPercentage = platformTestResult.curr.PassPercentage
			arrow = getArrowForTestResult(platformTestResult.curr, platformTestResult.prev)
		}
		color := overallInstallUpgradeColors.getColor(passPercentage)
		s += fmt.Sprintf("      <td class=\"text-center %v\"><nobr>%0.2f%% %v</nobr></td>", color, passPercentage, arrow)
	}
	s += "		</tr>"

	// now the main results by operator, by platform
	for _, testName := range sets.StringKeySet(operatorTests).List() {
		operatorName := strings.SplitN(testName, " ", 3)[2] // We happen to know the shape of this test name
		s += "    <tr>"
		s += "      <td class=\"\"><nobr>" + operatorName + "</nobr></td>\n"
		arrow := getArrowForFailedTestResult(operatorTests[testName].curr, operatorTests[testName].prev)
		color := individualInstallUpgradeColor.getColor(operatorTests[testName].curr.TestResultAcrossAllJobs.PassPercentage)
		s += fmt.Sprintf("      <td class=\"text-center %v\"><nobr>%0.2f%% %v</nobr></td>\n", color, operatorTests[testName].curr.TestResultAcrossAllJobs.PassPercentage, arrow)
		platformResults := operatorTestsToPlatformToTestResult[testName]
		for _, platformName := range testidentification.AllPlatforms.List() {
			platformTestResult, ok := platformResults[platformName]
			// we filter out 100% passing results, so this almost certainly means we always pass.  We default to 100
			passPercentage := 100.0
			arrow := flatdown
			if ok {
				passPercentage = platformTestResult.curr.PassPercentage
				arrow = getArrowForTestResult(platformTestResult.curr, platformTestResult.prev)
			}
			color := individualInstallUpgradeColor.getColor(passPercentage)
			s += fmt.Sprintf("      <td class=\"text-center %v\"><nobr>%0.2f%% %v</nobr></td>", color, passPercentage, arrow)
		}
		s += "		</tr>"
	}

	s = s + "</table>"

	return s
}

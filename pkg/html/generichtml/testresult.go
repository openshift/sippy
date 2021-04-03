package generichtml

import (
	"fmt"
	"net/url"
	"regexp"

	bugsv1 "github.com/openshift/sippy/pkg/apis/bugs/v1"
	"k8s.io/klog"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

func TestResultHasResults(in sippyprocessingv1.TestResult) bool {
	if in.Successes == 0 && in.Failures == 0 && in.Flakes == 0 {
		return false
	}
	return true
}

func FailingTestResultHasResults(in sippyprocessingv1.FailingTestResult) bool {
	return TestResultHasResults(in.TestResultAcrossAllJobs)
}

type testResultDisplay struct {
	displayName       string
	displayPercent    float64
	totalRuns         int
	flakedRuns        int
	bugList           []bugsv1.Bug
	associatedBugList []bugsv1.Bug

	jobResults []jobResultDisplay
}

func testResultToDisplay(in sippyprocessingv1.TestResult) testResultDisplay {
	ret := testResultDisplay{
		displayName:    in.Name,
		displayPercent: in.PassPercentage,
		totalRuns:      in.Successes + in.Failures,
		flakedRuns:     in.Flakes,
	}
	for _, bug := range in.BugList {
		ret.bugList = append(ret.bugList, bug)
	}
	for _, bug := range in.AssociatedBugList {
		ret.associatedBugList = append(ret.associatedBugList, bug)
	}
	return ret
}

func failedTestResultToDisplay(in sippyprocessingv1.FailingTestResult) testResultDisplay {
	ret := testResultToDisplay(in.TestResultAcrossAllJobs)

	for _, jobResult := range in.JobResults {
		ret.jobResults = append(ret.jobResults, failingJobResultToDisplay(jobResult))
	}
	return ret
}

type testResultRenderBuilder struct {
	// sectionBlock needs to be unique for each part of the report.  It is used to uniquely name the collapse/expand
	// sections so they open properly
	sectionBlock string

	currTestResult testResultDisplay
	prevTestResult *testResultDisplay

	release             string
	maxJobResultsToShow int
	colors              ColorizationCriteria
	startCollapsedBool  bool
	baseIndentDepth     int
}

func NewTestResultRenderer(sectionBlock string, curr testResultDisplay, release string) *testResultRenderBuilder {
	return &testResultRenderBuilder{
		sectionBlock:        sectionBlock,
		currTestResult:      curr,
		release:             release,
		maxJobResultsToShow: 10, // just a default, can be overridden
		colors: ColorizationCriteria{
			MinRedPercent:    0,  // failure.  In this range, there is a systemic failure so severe that a reliable signal isn't available.
			MinYellowPercent: 60, // at risk.  In this range, there is a systemic problem that needs to be addressed.
			MinGreenPercent:  80, // no action required. This *should* be closer to 85%
		},
	}
}

func NewTestResultRendererForTestResult(sectionBlock string, curr sippyprocessingv1.TestResult, release string) *testResultRenderBuilder {
	return NewTestResultRenderer(sectionBlock, testResultToDisplay(curr), release)
}

func NewTestResultRendererForFailedTestResult(sectionBlock string, curr sippyprocessingv1.FailingTestResult, release string) *testResultRenderBuilder {
	return NewTestResultRenderer(sectionBlock, failedTestResultToDisplay(curr), release)
}

func (b *testResultRenderBuilder) WithPrevious(prev *testResultDisplay) *testResultRenderBuilder {
	b.prevTestResult = prev
	return b
}

func (b *testResultRenderBuilder) WithPreviousTestResult(prev *sippyprocessingv1.TestResult) *testResultRenderBuilder {
	if prev == nil {
		b.prevTestResult = nil
		return b
	}
	t := testResultToDisplay(*prev)
	b.prevTestResult = &t
	return b
}

func (b *testResultRenderBuilder) WithPreviousFailedTestResult(prev *sippyprocessingv1.FailingTestResult) *testResultRenderBuilder {
	if prev == nil {
		b.prevTestResult = nil
		return b
	}
	t := failedTestResultToDisplay(*prev)
	b.prevTestResult = &t
	return b
}

func (b *testResultRenderBuilder) WithMaxJobResultsToShow(maxTestResultsToShow int) *testResultRenderBuilder {
	b.maxJobResultsToShow = maxTestResultsToShow
	return b
}

func (b *testResultRenderBuilder) WithColors(colors ColorizationCriteria) *testResultRenderBuilder {
	b.colors = colors
	return b
}

func (b *testResultRenderBuilder) WithIndent(depth int) *testResultRenderBuilder {
	b.baseIndentDepth = depth
	return b
}

func (b *testResultRenderBuilder) StartCollapsed() *testResultRenderBuilder {
	b.startCollapsedBool = true
	return b
}

func (b *testResultRenderBuilder) ToHTML() string {
	indentDepth := (b.baseIndentDepth)*50 + 10

	template := `
		<tr class="%s">
			<td style="padding-left:%dpx">
				%s
				%s
			</td>
			<td>%s</td><td>%0.2f%% <span class="text-nowrap">(%d runs, %d flakes)</span></td><td>%s</td><td>%0.2f%% <span class="text-nowrap">(%d runs, %d flakes)</span></td>
		</tr>
	`
	naTemplate := `
		<tr class="%s">
			<td style="padding-left:%dpx">
				%s
				%s
			</td>
			<td>%s</td><td>%0.2f%% <span class="text-nowrap">(%d runs, %d flakes)</span></td><td/><td>NA</td>
		</tr>
	`

	class := ""
	if b.startCollapsedBool {
		class += "collapse " + b.sectionBlock
	}

	jobCollapseSectionName := MakeSafeForCollapseName("test-result---" + b.sectionBlock + "---" + b.currTestResult.displayName)
	button := ""
	if len(b.currTestResult.jobResults) > 0 {
		button += `				<p>` + GetExpandingButtonHTML(jobCollapseSectionName, "Expand Failing Jobs") + " " + GetTestDetailsButtonHTML(b.release, b.currTestResult.displayName)
	} else {
		button += `				<p>` + GetTestDetailsButtonHTML(b.release, b.currTestResult.displayName)
	}

	s := ""

	encodedTestName := url.QueryEscape(regexp.QuoteMeta(b.currTestResult.displayName))
	testLink := fmt.Sprintf("<a target=\"_blank\" href=\"https://search.ci.openshift.org/?maxAge=168h&context=1&type=bug%%2Bjunit&name=%s&maxMatches=5&maxBytes=20971520&groupBy=job&search=%s\">%s</a>", b.release, encodedTestName, b.currTestResult.displayName)

	klog.V(2).Infof("processing top failing tests %s, bugs: %v", b.currTestResult.displayName, b.currTestResult.bugList)
	bugHTML := bugHTMLForTest(b.currTestResult.bugList, b.currTestResult.associatedBugList, b.release, b.currTestResult.displayName)
	if b.prevTestResult != nil {
		arrow := GetArrow(b.currTestResult.totalRuns, b.currTestResult.displayPercent, b.prevTestResult.displayPercent)

		s += fmt.Sprintf(template, class, indentDepth, testLink, button, bugHTML, b.currTestResult.displayPercent, b.currTestResult.totalRuns, b.currTestResult.flakedRuns, arrow, b.prevTestResult.displayPercent, b.prevTestResult.totalRuns, b.prevTestResult.flakedRuns)
	} else {
		s += fmt.Sprintf(naTemplate, class, indentDepth, testLink, button, bugHTML, b.currTestResult.displayPercent, b.currTestResult.totalRuns, b.currTestResult.flakedRuns)
	}

	// if we have no jobresults we're done
	if len(b.currTestResult.jobResults) == 0 {
		return s
	}

	jobIndentDepth := 50 + 10
	count := 10
	rowCount := 0
	rows := ""
	additionalMatches := 0
	for _, failingTestJobResult := range b.currTestResult.jobResults {
		if count == 0 {
			additionalMatches++
			continue
		}
		count--

		var prevTestJobResult *jobResultDisplay
		if b.prevTestResult != nil {
			for _, prevJobInstance := range b.prevTestResult.jobResults {
				if prevJobInstance.displayName == failingTestJobResult.displayName {
					prevTestJobResult = &prevJobInstance
					break
				}
			}
		}

		rows = rows + NewJobResultRenderer(jobCollapseSectionName, failingTestJobResult, b.release).
			WithIndent(b.baseIndentDepth+1).
			WithPrevious(prevTestJobResult).
			StartCollapsed().
			ToHTML()

		rowCount++
	}

	if additionalMatches > 0 {
		rows += fmt.Sprintf(`<tr class="collapse %s"><td colspan=2 style="padding-left:%dpx">Plus %d more jobs</td></tr>`, jobCollapseSectionName, jobIndentDepth, additionalMatches)
	}
	if rowCount > 0 {
		s = s + fmt.Sprintf(`<tr class="collapse %s"><td colspan=2 style="padding-left:%dpx" class="font-weight-bold">Job Name</td><td class="font-weight-bold">Job Pass Rate</td></tr>`, jobCollapseSectionName, jobIndentDepth)
		s = s + rows
		s = s + fmt.Sprintf(`<tr class="collapse %s"><td colspan=2 style="padding-left:60px" class="font-weight-bold"></td><td class="font-weight-bold"></td></tr>`, jobCollapseSectionName)
	} else {
		s = s + fmt.Sprintf(`<tr class="collapse %s"><td colspan=3 style="padding-left:%dpx" class="font-weight-bold">No Jobs Matched Filters</td></tr>`, jobCollapseSectionName, jobIndentDepth)
	}

	return s
}

// returns the table row html and a list of tests displayed
func getTestRowHTML(release, testsCollapseName string, currTestResults, prevTestResults []testResultDisplay, maxTestResultsToShow int) (string, []string) {
	s := ""
	testNames := []string{}

	testCount := maxTestResultsToShow
	testRowCount := 0
	testRows := ""
	for _, test := range currTestResults {
		if testCount <= 0 {
			break
		}
		// if the test isn't failing and isn't flaking, skip it. We do want to see the flakes, so keep iterating.
		if test.displayPercent > 99.99 && test.flakedRuns == 0 {
			continue
		}
		testCount--
		testNames = append(testNames, test.displayName)

		var prev *testResultDisplay
		for _, prevInstance := range prevTestResults {
			if prevInstance.displayName == test.displayName {
				prev = &prevInstance
				break
			}
		}

		testRows = testRows +
			NewTestResultRenderer(testsCollapseName, test, release).
				WithIndent(1).
				WithPrevious(prev).
				StartCollapsed().
				ToHTML()

		testRowCount++
	}
	additionalMatches := len(currTestResults) - len(testNames)

	if additionalMatches > 0 {
		testRows += fmt.Sprintf(`<tr class="collapse %s"><td colspan=2 style="padding-left:60px">Plus %d more tests</td></tr>`, testsCollapseName, additionalMatches)
	}
	if testRowCount > 0 {
		s = s + fmt.Sprintf(`<tr class="collapse %s"><td colspan=2 style="padding-left:60px" class="font-weight-bold">Test Name</td><td class="font-weight-bold">Test Pass Rate</td></tr>`, testsCollapseName)
		s = s + testRows
		s = s + fmt.Sprintf(`<tr class="collapse %s"><td colspan=2 style="padding-left:60px" class="font-weight-bold"></td><td class="font-weight-bold"></td></tr>`, testsCollapseName)
	} else {
		s = s + fmt.Sprintf(`<tr class="collapse %s"><td colspan=3 style="padding-left:60px" class="font-weight-bold">No Tests Matched Filters</td></tr>`, testsCollapseName)
	}

	return s, testNames
}

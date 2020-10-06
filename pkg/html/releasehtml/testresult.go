package releasehtml

import (
	"fmt"
	"net/url"
	"regexp"

	bugsv1 "github.com/openshift/sippy/pkg/apis/bugs/v1"
	"k8s.io/klog"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

type testResultDisplay struct {
	displayName    string
	displayPercent float64
	totalRuns      int
	flakedRuns     int
	bugList        []bugsv1.Bug

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
	colors              generichtml.colorizationCriteria
	startCollapsedBool  bool
	baseIndentDepth     int
}

func newTestResultRenderer(sectionBlock string, curr testResultDisplay, release string) *testResultRenderBuilder {
	return &testResultRenderBuilder{
		sectionBlock:        sectionBlock,
		currTestResult:      curr,
		release:             release,
		maxJobResultsToShow: 10, // just a default, can be overridden
		colors: generichtml.colorizationCriteria{
			minRedPercent:    0,  // failure.  In this range, there is a systemic failure so severe that a reliable signal isn't available.
			minYellowPercent: 60, // at risk.  In this range, there is a systemic problem that needs to be addressed.
			minGreenPercent:  80, // no action required. This *should* be closer to 85%
		},
	}
}

func newTestResultRendererForTestResult(sectionBlock string, curr sippyprocessingv1.TestResult, release string) *testResultRenderBuilder {
	return newTestResultRenderer(sectionBlock, testResultToDisplay(curr), release)
}

func newTestResultRendererForFailedTestResult(sectionBlock string, curr sippyprocessingv1.FailingTestResult, release string) *testResultRenderBuilder {
	return newTestResultRenderer(sectionBlock, failedTestResultToDisplay(curr), release)
}

func (b *testResultRenderBuilder) withPrevious(prev *testResultDisplay) *testResultRenderBuilder {
	b.prevTestResult = prev
	return b
}

func (b *testResultRenderBuilder) withPreviousTestResult(prev *sippyprocessingv1.TestResult) *testResultRenderBuilder {
	if prev == nil {
		b.prevTestResult = nil
		return b
	}
	t := testResultToDisplay(*prev)
	b.prevTestResult = &t
	return b
}

func (b *testResultRenderBuilder) withPreviousFailedTestResult(prev *sippyprocessingv1.FailingTestResult) *testResultRenderBuilder {
	if prev == nil {
		b.prevTestResult = nil
		return b
	}
	t := failedTestResultToDisplay(*prev)
	b.prevTestResult = &t
	return b
}

func (b *testResultRenderBuilder) withMaxJobResultsToShow(maxTestResultsToShow int) *testResultRenderBuilder {
	b.maxJobResultsToShow = maxTestResultsToShow
	return b
}

func (b *testResultRenderBuilder) withColors(colors generichtml.colorizationCriteria) *testResultRenderBuilder {
	b.colors = colors
	return b
}

func (b *testResultRenderBuilder) withIndent(depth int) *testResultRenderBuilder {
	b.baseIndentDepth = depth
	return b
}

func (b *testResultRenderBuilder) startCollapsed() *testResultRenderBuilder {
	b.startCollapsedBool = true
	return b
}

func (b *testResultRenderBuilder) toHTML() string {
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

	jobCollapseSectionName := generichtml.makeSafeForCollapseName("test-result---" + b.sectionBlock + "---" + b.currTestResult.displayName)
	button := ""
	if len(b.currTestResult.jobResults) > 0 {
		button = "<p>" + generichtml.getButtonHTML(jobCollapseSectionName, "Expand Failing Jobs")
	}

	// test name | bug | pass rate | higher/lower | pass rate
	s := ""

	encodedTestName := url.QueryEscape(regexp.QuoteMeta(b.currTestResult.displayName))
	testLink := fmt.Sprintf("<a target=\"_blank\" href=\"https://search.ci.openshift.org/?maxAge=168h&context=1&type=bug%%2Bjunit&name=%s&maxMatches=5&maxBytes=20971520&groupBy=job&search=%s\">%s</a>", b.release, encodedTestName, b.currTestResult.displayName)

	klog.V(2).Infof("processing top failing tests %s, bugs: %v", b.currTestResult.displayName, b.currTestResult.bugList)
	bugHTML := bugHTMLForTest(b.currTestResult.bugList, b.release, "", b.currTestResult.displayName)
	if b.prevTestResult != nil {
		arrow := generichtml.getArrow(b.currTestResult.totalRuns, b.currTestResult.displayPercent, b.prevTestResult.displayPercent)

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

		rows = rows + newJobResultRenderer(jobCollapseSectionName, failingTestJobResult, b.release).
			withIndent(b.baseIndentDepth+1).
			withPrevious(prevTestJobResult).
			startCollapsed().
			toHTML()

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

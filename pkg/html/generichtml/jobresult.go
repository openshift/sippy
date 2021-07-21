package generichtml

import (
	"fmt"

	bugsv1 "github.com/openshift/sippy/pkg/apis/bugs/v1"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

type jobResultDisplay struct {
	displayName            string
	testGridURL            string
	displayPercent         float64
	parenDisplayPercentage float64
	totalRuns              int
	bugList                []bugsv1.Bug
	associatedBugList      []bugsv1.Bug

	testResults []testResultDisplay
}

type jobResultRenderBuilder struct {
	// sectionBlock needs to be unique for each part of the report.  It is used to uniquely name the collapse/expand
	// sections so they open properly
	sectionBlock string

	currJobResult jobResultDisplay
	prevJobResult *jobResultDisplay

	release              string
	maxTestResultsToShow int
	colors               ColorizationCriteria
	startCollapsedBool   bool
	baseIndentDepth      int
}

func jobResultToDisplay(in sippyprocessingv1.JobResult) jobResultDisplay {
	ret := jobResultDisplay{
		displayName:            in.Name,
		testGridURL:            in.TestGridUrl,
		displayPercent:         in.PassPercentage,
		parenDisplayPercentage: in.PassPercentageWithoutInfrastructureFailures,
		totalRuns:              in.Successes + in.Failures,
	}

	for _, testResult := range in.TestResults {
		ret.testResults = append(ret.testResults, testResultToDisplay(testResult))
	}

	ret.bugList = append(ret.bugList, in.BugList...)

	ret.associatedBugList = append(ret.associatedBugList, in.AssociatedBugList...)

	return ret
}

func bugzillaJobResultToDisplay(in sippyprocessingv1.BugzillaJobResult) jobResultDisplay {
	ret := jobResultDisplay{
		displayName:            in.JobName,
		testGridURL:            in.JobName,
		displayPercent:         100.0 - in.FailPercentage,
		parenDisplayPercentage: 100.0 - in.FailPercentage,
		totalRuns:              in.TotalRuns,
	}

	for _, testResult := range in.Failures {
		ret.testResults = append(ret.testResults, testResultToDisplay(testResult))
	}

	return ret
}

func failingJobResultToDisplay(in sippyprocessingv1.FailingTestJobResult) jobResultDisplay {
	ret := jobResultDisplay{
		displayName:    in.Name,
		testGridURL:    in.TestGridUrl,
		displayPercent: in.PassPercentage,
		// TODO gather this info
		//displayPercentWithoutInfraFailures: in.PassPercentageWithoutInfrastructureFailures,
		totalRuns: in.TestSuccesses + in.TestFailures,
	}

	return ret
}

func NewJobResultRenderer(sectionBlock string, curr jobResultDisplay, release string) *jobResultRenderBuilder {
	return &jobResultRenderBuilder{
		sectionBlock:         sectionBlock,
		currJobResult:        curr,
		release:              release,
		maxTestResultsToShow: 10, // just a default, can be overridden
		colors: ColorizationCriteria{
			MinRedPercent:    0,  // failure.  In this range, there is a systemic failure so severe that a reliable signal isn't available.
			MinYellowPercent: 60, // at risk.  In this range, there is a systemic problem that needs to be addressed.
			MinGreenPercent:  80, // no action required. This *should* be closer to 85%
		},
	}
}

func NewJobResultRendererFromJobResult(sectionBlock string, curr sippyprocessingv1.JobResult, release string) *jobResultRenderBuilder {
	return NewJobResultRenderer(sectionBlock, jobResultToDisplay(curr), release)
}

func (b *jobResultRenderBuilder) WithPrevious(prevJobResult *jobResultDisplay) *jobResultRenderBuilder {
	b.prevJobResult = prevJobResult
	return b
}

func (b *jobResultRenderBuilder) WithPreviousJobResult(prevJobResult *sippyprocessingv1.JobResult) *jobResultRenderBuilder {
	if prevJobResult == nil {
		b.prevJobResult = nil
		return b
	}
	t := jobResultToDisplay(*prevJobResult)
	return b.WithPrevious(&t)
}

func (b *jobResultRenderBuilder) WithMaxTestResultsToShow(maxTestResultsToShow int) *jobResultRenderBuilder {
	b.maxTestResultsToShow = maxTestResultsToShow
	return b
}

func (b *jobResultRenderBuilder) WithColors(colors ColorizationCriteria) *jobResultRenderBuilder {
	b.colors = colors
	return b
}

func (b *jobResultRenderBuilder) WithIndent(depth int) *jobResultRenderBuilder {
	b.baseIndentDepth = depth
	return b
}

func (b *jobResultRenderBuilder) StartCollapsed() *jobResultRenderBuilder {
	b.startCollapsedBool = true
	return b
}

func (b *jobResultRenderBuilder) ToHTML() string {
	testCollapseSectionName := MakeSafeForCollapseName(b.sectionBlock + "---" + b.currJobResult.displayName + "---tests")

	s := ""

	// TODO either make this a template or make this a builder that takes args and then has branches.
	//  that will fix the funny link that goes nowhere.
	template := `
			<tr class="%s">
				<td style="padding-left:%dpx">
					<a target="_blank" href="%s">%s</a>
					%s
				</td>
				<td>
					%s
				</td>
				<td>
					%0.2f%% (%0.2f%%)<span class="text-nowrap">(%d runs)</span>
				</td>
				<td>
					%s
				</td>
				<td>
					%0.2f%% (%0.2f%%)<span class="text-nowrap">(%d runs)</span>
				</td>
			</tr>
		`

	naTemplate := `
			<tr class="%s">
				<td style="padding-left:%dpx">
					<a target="_blank" href="%s">%s</a>
					%s
				</td>
				<td>
					%s
				</td>
				<td>
					%0.2f%% (%0.2f%%)<span class="text-nowrap">(%d runs)</span>
				</td>
				<td/>
				<td>
					NA
				</td>
			</tr>
		`

	class := b.colors.GetColor(b.currJobResult.displayPercent, b.currJobResult.totalRuns)
	if b.startCollapsedBool {
		class += " collapse " + b.sectionBlock
	}

	prevTestResults := []testResultDisplay{}
	if b.prevJobResult != nil {
		prevTestResults = b.prevJobResult.testResults
	}

	testRows, displayedTests := getTestRowHTML(b.release, testCollapseSectionName, b.currJobResult.testResults, prevTestResults, b.maxTestResultsToShow)

	button := ""
	if len(displayedTests) > 0 {
		button = "<p>" + GetExpandingButtonHTML(testCollapseSectionName, "Expand Failing Tests") + " " + GetTestDetailsButtonHTML(b.release, displayedTests...)
	}

	bugHTML := bugHTMLForJob(b.currJobResult.bugList, b.currJobResult.associatedBugList, b.release, b.currJobResult.displayName, b.currJobResult.testGridURL)

	if b.prevJobResult != nil {
		arrow := GetArrow(b.currJobResult.totalRuns, b.currJobResult.displayPercent, b.prevJobResult.displayPercent)

		s = s + fmt.Sprintf(template,
			class, b.baseIndentDepth*50+10,
			b.currJobResult.testGridURL, b.currJobResult.displayName, button,
			bugHTML,
			b.currJobResult.displayPercent,
			b.currJobResult.parenDisplayPercentage,
			b.currJobResult.totalRuns,
			arrow,
			b.prevJobResult.displayPercent,
			b.prevJobResult.parenDisplayPercentage,
			b.prevJobResult.totalRuns,
		)
	} else {
		s = s + fmt.Sprintf(naTemplate,
			class, b.baseIndentDepth*50+10,
			b.currJobResult.testGridURL, b.currJobResult.displayName, button,
			bugHTML,
			b.currJobResult.displayPercent,
			b.currJobResult.parenDisplayPercentage,
			b.currJobResult.totalRuns,
		)
	}

	// if we have no test results, we're done
	if len(b.currJobResult.testResults) == 0 {
		return s
	}
	s += testRows

	return s
}

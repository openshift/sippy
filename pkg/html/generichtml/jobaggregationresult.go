package generichtml

import (
	"fmt"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

// VariantResults
type jobAggregationDisplay struct {
	displayName            string
	totalJobRuns           int
	displayPercentage      float64
	parenDisplayPercentage float64

	// jobResults for all jobs that match this variant, ordered by lowest JobRunPassPercentage to highest
	jobResults []jobResultDisplay

	// TestResults holds entries for each test that is a part of this aggregation.  Each entry aggregates the results of all runs of a single test.  The array is sorted from lowest JobRunPassPercentage to highest JobRunPassPercentage
	testResults []testResultDisplay
}

func variantResultToDisplay(in sippyprocessingv1.VariantResults) jobAggregationDisplay {
	ret := jobAggregationDisplay{
		displayName:            in.VariantName,
		totalJobRuns:           in.JobRunSuccesses + in.JobRunFailures,
		displayPercentage:      in.JobRunPassPercentage,
		parenDisplayPercentage: in.JobRunPassPercentageWithoutInfrastructureFailures,
	}

	for _, jobResult := range in.JobResults {
		ret.jobResults = append(ret.jobResults, jobResultToDisplay(jobResult))
	}
	for _, testResult := range in.AllTestResults {
		ret.testResults = append(ret.testResults, testResultToDisplay(testResult))
	}

	return ret
}

func bugzillaComponentReportToDisplay(in sippyprocessingv1.SortedBugzillaComponentResult) jobAggregationDisplay {
	worstJob := in.JobsFailed[0]
	ret := jobAggregationDisplay{
		displayName:            in.Name,
		totalJobRuns:           worstJob.TotalRuns,
		displayPercentage:      100.0 - worstJob.FailPercentage,
		parenDisplayPercentage: 100.0 - worstJob.FailPercentage, // this is the same because infrastructure isn't different for these.
	}

	for _, jobResult := range in.JobsFailed {
		ret.jobResults = append(ret.jobResults, bugzillaJobResultToDisplay(jobResult))
	}

	return ret
}

type jobAggregationResultRenderBuilder struct {
	// sectionBlock needs to be unique for each part of the report.  It is used to uniquely name the collapse/expand
	// sections so they open properly
	sectionBlock string

	currAggregationResult jobAggregationDisplay
	prevAggregationResult *jobAggregationDisplay

	release              string
	maxTestResultsToShow int
	maxJobResultsToShow  int
	colors               ColorizationCriteria
	collapsedAs          string
}

func NewJobAggregationResultRenderer(sectionBlock string, currJobResult jobAggregationDisplay, release string) *jobAggregationResultRenderBuilder {
	return &jobAggregationResultRenderBuilder{
		sectionBlock:          sectionBlock,
		currAggregationResult: currJobResult,
		release:               release,
		maxTestResultsToShow:  10, // just a default, can be overridden
		maxJobResultsToShow:   10, // just a default, can be overridden
		colors: ColorizationCriteria{
			MinRedPercent:    0,  // failure.  In this range, there is a systemic failure so severe that a reliable signal isn't available.
			MinYellowPercent: 60, // at risk.  In this range, there is a systemic problem that needs to be addressed.
			MinGreenPercent:  80, // no action required. This *should* be closer to 85%
		},
	}
}

func NewJobAggregationResultRendererFromVariantResults(sectionBlock string, curr sippyprocessingv1.VariantResults, release string) *jobAggregationResultRenderBuilder {
	return NewJobAggregationResultRenderer(sectionBlock, variantResultToDisplay(curr), release)
}

func NewJobAggregationResultRendererFromBugzillaComponentResult(sectionBlock string, curr sippyprocessingv1.SortedBugzillaComponentResult, release string) *jobAggregationResultRenderBuilder {
	return NewJobAggregationResultRenderer(sectionBlock, bugzillaComponentReportToDisplay(curr), release)
}

func (b *jobAggregationResultRenderBuilder) WithPrevious(prevJobResult *jobAggregationDisplay) *jobAggregationResultRenderBuilder {
	b.prevAggregationResult = prevJobResult
	return b
}

func (b *jobAggregationResultRenderBuilder) WithPreviousVariantResults(prev *sippyprocessingv1.VariantResults) *jobAggregationResultRenderBuilder {
	if prev == nil {
		b.prevAggregationResult = nil
		return b
	}
	t := variantResultToDisplay(*prev)
	b.prevAggregationResult = &t
	return b
}

func (b *jobAggregationResultRenderBuilder) WithPreviousBugzillaComponentResult(prev *sippyprocessingv1.SortedBugzillaComponentResult) *jobAggregationResultRenderBuilder {
	if prev == nil {
		b.prevAggregationResult = nil
		return b
	}
	t := bugzillaComponentReportToDisplay(*prev)
	b.prevAggregationResult = &t
	return b
}

func (b *jobAggregationResultRenderBuilder) WithMaxTestResultsToShow(maxTestResultsToShow int) *jobAggregationResultRenderBuilder {
	b.maxTestResultsToShow = maxTestResultsToShow
	return b
}

func (b *jobAggregationResultRenderBuilder) WithMaxJobResultsToShow(maxJobResultsToShow int) *jobAggregationResultRenderBuilder {
	b.maxJobResultsToShow = maxJobResultsToShow
	return b
}

func (b *jobAggregationResultRenderBuilder) WithColors(colors ColorizationCriteria) *jobAggregationResultRenderBuilder {
	b.colors = colors
	return b
}

func (b *jobAggregationResultRenderBuilder) StartCollapsedAs(collapsedAs string) *jobAggregationResultRenderBuilder {
	b.collapsedAs = collapsedAs
	return b
}

func (b *jobAggregationResultRenderBuilder) ToHTML() string {

	s := ""

	// TODO either make this a template or make this a builder that takes args and then has branches.
	//  that will fix the funny link that goes nowhere.
	template := `
			<tr class="%s">
				<td>
					%s
					<p>
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
				<td>
					%s
					<p>
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

	class := b.colors.GetColor(b.currAggregationResult.displayPercentage, b.currAggregationResult.totalJobRuns)
	if len(b.collapsedAs) > 0 {
		class += " collapse " + b.collapsedAs
	}

	prevTestResults := []testResultDisplay{}
	if b.prevAggregationResult != nil {
		prevTestResults = b.prevAggregationResult.testResults
	}

	testCollapseSectionName := MakeSafeForCollapseName(b.sectionBlock + "---" + b.currAggregationResult.displayName + "---tests")
	jobsCollapseName := MakeSafeForCollapseName(b.sectionBlock + "---" + b.currAggregationResult.displayName + "---jobs")
	testRows, displayedTests := getTestRowHTML(b.release, testCollapseSectionName, b.currAggregationResult.testResults, prevTestResults, b.maxTestResultsToShow)
	button := "					" + GetExpandingButtonHTML(jobsCollapseName, "Expand Failing Jobs")
	if len(displayedTests) > 0 { // add the button if we have tests to show
		button += " " + GetExpandingButtonHTML(testCollapseSectionName, "Expand Failing Tests")
		button += " " + GetTestDetailsButtonHTML(b.release, displayedTests...)
	}

	if b.prevAggregationResult != nil {
		arrow := GetArrow(b.currAggregationResult.totalJobRuns, b.currAggregationResult.displayPercentage, b.prevAggregationResult.displayPercentage)

		s += fmt.Sprintf(template,
			class,
			b.currAggregationResult.displayName,
			button,
			b.currAggregationResult.displayPercentage,
			b.currAggregationResult.parenDisplayPercentage,
			b.currAggregationResult.totalJobRuns,
			arrow,
			b.prevAggregationResult.displayPercentage,
			b.prevAggregationResult.parenDisplayPercentage,
			b.prevAggregationResult.totalJobRuns,
		)
	} else {
		s += fmt.Sprintf(naTemplate,
			class,
			b.currAggregationResult.displayName,
			button,
			b.currAggregationResult.displayPercentage,
			b.currAggregationResult.parenDisplayPercentage,
			b.currAggregationResult.totalJobRuns,
		)
	}

	// now render the individual jobs
	jobCount := b.maxJobResultsToShow
	jobRowCount := 0
	jobRows := ""
	jobAdditionalMatches := 0
	for _, job := range b.currAggregationResult.jobResults {
		if jobCount <= 0 {
			jobAdditionalMatches++
			continue
		}
		jobCount--

		var prev *jobResultDisplay
		if b.prevAggregationResult != nil {
			for i, prevJobInstance := range b.prevAggregationResult.jobResults {
				if prevJobInstance.displayName == job.displayName {
					prev = &b.prevAggregationResult.jobResults[i]
					break
				}
			}
		}

		jobRows += NewJobResultRenderer(jobsCollapseName, job, b.release).
			WithPrevious(prev).
			WithMaxTestResultsToShow(b.maxTestResultsToShow).
			StartCollapsed().
			WithIndent(1).
			ToHTML()

		jobRowCount++
	}
	if jobAdditionalMatches > 0 {
		jobRows += fmt.Sprintf(`<tr class="collapse %s"><td colspan=2 style="padding-left:60px"><a href="/variants?release=%s&variant=%s">Plus %d more jobs</a></td></tr>`, jobsCollapseName, b.release, b.currAggregationResult.displayName, jobAdditionalMatches)
	}
	if jobRowCount > 0 {
		s += fmt.Sprintf(`<tr class="collapse %s"><td colspan=2 style="padding-left:60px" class="font-weight-bold">Job Name</td><td class="font-weight-bold">Job Pass Rate</td></tr>`, jobsCollapseName)
		s += jobRows
		s += fmt.Sprintf(`<tr class="collapse %s"><td colspan=3 style="padding-left:60px" class="font-weight-bold"></td><td class="font-weight-bold"></td></tr>`, jobsCollapseName)
	} else {
		s += fmt.Sprintf(`<tr class="collapse %s"><td colspan=3 style="padding-left:60px" class="font-weight-bold">No Jobs Matched Filters</td></tr>`, jobsCollapseName)
	}

	// if we have no test results, we're done
	if len(b.currAggregationResult.testResults) == 0 {
		return s
	}
	s += testRows

	return s
}

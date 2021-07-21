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
	groupby              string
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

func (b *jobAggregationResultRenderBuilder) GroupBy(grouping string) *jobAggregationResultRenderBuilder {
	b.groupby = grouping
	return b
}

func (b *jobAggregationResultRenderBuilder) ToHTML() string {
	s := ""

	buttons := ""
	jobsCollapseName := b.sectionBlock

	prevTestResults := []testResultDisplay{}
	if b.prevAggregationResult != nil {
		prevTestResults = b.prevAggregationResult.testResults
	}

	testCollapseSectionName := MakeSafeForCollapseName(b.sectionBlock + "---" + b.currAggregationResult.displayName + "---tests")
	testRows, displayedTests := getTestRowHTML(b.release, testCollapseSectionName, b.currAggregationResult.testResults, prevTestResults, b.maxTestResultsToShow)

	if len(displayedTests) > 0 { // add the button if we have tests to show
		buttons += " " + GetExpandingButtonHTML(testCollapseSectionName, "Expand Failing Tests")
	}

	// TODO either make this a template or make this a builder that takes args and then has branches.
	// that will fix the funny link that goes nowhere.
	if b.groupby == "variant" {
		buttons += " " + GetTestDetailsButtonHTML(b.release, displayedTests...)

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

		jobsCollapseName += MakeSafeForCollapseName( "---" + b.currAggregationResult.displayName + "---jobs")
		buttons += "					" + GetExpandingButtonHTML(jobsCollapseName, "Expand Failing Jobs")
		if b.prevAggregationResult != nil {
			arrow := GetArrow(b.currAggregationResult.totalJobRuns, b.currAggregationResult.displayPercentage, b.prevAggregationResult.displayPercentage)

			s = s + fmt.Sprintf(template,
				class,
				b.currAggregationResult.displayName,
				buttons,
				b.currAggregationResult.displayPercentage,
				b.currAggregationResult.parenDisplayPercentage,
				b.currAggregationResult.totalJobRuns,
				arrow,
				b.prevAggregationResult.displayPercentage,
				b.prevAggregationResult.parenDisplayPercentage,
				b.prevAggregationResult.totalJobRuns,
			)
		} else {
			s = s + fmt.Sprintf(naTemplate,
				class,
				b.currAggregationResult.displayName,
				buttons,
				b.currAggregationResult.displayPercentage,
				b.currAggregationResult.parenDisplayPercentage,
				b.currAggregationResult.totalJobRuns,
			)
		}
	}

	// now render the individual jobs
	jobCount := b.maxJobResultsToShow
	if b.maxJobResultsToShow == 0 {
		// 0 means unlimited job rows
		jobCount = len(b.currAggregationResult.jobResults)
	}
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
			for _, prevJobInstance := range b.prevAggregationResult.jobResults {
				if prevJobInstance.displayName == job.displayName {
					prev = &prevJobInstance
					break
				}
			}
		}

		row := NewJobResultRenderer(jobsCollapseName, job, b.release).
			WithPrevious(prev).
			WithMaxTestResultsToShow(b.maxTestResultsToShow)
		if b.groupby == "variant" {
			row = row.StartCollapsed().
				WithIndent(1)
		}

		jobRows += row.ToHTML()
		jobRowCount++
	}
	if jobAdditionalMatches > 0 {
		jobRows += fmt.Sprintf(`<tr class="collapse %s"><td colspan=2 style="padding-left:60px"><a href="/variants?release=%s&variant=%s">Plus %d more jobs</a></td></tr>`, jobsCollapseName, b.release, b.currAggregationResult.displayName, jobAdditionalMatches)
	}
	if jobRowCount > 0 {
		s = s + fmt.Sprintf(`<tr class="collapse %s"><td colspan=2 style="padding-left:60px" class="font-weight-bold">Job Name</td><td class="font-weight-bold">Job Pass Rate</td></tr>`, jobsCollapseName)
		s = s + jobRows
		s = s + fmt.Sprintf(`<tr class="collapse %s"><td colspan=3 style="padding-left:60px" class="font-weight-bold"></td><td class="font-weight-bold"></td></tr>`, jobsCollapseName)
	} else {
		s = s + fmt.Sprintf(`<tr class="collapse %s"><td colspan=3 style="padding-left:60px" class="font-weight-bold">No Jobs Matched Filters</td></tr>`, jobsCollapseName)
	}

	s += testRows
	return s
}

// aggregationToJobSubsetOverrides provides a mapping to
var aggregationToJobSubsetOverrides = map[string]string{
	"metal":       "metal-upi",
	"realtime":    "rt",
	"vsphere-ipi": "vsphere",
}

func getCIJobSubstring(aggregationName string) string {
	if ret, ok := aggregationToJobSubsetOverrides[aggregationName]; ok {
		return ret
	}
	return aggregationName
}

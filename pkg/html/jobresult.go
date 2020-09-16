package html

import (
	"fmt"
	"net/url"
	"regexp"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

type jobResultRenderBuilder struct {
	// sectionBlock needs to be unique for each part of the report.  It is used to uniquely name the collapse/expand
	// sections so they open properly
	sectionBlock string

	currJobResult sippyprocessingv1.JobResult
	prevJobResult *sippyprocessingv1.JobResult

	release              string
	maxTestResultsToShow int
	colors               colorizationCriteria
	collapsedAs          string
	baseIndentDepth      int
}

func newJobResultRenderer(sectionBlock string, currJobResult sippyprocessingv1.JobResult, release string) *jobResultRenderBuilder {
	return &jobResultRenderBuilder{
		sectionBlock:         sectionBlock,
		currJobResult:        currJobResult,
		release:              release,
		maxTestResultsToShow: 10, // just a default, can be overridden
		colors: colorizationCriteria{
			minRedPercent:    0,  // failure.  In this range, there is a systemic failure so severe that a reliable signal isn't available.
			minYellowPercent: 60, // at risk.  In this range, there is a systemic problem that needs to be addressed.
			minGreenPercent:  80, // no action required. This *should* be closer to 85%
		},
	}
}

func (b *jobResultRenderBuilder) withPrevious(prevJobResult *sippyprocessingv1.JobResult) *jobResultRenderBuilder {
	b.prevJobResult = prevJobResult
	return b
}

func (b *jobResultRenderBuilder) withMaxTestResultsToShow(maxTestResultsToShow int) *jobResultRenderBuilder {
	b.maxTestResultsToShow = maxTestResultsToShow
	return b
}

func (b *jobResultRenderBuilder) withColors(colors colorizationCriteria) *jobResultRenderBuilder {
	b.colors = colors
	return b
}

func (b *jobResultRenderBuilder) withIndent(depth int) *jobResultRenderBuilder {
	b.baseIndentDepth = depth
	return b
}

func (b *jobResultRenderBuilder) startCollapsedAs(collapsedAs string) *jobResultRenderBuilder {
	b.collapsedAs = collapsedAs
	return b
}

func (b *jobResultRenderBuilder) toHTML() string {
	collapseName := makeSafeForCollapseName(b.sectionBlock + "---" + b.currJobResult.Name + "---tests")

	s := ""

	// TODO either make this a template or make this a builder that takes args and then has branches.
	//  that will fix the funny link that goes nowhere.
	template := `
			<tr class="%s">
				<td style="padding-left:%dpx">
					<a target="_blank" href="%s">%s</a>
					<p>
					<button class="btn btn-primary btn-sm py-0" style="font-size: 0.8em" type="button" data-toggle="collapse" data-target=".%[5]s" aria-expanded="false" aria-controls="%[5]s">Expand Failing Tests</button>
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
					<p>
					<button class="btn btn-primary btn-sm py-0" style="font-size: 0.8em" type="button" data-toggle="collapse" data-target=".%[5]s" aria-expanded="false" aria-controls="%[5]s">Expand Failing Tests</button>
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

	class := b.colors.getColor(b.currJobResult.PassPercentage)
	if len(b.collapsedAs) > 0 {
		class += " collapse " + b.collapsedAs
	}

	if b.prevJobResult != nil {
		arrow := getArrow(b.currJobResult.Successes+b.currJobResult.Failures, b.currJobResult.PassPercentage, b.prevJobResult.PassPercentage)

		s = s + fmt.Sprintf(template,
			class, b.baseIndentDepth*50+10,
			b.currJobResult.TestGridUrl, b.currJobResult.Name, collapseName,
			b.currJobResult.PassPercentage,
			b.currJobResult.PassPercentageWithKnownFailures,
			b.currJobResult.Successes+b.currJobResult.Failures,
			arrow,
			b.prevJobResult.PassPercentage,
			b.prevJobResult.PassPercentageWithKnownFailures,
			b.prevJobResult.Successes+b.prevJobResult.Failures,
		)
	} else {
		s = s + fmt.Sprintf(naTemplate,
			class, b.baseIndentDepth*50+10,
			b.currJobResult.TestGridUrl, b.currJobResult.Name, collapseName,
			b.currJobResult.PassPercentage,
			b.currJobResult.PassPercentageWithKnownFailures,
			b.currJobResult.Successes+b.currJobResult.Failures,
		)
	}

	testIndentDepth := (b.baseIndentDepth+1)*50 + 10
	count := b.maxTestResultsToShow
	rowCount := 0
	rows := ""
	additionalMatches := 0
	for _, test := range b.currJobResult.TestResults {
		if count <= 0 {
			additionalMatches++
			continue
		}
		count--

		encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.Name))
		bugHTML := bugHTMLForTest(test.BugList, b.release, "", test.Name)

		rows = rows + fmt.Sprintf(testGroupTemplate,
			collapseName,
			testIndentDepth,
			test.Name,
			b.currJobResult.Name,
			encodedTestName,
			bugHTML,
			test.PassPercentage,
			test.Successes+test.Failures,
		)
		rowCount++
	}

	if additionalMatches > 0 {
		rows += fmt.Sprintf(`<tr class="collapse %s"><td colspan=2 style="padding-left:%dpx">Plus %d more tests</td></tr>`, collapseName, testIndentDepth, additionalMatches)
	}
	if rowCount > 0 {
		s = s + fmt.Sprintf(`<tr class="collapse %s"><td colspan=2 style="padding-left:%dpx" class="font-weight-bold">Test Name</td><td class="font-weight-bold">Test Pass Rate</td></tr>`, collapseName, testIndentDepth)
		s = s + rows
		s = s + fmt.Sprintf(`<tr class="collapse %s"><td colspan=2 style="padding-left:60px" class="font-weight-bold"></td><td class="font-weight-bold"></td></tr>`, collapseName)
	} else {
		s = s + fmt.Sprintf(`<tr class="collapse %s"><td colspan=3 style="padding-left:%dpx" class="font-weight-bold">No Tests Matched Filters</td></tr>`, collapseName, testIndentDepth)
	}

	return s
}

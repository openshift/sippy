package html

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

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
}

func newJobResultRenderer(sectionBlock string, currJobResult sippyprocessingv1.JobResult, release string) *jobResultRenderBuilder {
	return &jobResultRenderBuilder{
		sectionBlock:         sectionBlock,
		currJobResult:        currJobResult,
		release:              release,
		maxTestResultsToShow: 10, // just a default, can be overridden
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

func (b *jobResultRenderBuilder) toHTML() string {
	collapseName := b.sectionBlock + "---" + b.currJobResult.Name
	collapseName = strings.ReplaceAll(collapseName, ".", "")

	s := ""

	template := `
			<tr class="%s">
				<td>
					<a target="_blank" href="%s">%s</a>
					<p>
					<button class="btn btn-primary btn-sm py-0" style="font-size: 0.8em" type="button" data-toggle="collapse" data-target=".%[4]s" aria-expanded="false" aria-controls="%[4]s">Expand Failing Tests</button>
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
					<a target="_blank" href="%s">%s</a>
					<p>
					<button class="btn btn-primary btn-sm py-0" style="font-size: 0.8em" type="button" data-toggle="collapse" data-target=".%[4]s" aria-expanded="false" aria-controls="%[4]s">Expand Failing Tests</button>
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

	rowColor := ""
	switch {
	case b.currJobResult.PassPercentage > 75:
		rowColor = "table-success"
	case b.currJobResult.PassPercentage > 30:
		rowColor = "table-warning"
	case b.currJobResult.PassPercentage > 0:
		rowColor = "table-danger"
	default:
		rowColor = "error"
	}

	if b.prevJobResult != nil {
		arrow := ""
		delta := 5.0
		if b.currJobResult.Successes+b.currJobResult.Failures > 80 {
			delta = 2
		}

		if b.currJobResult.PassPercentage > b.prevJobResult.PassPercentage+delta {
			arrow = fmt.Sprintf(up, b.currJobResult.PassPercentage-b.prevJobResult.PassPercentage)
		} else if b.currJobResult.PassPercentage < b.prevJobResult.PassPercentage-delta {
			arrow = fmt.Sprintf(down, b.prevJobResult.PassPercentage-b.currJobResult.PassPercentage)
		} else if b.currJobResult.PassPercentage > b.prevJobResult.PassPercentage {
			arrow = fmt.Sprintf(flatup, b.currJobResult.PassPercentage-b.prevJobResult.PassPercentage)
		} else {
			arrow = fmt.Sprintf(flatdown, b.prevJobResult.PassPercentage-b.currJobResult.PassPercentage)
		}
		s = s + fmt.Sprintf(template, rowColor, b.currJobResult.TestGridUrl, b.currJobResult.Name, collapseName,
			b.currJobResult.PassPercentage,
			b.currJobResult.PassPercentageWithKnownFailures,
			b.currJobResult.Successes+b.currJobResult.Failures,
			arrow,
			b.prevJobResult.PassPercentage,
			b.prevJobResult.PassPercentageWithKnownFailures,
			b.prevJobResult.Successes+b.prevJobResult.Failures,
		)
	} else {
		s = s + fmt.Sprintf(naTemplate, rowColor, b.currJobResult.TestGridUrl, b.currJobResult.Name, collapseName,
			b.currJobResult.PassPercentage,
			b.currJobResult.PassPercentageWithKnownFailures,
			b.currJobResult.Successes+b.currJobResult.Failures,
		)
	}

	count := b.maxTestResultsToShow
	rowCount := 0
	rows := ""
	additionalMatches := 0
	for _, test := range b.currJobResult.TestResults {
		if count == 0 {
			additionalMatches++
			continue
		}
		count--

		encodedTestName := url.QueryEscape(regexp.QuoteMeta(test.Name))
		bugHTML := bugHTMLForTest(test.BugList, b.release, "", test.Name)

		rows = rows + fmt.Sprintf(testGroupTemplate, collapseName,
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
		rows += fmt.Sprintf(`<tr class="collapse %s"><td colspan=2>Plus %d more tests</td></tr>`, collapseName, additionalMatches)
	}
	if rowCount > 0 {
		s = s + fmt.Sprintf(`<tr class="collapse %s"><td colspan=2 class="font-weight-bold">Test Name</td><td class="font-weight-bold">Test Pass Rate</td></tr>`, collapseName)
		s = s + rows
	} else {
		s = s + fmt.Sprintf(`<tr class="collapse %s"><td colspan=3 class="font-weight-bold">No Tests Matched Filters</td></tr>`, collapseName)
	}

	return s
}

package releasehtml

import (
	"fmt"
	"sort"
	"strings"

	"github.com/openshift/sippy/pkg/html/generichtml"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/util"
)

func summaryJobsFailuresByBugzillaComponent(report, reportPrev sippyprocessingv1.TestReport, numDays int, release string) string {
	failuresByBugzillaComponent := summarizeJobsFailuresByBugzillaComponent(report)
	failuresByBugzillaComponentPrev := summarizeJobsFailuresByBugzillaComponent(reportPrev)

	s := fmt.Sprintf(`
	<table class="table">
		<tr>
			<th colspan=4 class="text-center"><a class="text-dark" title="Bugzilla components ranked by maximum fail percentage of any job" id="JobImpactingBZComponents" href="#JobImpactingBZComponents">Job Impacting BZ Components</a></th>
		</tr>
		<tr>
			<th>Component</th><th>Latest %d days</th><th/><th>Previous 7 days</th>
		</tr>
	`, numDays)

	colors := generichtml.ColorizationCriteria{
		MinRedPercent:    0,
		MinYellowPercent: 90,
		MinGreenPercent:  95,
	}

	for _, bugzillaComponentResult := range failuresByBugzillaComponent {
		prev := util.FindBugzillaJobFailures(bugzillaComponentResult.Name, failuresByBugzillaComponentPrev)

		bugzillaComponentHTML := newJobAggregationResultRendererFromBugzillaComponentResult("by-bugzilla-component", bugzillaComponentResult, release).
			withColors(colors).
			withPreviousBugzillaComponentResult(prev).
			toHTML()

		s += bugzillaComponentHTML
	}

	s = s + "</table>"
	return s
}

func summarizeJobsFailuresByBugzillaComponent(report sippyprocessingv1.TestReport) []sippyprocessingv1.SortedBugzillaComponentResult {
	bzComponentResults := []sippyprocessingv1.SortedBugzillaComponentResult{}

	for _, bzJobFailures := range report.JobFailuresByBugzillaComponent {
		bzComponentResults = append(bzComponentResults, bzJobFailures)
	}
	// sort from highest to lowest
	sort.SliceStable(bzComponentResults, func(i, j int) bool {
		if bzComponentResults[i].JobsFailed[0].FailPercentage > bzComponentResults[j].JobsFailed[0].FailPercentage {
			return true
		}
		if bzComponentResults[i].JobsFailed[0].FailPercentage < bzComponentResults[j].JobsFailed[0].FailPercentage {
			return false
		}
		if strings.Compare(strings.ToLower(bzComponentResults[i].Name), strings.ToLower(bzComponentResults[j].Name)) < 0 {
			return true
		}
		return false
	})
	return bzComponentResults
}

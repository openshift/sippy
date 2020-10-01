package html

import (
	"fmt"
	"sort"
	"strings"

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

	bzGroupTemplate := `
		<tr class="%s">
			<td>
				%[2]s
					<p>
					%s
			</td>
			<td>
				%0.2f%% <span class="text-nowrap">(%d runs)</span>
			</td>
			<td>
				%s
			</td>
			<td>
				%0.2f%% <span class="text-nowrap">(%d runs)</span>
			</td>
		</tr>
	`

	naTemplate := `
			<tr class="%s">
				<td>
					%[2]s
					<p>
					%s
				</td>
				<td>
					%0.2f%% <span class="text-nowrap">(%d runs)</span>
				</td>
				<td/>
				<td>
					NA
				</td>
			</tr>
		`

	colors := colorizationCriteria{
		minRedPercent:    0,
		minYellowPercent: 90,
		minGreenPercent:  95,
	}

	for _, v := range failuresByBugzillaComponent {
		safeBZJob := makeSafeForCollapseName(fmt.Sprintf("%s---component", v.Name))
		button := ""
		button += "					" + getButtonHTML(safeBZJob, "Expand Failing Jobs")

		highestFailPercentage := v.JobsFailed[0].FailPercentage
		lowestPassPercentage := 100 - highestFailPercentage
		rowColor := colors.getColor(lowestPassPercentage)

		prev := util.FindBugzillaJobFailures(v.Name, failuresByBugzillaComponentPrev)
		if prev != nil && len(prev.JobsFailed) > 0 {
			previousHighestFailPercentage := prev.JobsFailed[0].FailPercentage
			previousLowestPassPercentage := 100 - previousHighestFailPercentage

			arrow := getArrow(v.JobsFailed[0].TotalRuns, lowestPassPercentage, previousLowestPassPercentage)

			s = s + fmt.Sprintf(bzGroupTemplate,
				rowColor,
				v.Name,
				button,
				lowestPassPercentage,
				v.JobsFailed[0].TotalRuns, // this is the total runs for the current, worst job which matches the pass percentage
				arrow,
				previousLowestPassPercentage,
				prev.JobsFailed[0].TotalRuns,
			)
		} else {
			s = s + fmt.Sprintf(naTemplate,
				rowColor,
				v.Name,
				button,
				lowestPassPercentage,
				v.JobsFailed[0].TotalRuns, // this is the total runs for the current, worst job which matches the pass percentage
			)
		}

		count := 0
		for _, failingJob := range v.JobsFailed {
			if count > 4 { // only show five
				break
			}
			count++

			// given the name, we can actually look up the original JobResult.  There aren't that many, just iterate.
			fullJobResult := util.FindJobResultForJobName(failingJob.JobName, report.ByJob)

			// create the synthetic JobResult for display purposes.
			// TODO with another refactor, we'll be able to tighten this up later.
			currJobResult := sippyprocessingv1.JobResult{
				Name:                            failingJob.JobName,
				Platform:                        "",
				Failures:                        failingJob.NumberOfJobRunsFailed,
				KnownFailures:                   0,
				Successes:                       failingJob.TotalRuns - failingJob.NumberOfJobRunsFailed,
				PassPercentage:                  100.0 - failingJob.FailPercentage,
				PassPercentageWithKnownFailures: 0,
				TestGridUrl:                     fullJobResult.TestGridUrl,
				TestResults:                     failingJob.Failures,
			}
			var prevJobResult *sippyprocessingv1.JobResult
			if prev != nil {
				var prevJob *sippyprocessingv1.BugzillaJobResult
				for _, prevJobI := range prev.JobsFailed {
					if prevJobI.JobName == failingJob.JobName {
						prevJob = &prevJobI
						break
					}
				}
				if prevJob != nil {
					prevJobResult = &sippyprocessingv1.JobResult{
						Name:                            prevJob.JobName,
						Platform:                        "",
						Failures:                        prevJob.NumberOfJobRunsFailed,
						KnownFailures:                   0,
						Successes:                       prevJob.TotalRuns - prevJob.NumberOfJobRunsFailed,
						PassPercentage:                  100.0 - prevJob.FailPercentage,
						PassPercentageWithKnownFailures: 0,
						TestGridUrl:                     fullJobResult.TestGridUrl,
						TestResults:                     prevJob.Failures,
					}
				}
			}

			jobHTML := newJobResultRendererFromJobResult(safeBZJob, currJobResult, release).
				withPreviousJobResult(prevJobResult).
				withColors(colors).
				startCollapsed().
				withIndent(1).
				toHTML()
			s += jobHTML
		}
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

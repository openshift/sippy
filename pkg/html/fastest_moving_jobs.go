package html

import (
	"fmt"
	"sort"

	"github.com/openshift/sippy/pkg/util"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

func summaryTopNegativelyMovingJobs(twoDaysJobs, prevJobs []sippyprocessingv1.JobResult, jobTestCount int, release string) string {
	type jobPassChange struct {
		jobName              string
		passPercentageChange float64
	}
	jobPassChanges := []jobPassChange{}

	for _, job := range twoDaysJobs {
		prevJob := util.FindJobResultForJobName(job.Name, prevJobs)
		if prevJob == nil {
			continue
		}
		jobPassChanges = append(jobPassChanges, jobPassChange{
			jobName:              job.Name,
			passPercentageChange: job.PassPercentage - prevJob.PassPercentage,
		})
	}
	sort.SliceStable(jobPassChanges, func(i, j int) bool {
		return jobPassChanges[i].passPercentageChange < jobPassChanges[j].passPercentageChange
	})

	s := fmt.Sprintf(`
	<table class="table">
		<tr>
			<th colspan=4 class="text-center"><a class="text-dark" title="Jobs that have changed their pass percentages the most in the last two days." id="JobByMostReducedPassRate" href="#JobByMostReducedPassRate">Job Pass Rates By Most Reduced Pass Rate</a></th>
		</tr>
		<tr>
			<th>Name</th><th>Latest 2 days</th><th/><th>Previous 7 days</th>
		</tr>
	`)

	jobDisplayed := 0
	for _, jobDetails := range jobPassChanges {
		jobDisplayed++
		if jobDisplayed > 10 {
			break
		}
		// don't display things moving in the right direction or that only dropped within the margin of error
		// The margin of error is currently just a guess.
		if jobDetails.passPercentageChange > -10 {
			break
		}
		currJobResult := util.FindJobResultForJobName(jobDetails.jobName, twoDaysJobs)
		prevJobResult := util.FindJobResultForJobName(currJobResult.Name, prevJobs)
		jobHTML := newJobResultRenderer("by-job-name", *currJobResult, release).
			withMaxTestResultsToShow(jobTestCount).
			withPrevious(prevJobResult).
			toHTML()

		s += jobHTML
	}

	s = s + "</table>"

	return s
}

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	gosort "sort"
	"strconv"
	"strings"

	"github.com/openshift/sippy/pkg/util"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	v1sippyprocessing "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

func (runs apiRunResults) sort(req *http.Request) apiRunResults {
	sortField := req.URL.Query().Get("sortField")
	sort := apitype.Sort(req.URL.Query().Get("sort"))

	if sortField == "" {
		sortField = "testFailures"
	}

	if sort == "" {
		sort = apitype.SortDescending
	}

	gosort.Slice(runs, func(i, j int) bool {
		if sort == apitype.SortAscending {
			return compare(runs[i], runs[j], sortField)
		}
		return compare(runs[j], runs[i], sortField)
	})

	return runs
}

func (runs apiRunResults) limit(req *http.Request) apiRunResults {
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	if limit > 0 && len(runs) >= limit {
		return runs[:limit]
	}

	return runs
}

type apiRunResults []apitype.JobRun

func jobRunToAPIJobRun(id int, job v1sippyprocessing.JobResult, result v1sippyprocessing.JobRunResult) apitype.JobRun {
	artifacts := ""
	if job.GCSJobHistoryLocationPrefix != "" && job.GCSBucketName != "" && result.ID != "" {
		artifacts = fmt.Sprintf("https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/%s/%s/%s", job.GCSBucketName, job.GCSJobHistoryLocationPrefix, result.ID)
	}

	return apitype.JobRun{
		ID:           id,
		BriefName:    briefName(job.Name),
		Variants:     job.Variants,
		TestGridURL:  job.TestGridURL,
		ArtifactsURL: artifacts,
		JobRunResult: result,
	}
}

// PrintJobRunsReport renders the detailed list of runs for matching jobs.
func PrintJobRunsReport(w http.ResponseWriter, req *http.Request, currReport, prevReport v1sippyprocessing.TestReport) {
	var filter *Filter
	curr := currReport.ByJob
	prev := prevReport.ByJob

	queryFilter := req.URL.Query().Get("filter")
	if queryFilter != "" {
		filter = &Filter{}
		if err := json.Unmarshal([]byte(queryFilter), filter); err != nil {
			RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": "Could not marshal query:" + err.Error()})
			return
		}
	}

	all := make([]apitype.JobRun, 0)
	next := 0
	for _, results := range append(curr, prev...) {
		for _, run := range results.AllRuns {
			apiRun := jobRunToAPIJobRun(next, results, run)

			if strings.Contains(results.Name, "-upgrade") {
				apiRun.Tags = []string{"upgrade"}
			}

			if filter != nil {
				include, err := filter.Filter(apiRun)

				// Job runs are a little special, in that we want to let users filter them by fields from the job
				// itself, too.
				if err != nil && strings.Contains(err.Error(), "unknown") {
					currJob := util.FindJobResultForJobName(run.Job, curr)
					if currJob != nil {
						prevJob := util.FindJobResultForJobName(run.Job, prev)
						include, err = filter.Filter(jobResultToAPI(next, currJob, prevJob))
					}
				}
				if err != nil {
					RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{"code": http.StatusBadRequest, "message": "Filter error:" + err.Error()})
					return
				}

				if !include {
					continue
				}
			}

			all = append(all, apiRun)
			next++
		}
	}

	RespondWithJSON(http.StatusOK, w,
		apiRunResults(all).
			sort(req).
			limit(req),
	)
}

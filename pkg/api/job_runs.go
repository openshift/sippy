package api

import (
	"encoding/json"
	"net/http"
	gosort "sort"
	"strconv"

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

// PrintJobRunsReport renders the detailed list of runs for matching jobs.
func PrintJobRunsReport(w http.ResponseWriter, req *http.Request, curr, prev []v1sippyprocessing.JobResult) {
	var filter *Filter

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
			apiRun := apitype.JobRun{
				ID:           next,
				Variants:     results.Variants,
				TestGridURL:  results.TestGridURL,
				JobRunResult: run,
			}

			if filter != nil {
				include, err := filter.Filter(apiRun)
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

	RespondWithJSON(http.StatusOK, w, apiRunResults(all).limit(req).sort(req))
}

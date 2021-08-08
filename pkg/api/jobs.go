package api

import (
	"fmt"
	"net/http"
	"regexp"
	gosort "sort"
	"strconv"
	"strings"
	"time"

	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"

	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	v1sippyprocessing "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/util"
)

func jobFilter(req *http.Request) []func(result v1sippyprocessing.JobResult) bool {
	filterBy := req.URL.Query()["filterBy"]
	runs, _ := strconv.Atoi(req.URL.Query().Get("runs"))
	job := req.URL.Query().Get("job")
	variants := req.URL.Query()["variant"]

	filters := make([]func(result v1sippyprocessing.JobResult) bool, 0)

	for _, filterName := range filterBy {
		switch filterName {
		case "name":
			filters = append(filters, func(jobResult v1sippyprocessing.JobResult) bool {
				return strings.Contains(jobResult.Name, job)
			})
		case "upgrade":
			filters = append(filters, func(jobResult v1sippyprocessing.JobResult) bool {
				return strings.Contains(jobResult.Name, "-upgrade-")
			})
		case "runs":
			filters = append(filters, func(jobResult v1sippyprocessing.JobResult) bool {
				return (jobResult.Failures + jobResult.Successes) >= runs
			})
		case "hasBug":
			filters = append(filters, func(jobResult v1sippyprocessing.JobResult) bool {
				return len(jobResult.BugList) > 0
			})
		case "noBug":
			filters = append(filters, func(jobResult v1sippyprocessing.JobResult) bool {
				return len(jobResult.BugList) == 0
			})
		case "variant":
			filters = append(filters, func(jobResult v1sippyprocessing.JobResult) bool {
				for _, variant := range variants {
					for _, jrv := range jobResult.Variants {
						if strings.Contains(jrv, variant) {
							return true
						}
					}
				}
				return false
			})
		default:
		}
	}

	return filters
}

type jobsAPIResult []v1.Job

func (jobs jobsAPIResult) sort(req *http.Request) jobsAPIResult {
	sortBy := req.URL.Query().Get("sortBy")

	switch sortBy {
	case "regression":
		gosort.Slice(jobs, func(i, j int) bool {
			return jobs[i].NetImprovement < jobs[j].NetImprovement
		})
	case "improvement":
		gosort.Slice(jobs, func(i, j int) bool {
			return jobs[i].NetImprovement > jobs[j].NetImprovement
		})
	}

	return jobs
}

func (jobs jobsAPIResult) limit(req *http.Request) jobsAPIResult {
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	if limit > 0 && len(jobs) >= limit {
		return jobs[:limit]
	}

	return jobs
}

// PrintJobsReport renders a filtered summary of matching jobs.
func PrintJobsReport(w http.ResponseWriter, req *http.Request, currentPeriod, twoDayPeriod, previousPeriod []v1sippyprocessing.JobResult, manager testidentification.VariantManager) {
	jobs := jobsAPIResult{}
	briefName := regexp.MustCompile("periodic-ci-openshift-(multiarch|release)-master-(ci|nightly)-[0-9]+.[0-9]+-")
	filters := jobFilter(req)

	// If requesting a two day report, we make the comparison between the last
	// period (typically 7 days) and the last two days.
	var current, previous []v1sippyprocessing.JobResult
	switch req.URL.Query().Get("period") {
	case "twoDay":
		current = twoDayPeriod
		previous = currentPeriod
	default:
		current = currentPeriod
		previous = previousPeriod
	}

jobResultLoop:
	for idx, jobResult := range current {
		for _, filter := range filters {
			if !filter(jobResult) {
				continue jobResultLoop
			}
		}

		job := v1.Job{
			ID:                             idx,
			Name:                           jobResult.Name,
			Variants:                       manager.IdentifyVariants(jobResult.Name),
			BriefName:                      briefName.ReplaceAllString(jobResult.Name, ""),
			CurrentPassPercentage:          jobResult.PassPercentage,
			CurrentProjectedPassPercentage: jobResult.PassPercentageWithoutInfrastructureFailures,
			CurrentRuns:                    jobResult.Failures + jobResult.Successes,
		}

		prevResult := util.FindJobResultForJobName(jobResult.Name, previous)
		if previous != nil {
			job.PreviousPassPercentage = prevResult.PassPercentage
			job.PreviousProjectedPassPercentage = prevResult.PassPercentageWithoutInfrastructureFailures
			job.PreviousRuns = prevResult.Failures + prevResult.Successes
			job.NetImprovement = jobResult.PassPercentage - prevResult.PassPercentage
		}

		job.Bugs = jobResult.BugList
		job.AssociatedBugs = jobResult.AssociatedBugList
		job.TestGridURL = jobResult.TestGridURL

		jobs = append(jobs, job)
	}

	RespondWithJSON(http.StatusOK, w, jobs.
		sort(req).
		limit(req))
}

type jobDetail struct {
	Name    string                          `json:"name"`
	Results []v1sippyprocessing.BuildResult `json:"results"`
}

type jobDetailAPIResult struct {
	Jobs  []jobDetail `json:"jobs"`
	Start int         `json:"start"`
	End   int         `json:"end"`
}

func (jobs jobDetailAPIResult) limit(req *http.Request) jobDetailAPIResult {
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	if limit > 0 && len(jobs.Jobs) >= limit {
		jobs.Jobs = jobs.Jobs[:limit]
	}

	return jobs
}

func getDateRange(req *http.Request) (*int64, *int64, error) {
	startDate := req.URL.Query().Get("startDate")
	endDate := req.URL.Query().Get("endDate")

	if startDate == "" && endDate == "" {
		return nil, nil, nil
	}

	if startDate != "" && endDate == "" {
		return nil, nil, fmt.Errorf("end date is missing")
	}

	if startDate == "" && endDate != "" {
		return nil, nil, fmt.Errorf("start date is missing")
	}

	startParsed, err := time.Parse(`2006-01-02`, startDate)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid start date: %s", err)
	}

	endParsed, err := time.Parse(`2006-01-02`, endDate)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid end date: %s", err)
	}

	startMillis := startParsed.UnixNano() / int64(time.Millisecond)
	endMillis := endParsed.UnixNano() / int64(time.Millisecond)

	return &startMillis, &endMillis, nil
}

// PrintJobDetailsReport renders the detailed list of runs for matching jobs.
func PrintJobDetailsReport(w http.ResponseWriter, req *http.Request, current, previous []v1sippyprocessing.JobResult) {
	var min, max int
	jobs := make([]jobDetail, 0)
	filters := jobFilter(req)

	start, end, err := getDateRange(req)
	if err != nil {
		RespondWithJSON(http.StatusBadRequest, w, map[string]string{
			"message": err.Error(),
		})
		return
	}

jobResultLoop:
	for _, jobResult := range current {
		for _, filter := range filters {
			if !filter(jobResult) {
				continue jobResultLoop
			}
		}

		prevResult := util.FindJobResultForJobName(jobResult.Name, previous)

		// Filter by date
		buildResults := make([]v1sippyprocessing.BuildResult, 0)
		if start != nil && end != nil {
			for _, result := range jobResult.BuildResults {
				if (int64(result.Timestamp) > *start) && int64(result.Timestamp) < *end {
					buildResults = append(buildResults, result)
				}
			}

			for _, result := range prevResult.BuildResults {
				if (int64(result.Timestamp) > *start) && int64(result.Timestamp) < *end {
					buildResults = append(buildResults, result)
				}
			}
		} else {
			buildResults = append(jobResult.BuildResults, prevResult.BuildResults...)
		}

		for _, result := range buildResults {
			if result.Timestamp < min || min == 0 {
				min = result.Timestamp
			}

			if result.Timestamp > max || max == 0 {
				max = result.Timestamp
			}
		}

		jobDetail := jobDetail{
			Name:    jobResult.Name,
			Results: buildResults,
		}

		jobs = append(jobs, jobDetail)
	}

	RespondWithJSON(http.StatusOK, w, jobDetailAPIResult{
		Jobs:  jobs,
		Start: min,
		End:   max,
	}.limit(req))
}

package componentreportserver

import (
	"bytes"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/lib/pq"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/util/sets"
)

func (s *Server) handleComponentsByJob(w http.ResponseWriter, req *http.Request) {
	var errToReport error
	buf := &bytes.Buffer{}
	defer func() {
		if errToReport == nil {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(buf, "ERROR: %v: %v", req.URL.Path, errToReport)
		}

		_, err := w.Write(buf.Bytes())
		if err != nil {
			fmt.Printf("ERROR: %v: %v", req.URL.Path, err)
		}
	}()

	componentJobSummaries, err := listComponentJobSummaries(s.databaseConnection)
	if err != nil {
		errToReport = err
		return
	}

	toDisplay, err := forDisplay(componentJobSummaries)
	if err != nil {
		errToReport = err
		return
	}

	fmt.Fprintf(buf, "<table>\n")
	fmt.Fprintf(buf, "\t<tr>\n")
	fmt.Fprintf(buf, "\t<th>%v</th>\n", "Component")
	for _, curr := range toDisplay[0] {
		fmt.Fprintf(buf, "\t<th>%v</th>\n", curr.JobName)
	}
	fmt.Fprintf(buf, "\t</tr>\n")

	for _, currRow := range toDisplay {
		fmt.Fprintf(buf, "\t<tr>\n")
		fmt.Fprintf(buf, "\t<td>%v</td>\n", currRow[0].ComponentName)
		for _, currJob := range currRow {
			fmt.Fprintf(buf, "\t<td>%v</td>\n", currJob.Status)
		}
		fmt.Fprintf(buf, "\t</tr>\n")
	}

	fmt.Fprintf(buf, "</table>\n")
}

type ComponentJobSummary struct {
	ComponentID   int            `json:"component_id"`
	JobID         int            `json:"job_id"`
	FailingMark   int            `json:"failing_mark"`
	WorkingMark   int            `json:"working_mark"`
	Status        string         `json:"status"`
	ComponentName string         `json:"component_name"`
	JobName       string         `json:"job_name"`
	Variants      pq.StringArray `json:"variants" gorm:"type:text[]"`
}

type byComponentAndJob []ComponentJobSummary

func (a byComponentAndJob) Len() int      { return len(a) }
func (a byComponentAndJob) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a byComponentAndJob) Less(i, j int) bool {
	if a[i].ComponentName != a[j].ComponentName {
		return strings.Compare(a[i].ComponentName, a[j].ComponentName) < 0
	}
	if a[i].JobName == "Summary" {
		return true
	}
	return strings.Compare(a[i].JobName, a[j].JobName) < 0
}

func listComponentJobSummaries(dbc *db.DB) ([]ComponentJobSummary, error) {
	ret := make([]ComponentJobSummary, 0)
	q := dbc.DB.Raw(`SELECT * from component_rollup_with_metadata`)
	if q.Error != nil {
		return nil, q.Error
	}
	q.Scan(&ret)
	return ret, nil
}

func forDisplay(in []ComponentJobSummary) ([][]ComponentJobSummary, error) {
	// simple, not efficient, not sparse

	components := sets.NewString()
	jobNameToID := map[string]int{}
	componentToJobs := map[string][]ComponentJobSummary{}
	for i := range in {
		curr := in[i]
		components.Insert(curr.ComponentName)
		jobNameToID[curr.JobName] = curr.JobID
		componentToJobs[curr.ComponentName] = append(componentToJobs[curr.ComponentName], curr)
	}

	// add entries for missing jobs and sort
	for componentName, jobData := range componentToJobs {
		if len(jobData) == len(jobNameToID) {
			sort.Sort(byComponentAndJob(jobData))
			componentToJobs[componentName] = jobData
		}

		for jobName, jobID := range jobNameToID {
			found := false
			for _, currJobData := range jobData {
				if currJobData.JobName == jobName {
					found = true
					break
				}
			}
			if found {
				continue
			}
			jobData = append(jobData, ComponentJobSummary{
				ComponentID:   jobData[0].ComponentID,
				JobID:         jobID,
				FailingMark:   0,
				WorkingMark:   0,
				Status:        "Missing",
				ComponentName: jobData[0].ComponentName,
				JobName:       jobName,
				Variants:      nil,
			})
			componentToJobs[componentName] = jobData
		}

		sort.Sort(byComponentAndJob(jobData))
		componentToJobs[componentName] = jobData
	}

	ret := [][]ComponentJobSummary{}
	for _, componentName := range components.List() {
		jobData := componentToJobs[componentName]
		jobData = append(jobData, summarizeComponentJobs(jobData))
		ret = append(ret, jobData)
	}

	return ret, nil
}

func summarizeComponentJobs(in []ComponentJobSummary) ComponentJobSummary {
	failed := false
	for i := range in {
		curr := in[i]
		if curr.Status == "Fail" {
			failed = true
			break
		}
	}

	ret := ComponentJobSummary{
		ComponentID:   in[0].ComponentID,
		JobID:         -2,
		ComponentName: in[0].ComponentName,
		JobName:       "Summary",
		Variants:      nil,
	}
	if failed {
		ret.FailingMark = 1
		ret.WorkingMark = 0
		ret.Status = "Fail"
	} else {
		ret.FailingMark = 0
		ret.WorkingMark = 1
		ret.Status = "Pass"
	}

	return ret
}

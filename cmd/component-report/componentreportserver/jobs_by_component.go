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

func (s *Server) handleJobsByComponent(w http.ResponseWriter, req *http.Request) {
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

	toDisplay, err := componentJobSummaryForDisplay(componentJobSummaries)
	if err != nil {
		errToReport = err
		return
	}

	fmt.Fprintf(buf, `
<head>
<style>
table, th, td {
  border: 1px solid black;
}
</style>
</head>
<body>
`)
	fmt.Fprintf(buf, "<table>\n")
	fmt.Fprintf(buf, "\t<tr>\n")
	fmt.Fprintf(buf, "\t<th>%v</th>\n", "Component")
	for _, curr := range toDisplay[0] {
		fmt.Fprintf(buf, "\t<th>%v</th>\n", curr.VariantSelectorName)
	}
	fmt.Fprintf(buf, "\t</tr>\n")

	for _, currRow := range toDisplay {
		fmt.Fprintf(buf, "\t<tr>\n")
		fmt.Fprintf(buf, "\t<td>%v</td>\n", currRow[0].ComponentName)
		for _, currJob := range currRow {
			color := ""
			switch currJob.Status {
			case "Fail":
				color = "#b35656"
			case "Pass":
				color = "##4a8242"
			}
			fmt.Fprintf(buf, "\t<td bgcolor=\"%v\">%v</td>\n", color, currJob.Status)
		}
		fmt.Fprintf(buf, "\t</tr>\n")
	}

	fmt.Fprintf(buf, "</table>\n")
	fmt.Fprintf(buf, "</body>\n")

}

type ComponentJobSummary struct {
	ComponentID         int            `json:"component_id"`
	VariantSelectorID   int            `json:"variant_selector_id"`
	FailingMark         int            `json:"failing_mark"`
	WorkingMark         int            `json:"working_mark"`
	Status              string         `json:"status"`
	ComponentName       string         `json:"component_name"`
	VariantSelectorName string         `json:"variant_selector_name"`
	Variants            pq.StringArray `json:"variants" gorm:"type:text[]"`
}

type byComponentAndJob []ComponentJobSummary

func (a byComponentAndJob) Len() int      { return len(a) }
func (a byComponentAndJob) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a byComponentAndJob) Less(i, j int) bool {
	if a[i].ComponentName != a[j].ComponentName {
		return strings.Compare(a[i].ComponentName, a[j].ComponentName) < 0
	}
	if a[i].VariantSelectorName == "Summary" {
		return true
	}
	return strings.Compare(a[i].VariantSelectorName, a[j].VariantSelectorName) < 0
}

func listComponentJobSummaries(dbc *db.DB) ([]ComponentJobSummary, error) {
	ret := make([]ComponentJobSummary, 0)
	q := dbc.DB.Raw(`SELECT * from component_rollup_with_metadata_2`)
	if q.Error != nil {
		return nil, q.Error
	}
	q.Scan(&ret)
	return ret, nil
}

func componentJobSummaryForDisplay(in []ComponentJobSummary) ([][]ComponentJobSummary, error) {
	// simple, not efficient, not sparse

	components := sets.NewString()
	jobNameToID := map[string]int{}
	componentToJobs := map[string][]ComponentJobSummary{}
	for i := range in {
		curr := in[i]
		components.Insert(curr.ComponentName)
		jobNameToID[curr.VariantSelectorName] = curr.VariantSelectorID
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
				if currJobData.VariantSelectorName == jobName {
					found = true
					break
				}
			}
			if found {
				continue
			}
			jobData = append(jobData, ComponentJobSummary{
				ComponentID:         jobData[0].ComponentID,
				VariantSelectorID:   jobID,
				FailingMark:         0,
				WorkingMark:         0,
				Status:              "Missing",
				ComponentName:       jobData[0].ComponentName,
				VariantSelectorName: jobName,
				Variants:            nil,
			})
			componentToJobs[componentName] = jobData
		}

		sort.Sort(byComponentAndJob(jobData))
		componentToJobs[componentName] = jobData
	}

	ret := [][]ComponentJobSummary{}
	for _, componentName := range components.List() {
		jobData := componentToJobs[componentName]
		currRow := []ComponentJobSummary{summarizeComponentJobs(jobData)}
		currRow = append(currRow, jobData...)
		ret = append(ret, currRow)
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
		ComponentID:         in[0].ComponentID,
		VariantSelectorID:   -2,
		ComponentName:       in[0].ComponentName,
		VariantSelectorName: "Summary",
		Variants:            nil,
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

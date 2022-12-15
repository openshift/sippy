package componentreportserver

import (
	"bytes"
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/lib/pq"
	"github.com/openshift/sippy/pkg/db"
)

func (s *Server) handleTestsForComponent(w http.ResponseWriter, req *http.Request) {
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
	segments := strings.Split(req.URL.Path, "/")
	if len(segments) != 3 {
		errToReport = fmt.Errorf("bad format")
		return
	}
	componentName := segments[2]

	testsForComponent, err := listTestsForComponent(s.databaseConnection, componentName)
	if err != nil {
		errToReport = err
		return
	}

	toDisplay, err := testForComponentForDisplay(testsForComponent, componentName)
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
	fmt.Fprintf(buf, "<h1>%v</h1>\n", componentName)
	fmt.Fprintf(buf, "<table>\n")
	fmt.Fprintf(buf, "<table>\n")
	fmt.Fprintf(buf, "<table>\n")
	fmt.Fprintf(buf, "\t<tr>\n")
	fmt.Fprintf(buf, "\t<th>%v</th>\n", "Suite")
	fmt.Fprintf(buf, "\t<th>%v</th>\n", "Test")
	for _, curr := range toDisplay[0] {
		fmt.Fprintf(buf, "\t<th>%v</th>\n", curr.JobName)
	}
	fmt.Fprintf(buf, "\t</tr>\n")

	for _, currRow := range toDisplay {
		fmt.Fprintf(buf, "\t<tr>\n")
		fmt.Fprintf(buf, "\t<td>%v</td>\n", currRow[0].SuiteName)
		fmt.Fprintf(buf, "\t<td>%v</td>\n", currRow[0].TestName)
		for _, currJob := range currRow {
			color := ""
			switch {
			case currJob.TotalCount == 0:
				// no color
			case currJob.WorkingPercentage < 95:
				color = "#b35656"
			default:
				color = "##4a8242"
			}
			fmt.Fprintf(buf, "\t<td bgcolor=\"%v\">%v%%</td>\n", color, currJob.WorkingPercentage)
		}
		fmt.Fprintf(buf, "\t</tr>\n")
	}

	fmt.Fprintf(buf, "</table>\n")
	fmt.Fprintf(buf, "</body>\n")

}

type TestForComponent struct {
	TestID            int `json:"test_id"`
	SuiteID           int `json:"suite_id"`
	ComponentID       int `json:"component_id"`
	JobID             int `json:"job_id"`
	SuccessCount      int `json:"success_count"`
	RunningCount      int `json:"running_count"`
	FailureCount      int `json:"failure_count"`
	FlakeCount        int `json:"flake_count"`
	WorkingCount      int `json:"working_count"`
	TotalCount        int `json:"total_count"`
	SuccessPercentage int `json:"success_percentage"`
	FailurePercentage int `json:"failure_percentage"`
	FlakingPercentage int `json:"flaking_percentage"`
	WorkingPercentage int `json:"working_percentage"`

	ComponentName string         `json:"component_name"`
	JobName       string         `json:"job_name"`
	SuiteName     string         `json:"suite_name"`
	TestName      string         `json:"test_name"`
	Variants      pq.StringArray `json:"variants" gorm:"type:text[]"`
}

type byComponentAndTest []TestForComponent

func (a byComponentAndTest) Len() int      { return len(a) }
func (a byComponentAndTest) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a byComponentAndTest) Less(i, j int) bool {
	if a[i].ComponentName != a[j].ComponentName {
		return strings.Compare(a[i].ComponentName, a[j].ComponentName) < 0
	}
	if a[i].SuiteName != a[j].SuiteName {
		return strings.Compare(a[i].SuiteName, a[j].SuiteName) < 0
	}
	if a[i].TestName != a[j].TestName {
		return strings.Compare(a[i].TestName, a[j].TestName) < 0
	}
	if a[i].JobName == "Summary" {
		return true
	}
	if a[i].JobName != a[j].JobName {
		return strings.Compare(a[i].JobName, a[j].JobName) < 0
	}

	return strings.Compare(a[i].JobName, a[j].JobName) < 0
}

func listTestsForComponent(dbc *db.DB, componentName string) ([]TestForComponent, error) {
	ret := make([]TestForComponent, 0)
	q := dbc.DB.Raw(`SELECT * from component_tests_by_job where component_name=@component_name`, sql.Named("component_name", componentName))
	if q.Error != nil {
		return nil, q.Error
	}
	q.Scan(&ret)
	return ret, nil
}

func testForComponentForDisplay(in []TestForComponent, componentName string) ([][]TestForComponent, error) {
	// simple, not efficient, not sparse

	type testKey struct {
		suiteName string
		testName  string
	}

	tests := map[testKey][]TestForComponent{}
	testKeys := []testKey{}
	jobNameToID := map[string]int{}
	testToID := map[string]int{}
	suiteToID := map[string]int{}
	for i := range in {
		curr := in[i]
		currKey := testKey{
			suiteName: curr.SuiteName,
			testName:  curr.TestName,
		}
		testKeys = append(testKeys, currKey)
		tests[currKey] = append(tests[currKey], curr)
		testToID[curr.TestName] = curr.TestID
		suiteToID[curr.SuiteName] = curr.SuiteID

		jobNameToID[curr.JobName] = curr.JobID
	}

	// add entries for missing jobs and sort
	for testKey, jobData := range tests {
		if len(jobData) == len(jobNameToID) {
			sort.Sort(byComponentAndTest(jobData))
			tests[testKey] = jobData
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
			jobData = append(jobData, TestForComponent{
				TestID:            testToID[testKey.testName],
				SuiteID:           suiteToID[testKey.suiteName],
				ComponentID:       jobData[0].ComponentID,
				JobID:             jobID,
				SuccessCount:      0,
				RunningCount:      0,
				FailureCount:      0,
				FlakeCount:        0,
				WorkingCount:      0,
				TotalCount:        0,
				SuccessPercentage: 0,
				FailurePercentage: 0,
				FlakingPercentage: 0,
				WorkingPercentage: 0,

				ComponentName: jobData[0].ComponentName,
				JobName:       jobName,
				SuiteName:     testKey.suiteName,
				TestName:      testKey.testName,

				Variants: nil,
			})
			tests[testKey] = jobData
		}

		sort.Sort(byComponentAndTest(jobData))
		tests[testKey] = jobData
	}

	ret := [][]TestForComponent{}
	for testKey := range tests {
		jobData := tests[testKey]
		currRow := []TestForComponent{summarizeTestForComponent(jobData)}
		currRow = append(currRow, jobData...)
		ret = append(ret, currRow)
	}

	// tidy names. This should be replaced by something that actually looks at variants
	for i := range ret {
		for j := range ret[i] {
			currJob := ret[i][j]
			if index := strings.Index(currJob.JobName, "4.12"); index > 0 {
				ret[i][j].JobName = currJob.JobName[index+4:]
			}
			ret[i][j].JobName = strings.ReplaceAll(ret[i][j].JobName, "-", " ")
		}
	}

	return ret, nil
}

func summarizeTestForComponent(in []TestForComponent) TestForComponent {
	failed := false
	for i := range in {
		curr := in[i]
		if curr.WorkingPercentage < 95 {
			failed = true
			break
		}
	}

	ret := TestForComponent{
		TestID:            in[0].TestID,
		SuiteID:           in[0].SuiteID,
		ComponentID:       in[0].ComponentID,
		JobID:             -2,
		SuccessCount:      0,
		RunningCount:      0,
		FailureCount:      0,
		FlakeCount:        0,
		WorkingCount:      0,
		TotalCount:        0,
		SuccessPercentage: 0,
		FailurePercentage: 0,
		FlakingPercentage: 0,
		WorkingPercentage: 0,
		ComponentName:     in[0].ComponentName,
		JobName:           "Summary",
		SuiteName:         in[0].SuiteName,
		TestName:          in[0].TestName,
		Variants:          nil,
	}
	if !failed {
		ret.SuccessCount = 1
		ret.TotalCount = 1
		ret.SuccessPercentage = 100
		ret.WorkingPercentage = 100
	}

	return ret
}

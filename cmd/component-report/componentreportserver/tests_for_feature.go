package componentreportserver

import (
	"bytes"
	"database/sql"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"

	"github.com/lib/pq"
	"github.com/openshift/sippy/pkg/db"
)

// aws, ovn, arm64, techpreview

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
	if len(segments) != 4 {
		errToReport = fmt.Errorf("bad format")
		return
	}
	componentName := segments[2]
	featureName := segments[3]

	testsForComponent, err := listTestsForComponent(s.databaseConnection, componentName, featureName)
	if err != nil {
		errToReport = err
		return
	}

	toDisplay, err := testForComponentForDisplay(testsForComponent)
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
	fmt.Fprintf(buf, "<h2>%v</h2>\n", featureName)
	fmt.Fprintf(buf, "<table>\n")
	fmt.Fprintf(buf, "<table>\n")
	fmt.Fprintf(buf, "<table>\n")
	fmt.Fprintf(buf, "\t<tr>\n")
	fmt.Fprintf(buf, "\t<th>%v</th>\n", "Suite")
	fmt.Fprintf(buf, "\t<th>%v</th>\n", "Test")
	for _, curr := range toDisplay[0] {
		fmt.Fprintf(buf, "\t<th>%v</th>\n", curr.VariantSelectorName)
	}
	fmt.Fprintf(buf, "\t</tr>\n")

	for _, currRow := range toDisplay {
		fmt.Fprintf(buf, "\t<tr>\n")
		fmt.Fprintf(buf, "\t<td>%v</td>\n", currRow[0].SuiteName)
		fmt.Fprintf(buf, "\t<td>%v</td>\n", currRow[0].TestName)
		for _, currJob := range currRow {
			color := ""
			text := fmt.Sprintf("%.0f%%", currJob.WorkingPercentage)
			switch {
			case currJob.TotalCount == 0:
				// no color
				text = ""
			case currJob.WorkingPercentage < 95:
				color = "#b35656"
			default:
				color = "##4a8242"
			}
			fmt.Fprintf(buf, "\t<td bgcolor=\"%v\">%v</td>\n", color, text)
		}
		fmt.Fprintf(buf, "\t</tr>\n")
	}

	fmt.Fprintf(buf, "</table>\n")
	fmt.Fprintf(buf, "</body>\n")

}

type TestForComponent struct {
	TestID            int     `json:"test_id"`
	SuiteID           int     `json:"suite_id"`
	VariantSelectorID int     `json:"variant_selector_id"`
	ComponentID       int     `json:"component_id"`
	FeatureID         int     `json:"feature_id"`
	SuccessCount      int     `json:"success_count"`
	RunningCount      int     `json:"running_count"`
	FailureCount      int     `json:"failure_count"`
	FlakeCount        int     `json:"flake_count"`
	WorkingCount      int     `json:"working_count"`
	TotalCount        int     `json:"total_count"`
	SuccessPercentage float64 `json:"success_percentage"`
	FailurePercentage float64 `json:"failure_percentage"`
	FlakingPercentage float64 `json:"flaking_percentage"`
	WorkingPercentage float64 `json:"working_percentage"`

	ComponentName       string         `json:"component_name"`
	FeatureName         string         `json:"feature_name"`
	VariantSelectorName string         `json:"variant_selector_name"`
	SuiteName           string         `json:"suite_name"`
	TestName            string         `json:"test_name"`
	Variants            pq.StringArray `json:"variants" gorm:"type:text[]"`
}

type byComponentAndTest []TestForComponent

func (a byComponentAndTest) Len() int      { return len(a) }
func (a byComponentAndTest) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a byComponentAndTest) Less(i, j int) bool {
	if a[i].ComponentName != a[j].ComponentName {
		return strings.Compare(a[i].ComponentName, a[j].ComponentName) < 0
	}
	if a[i].FeatureName != a[j].FeatureName {
		return strings.Compare(a[i].FeatureName, a[j].FeatureName) < 0
	}
	if a[i].SuiteName != a[j].SuiteName {
		return strings.Compare(a[i].SuiteName, a[j].SuiteName) < 0
	}
	if a[i].TestName != a[j].TestName {
		return strings.Compare(a[i].TestName, a[j].TestName) < 0
	}
	if a[i].VariantSelectorName == "Summary" {
		return true
	}
	if a[i].VariantSelectorName != a[j].VariantSelectorName {
		return strings.Compare(a[i].VariantSelectorName, a[j].VariantSelectorName) < 0
	}

	return strings.Compare(a[i].VariantSelectorName, a[j].VariantSelectorName) < 0
}

func listTestsForComponent(dbc *db.DB, componentName, featureName string) ([]TestForComponent, error) {
	ret := make([]TestForComponent, 0)
	q := dbc.DB.Raw(`SELECT * from feature_tests_by_job_2 where component_name=@component_name and feature_name=@feature_name`, sql.Named("component_name", componentName), sql.Named("feature_name", featureName))
	if q.Error != nil {
		return nil, q.Error
	}
	q.Scan(&ret)
	return ret, nil
}

type testKey struct {
	suiteName string
	testName  string
}

type bySuiteAndTest []testKey

func (a bySuiteAndTest) Len() int      { return len(a) }
func (a bySuiteAndTest) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a bySuiteAndTest) Less(i, j int) bool {
	if a[i].suiteName != a[j].suiteName {
		return strings.Compare(a[i].suiteName, a[j].suiteName) < 0
	}
	return strings.Compare(a[i].testName, a[j].testName) < 0
}

func testForComponentForDisplay(in []TestForComponent) ([][]TestForComponent, error) {
	// simple, not efficient, not sparse

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
		if _, ok := tests[currKey]; !ok {
			testKeys = append(testKeys, currKey)
		}
		tests[currKey] = append(tests[currKey], curr)
		testToID[curr.TestName] = curr.TestID
		suiteToID[curr.SuiteName] = curr.SuiteID

		jobNameToID[curr.VariantSelectorName] = curr.VariantSelectorID
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
				if currJobData.VariantSelectorName == jobName {
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
				FeatureID:         jobData[0].FeatureID,
				VariantSelectorID: jobID,
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

				ComponentName:       jobData[0].ComponentName,
				FeatureName:         jobData[0].FeatureName,
				VariantSelectorName: jobName,
				SuiteName:           testKey.suiteName,
				TestName:            testKey.testName,

				Variants: nil,
			})
			tests[testKey] = jobData
		}

		sort.Sort(byComponentAndTest(jobData))
		tests[testKey] = jobData
	}

	ret := [][]TestForComponent{}
	sort.Sort(bySuiteAndTest(testKeys))
	for _, testKey := range testKeys {
		jobData := tests[testKey]
		currRow := []TestForComponent{summarizeTestForComponent(jobData)}
		currRow = append(currRow, jobData...)
		ret = append(ret, currRow)
	}

	// tidy names. This should be replaced by something that actually looks at variants
	for i := range ret {
		for j := range ret[i] {
			currJob := ret[i][j]
			if index := strings.Index(currJob.VariantSelectorName, "4.12"); index > 0 {
				ret[i][j].VariantSelectorName = currJob.VariantSelectorName[index+4:]
			}
			ret[i][j].VariantSelectorName = strings.ReplaceAll(ret[i][j].VariantSelectorName, "-", " ")
		}
	}

	return ret, nil
}

func summarizeTestForComponent(in []TestForComponent) TestForComponent {
	failed := false
	lowestWorking := 100.0
	lowestSuccess := 100.0
	highestFailure := 0.0
	highestFlake := 0.0
	for i := range in {
		curr := in[i]
		if curr.TotalCount == 0 {
			continue
		}
		lowestWorking = math.Min(lowestWorking, curr.WorkingPercentage)
		lowestSuccess = math.Min(lowestSuccess, curr.SuccessPercentage)
		highestFailure = math.Max(highestFailure, curr.FailurePercentage)
		highestFlake = math.Max(highestFlake, curr.FlakingPercentage)

		if curr.WorkingPercentage < 95 {
			failed = true
		}
	}

	ret := TestForComponent{
		TestID:              in[0].TestID,
		SuiteID:             in[0].SuiteID,
		ComponentID:         in[0].ComponentID,
		FeatureID:           in[0].FeatureID,
		VariantSelectorID:   -2,
		SuccessCount:        0,
		RunningCount:        0,
		FailureCount:        0,
		FlakeCount:          0,
		WorkingCount:        0,
		TotalCount:          0,
		SuccessPercentage:   lowestWorking,
		FailurePercentage:   highestFailure,
		FlakingPercentage:   highestFlake,
		WorkingPercentage:   lowestWorking,
		ComponentName:       in[0].ComponentName,
		FeatureName:         in[0].FeatureName,
		VariantSelectorName: "Summary",
		SuiteName:           in[0].SuiteName,
		TestName:            in[0].TestName,
		Variants:            nil,
	}
	if !failed {
		ret.SuccessCount = 1
		ret.TotalCount = 1
		ret.SuccessPercentage = lowestWorking
		ret.WorkingPercentage = lowestWorking
	} else {
		ret.FailureCount = 1
		ret.TotalCount = 1
	}

	return ret
}

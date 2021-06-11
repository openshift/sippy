package v1

type TestStatus int

const (
	TestStatusAbsent  TestStatus = 0
	TestStatusSuccess TestStatus = 1
	TestStatusRunning TestStatus = 4
	TestStatusFailure TestStatus = 12
	TestStatusFlake   TestStatus = 13
)

// testgrid datastructures
type JobSummary struct {
	OverallStatus string `json:"overall_status"`
}

type JobDetails struct {
	Name       string
	Tests      []Test `json:"tests"`
	Timestamps []int  `json:"timestamps"`
	// append to https://prow.ci.openshift.org/view/gcs and suffix with changelist element to view job run details
	Query       string   `json:"query"`
	ChangeLists []string `json:"changelists"`
	// not part of testgrid json, but we want to store the url of the testgrid job page for later usage
	TestGridUrl string
}

type Test struct {
	Name     string       `json:"name"`
	Statuses []TestResult `json:"statuses"`
}

type TestResult struct {
	Count int        `json:"count"`
	Value TestStatus `json:"value"`
}

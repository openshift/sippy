package testgrid

// testgrid datastructures
type JobSummary struct {
	OverallStatus string `json:"overall_status"`
}

type JobDetails struct {
	Name       string
	Tests      []Test `json:"tests"`
	Timestamps []int  `json:"timestamps"`
	// append to https://prow.svc.ci.openshift.org/view/gcs and suffix with changelist element to view job run details
	Query       string   `json:"query"`
	ChangeLists []string `json:"changelists"`
}

type Test struct {
	Name     string       `json:"name"`
	Statuses []TestResult `json:"statuses"`
}

type TestResult struct {
	Count int `json:"count"`
	Value int `json:"value"`
}

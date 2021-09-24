// Package api contains types suitable for use with Material UI data tables.
package api

import (
	"fmt"

	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"

	bugsv1 "github.com/openshift/sippy/pkg/apis/bugs/v1"
)

type ColumnType int

const (
	ColumnTypeString ColumnType = iota
	ColumnTypeNumerical
	ColumnTypeArray
)

type Sort string

const (
	SortAscending  Sort = "asc"
	SortDescending Sort = "desc"
)

// Job contains the full accounting of a job's history, with a synthetic ID. The format of
// this struct is suitable for use in a data table.
type Job struct {
	ID        int      `json:"id"`
	Name      string   `json:"name"`
	BriefName string   `json:"brief_name"`
	Variants  []string `json:"variants"`

	Network                     string   `json:"network,omitempty"`
	IPMode                      string   `json:"ip_mode,omitempty"`
	Topology                    string   `json:"topology,omitempty"`
	TestSuites                  []string `json:"test_suites,omitempty"`
	GCSBucketName               string   `json:"gcs_bucket_name,omitempty"`
	GCSJobHistoryLocationPrefix string   `json:"gcs_job_history_location_prefix,omitempty"`

	CurrentPassPercentage          float64 `json:"current_pass_percentage"`
	CurrentProjectedPassPercentage float64 `json:"current_projected_pass_percentage"`
	CurrentRuns                    int     `json:"current_runs"`

	PreviousPassPercentage          float64 `json:"previous_pass_percentage"`
	PreviousProjectedPassPercentage float64 `json:"previous_projected_pass_percentage"`
	PreviousRuns                    int     `json:"previous_runs"`
	NetImprovement                  float64 `json:"net_improvement"`

	Tags           []string     `json:"tags"`
	TestGridURL    string       `json:"test_grid_url"`
	Bugs           []bugsv1.Bug `json:"bugs"`
	AssociatedBugs []bugsv1.Bug `json:"associated_bugs"`
}

func (job Job) GetFieldType(param string) ColumnType {
	switch param {
	//nolint:goconst
	case "name":
		return ColumnTypeString
	case "briefName":
		return ColumnTypeString
	//nolint:goconst
	case "variants":
		return ColumnTypeArray
	//nolint:goconst
	case "tags":
		return ColumnTypeArray
	case "test_grid_url":
		return ColumnTypeString
	case "network", "ip_mode", "topology", "gcs_bucket_name", "gcs_job_history_location_prefix":
		return ColumnTypeString
	case "test_suites":
		return ColumnTypeArray
	default:
		return ColumnTypeNumerical
	}
}

func (job Job) GetStringValue(param string) (string, error) {
	switch param {
	case "name":
		return job.Name, nil
	case "briefName":
		return job.BriefName, nil
	case "test_grid_url":
		return job.TestGridURL, nil
	case "network":
		return job.Network, nil
	case "ip_mode":
		return job.IPMode, nil
	case "topology":
		return job.Topology, nil
	case "gcs_bucket_name":
		return job.GCSBucketName, nil
	case "gcs_job_history_location_prefix":
		return job.GCSJobHistoryLocationPrefix, nil
	default:
		return "", fmt.Errorf("unknown string field %s", param)
	}
}

func (job Job) GetNumericalValue(param string) (float64, error) {
	switch param {
	case "id":
		return float64(job.ID), nil
	case "current_pass_percentage":
		return job.CurrentPassPercentage, nil
	case "current_projected_pass_percentage":
		return job.CurrentProjectedPassPercentage, nil
	case "current_runs":
		return float64(job.CurrentRuns), nil
	case "previous_pass_percentage":
		return job.PreviousPassPercentage, nil
	case "previous_projected_pass_percentage":
		return job.PreviousProjectedPassPercentage, nil
	case "previous_runs":
		return float64(job.PreviousRuns), nil
	case "net_improvement":
		return job.NetImprovement, nil
	case "bugs":
		return float64(len(job.Bugs)), nil
	case "associated_bugs":
		return float64(len(job.AssociatedBugs)), nil
	default:
		return 0, fmt.Errorf("unknown numerical field %s", param)
	}
}

func (job Job) GetArrayValue(param string) ([]string, error) {
	switch param {
	case "tags":
		return job.Tags, nil
	case "variants":
		return job.Variants, nil
	case "test_suites":
		return job.TestSuites, nil
	default:
		return nil, fmt.Errorf("unknown array value field %s", param)
	}
}

// JobRun contains a full accounting of a job run's history, with a synthetic ID.
type JobRun struct {
	ID           int      `json:"id"`
	BriefName    string   `json:"brief_name"`
	Variants     []string `json:"variants"`
	Tags         []string `json:"tags"`
	TestGridURL  string   `json:"testGridURL"`
	ArtifactsURL string   `json:"artifactsURL"`
	v1.JobRunResult
}

func (run JobRun) GetFieldType(param string) ColumnType {
	switch param {
	case "name":
		return ColumnTypeString
	case "tags":
		return ColumnTypeArray
	case "job":
		return ColumnTypeString
	case "result":
		return ColumnTypeString
	case "failedTestNames":
		return ColumnTypeArray
	case "variants":
		return ColumnTypeArray
	case "testGridURL", "artifactsURL", "url":
		return ColumnTypeString
	default:
		return ColumnTypeNumerical
	}
}

func (run JobRun) GetStringValue(param string) (string, error) {
	switch param {
	case "job", "name":
		return run.Job, nil
	case "result":
		return string(run.OverallResult), nil
	case "testGridURL":
		return run.TestGridURL, nil
	case "artifactsURL":
		return run.ArtifactsURL, nil
	case "url":
		return run.URL, nil
	default:
		return "", fmt.Errorf("unknown string field %s", param)
	}
}

func (run JobRun) GetNumericalValue(param string) (float64, error) {
	switch param {
	case "id":
		return float64(run.ID), nil
	case "testFailures":
		return float64(run.TestFailures), nil
	case "timestamp":
		return float64(run.Timestamp), nil
	default:
		return 0, fmt.Errorf("unknown numerical field %s", param)
	}
}

func (run JobRun) GetArrayValue(param string) ([]string, error) {
	switch param {
	case "failedTestNames":
		return run.FailedTestNames, nil
	case "tags":
		return run.Tags, nil
	case "variants":
		return run.Variants, nil
	default:
		return nil, fmt.Errorf("unknown array field %s", param)
	}
}

// Test contains the full accounting of a test's history, with a synthetic ID. The format
// of this struct is suitable for use in a data table.
type Test struct {
	ID   int    `json:"id"`
	Name string `json:"name"`

	CurrentSuccesses      int     `json:"current_successes"`
	CurrentFailures       int     `json:"current_failures"`
	CurrentFlakes         int     `json:"current_flakes"`
	CurrentPassPercentage float64 `json:"current_pass_percentage"`
	CurrentRuns           int     `json:"current_runs"`

	PreviousSuccesses      int     `json:"previous_successes"`
	PreviousFailures       int     `json:"previous_failures"`
	PreviousFlakes         int     `json:"previous_flakes"`
	PreviousPassPercentage float64 `json:"previous_pass_percentage"`
	PreviousRuns           int     `json:"previous_runs"`
	NetImprovement         float64 `json:"net_improvement"`

	Tags           []string     `json:"tags"`
	Bugs           []bugsv1.Bug `json:"bugs"`
	AssociatedBugs []bugsv1.Bug `json:"associated_bugs"`
}

func (test Test) GetFieldType(param string) ColumnType {
	switch param {
	case "name":
		return ColumnTypeString
	case "tags":
		return ColumnTypeArray
	default:
		return ColumnTypeNumerical
	}
}

func (test Test) GetStringValue(param string) (string, error) {
	switch param {
	case "name":
		return test.Name, nil
	default:
		return "", fmt.Errorf("unknown string field %s", param)
	}
}

func (test Test) GetNumericalValue(param string) (float64, error) {
	switch param {
	case "id":
		return float64(test.ID), nil
	case "current_successes":
		return float64(test.CurrentSuccesses), nil
	case "current_failures":
		return float64(test.CurrentFailures), nil
	case "current_flakes":
		return float64(test.CurrentFlakes), nil
	case "current_pass_percentage":
		return test.CurrentPassPercentage, nil
	case "current_runs":
		return float64(test.CurrentRuns), nil
	case "previous_successes":
		return float64(test.PreviousSuccesses), nil
	case "previous_failures":
		return float64(test.PreviousFailures), nil
	case "previous_flakes":
		return float64(test.PreviousFlakes), nil
	case "previous_pass_percentage":
		return test.PreviousPassPercentage, nil
	case "previous_runs":
		return float64(test.PreviousRuns), nil
	case "net_improvement":
		return test.NetImprovement, nil
	case "bugs":
		return float64(len(test.Bugs)), nil
	case "associated_bugs":
		return float64(len(test.AssociatedBugs)), nil
	default:
		return 0, fmt.Errorf("unknown numerical field %s", param)
	}
}

func (test Test) GetArrayValue(param string) ([]string, error) {
	switch param {
	case "tags":
		return test.Tags, nil
	default:
		return nil, fmt.Errorf("unknown array value field %s", param)
	}
}

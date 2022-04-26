// Package api contains types suitable for use with Material UI data tables.
package api

import (
	"fmt"

	"github.com/lib/pq"

	bugsv1 "github.com/openshift/sippy/pkg/apis/bugs/v1"
	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
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

type Variant struct {
	ID   int    `json:"id"`
	Name string `json:"name"`

	CurrentPassPercentage float64 `json:"current_pass_percentage"`
	CurrentRuns           int     `json:"current_runs"`
	CurrentPasses         int     `json:"current_passes,omitempty"`
	CurrentFails          int     `json:"current_fails,omitempty"`

	PreviousPassPercentage float64 `json:"previous_pass_percentage"`
	PreviousRuns           int     `json:"previous_runs"`
	PreviousPasses         int     `json:"previous_passes,omitempty"`
	PreviousFails          int     `json:"previous_fails,omitempty"`

	NetImprovement float64 `json:"net_improvement"`
}

// Job contains the full accounting of a job's history, with a synthetic ID. The format of
// this struct is suitable for use in a data table.
// TODO: with move to database, IDs will no longer be synthetic, although they will change in the event
// the database is rebuilt from testgrid data.
type Job struct {
	ID        int            `json:"id"`
	Name      string         `json:"name"`
	BriefName string         `json:"brief_name"`
	Variants  pq.StringArray `json:"variants" gorm:"type:text[]"`

	CurrentPassPercentage          float64 `json:"current_pass_percentage"`
	CurrentProjectedPassPercentage float64 `json:"current_projected_pass_percentage"`
	CurrentRuns                    int     `json:"current_runs"`
	CurrentPasses                  int     `json:"current_passes,omitempty"`
	CurrentFails                   int     `json:"current_fails,omitempty"`
	CurrentInfraFails              int     `json:"current_infra_fails,omitempty"`

	PreviousPassPercentage          float64 `json:"previous_pass_percentage"`
	PreviousProjectedPassPercentage float64 `json:"previous_projected_pass_percentage"`
	PreviousRuns                    int     `json:"previous_runs"`
	PreviousPasses                  int     `json:"previous_passes,omitempty"`
	PreviousFails                   int     `json:"previous_fails,omitempty"`
	PreviousInfraFails              int     `json:"previous_infra_fails,omitempty"`
	NetImprovement                  float64 `json:"net_improvement"`

	TestGridURL    string       `json:"test_grid_url"`
	Bugs           []bugsv1.Bug `json:"bugs" gorm:"-"`
	AssociatedBugs []bugsv1.Bug `json:"associated_bugs" gorm:"-"`
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
	//nolint:goconst
	case "test_grid_url":
		return ColumnTypeString
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
	case "variants":
		return job.Variants, nil
	default:
		return nil, fmt.Errorf("unknown array value field %s", param)
	}
}

// JobRun contains a full accounting of a job run's history, with a synthetic ID.
type JobRun struct {
	ID                    int                 `json:"id"`
	BriefName             string              `json:"brief_name"`
	Variants              pq.StringArray      `json:"variants" gorm:"type:text[]"`
	Tags                  pq.StringArray      `json:"tags" gorm:"type:text[]"`
	TestGridURL           string              `json:"test_grid_url"`
	ProwID                uint                `json:"prow_id"`
	Job                   string              `json:"job"`
	URL                   string              `json:"url"`
	TestFlakes            int                 `json:"test_flakes"`
	FlakedTestNames       pq.StringArray      `json:"flaked_test_names" gorm:"type:text[]"`
	TestFailures          int                 `json:"test_failures"`
	FailedTestNames       pq.StringArray      `json:"failed_test_names" gorm:"type:text[]"`
	Failed                bool                `json:"failed"`
	InfrastructureFailure bool                `json:"infrastructure_failure"`
	KnownFailure          bool                `json:"known_failure"`
	Succeeded             bool                `json:"succeeded"`
	Timestamp             int                 `json:"timestamp"`
	OverallResult         v1.JobOverallResult `json:"result"`
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
	case "failed_test_names":
		return ColumnTypeArray
	case "flaked_test_names":
		return ColumnTypeArray
	case "variants":
		return ColumnTypeArray
	case "test_grid_url":
		return ColumnTypeString
	case "timestamp":
		return ColumnTypeNumerical
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
	case "test_grid_url":
		return run.TestGridURL, nil
	default:
		return "", fmt.Errorf("unknown string field %s", param)
	}
}

func (run JobRun) GetNumericalValue(param string) (float64, error) {
	switch param {
	case "id":
		return float64(run.ID), nil
	case "test_failures":
		return float64(run.TestFailures), nil
	case "timestamp":
		return float64(run.Timestamp), nil
	default:
		return 0, fmt.Errorf("unknown numerical field %s", param)
	}
}

func (run JobRun) GetArrayValue(param string) ([]string, error) {
	switch param {
	case "failed_test_names":
		return run.FailedTestNames, nil
	case "flaked_test_names":
		return run.FlakedTestNames, nil
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
	ID      int    `json:"id,omitempty"`
	Name    string `json:"name"`
	Variant string `json:"variant,omitempty"`

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

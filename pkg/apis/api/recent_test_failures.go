package api

import (
	"fmt"
	"time"
)

// RecentTestFailure represents a test that failed during the queried period.
type RecentTestFailure struct {
	TestID        uint                      `json:"test_id"`
	TestName      string                    `json:"test_name"`
	SuiteName     string                    `json:"suite_name,omitempty"`
	JiraComponent string                    `json:"jira_component,omitempty"`
	FailureCount  int                       `json:"failure_count"`
	FirstFailure  time.Time                 `json:"first_failure"`
	LastFailure   time.Time                 `json:"last_failure"`
	LastPass      *time.Time                `json:"last_pass,omitempty"`
	Outputs       []RecentTestFailureOutput `json:"outputs,omitempty" gorm:"-"`
}

// RecentTestFailureOutput is a single failure instance with its job run context.
type RecentTestFailureOutput struct {
	ProwJobRunID uint      `json:"prow_job_run_id"`
	ProwJobName  string    `json:"prow_job_name"`
	ProwJobURL   string    `json:"prow_job_url"`
	FailedAt     time.Time `json:"failed_at"`
	Output       string    `json:"output"`
}

func (r RecentTestFailure) GetFieldType(param string) ColumnType {
	switch param {
	case "test_name", "suite_name", "jira_component":
		return ColumnTypeString
	default:
		return ColumnTypeNumerical
	}
}

func (r RecentTestFailure) GetStringValue(param string) (string, error) {
	switch param {
	case "test_name":
		return r.TestName, nil
	case "suite_name":
		return r.SuiteName, nil
	case "jira_component":
		return r.JiraComponent, nil
	default:
		return "", fmt.Errorf("unknown string field %s", param)
	}
}

func (r RecentTestFailure) GetNumericalValue(param string) (float64, error) {
	switch param {
	case "test_id":
		return float64(r.TestID), nil
	case "failure_count":
		return float64(r.FailureCount), nil
	case "first_failure":
		return float64(r.FirstFailure.Unix()), nil
	case "last_failure":
		return float64(r.LastFailure.Unix()), nil
	case "last_pass":
		if r.LastPass != nil {
			return float64(r.LastPass.Unix()), nil
		}
		return 0, nil
	default:
		return 0, fmt.Errorf("unknown numerical field %s", param)
	}
}

func (r RecentTestFailure) GetArrayValue(param string) ([]string, error) {
	return nil, fmt.Errorf("unknown array value field %s", param)
}

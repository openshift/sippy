// Package api contains types suitable for use with Material UI data tables.
package api

import (
	"fmt"

	"github.com/lib/pq"

	"github.com/openshift/sippy/pkg/db/models"

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
	Cluster               string              `json:"cluster"`
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
	OverallResult         v1.JobOverallResult `json:"overall_result"`
}

func (run JobRun) GetFieldType(param string) ColumnType {
	switch param {
	case "name":
		return ColumnTypeString
	case "cluster":
		return ColumnTypeString
	case "tags":
		return ColumnTypeArray
	case "job":
		return ColumnTypeString
	case "overall_result":
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
	case "cluster":
		return run.Cluster, nil
	case "overall_result":
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
	ID       int            `json:"id,omitempty"`
	Name     string         `json:"name"`
	Variant  string         `json:"variant,omitempty"`
	Variants pq.StringArray `json:"variants" gorm:"type:text[]"`

	CurrentSuccesses         int     `json:"current_successes"`
	CurrentFailures          int     `json:"current_failures"`
	CurrentFlakes            int     `json:"current_flakes"`
	CurrentPassPercentage    float64 `json:"current_pass_percentage"`
	CurrentFailurePercentage float64 `json:"current_failure_percentage"`
	CurrentFlakePercentage   float64 `json:"current_flake_percentage"`
	CurrentWorkingPercentage float64 `json:"current_working_percentage"`
	CurrentRuns              int     `json:"current_runs"`

	PreviousSuccesses         int     `json:"previous_successes"`
	PreviousFailures          int     `json:"previous_failures"`
	PreviousFlakes            int     `json:"previous_flakes"`
	PreviousPassPercentage    float64 `json:"previous_pass_percentage"`
	PreviousFailurePercentage float64 `json:"previous_failure_percentage"`
	PreviousFlakePercentage   float64 `json:"previous_flake_percentage"`
	PreviousWorkingPercentage float64 `json:"previous_working_percentage"`
	PreviousRuns              int     `json:"previous_runs"`

	NetFailureImprovement float64 `json:"net_failure_improvement"`
	NetFlakeImprovement   float64 `json:"net_flake_improvement"`
	NetWorkingImprovement float64 `json:"net_working_improvement"`
	NetImprovement        float64 `json:"net_improvement"`

	WorkingAverage           float64 `json:"working_average,omitempty"`
	WorkingStandardDeviation float64 `json:"working_standard_deviation,omitempty"`
	DeltaFromWorkingAverage  float64 `json:"delta_from_working_average,omitempty"`
	PassingAverage           float64 `json:"passing_average,omitempty"`
	PassingStandardDeviation float64 `json:"passing_standard_deviation,omitempty"`
	DeltaFromPassingAverage  float64 `json:"delta_from_passing_average,omitempty"`
	FlakeAverage             float64 `json:"flake_average,omitempty"`
	FlakeStandardDeviation   float64 `json:"flake_standard_deviation,omitempty"`
	DeltaFromFlakeAverage    float64 `json:"delta_from_flake_average,omitempty"`

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
	case "variant":
		return ColumnTypeString
	case "variants":
		return ColumnTypeArray
	default:
		return ColumnTypeNumerical
	}
}

func (test Test) GetStringValue(param string) (string, error) {
	switch param {
	case "name":
		return test.Name, nil
	case "variant":
		return test.Variant, nil
	default:
		return "", fmt.Errorf("unknown string field %s", param)
	}
}

//
// nolint:gocyclo
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
	case "current_flake_percentage":
		return test.CurrentFlakePercentage, nil
	case "current_failure_percentage":
		return test.CurrentFailurePercentage, nil
	case "current_working_percentage":
		return test.CurrentWorkingPercentage, nil
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
	case "previous_flake_percentage":
		return test.PreviousFlakePercentage, nil
	case "previous_failure_percentage":
		return test.PreviousFailurePercentage, nil
	case "previous_working_percentage":
		return test.PreviousWorkingPercentage, nil
	case "previous_runs":
		return float64(test.PreviousRuns), nil
	case "net_failure_improvement":
		return test.NetFailureImprovement, nil
	case "net_flake_improvement":
		return test.NetFlakeImprovement, nil
	case "net_improvement":
		return test.NetImprovement, nil
	case "net_working_improvement":
		return test.NetWorkingImprovement, nil
	case "bugs":
		return float64(len(test.Bugs)), nil
	case "associated_bugs":
		return float64(len(test.AssociatedBugs)), nil
	case "delta_from_working_average":
		return test.DeltaFromWorkingAverage, nil
	case "working_average":
		return test.WorkingAverage, nil
	case "working_standard_deviation":
		return test.WorkingStandardDeviation, nil
	case "delta_from_passing_average":
		return test.DeltaFromPassingAverage, nil
	case "passing_average":
		return test.PassingAverage, nil
	case "passing_standard_deviation":
		return test.PassingStandardDeviation, nil
	case "delta_from_flake_average":
		return test.DeltaFromFlakeAverage, nil
	case "flake_average":
		return test.FlakeAverage, nil
	case "flake_standard_deviation":
		return test.FlakeStandardDeviation, nil
	default:
		return 0, fmt.Errorf("unknown numerical field %s", param)
	}
}

func (test Test) GetArrayValue(param string) ([]string, error) {
	switch param {
	case "tags":
		return test.Tags, nil
	case "variants":
		return test.Variants, nil
	default:
		return nil, fmt.Errorf("unknown array value field %s", param)
	}
}

const (
	PayloadAccepted = "Accepted"
	PayloadRejected = "Rejected"
)

// ReleaseHealthReport contains information about the latest health of release payloads for a specific tag.
type ReleaseHealthReport struct {
	models.ReleaseTag
	LastPhase string `json:"last_phase"`
	Count     int    `json:"count"`
	// PhaseCounts contains the total count of payloads in each phase over several time periods.
	PhaseCounts PayloadPhaseCounts `json:"phase_counts"`
	// PayloadStatistics contains the min, mean, and max times between accepted payloads
	// over several time periods.
	PayloadStatistics PayloadStatistics `json:"acceptance_statistics"`
}

type PayloadPhaseCounts struct {
	// CurrentWeek contains payload phase counts over the past week.
	CurrentWeek PayloadPhaseCount `json:"current_week"`
	// Total contains payload phase counts over the entire release.
	Total PayloadPhaseCount `json:"total"`
}

type PayloadStatistics struct {
	// CurrentWeek contains the min, mean, and max times between accepted payloads over the past week.
	CurrentWeek PayloadStatistic `json:"current_week"`
	// Total contains the min, mean, and max times between accepted payloads over the entire release.
	Total PayloadStatistic `json:"total"`
}

type PayloadStatistic struct {
	models.PayloadStatistics
}

type PayloadPhaseCount struct {
	Accepted int `json:"accepted"`
	Rejected int `json:"rejected"`
}

// PayloadStreamAnalysis contains a report on the health of a given payload stream.
type PayloadStreamAnalysis struct {
	TestFailures     []*TestFailureAnalysis `json:"test_failures"`
	PayloadsAnalyzed int                    `json:"payloads_analyzed"`
	LastPhase        string                 `json:"last_phase"`
	// LastPhaseCount is the number of payloads in LastPhase. (i.e. there have been X concurrent Accepted/Rejected payloads)
	LastPhaseCount int `json:"last_phase_count"`
	// ConsecutiveFailedPayloads contains the list of most recent consecutive failed payloads, assuming LastPhase
	// is Rejected. If it is Accepted, this slice will be empty.
	ConsecutiveFailedPayloads []string `json:"consecutive_failed_payloads"`
}

// TestFailureAnalysis represents a test and the number of times it failed over some number of jobs.
type TestFailureAnalysis struct {
	Name string `json:"name"`
	ID   uint   `json:"id"`
	// FailureCount is the total number of times this test failed in the payloads queried.
	FailureCount int `json:"failure_count"`

	// BlockerScore represents our confidence this is a blocker, ranges from 0 -> 100, with 100 being near
	// certain this is a payload blocker.
	BlockerScore int `json:"blocker_score"`

	// BlockerScoreReasons explain to humans why the blocker_score was given.
	BlockerScoreReasons []string `json:"blocker_score_reasons"`

	// FailedPayloads contains information about where this test failed in a specific rejected payload.
	FailedPayloads map[string]*FailedPayload `json:"failed_payloads"`
}

type FailedPayload struct {
	// FailedJobs is a list of job names the test failed in for this payload.
	FailedJobs []string `json:"failed_jobs"`
	// FailedJobRuns is a list of prow job URLs the test failed in for this payload.
	FailedJobRuns []string `json:"failed_job_runs"`
}

// PayloadEvent is an API type representing a FullCalendar.io event type, for use
// with calendering.
type PayloadEvent struct {
	Title   string `json:"title"`
	Start   string `json:"start"`
	Phase   string `json:"phase"`
	AllDay  bool   `json:"allDay"`
	Display string `json:"display,omitempty"`
}

type BuildClusterHealthAnalysis struct {
	ByPeriod map[string]BuildClusterHealth `json:"by_period"`
}

type BuildClusterHealth = models.BuildClusterHealthReport

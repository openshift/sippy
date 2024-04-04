// Package api contains types suitable for use with Material UI data tables.
package api

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
	"github.com/lib/pq"

	sippyv1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db/models"
)

type ColumnType int

const (
	ColumnTypeString ColumnType = iota
	ColumnTypeNumerical
	ColumnTypeArray
	ColumnTypeTimestamp
)

type Sort string

const (
	SortAscending  Sort = "asc"
	SortDescending Sort = "desc"
)

// PaginationResult is a type used by API endpoints that enable server-side
// pagination. It wraps the returned rows  with page information such as page
// size, which page, and the total rows.
type PaginationResult struct {
	Rows      interface{} `json:"rows"`
	PageSize  int         `json:"page_size"`
	Page      int         `json:"page"`
	TotalRows int64       `json:"total_rows"`
}

// Pagination is a type used to request specific per-page and offset values
// in an API request.
type Pagination struct {
	PerPage int `json:"per_page"`
	Page    int `json:"page"`
}

type Repository struct {
	ID          int    `json:"id"`
	Org         string `json:"org"`
	Repo        string `json:"repo"`
	JobCount    int    `json:"job_count"`
	RevertCount int    `json:"revert_count"`

	// WorstPremergeJobFailures is the average number of tries on the worst
	// performing presubmit job. For example, if e2e-aws-upgrade takes 7 tries
	// on average to merge, and e2e-gcp takes 5, this value will be 7.
	WorstPremergeJobFailures float64 `json:"worst_premerge_job_failures"`
}

func (r Repository) GetFieldType(param string) ColumnType {
	switch param {
	case "id":
		return ColumnTypeNumerical
	//nolint:goconst
	case "org":
		return ColumnTypeString
	//nolint:goconst
	case "repo":
		return ColumnTypeString
	case "job_count":
		return ColumnTypeNumerical
	case "worst_premerge_job_failures":
		return ColumnTypeNumerical
	default:
		return ColumnTypeNumerical
	}
}

func (r Repository) GetStringValue(param string) (string, error) {
	switch param {
	//nolint:goconst
	case "org":
		return r.Org, nil
	//nolint:goconst
	case "repo":
		return r.Repo, nil
	default:
		return "", fmt.Errorf("unknown string field %s", param)
	}
}

func (r Repository) GetNumericalValue(param string) (float64, error) {
	switch param {
	case "id":
		return float64(r.ID), nil
	case "job_count":
		return float64(r.JobCount), nil
	case "worst_premerge_job_failures":
		return r.WorstPremergeJobFailures, nil
	default:
		return 0, fmt.Errorf("unknown numerical field %s", param)
	}
}

func (r Repository) GetArrayValue(param string) ([]string, error) {
	return nil, fmt.Errorf("unknown array value field %s", param)
}

type PullRequest struct {
	ID       int        `json:"id"`
	Org      string     `json:"org"`
	Repo     string     `json:"repo"`
	Number   int        `json:"number"`
	Title    string     `json:"title"`
	Author   string     `json:"author"`
	SHA      string     `json:"sha"`
	Link     string     `json:"link"`
	MergedAt *time.Time `json:"merged_at"`

	FirstCiPayload             string `json:"first_ci_payload"`
	FirstCiPayloadPhase        string `json:"first_ci_payload_phase"`
	FirstCiPayloadRelease      string `json:"first_ci_payload_release"`
	FirstNightlyPayload        string `json:"first_nightly_payload"`
	FirstNightlyPayloadPhase   string `json:"first_nightly_payload_phase"`
	FirstNightlyPayloadRelease string `json:"first_nightly_payload_release"`
}

func (pr PullRequest) GetFieldType(param string) ColumnType {
	switch param {
	case "id":
		return ColumnTypeNumerical
	case "author":
		return ColumnTypeString
	//nolint:goconst
	case "org":
		return ColumnTypeString
	//nolint:goconst
	case "repo":
		return ColumnTypeString
	case "title":
		return ColumnTypeString
	case "number":
		return ColumnTypeNumerical
	case "sha":
		return ColumnTypeString
	case "link":
		return ColumnTypeString
	case "merged_at":
		return ColumnTypeTimestamp
	default:
		return ColumnTypeNumerical
	}
}

func (pr PullRequest) GetStringValue(param string) (string, error) {
	switch param {
	case "author":
		return pr.Author, nil
	case "sha":
		return pr.SHA, nil
	case "link":
		return pr.Link, nil
	case "title":
		return pr.Title, nil
	//nolint:goconst
	case "org":
		return pr.Org, nil
	//nolint:goconst
	case "repo":
		return pr.Repo, nil
	default:
		return "", fmt.Errorf("unknown string field %s", param)
	}
}

func (pr PullRequest) GetNumericalValue(param string) (float64, error) {
	switch param {
	case "id":
		return float64(pr.ID), nil
	case "number":
		return float64(pr.Number), nil
	case "merged_at":
		return float64(pr.MergedAt.Unix()), nil
	default:
		return 0, fmt.Errorf("unknown numerical field %s", param)
	}
}

func (pr PullRequest) GetArrayValue(param string) ([]string, error) {
	return nil, fmt.Errorf("unknown array value field %s", param)
}

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
	Org       string         `json:"org,omitempty"`
	Repo      string         `json:"repo,omitempty"`
	BriefName string         `json:"brief_name"`
	Variants  pq.StringArray `json:"variants" gorm:"type:text[]"`
	LastPass  *time.Time     `json:"last_pass,omitempty"`

	AverageRetestsToMerge          float64 `json:"average_retests_to_merge"`
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

	TestGridURL string `json:"test_grid_url"`
	OpenBugs    int    `json:"open_bugs"`
}

func (job Job) GetFieldType(param string) ColumnType {
	switch param {
	//nolint:goconst
	case "name":
		return ColumnTypeString
	case "briefName":
		return ColumnTypeString
	//nolint:goconst
	case "org":
		return ColumnTypeString
	//nolint:goconst
	case "repo":
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
	//nolint:goconst
	case "org":
		return job.Org, nil
	//nolint:goconst
	case "repo":
		return job.Repo, nil
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
	case "open_bugs":
		return float64(job.OpenBugs), nil
	case "average_runs_to_merge":
		return job.AverageRetestsToMerge, nil
	case "last_pass":
		return float64(job.LastPass.Unix()), nil
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
	PullRequestOrg        string              `json:"pull_request_org"`
	PullRequestRepo       string              `json:"pull_request_repo"`
	PullRequestLink       string              `json:"pull_request_link"`
	PullRequestSHA        string              `json:"pull_request_sha"`
	PullRequestAuthor     string              `json:"pull_request_author"`
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
	case "pull_request_org":
		return ColumnTypeString
	case "pull_request_repo":
		return ColumnTypeString
	case "pull_request_author":
		return ColumnTypeString
	case "pull_request_sha":
		return ColumnTypeString
	case "pull_request_link":
		return ColumnTypeString
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
	case "pull_request_org":
		return run.PullRequestOrg, nil
	case "pull_request_repo":
		return run.PullRequestRepo, nil
	case "pull_request_author":
		return run.PullRequestAuthor, nil
	case "pull_request_sha":
		return run.PullRequestSHA, nil
	case "pull_request_link":
		return run.PullRequestLink, nil
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
	ID        int            `json:"id,omitempty"`
	Name      string         `json:"name"`
	SuiteName string         `json:"suite_name"`
	Variant   string         `json:"variant,omitempty"`
	Variants  pq.StringArray `json:"variants" gorm:"type:text[]"`

	JiraComponent   string `json:"jira_component"`
	JiraComponentID int    `json:"jira_component_id"`

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
	Watchlist                bool    `json:"watchlist"`

	Tags     []string `json:"tags"`
	OpenBugs int      `json:"open_bugs"`
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
	case "watchlist":
		return ColumnTypeString
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
	case "watchlist":
		return strconv.FormatBool(test.Watchlist), nil
	default:
		return "", fmt.Errorf("unknown string field %s", param)
	}
}

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
	case "open_bugs":
		return float64(test.OpenBugs), nil
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

// CalendarEvent is an API type representing a FullCalendar.io event type, for use
// with calendering.
type CalendarEvent struct {
	Title   string `json:"title"`
	Start   string `json:"start"`
	End     string `json:"end"`
	AllDay  bool   `json:"allDay"`
	Display string `json:"display,omitempty"`
	Phase   string `json:"phase"`
	JIRA    string `json:"jira"`
}

type BuildClusterHealthAnalysis struct {
	ByPeriod map[string]BuildClusterHealth `json:"by_period"`
}

type BuildClusterHealth = models.BuildClusterHealthReport

type AnalysisResult struct {
	TotalRuns        int                         `json:"total_runs"`
	ResultCount      map[v1.JobOverallResult]int `json:"result_count"`
	TestFailureCount map[string]int              `json:"test_count"`
}

type JobAnalysisResult struct {
	ByPeriod map[string]AnalysisResult `json:"by_period"`
}

type TestOutput struct {
	URL    string `json:"url"`
	Output string `json:"output"`
}

type Releases struct {
	Releases    []string             `json:"releases"`
	GADates     map[string]time.Time `json:"ga_dates"`
	LastUpdated time.Time            `json:"last_updated"`
}

type Indicator struct {
	Current  sippyv1.PassRate `json:"current"`
	Previous sippyv1.PassRate `json:"previous"`
}

type Variants struct {
	Current  v1.VariantHealth `json:"current"`
	Previous v1.VariantHealth `json:"previous"`
}

type Health struct {
	Indicators  map[string]Test      `json:"indicators"`
	Variants    Variants             `json:"variants"`
	LastUpdated time.Time            `json:"last_updated"`
	Promotions  map[string]time.Time `json:"promotions"`
	Warnings    []string             `json:"warnings"`
	Current     v1.Statistics        `json:"current_statistics"`
	Previous    v1.Statistics        `json:"previous_statistics"`
}

type ProwJobRunRiskAnalysis struct {
	ProwJobName    string
	ProwJobRunID   uint
	Release        string
	CompareRelease string
	Tests          []ProwJobRunTestRiskAnalysis
	OverallRisk    FailureRisk
	OpenBugs       []models.Bug
}

type ProwJobRunTestRiskAnalysis struct {
	Name     string
	Risk     FailureRisk
	OpenBugs []models.Bug
}

type FailureRisk struct {
	Level   RiskLevel
	Reasons []string
}

type RiskSummary struct {
	OverallRisk FailureRisk
	Tests       []ProwJobRunTestRiskAnalysis
}

type RiskLevel struct {
	// Name is a human readable name for the given risk level.
	Name string
	// Level represents a numerical risk level, higher implies more risk.
	Level int
}

type ComponentReportRequestReleaseOptions struct {
	Release string
	Start   time.Time
	End     time.Time
}

type ComponentReportRequestTestIdentificationOptions struct {
	Component  string
	Capability string
	// TestID is a unique identification for the test defined in the DB.
	// It matches the test_id in the bigquery ci_analysis_us.junit table.
	TestID string
}

// ComponentReportRequestExcludeOptions group all the exclude options passed in the request.
// Each of the variable is a comma separated string.
type ComponentReportRequestExcludeOptions struct {
	ExcludePlatforms string
	ExcludeArches    string
	ExcludeNetworks  string
	ExcludeUpgrades  string
	ExcludeVariants  string
}

type ComponentReportRequestVariantOptions struct {
	GroupBy  string
	Platform string
	Upgrade  string
	Arch     string
	Network  string
	Variant  string
}

type ComponentReportRequestAdvancedOptions struct {
	MinimumFailure   int
	Confidence       int
	PityFactor       int
	IgnoreMissing    bool
	IgnoreDisruption bool
}

type ComponentTestStatus struct {
	TestName     string   `json:"test_name"`
	TestSuite    string   `json:"test_suite"`
	Component    string   `json:"component"`
	Capabilities []string `json:"capabilities"`
	Variants     []string `json:"variants"`
	TotalCount   int      `json:"total_count"`
	SuccessCount int      `json:"success_count"`
	FlakeCount   int      `json:"flake_count"`
}

type ComponentReportTestStatus struct {
	BaseStatus   map[ComponentTestIdentification]ComponentTestStatus `json:"base_status"`
	SampleStatus map[ComponentTestIdentification]ComponentTestStatus `json:"sample_status"`
	GeneratedAt  *time.Time                                          `json:"generated_at"`
}

type ComponentTestIdentification struct {
	TestID       string `json:"test_id"`
	Network      string `json:"network"`
	Upgrade      string `json:"upgrade"`
	Arch         string `json:"arch"`
	Platform     string `json:"platform"`
	FlatVariants string `json:"flat_variants"`
}

// implement encoding.TextMarshaler for json map key marshalling support
func (s ComponentTestIdentification) MarshalText() (text []byte, err error) {
	type t ComponentTestIdentification
	return json.Marshal(t(s))
}

func (s *ComponentTestIdentification) UnmarshalText(text []byte) error {
	type t ComponentTestIdentification
	return json.Unmarshal(text, (*t)(s))
}

type ComponentTestStatusRow struct {
	TestName     string   `bigquery:"test_name"`
	TestSuite    string   `bigquery:"test_suite"`
	TestID       string   `bigquery:"test_id"`
	Network      string   `bigquery:"network"`
	Upgrade      string   `bigquery:"upgrade"`
	Arch         string   `bigquery:"arch"`
	Platform     string   `bigquery:"platform"`
	FlatVariants string   `bigquery:"flat_variants"`
	Variants     []string `bigquery:"variants"`
	TotalCount   int      `bigquery:"total_count"`
	SuccessCount int      `bigquery:"success_count"`
	FlakeCount   int      `bigquery:"flake_count"`
	Component    string   `bigquery:"component"`
	Capabilities []string `bigquery:"capabilities"`
}

type ComponentReport struct {
	Rows        []ComponentReportRow `json:"rows,omitempty"`
	GeneratedAt *time.Time           `json:"generated_at"`
}

type ComponentReportRow struct {
	ComponentReportRowIdentification
	Columns []ComponentReportColumn `json:"columns,omitempty"`
}

type ComponentReportRowIdentification struct {
	Component  string `json:"component"`
	Capability string `json:"capability,omitempty"`
	TestName   string `json:"test_name,omitempty"`
	TestSuite  string `json:"test_suite,omitempty"`
	TestID     string `json:"test_id,omitempty"`
}

type ComponentReportColumn struct {
	ComponentReportColumnIdentification
	Status         ComponentReportStatus        `json:"status"`
	RegressedTests []ComponentReportTestSummary `json:"regressed_tests,omitempty"`
}

type ComponentReportColumnIdentification struct {
	Network  string `json:"network,omitempty"`
	Upgrade  string `json:"upgrade,omitempty"`
	Arch     string `json:"arch,omitempty"`
	Platform string `json:"platform,omitempty"`
	Variant  string `json:"variant,omitempty"`
}

type ComponentReportStatus int

type ComponentReportTestIdentification struct {
	ComponentReportRowIdentification
	ComponentReportColumnIdentification
}

type ComponentReportTestSummary struct {
	ComponentReportTestIdentification
	Status ComponentReportStatus `json:"status"`
}

type ComponentReportTestDetails struct {
	ComponentReportTestIdentification
	JiraComponent   string                                 `json:"jira_component"`
	JiraComponentID *big.Rat                               `json:"jira_component_id"`
	SampleStats     ComponentReportTestDetailsReleaseStats `json:"sample_stats"`
	BaseStats       ComponentReportTestDetailsReleaseStats `json:"base_stats"`
	FisherExact     float64                                `json:"fisher_exact"`
	ReportStatus    ComponentReportStatus                  `json:"report_status"`
	JobStats        []ComponentReportTestDetailsJobStats   `json:"job_stats,omitempty"`
	GeneratedAt     *time.Time                             `json:"generated_at"`
}

type ComponentReportTestDetailsReleaseStats struct {
	Release string `json:"release"`
	ComponentReportTestDetailsTestStats
}

type ComponentReportTestDetailsTestStats struct {
	SuccessRate  float64 `json:"success_rate"`
	SuccessCount int     `json:"success_count"`
	FailureCount int     `json:"failure_count"`
	FlakeCount   int     `json:"flake_count"`
}

type ComponentReportTestDetailsJobStats struct {
	JobName           string                                  `json:"job_name"`
	SampleStats       ComponentReportTestDetailsTestStats     `json:"sample_stats"`
	BaseStats         ComponentReportTestDetailsTestStats     `json:"base_stats"`
	SampleJobRunStats []ComponentReportTestDetailsJobRunStats `json:"sample_job_run_stats,omitempty"`
	BaseJobRunStats   []ComponentReportTestDetailsJobRunStats `json:"base_job_run_stats,omitempty"`
	Significant       bool                                    `json:"significant"`
}

type ComponentReportTestDetailsJobRunStats struct {
	JobURL string `json:"job_url"`
	// TestStats is the test stats from one particular job run.
	// For the majority of the tests, there is only one junit. But
	// there are cases multiple junits are generated for the same test.
	TestStats ComponentReportTestDetailsTestStats `json:"test_stats"`
}

type ComponentJobRunTestStatus struct {
	Component    string
	Capabilities []string
	Network      string
	Upgrade      string
	Arch         string
	Platform     string
	Variants     []string
	TotalCount   int
	SuccessCount int
	FlakeCount   int
}

type ComponentJobRunTestIdentification struct {
	TestName string
	TestID   string
	FilePath string
}

type ComponentJobRunTestStatusRow struct {
	ProwJob         string   `bigquery:"prowjob_name"`
	TestID          string   `bigquery:"test_id"`
	TestName        string   `bigquery:"test_name"`
	FilePath        string   `bigquery:"file_path"`
	TotalCount      int      `bigquery:"total_count"`
	SuccessCount    int      `bigquery:"success_count"`
	FlakeCount      int      `bigquery:"flake_count"`
	JiraComponent   string   `bigquery:"jira_component"`
	JiraComponentID *big.Rat `bigquery:"jira_component_id"`
}

type ComponentJobRunTestReportStatus struct {
	BaseStatus   map[string][]ComponentJobRunTestStatusRow `json:"base_status"`
	SampleStatus map[string][]ComponentJobRunTestStatusRow `json:"sample_status"`
	GeneratedAt  *time.Time                                `json:"generated_at"`
}

const (
	// ExtremeRegression shows regression with >15% pass rate change
	ExtremeRegression ComponentReportStatus = -3
	// SignificantRegression shows significant regression
	SignificantRegression ComponentReportStatus = -2
	// MissingSample indicates sample data missing
	MissingSample ComponentReportStatus = -1
	// NotSignificant indicates no significant difference
	NotSignificant ComponentReportStatus = 0
	// MissingBasis indicates basis data missing
	MissingBasis ComponentReportStatus = 1
	// MissingBasisAndSample indicates basis and sample data missing
	MissingBasisAndSample ComponentReportStatus = 2
	// SignificantImprovement indicates improved sample rate
	SignificantImprovement ComponentReportStatus = 3
)

type ComponentReportResponse []ComponentReportRow

type ComponentReportTestVariants struct {
	Network  []string `json:"network,omitempty"`
	Upgrade  []string `json:"upgrade,omitempty"`
	Arch     []string `json:"arch,omitempty"`
	Platform []string `json:"platform,omitempty"`
	Variant  []string `json:"variant,omitempty"`
}

var FailureRiskLevelNone = RiskLevel{Name: "None", Level: 0}
var FailureRiskLevelLow = RiskLevel{Name: "Low", Level: 1}
var FailureRiskLevelUnknown = RiskLevel{Name: "Unknown", Level: 25}
var FailureRiskLevelMedium = RiskLevel{Name: "Medium", Level: 50}
var FailureRiskLevelIncompleteTests = RiskLevel{Name: "IncompleteTests", Level: 75}
var FailureRiskLevelMissingData = RiskLevel{Name: "MissingData", Level: 76}
var FailureRiskLevelHigh = RiskLevel{Name: "High", Level: 100}

type DisruptionReportDeltaRequestOptions struct {
	Release string
}

type DisruptionReport struct {
	Rows []DisruptionReportRow `json:"rows,omitempty"`
}

type DisruptionReportRow struct {
	P50                      float32 `json:"p50"`
	P75                      float32 `json:"p75"`
	P95                      float32 `json:"p95"`
	PercentageAboveZeroDelta float32 `json:"percentage_above_zero_delta"`
	Release                  string  `json:"release"`
	CompareRelease           string  `json:"compare_release,omitempty"` // only present in the vs prev GA view
	BackendName              string  `json:"backend_name"`
	Platform                 string  `json:"platform"`
	UpgradeType              string  `json:"upgrade_type"`
	MasterNodesUpdated       string  `json:"master_nodes_updated"`
	Network                  string  `json:"network"`
	Topology                 string  `json:"topology"`
	Architecture             string  `json:"architecture"`
	Relevance                int     `json:"relevance"`
}

type ReleaseRow struct {
	// Release contains the X.Y version of the payload, e.g. 4.8
	Release string `bigquery:"release"`

	// Major contains the major part of the release, e.g. 4
	Major int `bigquery:"Major"`

	// Minor contains the minor part of the release, e.g. 8
	Minor int `bigquery:"Minor"`

	// GADate contains GA date for the release, i.e. the -YYYY-MM-DD
	GADate bigquery.NullDate `bigquery:"GADate"`

	// DevelStartDate contains start date of development of the release, i.e. the -YYYY-MM-DD
	DevelStartDate civil.Date `bigquery:"DevelStartDate"`

	// Product contains the product for the release, e.g. OCP
	Product bigquery.NullString `bigquery:"Product"`

	// Patch contains the patch version number of the release, e.g. 1
	Patch bigquery.NullInt64 `bigquery:"Patch"`

	// ReleaseStatus contains the status of the release, e.g. Full Support
	ReleaseStatus bigquery.NullString `bigquery:"ReleaseStatus"`
}

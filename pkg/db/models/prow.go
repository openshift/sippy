package models

import (
	"time"

	"cloud.google.com/go/civil"
	"github.com/lib/pq"
	"gorm.io/gorm"

	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

type ProwKind string

// VariantCombination assigns an integer ID to each unique variants array,
// enabling efficient GROUP BY in matviews. Populated via a trigger on prow_jobs.
type VariantCombination struct {
	ID       uint           `gorm:"primaryKey"`
	Variants pq.StringArray `gorm:"type:text[];uniqueIndex:idx_variant_combinations_variants;not null"`
}

// ProwJob represents a prow job and stores data about its variants, associated bugs, etc.
type ProwJob struct {
	gorm.Model

	Kind     ProwKind
	Name     string         `gorm:"unique"`
	Release  string         `gorm:"index"`
	Variants pq.StringArray `gorm:"type:text[]"`
	// VariantCombinationID references variant_combinations.id, maintained by a
	// BEFORE INSERT/UPDATE trigger. NULL only when Variants is NULL.
	VariantCombinationID *uint `gorm:"column:variant_combination_id"`
	VariantCombination   *VariantCombination
	TestGridURL          string
	// Bugs maps to all the bugs we scanned and found this prowjob name mentioned in the description or any comment.
	Bugs    []Bug        `gorm:"many2many:bug_jobs;"`
	JobRuns []ProwJobRun `gorm:"constraint:OnDelete:CASCADE;"`
}

// IDName is a partial struct to query limited fields we need for caching. Can be used
// with any type that has a unique name and an ID we need to lookup.
// https://gorm.io/docs/advanced_query.html#Smart-Select-Fields
type IDName struct {
	ID   uint
	Name string `gorm:"unique"`
}

type ProwJobRun struct {
	gorm.Model

	// ProwJob is a link to the prow job this run belongs to.
	ProwJob   ProwJob
	ProwJobID uint `gorm:"index"`
	// Used for partitioning (denormalized for prow_job_run_tests)
	ProwJobRelease string `gorm:"index:idx_prow_job_runs_release_timestamp"`

	// Cluster is the cluster where the prow job was run.
	Cluster string

	GCSBucket    string
	URL          string
	TestFailures int
	Tests        []ProwJobRunTest
	PullRequests []ProwPullRequest      `gorm:"many2many:prow_job_run_prow_pull_requests;constraint:OnDelete:CASCADE;"`
	Annotations  []ProwJobRunAnnotation `gorm:"constraint:OnDelete:CASCADE;"`
	Failed       bool
	// InfrastructureFailure is true if the job run failed, for reasons which appear to be related to test/CI infra.
	InfrastructureFailure bool
	// KnownFailure is true if the job run failed, but we found a bug that is likely related already filed.
	KnownFailure  bool
	Succeeded     bool
	Timestamp     time.Time `gorm:"index;index:idx_prow_job_runs_timestamp_date,expression:DATE(timestamp AT TIME ZONE 'UTC');index:idx_prow_job_runs_release_timestamp"`
	Duration      time.Duration
	OverallResult v1.JobOverallResult `gorm:"index"`
	// Labels stores the IDs of labels applied to this job run
	// This is populated from symptom detection or manual annotation
	Labels pq.StringArray `gorm:"type:text[];index:idx_prow_job_runs_labels,type:gin" json:"labels"`
	// used to pass the TestCount in via the api, we have the actual tests in the db and can calculate it here so don't persist
	TestCount   int         `gorm:"-"`
	ClusterData ClusterData `gorm:"-"`
}

// ProwJobRunProwPullRequest is the explicit join table for the many-to-many relationship
// between ProwJobRun and ProwPullRequest. Release and timestamp are denormalized from
// ProwJobRun for query optimization.
type ProwJobRunProwPullRequest struct {
	ProwJobRunID        uint      `gorm:"primaryKey"`
	ProwPullRequestID   uint      `gorm:"primaryKey;index:idx_prow_job_run_prow_pull_requests_pr_id"`
	ProwJobRunRelease   string    `gorm:"index:idx_prow_job_run_prow_pull_requests_release_timestamp"`
	ProwJobRunTimestamp time.Time `gorm:"index:idx_prow_job_run_prow_pull_requests_release_timestamp"`
}

// ProwJobRunAnnotation stores a single key-value annotation for a ProwJobRun.
type ProwJobRunAnnotation struct {
	gorm.Model
	ProwJobRunID        uint   `gorm:"index;uniqueIndex:idx_prow_job_run_annotations_key"`
	Key                 string `gorm:"uniqueIndex:idx_prow_job_run_annotations_key"`
	Value               string
	ProwJobRunRelease   string    `gorm:"index:idx_prow_job_run_annotations_release_timestamp"`
	ProwJobRunTimestamp time.Time `gorm:"index:idx_prow_job_run_annotations_release_timestamp"`
}

type Test struct {
	gorm.Model
	Name           string          `gorm:"uniqueIndex"`
	Bugs           []Bug           `gorm:"many2many:bug_tests;"`
	TestOwnerships []TestOwnership `gorm:"constraint:OnDelete:CASCADE;"`
}

// ProwJobRunTest defines a join table linking tests to the job runs they execute in, along with the status for
// that execution.
// Table is partitioned (LIST→RANGE) - schema managed by migration 000002, not AutoMigrate
type ProwJobRunTest struct {
	gorm.Model
	ProwJobRunID uint
	ProwJobRun   ProwJobRun
	// used for variants
	// skips joining on ProwJobRunID just to get ProwJobID
	ProwJobID uint
	// used for partitioning - must be in primary key for RANGE partitioning
	ProwJobRunTimestamp time.Time `gorm:"primaryKey"`
	// denormalized for query optimization and LIST partitioning
	ProwJobRunRelease string `gorm:"primaryKey"`
	TestID            uint
	Test              Test
	// SuiteID may be nil if no suite name could be parsed from the testgrid test name.
	SuiteID   *uint
	Suite     Suite
	Status    int
	Duration  float64
	CreatedAt time.Time
	DeletedAt gorm.DeletedAt

	// ProwJobRunTestOutput collect the output of a failed test run. This is stored as a separate object in the DB, so
	// we can keep the test result for a longer period of time than we keep the full failure output.
	// No FK constraint - both tables partitioned, FK managed by migration
	// Relationship uses composite key (id, timestamp, release) to match partitioned table structure
	ProwJobRunTestOutput *ProwJobRunTestOutput `gorm:"foreignKey:ProwJobRunTestID,ProwJobRunTestTimestamp,ProwJobRunTestRelease;references:ID,ProwJobRunTimestamp,ProwJobRunRelease"`
}

// ProwJobRunTestOutput stores test failure output.
// Table is partitioned (LIST→RANGE) - schema managed by migration 000002, not AutoMigrate
type ProwJobRunTestOutput struct {
	gorm.Model
	ProwJobRunTestID uint
	// Output stores the output of a ProwJobRunTest.
	Output string
	// Denormalized from parent for composite foreign key and partitioning
	// primaryKey required for RANGE partitioning
	ProwJobRunTestTimestamp time.Time `gorm:"primaryKey"`
	// Denormalized for query optimization and LIST partitioning
	ProwJobRunTestRelease string `gorm:"primaryKey"`
}

// Suite defines a junit testsuite. Used to differentiate the same test being run in different suites in ProwJobRunTest.
type Suite struct {
	gorm.Model
	Name string `gorm:"uniqueIndex"`
}

type TestAnalysisByJobByDate struct {
	Date     time.Time `gorm:"index:test_release_date,unique"`
	TestID   uint      `gorm:"index:test_release_date,unique"`
	Release  string    `gorm:"index:test_release_date,unique"`
	JobName  string    `gorm:"index:test_release_date,unique"`
	TestName string
	Runs     int
	Passes   int
	Flakes   int
	Failures int
}

// TestDailyTotal stores pre-aggregated daily test results.
// Table is partitioned (LIST by release, RANGE by date) -
// schema managed by migration 000006, not AutoMigrate.
type TestDailyTotal struct {
	TestID    uint       `gorm:"column:test_id;not null"`
	ProwJobID uint       `gorm:"column:prow_job_id;not null"`
	SuiteID   uint       `gorm:"column:suite_id;not null;default:0"`
	Release   string     `gorm:"column:release;not null"`
	Date      civil.Date `gorm:"column:date;type:date;not null"`
	Successes int32      `gorm:"column:successes;not null;default:0"`
	Failures  int32      `gorm:"column:failures;not null;default:0"`
	Flakes    int32      `gorm:"column:flakes;not null;default:0"`
	Runs      int32      `gorm:"column:runs;not null;default:0"`
}

// TestCumulativeSummary stores running totals of test_daily_totals values,
// ordered by date. Any date range [start, end] can be computed as
// cumulative(end) - cumulative(start-1). Keyed by immutable fields only
// (no variant_combination_id) so variant changes do not invalidate the data.
// Entities are carried forward on days with no data so the chain is unbroken.
// Table is partitioned (LIST by release, RANGE by date) -
// schema managed by migration 000006, not AutoMigrate.
type TestCumulativeSummary struct {
	Date               civil.Date `gorm:"column:date;type:date;not null;primaryKey;priority:1"`
	Release            string     `gorm:"column:release;not null;primaryKey;priority:2"`
	TestID             uint       `gorm:"column:test_id;not null;primaryKey;priority:3"`
	ProwJobID          uint       `gorm:"column:prow_job_id;not null;primaryKey;priority:4;index:idx_test_cumulative_summaries_prow_job_id"`
	SuiteID            uint       `gorm:"column:suite_id;not null;default:0;primaryKey;priority:5"`
	PrefixSumSuccesses int64      `gorm:"column:prefix_sum_successes;not null;default:0"`
	PrefixSumFailures  int64      `gorm:"column:prefix_sum_failures;not null;default:0"`
	PrefixSumFlakes    int64      `gorm:"column:prefix_sum_flakes;not null;default:0"`
	PrefixSumRuns      int64      `gorm:"column:prefix_sum_runs;not null;default:0"`
}

// ProwGARawTestDatum stores raw BigQuery test results for GA release windows.
// Fetched once per GA date and persisted so that the aggregation into
// prow_ga_test_statuses_matview can be re-run cheaply when dimension tables change.
// Each (release, window_days) pair holds results aggregated over a different lookback
// period (e.g. 1, 30, or 90 days before GA).
type ProwGARawTestDatum struct {
	Release    string `gorm:"not null;index:idx_prow_ga_raw_release_window"`
	WindowDays int    `gorm:"not null;default:30;index:idx_prow_ga_raw_release_window"`
	TestID     uint   `gorm:"not null"`
	ProwJobID  uint   `gorm:"not null"`
	SuiteID    uint   `gorm:"not null;default:0"`
	Passes     int64  `gorm:"not null;default:0"`
	Failures   int64  `gorm:"not null;default:0"`
	Flakes     int64  `gorm:"not null;default:0"`
	Runs       int64  `gorm:"not null;default:0"`
}

// Bug represents a Jira bug.
type Bug struct {
	ID              uint           `json:"id" gorm:"primaryKey"`
	Key             string         `json:"key" gorm:"index"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `json:"deleted_at" gorm:"index"`
	Status          string         `json:"status"`
	LastChangeTime  time.Time      `json:"last_change_time"`
	Summary         string         `json:"summary"`
	AffectsVersions pq.StringArray `json:"affects_versions" gorm:"type:text[]"`
	FixVersions     pq.StringArray `json:"fix_versions" gorm:"type:text[]"`
	TargetVersions  pq.StringArray `json:"target_versions" gorm:"type:text[]"`
	Components      pq.StringArray `json:"components" gorm:"type:text[]"`
	Labels          pq.StringArray `json:"labels" gorm:"type:text[]"`
	URL             string         `json:"url"`
	ReleaseBlocker  string         `json:"release_blocker"`
	Tests           []Test         `json:"-" gorm:"many2many:bug_tests;constraint:OnDelete:CASCADE;"`
	Jobs            []ProwJob      `json:"-" gorm:"many2many:bug_jobs;constraint:OnDelete:CASCADE;"`
}

// ProwPullRequest represents a GitHub pull request, there can be multiple entries
// for a pull request, if it was tested with different HEADs (SHA). This lets us
// track jobs at a more granular level, allowing us to differentiate between code pushes
// and retests.
type ProwPullRequest struct {
	Model

	// Org is something like kubernetes or k8s.io
	Org string `json:"org" gorm:"index:idx_prow_pull_requests_org_repo_number"`
	// Repo is something like test-infra
	Repo string `json:"repo" gorm:"index:idx_prow_pull_requests_org_repo_number"`

	Number int    `json:"number" gorm:"index:idx_prow_pull_requests_org_repo_number"`
	Author string `json:"author"`
	Title  string `json:"title,omitempty"`

	// SHA is the specific commit at HEAD.
	SHA string `json:"sha" gorm:"index:pr_link_sha,unique"`
	// Link links to the pull request itself.
	Link string `json:"link,omitempty" gorm:"index:pr_link_sha,unique"`

	// MergedAt contains the time retrieved from GitHub that this PR was merged.
	MergedAt *time.Time `json:"merged_at,omitempty" gorm:"merged_at"`
}

type ClusterData struct {
	Release               string
	FromRelease           string
	Platform              string
	Architecture          string
	Network               string
	Topology              string
	NetworkStack          string
	CloudRegion           string
	CloudZone             string
	ClusterVersionHistory []string
}

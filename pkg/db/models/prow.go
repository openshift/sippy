package models

import (
	"time"

	"github.com/jackc/pgtype"
	"github.com/lib/pq"
	"gorm.io/gorm"

	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

type ProwKind string

const ProwPeriodic ProwKind = "periodic"
const ProwPresubmit ProwKind = "presubmit"

// ProwJob represents a prow job with various fields inferred from it's name. (release, variants, etc)
type ProwJob struct {
	gorm.Model

	Kind        ProwKind
	Name        string         `gorm:"unique"`
	Release     string         `gorm:"varchar(10)"`
	Variants    pq.StringArray `gorm:"type:text[];index:idx_prow_jobs_variants,type:gin"`
	TestGridURL string
	Bugs        []Bug        `gorm:"many2many:bug_jobs;"`
	JobRuns     []ProwJobRun `gorm:"constraint:OnDelete:CASCADE;"`
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

	// Cluster is the cluster where the prow job was run.
	Cluster string

	URL          string
	TestFailures int
	Tests        []ProwJobRunTest  `gorm:"constraint:OnDelete:CASCADE;"`
	PullRequests []ProwPullRequest `gorm:"many2many:prow_job_run_prow_pull_requests;constraint:OnDelete:CASCADE;"`
	Failed       bool
	// InfrastructureFailure is true if the job run failed, for reasons which appear to be related to test/CI infra.
	InfrastructureFailure bool
	// KnownFailure is true if the job run failed, but we found a bug that is likely related already filed.
	KnownFailure  bool
	Succeeded     bool
	Timestamp     time.Time `gorm:"index;index:idx_prow_job_runs_timestamp_date,expression:DATE(timestamp AT TIME ZONE 'UTC')"`
	Duration      time.Duration
	OverallResult v1.JobOverallResult `gorm:"index"`
	// used to pass the TestCount in via the api, we have the actual tests in the db and can calculate it here so don't persist
	TestCount   int         `gorm:"-"`
	ClusterData ClusterData `gorm:"-"`
}

type Test struct {
	gorm.Model
	Name string `gorm:"uniqueIndex"`
	Bugs []Bug  `gorm:"many2many:bug_tests;"`
	// Watchlist are tests TRT is interested in keeping an eye on.
	Watchlist bool
}

// ProwJobRunTest defines a join table linking tests to the job runs they execute in, along with the status for
// that execution.
type ProwJobRunTest struct {
	gorm.Model
	ProwJobRunID uint `gorm:"index"`
	ProwJobRun   ProwJobRun
	TestID       uint `gorm:"index"`
	Test         Test
	// SuiteID may be nil if no suite name could be parsed from the testgrid test name.
	SuiteID   *uint `gorm:"index"`
	Suite     Suite
	Status    int `gorm:"index"`
	Duration  float64
	CreatedAt time.Time `gorm:"index"`
	DeletedAt gorm.DeletedAt

	// ProwJobRunTestOutput collect the output of a failed test run. This is stored as a separate object in the DB, so
	// we can keep the test result for a longer period of time than we keep the full failure output.
	ProwJobRunTestOutput *ProwJobRunTestOutput `gorm:"constraint:OnDelete:CASCADE;"`
}

type ProwJobRunTestOutput struct {
	gorm.Model
	ProwJobRunTestID uint `gorm:"index"`
	// Output stores the output of a ProwJobRunTest.
	Output string

	// Metadata optionally contains metadata extracted from a select few generic backstop tests
	// we use to catch problems. This metadata helps us identify developing problems in these broad
	// tests and figure out what next needs to be broken out into its own test.
	Metadata []ProwJobRunTestOutputMetadata `gorm:"constraint:OnDelete:CASCADE;"`
}

type ProwJobRunTestOutputMetadata struct {
	gorm.Model
	ProwJobRunTestOutputID uint         `gorm:"index"`
	Metadata               pgtype.JSONB `gorm:"type:jsonb"`
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
	TestName string
	JobName  string
	Runs     int
	Passes   int
	Flakes   int
	Failures int
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
	Components      pq.StringArray `json:"components" gorm:"type:text[]"`
	Labels          pq.StringArray `json:"labels" gorm:"type:text[]"`
	URL             string         `json:"url"`
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
	Org string `json:"org"`
	// Repo is something like test-infra
	Repo string `json:"repo"`

	Number int    `json:"number"`
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

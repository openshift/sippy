package models

import (
	"time"

	"github.com/lib/pq"
	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"gorm.io/gorm"
)

// ProwJob represents a prow job with various fields inferred from it's name. (release, variants, etc)
type ProwJob struct {
	gorm.Model

	Name        string         `gorm:"unique"`
	Release     string         `gorm:"varchar(10)"`
	Variants    pq.StringArray `gorm:"type:text[]"`
	TestGridURL string
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
	ProwJobID uint

	URL          string
	TestFailures int
	FailedTests  []Test `gorm:"many2many:prow_job_run_failed_tests;"`
	Tests        []ProwJobRunTest
	Failed       bool
	// InfrastructureFailure is true if the job run failed, for reasons which appear to be related to test/CI infra.
	InfrastructureFailure bool
	// KnownFailure is true if the job run failed, but we found a bug that is likely related already filed.
	KnownFailure  bool
	Succeeded     bool
	Timestamp     time.Time
	OverallResult v1.JobOverallResult
}

type Test struct {
	gorm.Model
	Name string `gorm:"uniqueIndex"`
}

// ProwJobRunTest defines a join table linking tests to the job runs they execute in, along with the status for
// that execution.
type ProwJobRunTest struct {
	gorm.Model
	ProwJobRunID uint
	TestID       uint
	// SuiteID may be nil if no suite name could be parsed from the testgrid test name.
	SuiteID   *uint
	Status    int // would like to use smallint here, but gorm auto-migrate breaks trying to change the type every start
	CreatedAt time.Time
	DeletedAt gorm.DeletedAt
}

// Suite defines a junit testsuite. Used to differentiate the same test being run in different suites in ProwJobRunTest.
type Suite struct {
	gorm.Model
	Name string `gorm:"uniqueIndex"`
}

// TestAnalysisRow models our materialize view for test results by date, and job+variant.
// The only one of the Variant/JobName fields will be used depending on which view
// we're querying.
type TestAnalysisRow struct {
	Date     time.Time
	TestID   uint
	TestName string
	Variant  string // may not be used depending on calling query
	JobName  string // may not be used depending on calling query
	Release  string
	Runs     int
	Passes   int
	Flakes   int
	Failures int
}

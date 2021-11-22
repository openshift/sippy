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

	//BugList []bugsv1.Bug `json:"bugList" gorm:"-"`
	// AssociatedBugList are bugs that match the test/job, but do not match the target release
	//AssociatedBugList []bugsv1.Bug `json:"associatedBugList" gorm:"-"`

	// TestResults holds entries for each test that is a part of this aggregation.  Each entry aggregates the results
	// of all runs of a single test.  The array is sorted from lowest PassPercentage to highest PassPercentage
	//TestResults []TestResult `json:"results" gorm:"foreignKey:Job;References:Name"`
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
	//FailedTestNames pq.StringArray   `gorm:"type:text[]"`
	Failed        bool
	Succeeded     bool
	Timestamp     time.Time
	OverallResult v1.JobOverallResult
}

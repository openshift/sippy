package models

import (
	"database/sql"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

// Triage contains data tying failures or regressions to specific bugs.
type Triage struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
	// URL references the core URL to follow for details on this triage, typically a Jira bug.
	URL string `json:"url" gorm:"not null"`

	// Type provides information about the type of regression being triaged, a best guess by the
	// individual performing triage.
	Type TriageType `json:"type" gorm:"not null"`

	// Bug is populated if and when we have a URL for a valid Jira imported into our db.
	// Because a user may link a URL that is not yet in sippy's db, we need to differentiate
	// between the two. During bug import we explicitly bring in any triaged bugs we can find.
	// Once imported, useful data is available for the triage via this reference.
	Bug   *Bug  `json:"bug,omitempty"`
	BugID *uint `json:"bug_id,omitempty"`

	// Links contains REST links for clients to follow for this specific triage. Most notably "self".
	// These are injected by the API and not stored in the DB.
	Links map[string]string `json:"links" gorm:"-"`

	// Regressions ties this triage record to specific component readiness test regressions.
	// Triage will often tie to regressions, often multiple (as regressions are broken out by variant / suite),
	// but it may also be empty in
	// cases where we're mass triaging something that has not surfaced in component readiness.
	// TODO: still not sure about this, it feels like there would always be at least one...
	// otherwise what are we triaging? We might later map to job runs that are not associated with
	// any regression, but this would be fine...
	// If we could establish this, it may mean less data duplication.
	Regressions []TestRegression `json:"regressions" gorm:"constraint:OnDelete:CASCADE;many2many:triage_regressions;"`
}

type TriageType string

const (
	// TriageTypeCIInfra is used for CI infra problems that did not impact actual customers. (build cluster outages
	// cloud account problems, etc.)
	TriageTypeCIInfra TriageType = "ci-infra"
	// TriageTypeProductInfra is used for infrastructure problems that impacted CI but also would have hit customers.
	// (registry outages/caching issues, etc)
	TriageTypeProductInfra TriageType = "product-infra"
	// TriageProduct is used for actual product regressions.
	TriageTypeProduct TriageType = "product"
	// TriageTest is used for regressions isolated to the test framework where no product fix is actually required.
	TriageTypeTest TriageType = "test"
)

func ValidTriageType(triageType TriageType) bool {
	switch triageType {
	case TriageTypeCIInfra:
	case TriageTypeProductInfra:
	case TriageTypeProduct:
	case TriageTypeTest:
	default:
		return false
	}
	return true
}

// TestRegression is used for rows in the test_regressions table and is used to track when we detect test
// regressions opening and closing.
type TestRegression struct {
	ID       uint           `json:"id" gorm:"primaryKey,column:id"`
	View     string         `json:"view" gorm:"not null"`
	Release  string         `json:"release" gorm:"not null;index:idx_test_regression_release"`
	TestID   string         `json:"test_id" gorm:"not null"`
	TestName string         `json:"test_name" gorm:"not null;index:idx_test_regression_test_name"`
	Variants pq.StringArray `json:"variants" gorm:"not null;type:text[]"`
	Opened   time.Time      `json:"opened" gorm:"not null"`
	Closed   sql.NullTime   `json:"closed"`
	Triages  []Triage       `json:"-" gorm:"many2many:triage_regressions;"`
}

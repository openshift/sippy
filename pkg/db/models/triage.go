package models

import (
	"database/sql"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

// Triage contains data tying failures or regressions to specific bugs.
type Triage struct {
	gorm.Model
	URL string
	// Regressions ties this triage record to specific component readiness test regressions.
	// Triage will often tie to regressions, often multiple (as regressions are broken out by variant / suite),
	// but it may also be empty in
	// cases where we're mass triaging something that has not surfaced in component readiness.
	// TODO: still not sure about this, it feels like there would always be at least one...
	// otherwise what are we triaging? We might later map to job runs that are not associated with
	// any regression, but this would be fine...
	// If we could establish this, it may mean less data copying.
	Regressions []TestRegression `gorm:"constraint:OnDelete:CASCADE;many2many:triage_regressions;"`
}

// TestRegression is used for rows in the test_regressions table and is used to track when we detect test
// regressions opening and closing.
type TestRegression struct {
	ID       uint           `json:"-" gorm:"primaryKey,column:id"`
	View     string         `json:"view" gorm:"not null"`
	Release  string         `json:"release" gorm:"not null;index:idx_test_regression_release"`
	TestID   string         `json:"test_id" gorm:"not null"`
	TestName string         `json:"test_name" gorm:"not null;index:idx_test_regression_test_name"`
	Variants pq.StringArray `json:"variants" gorm:"not null;type:text[]"`
	Opened   time.Time      `json:"opened" gorm:"not null"`
	Closed   sql.NullTime   `json:"closed"`
	Triages  []Triage       `json:"-" gorm:"many2many:triage_regressions;"`
}

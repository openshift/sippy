package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

// Triage contains data tying failures or regressions to specific bugs.
type Triage struct {
	ID        uint      `json:"id" gorm:"primarykey"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// URL references the core URL to follow for details on this triage, typically a Jira bug.
	URL string `json:"url" gorm:"not null"`

	// Description contains a short description regarding the URL
	Description string `json:"description,omitempty"`

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

	// Resolution is an important field presently set by a user indicating a claimed time this issue was resolved,
	// and thus all associated regressions should be fixed.
	// Setting this will immediately change the regressions icon to one indicate the issue is believed to
	// be fixed. If we see failures beyond the resolved time, you will see another icon to highlight this situation.
	Resolved sql.NullTime `json:"resolved"`

	// ResolutionReason details the cause of the triage being resolved. It will be set by the system, and not be editable
	// by the user. If the triage is resolved multiple times, this will store the latest reason
	ResolutionReason resolutionReason `json:"resolution_reason"`
}

type resolutionReason string

const (
	User                 resolutionReason = "user"
	RegressionsRolledOff resolutionReason = "regressions-rolled-off"
	JiraProgression      resolutionReason = "jira-progression"
)

type contextKey string

const (
	OldTriageKey   contextKey = "old_triage"
	CurrentUserKey contextKey = "current_user"
)

func (t *Triage) BeforeUpdate(db *gorm.DB) error {
	return t.before(db)
}

func (t *Triage) BeforeDelete(db *gorm.DB) error {
	return t.before(db)
}

func (t *Triage) before(db *gorm.DB) error {
	// Check if we've already captured the old triage in this transaction
	if existing := db.Statement.Context.Value(OldTriageKey); existing != nil {
		return nil
	}

	var old Triage
	if err := db.Preload("Regressions").First(&old, t.ID).Error; err != nil {
		return err
	}

	db.Statement.Context = context.WithValue(db.Statement.Context, OldTriageKey, old)
	return nil
}

func (t *Triage) AfterUpdate(db *gorm.DB) error {
	return t.after(db, Update)
}

func (t *Triage) AfterCreate(db *gorm.DB) error {
	return t.after(db, Create)
}

func (t *Triage) AfterDelete(db *gorm.DB) error {
	return t.after(db, Delete)
}

func (t *Triage) after(db *gorm.DB, operation OperationType) error {
	var oldTriageJSON []byte
	if operation == Update || operation == Delete {
		var err error
		oldTriage, ok := db.Statement.Context.Value(OldTriageKey).(Triage)
		if !ok {
			return fmt.Errorf("value of old_triage is not a Triage type")
		}
		oldTriageJSON, err = oldTriage.marshalJSONForAudit()
		if err != nil {
			return fmt.Errorf("error marshalling old triage record: %w", err)
		}
	}

	var newTriageJSON []byte
	if operation != Delete {
		var err error
		newTriageJSON, err = t.marshalJSONForAudit()
		if err != nil {
			return fmt.Errorf("error marshalling new triage record: %w", err)
		}
	}
	user := db.Statement.Context.Value(CurrentUserKey)
	if user == nil {
		return fmt.Errorf("current user not found in context")
	}
	audit := AuditLog{
		TableName: "triage",
		Operation: string(operation),
		RowID:     t.ID,
		User:      user.(string),
		OldData:   oldTriageJSON,
		NewData:   newTriageJSON,
	}

	return db.Create(&audit).Error
}

// marshalJSONForAudit serializes only the necessary details for audit purposes, leaving out the rest.
// Critically, it removes all details except for the id from the included regressions.
func (t *Triage) marshalJSONForAudit() ([]byte, error) {
	type Alias Triage

	type MinimalRegression struct {
		ID uint `json:"id"`
	}
	regressions := make([]MinimalRegression, len(t.Regressions))
	for i, reg := range t.Regressions {
		regressions[i] = MinimalRegression{ID: reg.ID}
	}

	auditJSON := struct {
		Alias
		Bug         *Bug                `json:"bug,omitempty"`
		Links       map[string]string   `json:"links,omitempty"`
		Regressions []MinimalRegression `json:"regressions"`
	}{
		Alias:       Alias(*t),
		Bug:         nil,
		Links:       nil,
		Regressions: regressions,
	}

	return json.Marshal(auditJSON)
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
	ID      uint   `json:"id" gorm:"primaryKey,column:id"`
	View    string `json:"view" gorm:"not null"`
	Release string `json:"release" gorm:"not null;index:idx_test_regression_release"`
	// BaseRelease is the release this test was marked regressed against. It may not match the view's base release
	// if the view uses release fallback and this test was flagged regressed against a prior release with better pass rate.
	BaseRelease string         `json:"base_release"`
	Component   string         `json:"component"`
	Capability  string         `json:"capability"`
	TestID      string         `json:"test_id" gorm:"not null"`
	TestName    string         `json:"test_name" gorm:"not null;index:idx_test_regression_test_name"`
	Variants    pq.StringArray `json:"variants" gorm:"not null;type:text[]"`
	Opened      time.Time      `json:"opened" gorm:"not null"`
	Closed      sql.NullTime   `json:"closed"`
	Triages     []Triage       `json:"triages" gorm:"many2many:triage_regressions;"`
	// LastFailure is the last failure in the sample we saw while this regression was open.
	LastFailure sql.NullTime `json:"last_failure"`
	// MaxFailures is the maximum number of failures we found in the reporting window while this regression was open.
	// This is intended to help inform the min fail thresholds we should be using, and what kind of regressions
	// disappear on their own.
	MaxFailures int `json:"max_failures"`

	// Links contains HATEOAS-style links for this regression record (not stored in database)
	Links map[string]string `json:"links,omitempty" gorm:"-"`
}

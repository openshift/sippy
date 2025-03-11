package models

import (
	"github.com/openshift/sippy/pkg/apis/api/componentreport"
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
	Regressions []componentreport.TestRegression `gorm:"constraint:OnDelete:CASCADE;many2many:triage_regressions;"`
}

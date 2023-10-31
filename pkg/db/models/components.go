package models

import "github.com/lib/pq"

type JiraComponent struct {
	Model

	// Name is the name of the component
	Name string

	// Description is the description of the component
	Description string

	// LeadName is the component owner
	LeadName string

	// LeadEmail is the component owner's e-mail
	LeadEmail string
}

type TestOwnership struct {
	Model

	// APIVersion specifies the schema version, in case we ever need to make
	// changes to the bigquery table that are not simple column additions.
	APIVersion string `bigquery:"apiVersion"`

	// Kind is a string value representing the resource this object represents.
	Kind string `bigquery:"kind"`

	// UniqueID is a stable name for the test. This should hold the oldest name
	// of the test, which allows us to make comparisons even when the test
	// name has changed.
	UniqueID string `bigquery:"id"`

	// Name is the current name for the test.
	Name string `bigquery:"name" gorm:"uniqueIndex:idx_name_suite"`

	// TestID is the ID of the test in Sippy's database.
	TestID uint `gorm:"index"`

	// Suite is the name for the junit test suite, if any.  Generally leave this blank, and we'll
	// fill it in from the supplied TestInfo.
	Suite string `bigquery:"suite" gorm:"uniqueIndex:idx_name_suite"`

	// SuiteID is the ID of the suite in Sippy's database.
	SuiteID *uint `gorm:"index"`

	// Product is the layer product name, to support the possibility of multiple
	// component readiness dashboards. Generally leave this blank.
	Product string `bigquery:"product"`

	// Priority allows the ability to take priority on a test's ownership. If
	// two components are vying for a test's ownership and one wants to force
	// the matter, you may use a higher priority value (default: 0). The highest
	// value wins.
	Priority int `bigquery:"priority"`

	// StaffApprovedObsolete controls removal of tests.  If tests are removed but was
	// previously assigned to a component without this flag being set, then the component
	// readiness dashboard will show this as a problem. This should always be false, unless
	// a staff engineer approves returning true.
	StaffApprovedObsolete bool `bigquery:"staff_approved_obsolete"`

	// Component is the principal owner of a test. It should map directly to a JIRA bug component.
	// A test should only have one component owner, see above about the priority flag when contention
	// for ownership of a test arises.
	Component string `bigquery:"component"`

	// Capabilities are the particular capability a test is testing.  A test may map to multiple
	// capabilities. For example, a networking test could belong to OVN, IPv6, and EndpointSlices capabilities.
	Capabilities pq.StringArray `json:"capabilities" gorm:"type:text[]"`

	// JiraComponent specifies the JIRA component that this test belongs to.
	JiraComponent string `bigquery:"jira_component"`

	// JiraComponent specifies the JIRA component that this test belongs to.
	JiraComponentID *uint `gorm:"index"`
}

package v1

import (
	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
)

// Component is the interface that component owners should implement to claim
// ownership and capabilities for their tests.
type Component interface {
	// IdentifyTest returns ownership information about a test.  An implementation
	// should return `nil, nil` when the test is not theirs. Implementations should
	// only return an error on a fatal error such as inability to read from a file.
	IdentifyTest(*TestInfo) (*TestOwnership, error)

	// StableID returns the stable ID for a test, given a particular TestInfo. This
	// is generally the suite name + . + test name.  If a test gets renamed, this should return
	// the suite `name + . + the oldest test name` to ensure a stable mapping.
	StableID(*TestInfo) string

	JiraComponents() []string

	// Namespaces returns the list of namespaces owned by this component.
	ListNamespaces() []string
}

// TestInfo is the input to the component owners with metadata about a test. It currently includes only
// the test name and suite, but could in the future contain additional metadata.
type TestInfo struct {
	Name  string
	Suite string
}

const APIVersion = "v1"
const Kind = "TestOwnership"

// TestOwnership is the information a component owner needs to return about their ownership of a test.
type TestOwnership struct {
	// APIVersion specifies the schema version, in case we ever need to make
	// changes to the bigquery table that are not simple column additions.
	//
	// Components should not set this value.
	APIVersion string `bigquery:"apiVersion"`

	// Kind is a string value representing the resource this object represents.
	//
	// Components should not set this value.
	Kind string `bigquery:"kind"`

	// ID is a stable name for the test. This should hold the oldest name
	// of the test, which allows us to make comparisons even when the test
	// name has changed.
	//
	// Components should not set this value.
	ID string `bigquery:"id"`

	// Name is the current name for the test.
	Name string `bigquery:"name"`

	// Suite is the name for the junit test suite, if any.  Generally leave this blank, and we'll
	// fill it in from the supplied TestInfo.
	Suite string `bigquery:"suite"`

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
	Capabilities []string `bigquery:"capabilities"`

	// JIRAComponent specifies the JIRA component that this test belongs to.
	JIRAComponent string `bigquery:"jira_component"`

	// CreatedAt is the time this particular record was created.
	//
	// Components do not need to set this value.
	CreatedAt civil.DateTime `bigquery:"created_at" json:"-"`
}

var MappingTableSchema = bigquery.Schema{
	{
		Name: "kind",
		Type: bigquery.StringFieldType,
	},
	{
		Name: "apiVersion",
		Type: bigquery.StringFieldType,
	},
	{
		Name: "id",
		Type: bigquery.StringFieldType,
	},
	{
		Name: "name",
		Type: bigquery.StringFieldType,
	},
	{
		Name: "suite",
		Type: bigquery.StringFieldType,
	},
	{
		Name: "product",
		Type: bigquery.StringFieldType,
	},
	{
		Name: "component",
		Type: bigquery.StringFieldType,
	},
	{
		Name: "jira_component",
		Type: bigquery.StringFieldType,
	},
	{
		Name:     "capabilities",
		Type:     bigquery.StringFieldType,
		Repeated: true,
	},
	{
		Name: "staff_approved_obsolete",
		Type: bigquery.BooleanFieldType,
	},
	{
		Name: "priority",
		Type: bigquery.IntegerFieldType,
	},
	{
		Name: "created_at",
		Type: bigquery.DateTimeFieldType,
	},
}

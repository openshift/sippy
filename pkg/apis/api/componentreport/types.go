package componentreport

import (
	"time"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
)

// These are top-level types for a component report API response,
// based on lower-level types shared with testdetails reports, as well as foundational types for tests.
// There should be few reasons to add types here; usually new concepts belong in a more tightly-scoped subpackage.

type ComponentReport struct {
	Rows        []ReportRow `json:"rows,omitempty"`
	GeneratedAt *time.Time  `json:"generated_at"`
	Warnings    []string    `json:"warnings,omitempty"`
}

type ReportRow struct {
	crtest.RowIdentification
	Columns []ReportColumn `json:"columns,omitempty"`
}

type ReportColumn struct {
	crtest.ColumnIdentification
	Status         crtest.Status       `json:"status"`
	RegressedTests []ReportTestSummary `json:"regressed_tests,omitempty"`
}

type ReportTestSummary struct {
	// TODO: really feels like this could just be moved  TestComparison, eliminating the need for ReportTestSummary
	crtest.Identification
	testdetails.TestComparison
}

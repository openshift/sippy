package componentreport

import (
	"time"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
)

type Release struct {
	Release string
	End     *time.Time
	Start   *time.Time
}

//nolint:revive

type ComponentReport struct {
	Rows        []ReportRow `json:"rows,omitempty"`
	GeneratedAt *time.Time  `json:"generated_at"`
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

type ReportResponse []ReportRow

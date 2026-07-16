package query

import (
	"testing"
	"time"

	"cloud.google.com/go/civil"

	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/filter"
)

func TestEscapeLikeMetachars(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no metacharacters",
			input:    "install should succeed",
			expected: "install should succeed",
		},
		{
			name:     "underscore escaped",
			input:    "test_name_here",
			expected: `test\_name\_here`,
		},
		{
			name:     "percent escaped",
			input:    "100% complete",
			expected: `100\% complete`,
		},
		{
			name:     "backslash escaped",
			input:    `path\to\test`,
			expected: `path\\to\\test`,
		},
		{
			name:     "all metacharacters",
			input:    `a_b%c\d`,
			expected: `a\_b\%c\\d`,
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := escapeLikeMetachars(tc.input)
			if got != tc.expected {
				t.Errorf("escapeLikeMetachars(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestBuildTestsJoinCondition(t *testing.T) {
	tests := []struct {
		name         string
		matches      TestNameMatches
		wantClause   string
		wantArgCount int
		wantFirstArg string
	}{
		{
			name:         "empty matches joins all tests",
			wantClause:   "JOIN tests ON tests.id = e.test_id",
			wantArgCount: 0,
		},
		{
			name:         "exact name",
			matches:      TestNameMatches{ExactNames: []string{"my test"}},
			wantClause:   "JOIN tests ON tests.id = e.test_id AND (tests.name = ?)",
			wantArgCount: 1,
			wantFirstArg: "my test",
		},
		{
			name:         "prefix with underscore is escaped",
			matches:      TestNameMatches{Prefixes: []string{"install_should"}},
			wantClause:   "JOIN tests ON tests.id = e.test_id AND (tests.name LIKE ?)",
			wantArgCount: 1,
			wantFirstArg: `install\_should%`,
		},
		{
			name:         "substring uses ILIKE and is escaped",
			matches:      TestNameMatches{Substrings: []string{"test_name"}},
			wantClause:   "JOIN tests ON tests.id = e.test_id AND (tests.name ILIKE ?)",
			wantArgCount: 1,
			wantFirstArg: `%test\_name%`,
		},
		{
			name: "multiple match types combined with OR",
			matches: TestNameMatches{
				ExactNames: []string{"exact"},
				Prefixes:   []string{"pre"},
				Substrings: []string{"sub"},
			},
			wantClause:   "JOIN tests ON tests.id = e.test_id AND (tests.name = ? OR tests.name LIKE ? OR tests.name ILIKE ?)",
			wantArgCount: 3,
			wantFirstArg: "exact",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clause, args := buildTestsJoinCondition(tc.matches)
			if clause != tc.wantClause {
				t.Errorf("clause = %q, want %q", clause, tc.wantClause)
			}
			if len(args) != tc.wantArgCount {
				t.Errorf("len(args) = %d, want %d", len(args), tc.wantArgCount)
			}
			if tc.wantArgCount > 0 && len(args) > 0 {
				if got, ok := args[0].(string); !ok || got != tc.wantFirstArg {
					t.Errorf("args[0] = %v, want %q", args[0], tc.wantFirstArg)
				}
			}
		})
	}
}

func TestNameFilterConditions(t *testing.T) {
	tests := []struct {
		name         string
		filter       *filter.Filter
		wantCount    int
		wantFirstCon string
		wantFirstArg string
	}{
		{
			name:      "nil filter",
			filter:    nil,
			wantCount: 0,
		},
		{
			name: "negative exact name",
			filter: &filter.Filter{Items: []filter.FilterItem{
				{Field: "name", Operator: filter.OperatorEquals, Value: "excluded", Not: true},
			}},
			wantCount:    1,
			wantFirstCon: "NOT(tests.name = ?)",
			wantFirstArg: "excluded",
		},
		{
			name: "negative substring",
			filter: &filter.Filter{Items: []filter.FilterItem{
				{Field: "name", Operator: filter.OperatorContains, Value: "skip_this", Not: true},
			}},
			wantCount:    1,
			wantFirstCon: "NOT(tests.name ILIKE ?)",
			wantFirstArg: `%skip\_this%`,
		},
		{
			name: "positive and negative combined",
			filter: &filter.Filter{
				Items: []filter.FilterItem{
					{Field: "name", Operator: filter.OperatorContains, Value: "network"},
					{Field: "name", Operator: filter.OperatorContains, Value: "ipv6", Not: true},
				},
				LinkOperator: filter.LinkOperatorAnd,
			},
			wantCount:    2,
			wantFirstCon: "tests.name ILIKE ?",
			wantFirstArg: "%network%",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			conditions, args := nameFilterConditions(tc.filter)
			if len(conditions) != tc.wantCount {
				t.Errorf("len(conditions) = %d, want %d", len(conditions), tc.wantCount)
			}
			if tc.wantCount > 0 && len(conditions) > 0 {
				if conditions[0] != tc.wantFirstCon {
					t.Errorf("conditions[0] = %q, want %q", conditions[0], tc.wantFirstCon)
				}
				if got, ok := args[0].(string); !ok || got != tc.wantFirstArg {
					t.Errorf("args[0] = %v, want %q", args[0], tc.wantFirstArg)
				}
			}
		})
	}
}

func TestPeriodsForReportType(t *testing.T) {
	tomorrow := civil.DateOf(time.Now().UTC()).AddDays(1)

	tests := []struct {
		name       string
		reportType v1.ReportType
		wantSample DateRange
		wantBase   DateRange
	}{
		{
			name:       "current report has 7-day sample and 7-day base",
			reportType: v1.CurrentReport,
			wantSample: DateRange{Start: tomorrow.AddDays(-8), End: tomorrow},
			wantBase:   DateRange{Start: tomorrow.AddDays(-15), End: tomorrow.AddDays(-8)},
		},
		{
			name:       "two-day report has 3-day sample and 7-day base",
			reportType: v1.TwoDayReport,
			wantSample: DateRange{Start: tomorrow.AddDays(-3), End: tomorrow},
			wantBase:   DateRange{Start: tomorrow.AddDays(-10), End: tomorrow.AddDays(-3)},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sample, base := PeriodsForReportType(tc.reportType)
			if sample != tc.wantSample {
				t.Errorf("sample = %+v, want %+v", sample, tc.wantSample)
			}
			if base != tc.wantBase {
				t.Errorf("base = %+v, want %+v", base, tc.wantBase)
			}
		})
	}
}

func TestPeriodsForReportType_Contiguous(t *testing.T) {
	tests := []struct {
		name       string
		reportType v1.ReportType
	}{
		{name: "current", reportType: v1.CurrentReport},
		{name: "twoDay", reportType: v1.TwoDayReport},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sample, base := PeriodsForReportType(tc.reportType)
			if sample.Start != base.End {
				t.Errorf("sample.Start (%v) != base.End (%v): periods are not contiguous", sample.Start, base.End)
			}
		})
	}
}

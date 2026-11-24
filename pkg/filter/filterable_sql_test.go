package filter

import (
	"errors"
	"slices"
	"testing"

	"cloud.google.com/go/bigquery"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	apitype "github.com/openshift/sippy/pkg/apis/api"
)

// injectionField closes a Go-%q-quoted identifier in PostgreSQL, which does not treat
// backslash as an escape inside double-quoted identifiers.
const injectionField = `name" = 'x' OR 1=1 --`

func TestValidateField(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		wantErr bool
	}{
		{name: "snake case column", field: "current_pass_percentage"},
		{name: "camel case in-memory field", field: "upstreamJob"},
		{name: "leading underscore", field: "_internal"},
		{name: "digits after first character", field: "v5name"},
		{name: "empty", field: "", wantErr: true},
		{name: "double quote breakout", field: injectionField, wantErr: true},
		{name: "backtick breakout", field: "name` OR TRUE --", wantErr: true},
		{name: "parenthesis breakout", field: "name) OR (TRUE", wantErr: true},
		{name: "whitespace", field: "current runs", wantErr: true},
		{name: "qualified name", field: "prow_jobs.name", wantErr: true},
		{name: "leading digit", field: "1name", wantErr: true},
		{name: "hyphen", field: "pull-request", wantErr: true},
		{name: "statement terminator", field: "name;", wantErr: true},
		{name: "placeholder", field: "name?", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateField(tc.field)
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidField) {
					t.Fatalf("ValidateField(%q) error = %v, want ErrInvalidField", tc.field, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateField(%q) unexpected error: %v", tc.field, err)
			}
		})
	}
}

func TestFilterValidateFields(t *testing.T) {
	tests := []struct {
		name    string
		items   []FilterItem
		wantErr bool
	}{
		{name: "no items"},
		{name: "all valid", items: []FilterItem{{Field: "name"}, {Field: "current_runs"}}},
		{name: "invalid after valid", items: []FilterItem{{Field: "name"}, {Field: injectionField}}, wantErr: true},
		{name: "invalid first", items: []FilterItem{{Field: "name) OR (TRUE"}, {Field: "name"}}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Filter{Items: tc.items}.ValidateFields()
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidField) {
					t.Fatalf("ValidateFields() error = %v, want ErrInvalidField", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateFields() unexpected error: %v", err)
			}
		})
	}
}

func TestFilterFieldToSQL(t *testing.T) {
	tests := []struct {
		name      string
		item      FilterItem
		wantSQL   string
		wantParam any
		wantErr   bool
		wantErrIs error // optional sentinel the error must wrap
	}{
		{
			name:      "equals quotes column and binds value",
			item:      FilterItem{Field: "name", Operator: OperatorEquals, Value: "e2e-aws"},
			wantSQL:   `"name" = ?`,
			wantParam: "e2e-aws",
		},
		{
			name:      "negated arithmetic comparison",
			item:      FilterItem{Field: "current_runs", Operator: OperatorArithmeticGreaterThan, Value: "7", Not: true},
			wantSQL:   `NOT("current_runs" > ?)`,
			wantParam: "7",
		},
		{
			name:      "contains on array column unnests",
			item:      FilterItem{Field: "tags", Operator: OperatorContains, Value: "50%"},
			wantSQL:   `EXISTS (SELECT 1 FROM unnest("tags") AS elem WHERE elem ILIKE ?)`,
			wantParam: `%50\%%`,
		},
		{
			name:    "is empty on array column",
			item:    FilterItem{Field: "labels", Operator: OperatorIsEmpty},
			wantSQL: `("labels" IS NULL or "labels" = '{}')`,
		},
		{
			name:      "quote breakout field is rejected",
			item:      FilterItem{Field: injectionField, Operator: OperatorEquals, Value: "x"},
			wantErr:   true,
			wantErrIs: ErrInvalidField,
		},
		{
			name:    "unsupported operator is an error",
			item:    FilterItem{Field: "name", Operator: "matches", Value: "x"},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sql, param, err := tc.item.FilterFieldToSQL(apitype.JobRun{})
			if tc.wantErr {
				if err == nil || (tc.wantErrIs != nil && !errors.Is(err, tc.wantErrIs)) {
					t.Fatalf("error = %v, want error wrapping %v", err, tc.wantErrIs)
				}
				if sql != "" {
					t.Errorf("expected no SQL on error, got %q", sql)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sql != tc.wantSQL {
				t.Errorf("sql = %q, want %q", sql, tc.wantSQL)
			}
			if param != tc.wantParam {
				t.Errorf("param = %#v, want %#v", param, tc.wantParam)
			}
		})
	}
}

func TestFilterItemToSQL(t *testing.T) {
	tests := []struct {
		name      string
		item      FilterItem
		column    string
		wantSQL   string
		wantParam any
		wantErrIs error
	}{
		{
			name:      "renders the given column and binds value",
			item:      FilterItem{Field: "job", Operator: OperatorStartsWith, Value: "e2e_"},
			column:    "prow_jobs.name",
			wantSQL:   "prow_jobs.name ILIKE ?",
			wantParam: `e2e\_%`,
		},
		{
			name:    "negated is not empty",
			item:    FilterItem{Field: "job", Operator: OperatorIsNotEmpty, Not: true},
			column:  "prow_jobs.name",
			wantSQL: "NOT(prow_jobs.name IS NOT NULL)",
		},
		{
			name:      "quote breakout field is rejected",
			item:      FilterItem{Field: injectionField, Operator: OperatorEquals, Value: "x"},
			column:    "prow_jobs.name",
			wantErrIs: ErrInvalidField,
		},
		{
			name:      "unsupported operator is an error",
			item:      FilterItem{Field: "job", Operator: "matches", Value: "x"},
			column:    "prow_jobs.name",
			wantErrIs: errAny,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sql, param, err := tc.item.FilterItemToSQL(tc.column)
			if tc.wantErrIs != nil {
				if err == nil || (tc.wantErrIs != errAny && !errors.Is(err, tc.wantErrIs)) {
					t.Fatalf("error = %v, want error wrapping %v", err, tc.wantErrIs)
				}
				if sql != "" {
					t.Errorf("expected no SQL on error, got %q", sql)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sql != tc.wantSQL {
				t.Errorf("sql = %q, want %q", sql, tc.wantSQL)
			}
			if param != tc.wantParam {
				t.Errorf("param = %#v, want %#v", param, tc.wantParam)
			}
		})
	}
}

// errAny marks a test case that expects some error without a specific sentinel.
var errAny = errors.New("any error")

// dryRunDB renders SQL through the real PostgreSQL dialect without connecting.
func dryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbc, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 port=1 dbname=none sslmode=disable"}),
		&gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("opening dry-run gorm DB: %v", err)
	}
	return dbc
}

func TestFilterToSQL(t *testing.T) {
	tests := []struct {
		name     string
		filter   Filter
		wantSQL  string
		wantVars []any
		wantErr  error
	}{
		{
			name: "and filters become separate bound conditions",
			filter: Filter{Items: []FilterItem{
				{Field: "name", Operator: OperatorContains, Value: "aws"},
				{Field: "current_runs", Operator: OperatorArithmeticGreaterThanOrEquals, Value: "7"},
			}},
			wantSQL:  `SELECT * FROM "jobs" WHERE "name" ILIKE $1 AND "current_runs" >= $2`,
			wantVars: []any{"%aws%", "7"},
		},
		{
			name: "or filters are grouped and bound",
			filter: Filter{LinkOperator: LinkOperatorOr, Items: []FilterItem{
				{Field: "name", Operator: OperatorEquals, Value: "a"},
				{Field: "name", Operator: OperatorEquals, Value: "b"},
			}},
			wantSQL:  `SELECT * FROM "jobs" WHERE "name" = $1 or "name" = $2`,
			wantVars: []any{"a", "b"},
		},
		{
			name: "invalid field in or filter fails the query",
			filter: Filter{LinkOperator: LinkOperatorOr, Items: []FilterItem{
				{Field: "name", Operator: OperatorEquals, Value: "a"},
				{Field: injectionField, Operator: OperatorEquals, Value: "b"},
			}},
			wantErr: ErrInvalidField,
		},
		{
			name: "invalid field in and filter fails the query",
			filter: Filter{Items: []FilterItem{
				{Field: injectionField, Operator: OperatorEquals, Value: "b"},
			}},
			wantErr: ErrInvalidField,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := dryRunDB(t)
			q := tc.filter.ToSQL(root.Table("jobs"), apitype.Job{})
			var rows []map[string]any
			result := q.Find(&rows)
			if tc.wantErr != nil {
				if !errors.Is(result.Error, tc.wantErr) {
					t.Fatalf("error = %v, want %v", result.Error, tc.wantErr)
				}
				if root.Error != nil {
					t.Errorf("filter error leaked onto the shared root handle: %v", root.Error)
				}
				return
			}
			if result.Error != nil {
				t.Fatalf("unexpected error: %v", result.Error)
			}
			if got := result.Statement.SQL.String(); got != tc.wantSQL {
				t.Errorf("sql = %q, want %q", got, tc.wantSQL)
			}
			if !slices.Equal(result.Statement.Vars, tc.wantVars) {
				t.Errorf("vars = %#v, want %#v", result.Statement.Vars, tc.wantVars)
			}
		})
	}
}

func TestToBQStr(t *testing.T) {
	tests := []struct {
		name       string
		filter     Filter
		wantSQL    string
		wantParams []bigquery.QueryParameter
		wantIndex  int
		wantErr    error
	}{
		{
			name: "valid fields stay bare and values are named parameters",
			filter: Filter{LinkOperator: LinkOperatorOr, Items: []FilterItem{
				{Field: "name", Operator: OperatorContains, Value: "AWS"},
				{Field: "current_runs", Operator: OperatorArithmeticGreaterThan, Value: "7"},
			}},
			wantSQL: "( LOWER(name) LIKE @filterParam1 OR current_runs > @filterParam2)",
			wantParams: []bigquery.QueryParameter{
				{Name: "filterParam1", Value: "%aws%"},
				{Name: "filterParam2", Value: float64(7)},
			},
			wantIndex: 2,
		},
		{
			name: "field that would be unquoted SQL is rejected",
			filter: Filter{Items: []FilterItem{
				{Field: "name) OR (TRUE", Operator: OperatorEquals, Value: "x"},
			}},
			wantErr: ErrInvalidField,
		},
		{
			name: "quote breakout field is rejected",
			filter: Filter{Items: []FilterItem{
				{Field: injectionField, Operator: OperatorEquals, Value: "x"},
			}},
			wantErr: ErrInvalidField,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			paramIndex := 0
			result, err := tc.filter.ToBQStr(apitype.Test{}, &paramIndex)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.SQL != tc.wantSQL {
				t.Errorf("sql = %q, want %q", result.SQL, tc.wantSQL)
			}
			if !slices.Equal(result.Parameters, tc.wantParams) {
				t.Errorf("params = %#v, want %#v", result.Parameters, tc.wantParams)
			}
			if paramIndex != tc.wantIndex {
				t.Errorf("paramIndex = %d, want %d", paramIndex, tc.wantIndex)
			}
		})
	}
}

func TestFilterHelpersReturnRenderErrors(t *testing.T) {
	invalid := &Filter{Items: []FilterItem{{Field: injectionField, Operator: OperatorEquals, Value: "x"}}}
	valid := &Filter{Items: []FilterItem{{Field: "name", Operator: OperatorEquals, Value: "x"}}}
	tests := []struct {
		name    string
		apply   func(*gorm.DB, *Filter) (*gorm.DB, error)
		filter  *Filter
		wantErr error
	}{
		{
			name: "FilterableDBResult rejects invalid field",
			apply: func(db *gorm.DB, f *Filter) (*gorm.DB, error) {
				return FilterableDBResult(db, &FilterOptions{Filter: f, SortField: "name"}, apitype.Job{})
			},
			filter:  invalid,
			wantErr: ErrInvalidField,
		},
		{
			name: "ApplyFilters rejects invalid field",
			apply: func(db *gorm.DB, f *Filter) (*gorm.DB, error) {
				return ApplyFilters(f, "name", apitype.SortDescending, 10, db, apitype.Job{})
			},
			filter:  invalid,
			wantErr: ErrInvalidField,
		},
		{
			name: "FilterableDBResult accepts valid field",
			apply: func(db *gorm.DB, f *Filter) (*gorm.DB, error) {
				return FilterableDBResult(db, &FilterOptions{Filter: f}, apitype.Job{})
			},
			filter: valid,
		},
		{
			name: "ApplyFilters accepts valid field",
			apply: func(db *gorm.DB, f *Filter) (*gorm.DB, error) {
				return ApplyFilters(f, "name", apitype.SortDescending, 10, db, apitype.Job{})
			},
			filter: valid,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q, err := tc.apply(dryRunDB(t).Table("jobs"), tc.filter)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				if q != nil {
					t.Errorf("expected nil query on error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q == nil || q.Error != nil {
				t.Fatalf("expected usable query, got %v", q)
			}
		})
	}
}

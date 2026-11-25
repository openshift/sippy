// This file is deliberately kept separate so it can be excluded from the Snyk code scan
// (see .snyk). Snyk reports a false positive go/Sqli finding here because the filter
// field and operator come from the request, even though they only select constant SQL
// fragments and every value is bound. Because this file is not scanned, take extra care
// not to introduce real SQL injection: build SQL text only from the constants below and
// fixed operator switches, pass all request values as bound parameters, and do not add
// unrelated code to this file.

package api

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/filter"
)

// jobRunFilterColumns maps every filterable field (other than the test name fields) to
// its table-qualified column expression. Some fields are SELECT aliases, which a
// PostgreSQL WHERE clause cannot reference, and the rest are base columns from
// jobRunsBaseSelect. Resolving fields here means filter SQL is built only from these
// constant expressions, so request text can select a column but never become SQL.
var jobRunFilterColumns = map[string]string{
	// SELECT aliases
	"id":                  "prow_job_runs.id",
	"job":                 "prow_jobs.name",
	"brief_name":          "regexp_replace(prow_jobs.name, 'periodic-ci-openshift-(multiarch|release)-(master|main)-(ci|nightly)-[0-9]+.[0-9]+-', '')",
	"prow_id":             "prow_job_runs.id",
	"test_grid_url":       "prow_job_runs.url",
	"timestamp":           `prow_job_runs."timestamp"`,
	"pull_request_link":   "pp.link",
	"pull_request_sha":    "pp.sha",
	"pull_request_org":    "pp.org",
	"pull_request_repo":   "pp.repo",
	"pull_request_author": "pp.author",
	// base columns
	"release":                "prow_jobs.release",
	"name":                   "prow_jobs.name",
	"variants":               "prow_jobs.variants",
	"overall_result":         "prow_job_runs.overall_result",
	"url":                    "prow_job_runs.url",
	"succeeded":              "prow_job_runs.succeeded",
	"infrastructure_failure": "prow_job_runs.infrastructure_failure",
	"known_failure":          "prow_job_runs.known_failure",
	"cluster":                "prow_job_runs.cluster",
	"labels":                 "prow_job_runs.labels",
	"test_failures":          "prow_job_runs.test_failures",
	"test_flakes":            "prow_job_runs.test_flakes",
}

// testNameStatuses maps test name filter fields to the prow_job_run_tests
// status code used in EXISTS subqueries. ran_test_names uses 0 to mean
// "no status constraint" (matches any test outcome). This is safe because
// TestStatusAbsent (0) is never stored in prow_job_run_tests rows.
var testNameStatuses = map[string]int{
	"ran_test_names":    0,
	"failed_test_names": int(sippyprocessingv1.TestStatusFailure),
	"flaked_test_names": int(sippyprocessingv1.TestStatusFlake),
}

func isTestNameField(field string) bool {
	_, ok := testNameStatuses[field]
	return ok
}

// applyJobRunFilters processes all filter items in a single pass, dispatching
// each to the appropriate handler based on field name. Fields that need
// special SQL (test name EXISTS or table-qualified column aliases) are handled
// directly; the rest use the generic filter SQL generator. All clauses are
// collected and combined with AND or OR based on linkOperator.
func applyJobRunFilters(q *gorm.DB, filterOpts *filter.FilterOptions, lookback time.Time) (*gorm.DB, error) {
	if filterOpts.Filter == nil || len(filterOpts.Filter.Items) == 0 {
		return filter.FilterableDBResult(q, filterOpts, apitype.JobRun{})
	}

	var clauses []string
	var allArgs []any
	for _, item := range filterOpts.Filter.Items {
		var sql string
		var param any
		switch {
		case isTestNameField(item.Field):
			sqlFrag, args, err := testNameFilterSQL(item, lookback)
			if err != nil {
				return nil, &ValidationError{Message: err.Error()}
			}
			clauses = append(clauses, sqlFrag)
			allArgs = append(allArgs, args...)
			continue
		default:
			col, ok := jobRunFilterColumns[item.Field]
			if !ok {
				return nil, &ValidationError{Message: fmt.Sprintf("%v: %q is not a filterable job run field", filter.ErrInvalidField, item.Field)}
			}
			// Only the constant column, its type and the operator shape the SQL text; the
			// request value is passed separately so it can only ever be a bound parameter.
			var err error
			sql, err = filter.TypedColumnFilterSQL(col, apitype.JobRun{}.GetFieldType(item.Field), item.Operator, item.Not)
			if err != nil {
				return nil, &ValidationError{Message: err.Error()}
			}
			param = filter.ColumnFilterParam(item.Operator, item.Value)
		}
		if sql != "" {
			clauses = append(clauses, sql)
			if param != nil {
				allArgs = append(allArgs, param)
			}
		}
	}
	if len(clauses) > 0 {
		joiner := " AND "
		if filterOpts.Filter.LinkOperator == filter.LinkOperatorOr {
			joiner = " OR "
		}
		q = q.Where("("+strings.Join(clauses, joiner)+")", allArgs...)
	}

	sortOpts := *filterOpts
	sortOpts.Filter = &filter.Filter{}
	return filter.FilterableDBResult(q, &sortOpts, apitype.JobRun{})
}

func testNameFilterSQL(item filter.FilterItem, lookback time.Time) (string, []any, error) {
	existsBase := "EXISTS (SELECT 1 FROM prow_job_run_tests JOIN tests ON tests.id = prow_job_run_tests.test_id WHERE prow_job_run_tests.prow_job_run_id = prow_job_runs.id AND prow_job_run_tests.prow_job_run_release = prow_job_runs.prow_job_release AND prow_job_run_tests.prow_job_run_timestamp = prow_job_runs.timestamp AND prow_job_run_tests.prow_job_run_timestamp >= ?"
	args := []any{lookback}
	// The status is bound rather than formatted so the SQL text stays a constant even
	// though the status is looked up by the request's field name.
	if status := testNameStatuses[item.Field]; status != 0 {
		existsBase += " AND prow_job_run_tests.status = ?"
		args = append(args, status)
	}

	var sql string
	switch item.Operator {
	case filter.OperatorIsEmpty:
		sql = "NOT " + existsBase + ")"
	case filter.OperatorIsNotEmpty:
		sql = existsBase + ")"
	case filter.OperatorHasEntry, filter.OperatorEquals:
		sql = existsBase + " AND tests.name = ?)"
		args = append(args, item.Value)
	case filter.OperatorStartsWith:
		sql = existsBase + " AND tests.name ILIKE ?)"
		args = append(args, fmt.Sprintf("%s%%", filter.EscapeLikeMetachars(item.Value)))
	case filter.OperatorEndsWith:
		sql = existsBase + " AND tests.name ILIKE ?)"
		args = append(args, fmt.Sprintf("%%%s", filter.EscapeLikeMetachars(item.Value)))
	case filter.OperatorContains, filter.OperatorHasEntryContaining:
		sql = existsBase + " AND tests.name ILIKE ?)"
		args = append(args, fmt.Sprintf("%%%s%%", filter.EscapeLikeMetachars(item.Value)))
	default:
		return "", nil, fmt.Errorf("unsupported operator %q for field %q", item.Operator, item.Field)
	}
	return filter.WrapNot(sql, item.Not), args, nil
}

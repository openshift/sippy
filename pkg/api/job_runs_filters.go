// This file is deliberately kept separate so it can be excluded from the Snyk code scan
// (see .snyk). Snyk reports a false positive go/Sqli finding here because the filter
// field and operator come from the request, even though alias fields only select the
// constant columnAliases expressions, other fields pass filter.ValidateField and are quoted
// as identifiers, and every value is bound. Because this file is not scanned, take extra
// care not to introduce real SQL injection: build SQL text only from the constants below,
// validated and quoted identifiers, and fixed operator switches, pass all request values
// as bound parameters, and do not add unrelated code to this file.

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

// columnAliases maps filter field names that are SELECT aliases (not base
// table columns) to their table-qualified column expressions. PostgreSQL
// WHERE clauses cannot reference SELECT aliases, so these fields must be
// rewritten before they reach the generic filter system.
var columnAliases = map[string]string{
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
			var err error
			if col, ok := columnAliases[item.Field]; ok {
				sql, param, err = item.FilterItemToSQL(col)
			} else {
				sql, param, err = item.FilterFieldToSQL(apitype.JobRun{})
			}
			if err != nil {
				return nil, &ValidationError{Message: err.Error()}
			}
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
	statusClause := ""
	if status := testNameStatuses[item.Field]; status != 0 {
		statusClause = fmt.Sprintf(" AND prow_job_run_tests.status = %d", status)
	}

	existsBase := fmt.Sprintf(
		"EXISTS (SELECT 1 FROM prow_job_run_tests JOIN tests ON tests.id = prow_job_run_tests.test_id WHERE prow_job_run_tests.prow_job_run_id = prow_job_runs.id AND prow_job_run_tests.prow_job_run_release = prow_job_runs.prow_job_release AND prow_job_run_tests.prow_job_run_timestamp = prow_job_runs.timestamp AND prow_job_run_tests.prow_job_run_timestamp >= ?%s",
		statusClause,
	)

	var sql string
	var args []any
	switch item.Operator {
	case filter.OperatorIsEmpty:
		sql = "NOT " + existsBase + ")"
		args = []any{lookback}
	case filter.OperatorIsNotEmpty:
		sql = existsBase + ")"
		args = []any{lookback}
	case filter.OperatorHasEntry, filter.OperatorEquals:
		sql = existsBase + " AND tests.name = ?)"
		args = []any{lookback, item.Value}
	case filter.OperatorStartsWith:
		sql = existsBase + " AND tests.name ILIKE ?)"
		args = []any{lookback, fmt.Sprintf("%s%%", filter.EscapeLikeMetachars(item.Value))}
	case filter.OperatorEndsWith:
		sql = existsBase + " AND tests.name ILIKE ?)"
		args = []any{lookback, fmt.Sprintf("%%%s", filter.EscapeLikeMetachars(item.Value))}
	case filter.OperatorContains, filter.OperatorHasEntryContaining:
		sql = existsBase + " AND tests.name ILIKE ?)"
		args = []any{lookback, fmt.Sprintf("%%%s%%", filter.EscapeLikeMetachars(item.Value))}
	default:
		return "", nil, fmt.Errorf("unsupported operator %q for field %q", item.Operator, item.Field)
	}
	return filter.WrapNot(sql, item.Not), args, nil
}

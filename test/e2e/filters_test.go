package e2e

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/test/e2e/util"
)

// TestFilterFieldValidation verifies that filter column names that are not simple identifiers are
// rejected as bad requests on endpoints that render filter fields into SQL, while an otherwise
// identical filter on a real column still succeeds.
func TestFilterFieldValidation(t *testing.T) {
	const quoteBreakout = `current_runs" >= 0 OR 1=1 --`

	tests := []struct {
		name       string
		path       string
		validField string
	}{
		{name: "tests report", path: "/api/tests?release=" + util.Release, validField: "current_runs"},
		{name: "uncollapsed tests report", path: "/api/tests?collapse=false&release=" + util.Release, validField: "working_average"},
		{name: "job runs", path: "/api/jobs/runs?release=" + util.Release, validField: "id"},
		{name: "job analysis", path: "/api/jobs/analysis?release=" + util.Release, validField: "current_runs"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var result any
			err := util.SippyGet(filterPath(tc.path, tc.validField, ">="), &result)
			require.NoError(t, err, "valid filter field should succeed")

			err = util.SippyGet(filterPath(tc.path, quoteBreakout, ">="), &result)
			require.Error(t, err, "invalid filter field should be rejected")
			assert.Contains(t, err.Error(), "400", "expected 400 status code")
		})
	}
}

// TestFilterOperatorValidation verifies that an unsupported operator is a bad request. Job analysis
// sends cluster filters to its job runs subquery, whose render error GORM would otherwise drop.
func TestFilterOperatorValidation(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		field string
	}{
		{name: "job analysis job runs subquery", path: "/api/jobs/analysis?release=" + util.Release, field: "cluster"},
		{name: "job runs alias column", path: "/api/jobs/runs?release=" + util.Release, field: "job"},
		{name: "tests report", path: "/api/tests?release=" + util.Release, field: "current_runs"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var result any
			err := util.SippyGet(filterPath(tc.path, tc.field, "matches"), &result)
			require.Error(t, err, "unsupported filter operator should be rejected")
			assert.Contains(t, err.Error(), "400", "expected 400 status code")
		})
	}
}

// filterPath appends a single-item filter on field to path.
func filterPath(path, field, operator string) string {
	filterJSON := fmt.Sprintf(`{"items":[{"columnField":%q,"operatorValue":%q,"value":"0"}],"linkOperator":"and"}`, field, operator)
	return path + "&filter=" + url.QueryEscape(filterJSON)
}

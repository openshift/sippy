package e2e

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/test/e2e/util"
)

// TestFilterFieldValidation verifies that filter column names that are not simple identifiers are
// rejected as bad requests on endpoints that render filter fields into SQL, while an otherwise
// identical filter on a real column still succeeds.
func TestFilterFieldValidation(t *testing.T) {
	const filterTemplate = `{"items":[{"columnField":%q,"operatorValue":">=","value":"0"}],"linkOperator":"and"}`
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
			validFilter := url.QueryEscape(fmt.Sprintf(filterTemplate, tc.validField))
			status, body := getStatus(t, tc.path+"&filter="+validFilter)
			assert.Equal(t, http.StatusOK, status, "valid filter field should succeed: %s", body)

			invalidFilter := url.QueryEscape(fmt.Sprintf(filterTemplate, quoteBreakout))
			status, body = getStatus(t, tc.path+"&filter="+invalidFilter)
			assert.Equal(t, http.StatusBadRequest, status, "invalid filter field should be rejected: %s", body)
		})
	}
}

func getStatus(t *testing.T, path string) (int, string) {
	t.Helper()
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(util.BuildE2EURL(path)) //nolint:gosec // G107: URL is built from the hardcoded local e2e endpoint
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

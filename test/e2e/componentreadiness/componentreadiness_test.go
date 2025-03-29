package componentreadiness

import (
	"fmt"
	"testing"

	"github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/test/e2e/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComponentReadinessViews(t *testing.T) {
	var views []componentreport.View
	err := util.SippyGet("/api/component_readiness/views", &views)
	require.NoError(t, err, "error making http request")
	t.Logf("found %d views", len(views))
	require.Greater(t, len(views), 0, "no views returned, check server cli params")

	// Make a basic request for the first view and ensure we get some data back
	var report componentreport.ComponentReport
	err = util.SippyGet(fmt.Sprintf("/api/component_readiness?view=%s", views[0].Name), &report)
	require.NoError(t, err, "error making http request")
	// We expect over 50 components at time of writing, asserting 25 should be safe
	assert.Greater(t, len(report.Rows), 25, "component report does not have rows we would expect")
}

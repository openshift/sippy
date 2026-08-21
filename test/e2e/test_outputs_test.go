package e2e

import (
	"net/url"
	"testing"

	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/test/e2e/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestOutputsAPI(t *testing.T) {
	testName := "[sig-network] Services should serve endpoints on same port and different protocol"
	path := "/api/tests/outputs?release=" + util.Release + "&test=" + url.QueryEscape(testName)

	var outputs []api.TestOutput
	err := util.SippyGet(path, &outputs)
	require.NoError(t, err, "error fetching test outputs")
	require.Greater(t, len(outputs), 0, "no test outputs returned for periodic job failures")

	for _, o := range outputs {
		assert.NotEmpty(t, o.Output, "output text should not be empty")
	}
}

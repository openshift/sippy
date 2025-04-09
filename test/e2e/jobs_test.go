package e2e

import (
	"testing"

	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/test/e2e/util"
	"github.com/stretchr/testify/assert"
)

func TestJobsAPIs(t *testing.T) {
	var jobs []api.Job
	err := util.SippyGet("/api/jobs?release="+util.Release, &jobs)
	if !assert.NoError(t, err, "error making http request") {
		return
	}
	t.Logf("found %d jobs", len(jobs))
	assert.Greater(t, len(jobs), 0, "no jobs returned")
}

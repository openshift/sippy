package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db/query"
	intutil "github.com/openshift/sippy/test/integration/util"
)

func TestLookupProwJobRunPartitionKeys(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	job := intutil.CreateProwJob(t, dbc, "periodic-e2e-aws", "4.18", nil)
	ts := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	run := intutil.CreateProwJobRun(t, dbc, job.ID, "4.18", ts, true, v1.JobSucceeded)

	keys, err := query.LookupProwJobRunPartitionKeys(dbc.DB, int64(run.ID))
	require.NoError(t, err)
	assert.Equal(t, "4.18", keys.ProwJobRelease)
	assert.True(t, ts.Equal(keys.Timestamp))

	_, err = query.LookupProwJobRunPartitionKeys(dbc.DB, 999999)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

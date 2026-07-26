package util

import (
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
)

func CreateProwJob(t *testing.T, dbc *db.DB, name, release string, variants []string) models.ProwJob {
	t.Helper()
	job := models.ProwJob{
		Name:     name,
		Release:  release,
		Variants: pq.StringArray(variants),
	}
	require.NoError(t, dbc.DB.Create(&job).Error, "creating ProwJob %q", name)
	return job
}

type ProwJobOption func(*models.ProwJob)

func WithKind(kind models.ProwKind) ProwJobOption {
	return func(j *models.ProwJob) {
		j.Kind = kind
	}
}

func CreateProwJobWithOptions(t *testing.T, dbc *db.DB, name, release string, variants []string, opts ...ProwJobOption) models.ProwJob {
	t.Helper()
	job := models.ProwJob{
		Name:     name,
		Release:  release,
		Variants: pq.StringArray(variants),
	}
	for _, opt := range opts {
		opt(&job)
	}
	require.NoError(t, dbc.DB.Create(&job).Error, "creating ProwJob %q", name)
	return job
}

func CreateProwJobRun(t *testing.T, dbc *db.DB, prowJobID uint, release string, timestamp time.Time, succeeded bool, overallResult v1.JobOverallResult) models.ProwJobRun {
	t.Helper()
	run := models.ProwJobRun{
		ProwJobID:      prowJobID,
		ProwJobRelease: release,
		Timestamp:      timestamp,
		Succeeded:      succeeded,
		Failed:         !succeeded,
		OverallResult:  overallResult,
	}
	require.NoError(t, dbc.DB.Create(&run).Error, "creating ProwJobRun")
	return run
}

func CreateTest(t *testing.T, dbc *db.DB, name string) models.Test {
	t.Helper()
	test := models.Test{Name: name}
	require.NoError(t, dbc.DB.Create(&test).Error, "creating Test %q", name)
	return test
}

func CreateSuite(t *testing.T, dbc *db.DB, name string) models.Suite {
	t.Helper()
	suite := models.Suite{Name: name}
	require.NoError(t, dbc.DB.Create(&suite).Error, "creating Suite %q", name)
	return suite
}

func CreateProwJobRunTest(t *testing.T, dbc *db.DB, prowJobRunID, prowJobID, testID uint, release string, timestamp time.Time, status int) models.ProwJobRunTest {
	t.Helper()
	pjrt := models.ProwJobRunTest{
		ProwJobRunID:        prowJobRunID,
		ProwJobID:           prowJobID,
		TestID:              testID,
		ProwJobRunRelease:   release,
		ProwJobRunTimestamp: timestamp,
		Status:              status,
	}
	require.NoError(t, dbc.DB.Create(&pjrt).Error, "creating ProwJobRunTest")
	return pjrt
}

func CreateBug(t *testing.T, dbc *db.DB, key, status, summary string, lastChangeTime time.Time, jobs []models.ProwJob) models.Bug {
	t.Helper()
	bug := models.Bug{
		Key:            key,
		Status:         status,
		Summary:        summary,
		LastChangeTime: lastChangeTime,
		Jobs:           jobs,
	}
	require.NoError(t, dbc.DB.Create(&bug).Error, "creating Bug %q", key)
	return bug
}

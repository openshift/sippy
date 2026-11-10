package testidentification

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJobVariantsQueryForDataset(t *testing.T) {
	query, err := jobVariantsQueryForDataset("openshift-gce-devel.ci_analysis_us")
	require.NoError(t, err)
	require.Contains(t, query, "`openshift-gce-devel.ci_analysis_us.job_variants`")
	require.NotContains(t, query, "$$TABLE$$")

	_, err = jobVariantsQueryForDataset("ci_analysis_us` WHERE true --")
	require.ErrorContains(t, err, "invalid BigQuery dataset")
}

package bigquery

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateDatasetIdentifier(t *testing.T) {
	for _, dataset := range []string{
		"ci_analysis_us",
		"openshift-gce-devel.ci_analysis_us",
		"project.dataset.region",
	} {
		t.Run("valid_"+dataset, func(t *testing.T) {
			require.NoError(t, ValidateDatasetIdentifier(dataset))
		})
	}

	for _, dataset := range []string{
		"",
		"ci analysis",
		"project.dataset`",
		"dataset;SELECT 1",
		"project.dataset --comment",
		"project.dataset.extra.fourth",
	} {
		t.Run("invalid_"+dataset, func(t *testing.T) {
			require.ErrorContains(t, ValidateDatasetIdentifier(dataset), "invalid BigQuery dataset")
		})
	}
}

package variantregistry

import (
	"testing"

	"github.com/openshift/sippy/pkg/testidentification"
	"github.com/stretchr/testify/assert"
)

func TestVariantSyncer(t *testing.T) {
	variantSyncer := OCPVariantLoader{VariantManager: testidentification.NewOpenshiftVariantManager()}
	tests := []struct {
		job      string
		expected []string
	}{
		{
			"periodic-ci-openshift-release-master-nightly-4.16-e2e-gcp-ovn-fips",
			[]string{"gcp", "amd64", "ovn", "ha", "fips"},
		},
	}
	for _, test := range tests {
		t.Run(test.job, func(t *testing.T) {
			assert.Equal(t, test.expected, variantSyncer.GetVariantsForJob(test.job))
		})
	}
}

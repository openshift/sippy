package prowloader

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTagExtractor_ExtractTags(t *testing.T) {
	tests := []struct {
		name         string
		testName     string
		testOutput   string
		expectedTags []string
	}{
		{
			name:     "upgrade alerts 1 firing duplicated text",
			testName: "Cluster upgrade.[sig-arch] Check if alerts are firing during or after upgrade success",
			testOutput: `{Nov  2 12:52:51.223: Unexpected alerts fired or pending during the upgrade:

alert ExtremelyHighIndividualControlPlaneCPU fired for 330 seconds with labels: {instance="ip-10-0-155-22.us-west-2.compute.internal", namespace="openshift-kube-apiserver", severity="warning"} Failure Nov  2 12:52:51.223: Unexpected alerts fired or pending during the upgrade:

alert ExtremelyHighIndividualControlPlaneCPU fired for 330 seconds with labels: {instance="ip-10-0-155-22.us-west-2.compute.internal", namespace="openshift-kube-apiserver", severity="warning"}

github.com/openshift/origin/test/extended/util/disruption.(*chaosMonkeyAdapter).Test(0xc0096b29b0, 0xc000661e18)
	github.com/openshift/origin/test/extended/util/disruption/disruption.go:197 +0x315
k8s.io/kubernetes/test/e2e/chaosmonkey.(*Chaosmonkey).Do.func1()
	k8s.io/kubernetes@v1.25.0/test/e2e/chaosmonkey/chaosmonkey.go:94 +0x6a
created by k8s.io/kubernetes/test/e2e/chaosmonkey.(*Chaosmonkey).Do
	k8s.io/kubernetes@v1.25.0/test/e2e/chaosmonkey/chaosmonkey.go:91 +0x88}`,
			expectedTags: []string{"ExtremelyHighIndividualControlPlaneCPU"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			extractor := TagExtractor{}
			resultTags := extractor.ExtractTags(tc.testName, tc.testOutput)
			assert.Equal(t, tc.expectedTags, resultTags)
		})
	}
}

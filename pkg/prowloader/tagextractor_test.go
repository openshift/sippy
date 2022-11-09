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
		expectedTags []map[string]string
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
			expectedTags: []map[string]string{
				{
					"alert":     "ExtremelyHighIndividualControlPlaneCPU",
					"state":     "fired",
					"namespace": "openshift-kube-apiserver",
				},
			},
		},
		{
			name:     "upgrade alerts 2 firing duplicated text",
			testName: "Cluster upgrade.[sig-arch] Check if alerts are firing during or after upgrade success",
			testOutput: `{Nov  9 06:45:48.894: Unexpected alerts fired or pending during the upgrade:

alert ClusterOperatorDegraded fired for 4288 seconds with labels: {name="network", namespace="openshift-cluster-version", reason="RolloutHung", severity="warning"}
alert TargetDown fired for 5887 seconds with labels: {job="ovnkube-node", namespace="openshift-ovn-kubernetes", service="ovn-kubernetes-node", severity="warning"} Failure Nov  9 06:45:48.894: Unexpected alerts fired or pending during the upgrade:

alert ClusterOperatorDegraded fired for 4288 seconds with labels: {name="network", namespace="openshift-cluster-version", reason="RolloutHung", severity="warning"}
alert TargetDown fired for 5887 seconds with labels: {job="ovnkube-node", namespace="openshift-ovn-kubernetes", service="ovn-kubernetes-node", severity="warning"}

github.com/openshift/origin/test/extended/util/disruption.(*chaosMonkeyAdapter).Test(0xc004840aa0, 0xc0007cd290)
	github.com/openshift/origin/test/extended/util/disruption/disruption.go:197 +0x315
k8s.io/kubernetes/test/e2e/chaosmonkey.(*Chaosmonkey).Do.func1()
	k8s.io/kubernetes@v1.25.0/test/e2e/chaosmonkey/chaosmonkey.go:94 +0x6a
created by k8s.io/kubernetes/test/e2e/chaosmonkey.(*Chaosmonkey).Do
	k8s.io/kubernetes@v1.25.0/test/e2e/chaosmonkey/chaosmonkey.go:91 +0x8b}`,
			expectedTags: []map[string]string{
				{
					"alert":     "ClusterOperatorDegraded",
					"state":     "fired",
					"namespace": "openshift-cluster-version",
				},
				{
					"alert":     "TargetDown",
					"state":     "fired",
					"namespace": "openshift-ovn-kubernetes",
				},
			},
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

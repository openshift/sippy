package prowloader

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFailureMetadataExtractor_ExtractMetadata(t *testing.T) {
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
		{
			name:         "upgrade alerts no match",
			testName:     "Cluster upgrade.[sig-arch] Check if alerts are firing during or after upgrade success",
			testOutput:   `gibberish that won't match anything`,
			expectedTags: []map[string]string{},
		},
		{
			name:     "conformance alerts 2 firing",
			testName: "[sig-instrumentation][Late] Alerts shouldn't report any unexpected alerts in firing or pending state [apigroup:config.openshift.io] [Suite:openshift/conformance/parallel]",
			testOutput: `{  fail [github.com/onsi/ginkgo@v4.7.0-origin.0+incompatible/internal/leafnodes/runner.go:113]: Nov  9 12:38:34.177: Unexpected alerts fired or pending after the test run:

alert ClusterOperatorDown fired for 287 seconds with labels: {name="insights", namespace="openshift-cluster-version", severity="critical"}
alert OperatorHubSourceError fired for 726 seconds with labels: {container="catalog-operator", endpoint="https-metrics", exported_namespace="openshift-marketplace", instance="10.129.0.18:8443", job="catalog-operator-metrics", name="redhat-operators", namespace="openshift-operator-lifecycle-manager", pod="catalog-operator-5988994647-wlfx8", service="catalog-operator-metrics", severity="warning"}
Ginkgo exit error 1: exit with code 1}`,
			expectedTags: []map[string]string{
				{
					"alert":     "ClusterOperatorDown",
					"state":     "fired",
					"namespace": "openshift-cluster-version",
				},
				{
					"alert":     "OperatorHubSourceError",
					"state":     "fired",
					"namespace": "openshift-operator-lifecycle-manager",
				},
			},
		},
		{
			name:     "conformance alerts firing twice in different namespaces",
			testName: "[sig-instrumentation][Late] Alerts shouldn't report any unexpected alerts in firing or pending state [apigroup:config.openshift.io] [Suite:openshift/conformance/parallel]",
			testOutput: `{  fail [github.com/onsi/ginkgo/v2@v2.1.5-0.20220909190140-b488ab12695a/internal/suite.go:612]: Nov  9 09:59:59.990: Unexpected alerts fired or pending after the test run:

alert TargetDown fired for 2632 seconds with labels: {job="metrics", namespace="openshift-cluster-samples-operator", service="metrics", severity="warning"}
alert TargetDown fired for 2662 seconds with labels: {job="machine-api-controllers", namespace="openshift-machine-api", service="machine-api-controllers", severity="warning"}
Ginkgo exit error 1: exit with code 1}`,
			expectedTags: []map[string]string{
				{
					"alert":     "TargetDown",
					"state":     "fired",
					"namespace": "openshift-cluster-samples-operator",
				},
				{
					"alert":     "TargetDown",
					"state":     "fired",
					"namespace": "openshift-machine-api",
				},
			},
		},
		{
			name:     "conformance alerts firing no namespace",
			testName: "[sig-instrumentation][Late] Alerts shouldn't report any unexpected alerts in firing or pending state [apigroup:config.openshift.io] [Suite:openshift/conformance/parallel]",
			testOutput: `{  fail [github.com/onsi/ginkgo/v2@v2.1.5-0.20220909190140-b488ab12695a/internal/suite.go:612]: Nov  9 09:59:59.990: Unexpected alerts fired or pending after the test run:

alert TargetDown fired for 2632 seconds with labels: {job="metrics", service="metrics", severity="warning"}
alert TargetDown fired for 2662 seconds with labels: {job="machine-api-controllers", namespace="openshift-machine-api", service="machine-api-controllers", severity="warning"}
Ginkgo exit error 1: exit with code 1}`,
			expectedTags: []map[string]string{
				{
					"alert": "TargetDown",
					"state": "fired",
				},
				{
					"alert":     "TargetDown",
					"state":     "fired",
					"namespace": "openshift-machine-api",
				},
			},
		},
		{
			name:     "pathological events single",
			testName: "[sig-arch] events should not repeat pathologically",
			testOutput: `{  1 events happened too frequently

event happened 31 times, something is wrong: ns/openshift-console pod/console-7f577bcddd-t8qg4 node/ci-op-n960xqmi-7f65d-2wn89-master-0 - reason/ProbeError Readiness probe error: Get "https://10.130.0.49:8443/health": dial tcp 10.130.0.49:8443: connect: connection refused
body: 
}`,
			expectedTags: []map[string]string{
				{
					"reason": "ProbeError",
					"ns":     "openshift-console",
				},
			},
		},
		{
			// It would in the real world but I want to make sure the regex handles it
			name:     "pathological events single without ns",
			testName: "[sig-arch] events should not repeat pathologically",
			testOutput: `{  1 events happened too frequently

event happened 31 times, something is wrong: pod/console-7f577bcddd-t8qg4 node/ci-op-n960xqmi-7f65d-2wn89-master-0 - reason/ProbeError Readiness probe error: Get "https://10.130.0.49:8443/health": dial tcp 10.130.0.49:8443: connect: connection refused
body: 
}`,
			expectedTags: []map[string]string{
				{
					"reason": "ProbeError",
				},
			},
		},
		{
			name:     "pathological events many hits",
			testName: "[sig-arch] events should not repeat pathologically",
			testOutput: `{  12 events happened too frequently

event happened 33 times, something is wrong: node/gfq9r5kv-c805c-b7tdc-master-2 - reason/NodeHasSufficientPID roles/control-plane,master Node gfq9r5kv-c805c-b7tdc-master-2 status is now: NodeHasSufficientPID
event happened 31 times, something is wrong: node/gfq9r5kv-c805c-b7tdc-master-0 - reason/NodeHasSufficientMemory roles/control-plane,master Node gfq9r5kv-c805c-b7tdc-master-0 status is now: NodeHasSufficientMemory
event happened 26 times, something is wrong: node/gfq9r5kv-c805c-b7tdc-master-0-clone - reason/NodeHasNoDiskPressure roles/control-plane,master Node gfq9r5kv-c805c-b7tdc-master-0-clone status is now: NodeHasNoDiskPressure
event happened 24 times, something is wrong: ns/openshift-machine-api machine/gfq9r5kv-c805c-b7tdc-worker-0-wp9g8 - reason/Reconciled Reconciled machine gfq9r5kv-c805c-b7tdc-worker-0-wp9g8
event happened 25 times, something is wrong: ns/openshift-machine-api machine/gfq9r5kv-c805c-b7tdc-worker-0-q2krf - reason/Reconciled Reconciled machine gfq9r5kv-c805c-b7tdc-worker-0-q2krf
event happened 33 times, something is wrong: node/gfq9r5kv-c805c-b7tdc-master-2 - reason/NodeHasNoDiskPressure roles/control-plane,master Node gfq9r5kv-c805c-b7tdc-master-2 status is now: NodeHasNoDiskPressure
event happened 33 times, something is wrong: node/gfq9r5kv-c805c-b7tdc-master-2 - reason/NodeHasSufficientMemory roles/control-plane,master Node gfq9r5kv-c805c-b7tdc-master-2 status is now: NodeHasSufficientMemory
event happened 22 times, something is wrong: ns/openshift-machine-api machine/gfq9r5kv-c805c-b7tdc-worker-0-sbxp6 - reason/Reconciled Reconciled machine gfq9r5kv-c805c-b7tdc-worker-0-sbxp6
event happened 31 times, something is wrong: node/gfq9r5kv-c805c-b7tdc-master-0 - reason/NodeHasNoDiskPressure roles/control-plane,master Node gfq9r5kv-c805c-b7tdc-master-0 status is now: NodeHasNoDiskPressure
event happened 31 times, something is wrong: node/gfq9r5kv-c805c-b7tdc-master-0 - reason/NodeHasSufficientPID roles/control-plane,master Node gfq9r5kv-c805c-b7tdc-master-0 status is now: NodeHasSufficientPID
event happened 26 times, something is wrong: node/gfq9r5kv-c805c-b7tdc-master-0-clone - reason/NodeHasSufficientMemory roles/control-plane,master Node gfq9r5kv-c805c-b7tdc-master-0-clone status is now: NodeHasSufficientMemory}`,
			expectedTags: []map[string]string{
				{
					"reason": "NodeHasSufficientPID",
				},
				{
					"reason": "NodeHasSufficientMemory",
				},
				{
					"reason": "NodeHasNoDiskPressure",
				},
				{
					"reason": "Reconciled",
					"ns":     "openshift-machine-api",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			extractor := TestFailureMetadataExtractor{}
			resultTags := extractor.ExtractMetadata(tc.testName, tc.testOutput)
			assert.Equal(t, tc.expectedTags, resultTags)
		})
	}
}

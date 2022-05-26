package junit

import (
	"encoding/xml"
	"testing"
)

const junitXML = `<testsuites>
<testsuite tests="14" failures="0" time="2532.710000" name="">
<properties>
<property name="go.version" value="go1.17.5 linux/amd64"/>
</properties>
<testcase classname="" name="TestUpgradeControlPlane/EnsureNodeCountMatchesNodePoolReplicas" time="0.030000"/>
<testcase classname="" name="TestUpgradeControlPlane/EnsureNoCrashingPods" time="0.030000"/>
<testcase classname="" name="TestUpgradeControlPlane/EnsureHCPContainersHaveResourceRequests" time="0.040000"/>
<testcase classname="" name="TestUpgradeControlPlane/PreTeardownClusterDump" time="105.690000"/>
<testcase classname="" name="TestUpgradeControlPlane/DestroyCluster_1" time="434.970000"/>
<testcase classname="" name="TestUpgradeControlPlane/EnsureAPIBudget/control-plane-operator_read" time="0.030000"/>
<testcase classname="" name="TestUpgradeControlPlane/EnsureAPIBudget/control-plane-operator_mutate" time="0.020000"/>
<testcase classname="" name="TestUpgradeControlPlane/EnsureAPIBudget/control-plane-operator_no_404_deletes" time="0.000000"/>
<testcase classname="" name="TestUpgradeControlPlane/EnsureAPIBudget/hypershift-operator_read" time="0.000000"/>
<testcase classname="" name="TestUpgradeControlPlane/EnsureAPIBudget/hypershift-operator_mutate" time="0.000000"/>
<testcase classname="" name="TestUpgradeControlPlane/EnsureAPIBudget/hypershift-operator_no_404_deletes" time="0.000000"/>
<testcase classname="" name="TestUpgradeControlPlane/EnsureAPIBudget" time="0.100000"/>
<testcase classname="" name="TestUpgradeControlPlane/DeleteTestNamespace" time="0.010000"/>
<testcase classname="" name="TestUpgradeControlPlane" time="1991.790000"/>
</testsuite>
</testsuites>`

func Test_CanUnmarshalJunit(t *testing.T) {
	suites := &TestSuites{}
	if err := xml.Unmarshal([]byte(junitXML), suites); err != nil {
		t.Fatalf("could not unmarshal: %s", err.Error())
	}

}

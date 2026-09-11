package api

import (
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/sippy/pkg/testidentification"
)

// lifecycleTestSelection is the set of testcase names, prefixes, and substrings the install,
// upgrade, and release health endpoints use to select install/upgrade test results for a release.
type lifecycleTestSelection struct {
	InstallExactNames sets.Set[string]
	InstallPrefixes   sets.Set[string]

	UpgradeExactNames sets.Set[string]
	UpgradePrefixes   sets.Set[string]
	UpgradeSubstrings sets.Set[string]

	HealthInstallTestName string
	HealthUpgradeTestName string
	HealthInfraTestName   string
}

// useNewInstallTest decides which install test name to use based on releases. For
// release 4.11 and above, it uses the new install test names
func useNewInstallTest(release string) bool {
	digits := strings.Split(release, ".")
	if len(digits) < 2 {
		return false
	}
	major, err := strconv.Atoi(digits[0])
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(digits[1])
	if err != nil {
		return false
	}
	if major < 4 {
		return false
	} else if major == 4 && minor < 11 {
		return false
	}
	return true
}

// lifecycleProduct returns the product name for a synthetic "<product>-<version>" release, e.g.
// "acm-2.14" (-> "acm"), or "" for a bare OpenShift release like "4.21" or "4.10" that has no
// "-" separator, or for an OpenShift release variant like "4.21-okd" or "5.0-okd" whose text
// before the "-" is itself a release number rather than a product name.
func lifecycleProduct(release string) string {
	product, _, found := strings.Cut(release, "-")
	if !found {
		return ""
	}
	major, _, _ := strings.Cut(product, ".")
	if _, err := strconv.Atoi(major); err == nil {
		return ""
	}
	return product
}

// lifecycleTestsForRelease returns the testcase selection used by the install, upgrade, and
// release health endpoints for the given release. Synthetic "<product>-<version>" releases select
// the "[sig-<product>] install/upgrade should succeed" testcases emitted by that product's
// deploy/upgrade CI steps instead of the OpenShift install/upgrade tests.
func lifecycleTestsForRelease(release string) lifecycleTestSelection {
	if product := lifecycleProduct(release); product != "" {
		sig := "[sig-" + product + "] "
		return lifecycleTestSelection{
			InstallExactNames: sets.New(sig + testidentification.LifecycleInstallTestName),
			InstallPrefixes:   sets.New[string](),

			UpgradeExactNames: sets.New(sig + testidentification.LifecycleUpgradeTestName),
			UpgradePrefixes:   sets.New[string](),
			UpgradeSubstrings: sets.New[string](),

			HealthInstallTestName: sig + testidentification.LifecycleInstallTestName,
			HealthUpgradeTestName: sig + testidentification.LifecycleUpgradeTestName,
			HealthInfraTestName:   testidentification.InfrastructureTestName,
		}
	}

	installExactNames := sets.New[string]()
	installPrefixes := sets.New(testidentification.OperatorInstallPrefix)
	healthInstallTestName := testidentification.InstallTestName
	infraTestName := testidentification.InfrastructureTestName
	if useNewInstallTest(release) {
		installPrefixes.Insert(testidentification.InstallTestNamePrefix)
		healthInstallTestName = testidentification.NewInstallTestName
		infraTestName = testidentification.NewInfrastructureTestName
	} else {
		installExactNames.Insert(testidentification.InstallTestName)
	}

	return lifecycleTestSelection{
		InstallExactNames: installExactNames,
		InstallPrefixes:   installPrefixes,

		UpgradeExactNames: sets.New(testidentification.UpgradeTestName),
		UpgradePrefixes:   sets.New(testidentification.OperatorUpgradePrefix),
		UpgradeSubstrings: sets.New(
			testidentification.OperatorsUpgradedTest,
			testidentification.APIsRemainAvailTest,
			testidentification.MachineConfigsUpgradedTest,
			testidentification.CVOAcknowledgesUpgradeTest,
		),

		HealthInstallTestName: healthInstallTestName,
		HealthUpgradeTestName: testidentification.UpgradeTestName,
		HealthInfraTestName:   infraTestName,
	}
}

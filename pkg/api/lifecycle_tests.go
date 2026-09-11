package api

import (
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/sippy/pkg/testidentification"
)

// lifecycleTestSelection is the set of testcase names, prefixes, and substrings the install,
// upgrade, and release health endpoints use to select install/upgrade test results for a release.
// The selection is additive: OpenShift install/upgrade rows are always present. For a product
// release, ProductInstallTestName and ProductUpgradeTestName are also set (and already folded
// into the Install/Upgrade exact-name sets above) so the release health endpoint can report them
// as separate indicators; they are "" for a non-product release.
type lifecycleTestSelection struct {
	InstallExactNames sets.Set[string]
	InstallPrefixes   sets.Set[string]

	UpgradeExactNames sets.Set[string]
	UpgradePrefixes   sets.Set[string]
	UpgradeSubstrings sets.Set[string]

	HealthInstallTestName string
	HealthUpgradeTestName string
	HealthInfraTestName   string

	ProductInstallTestName string
	ProductUpgradeTestName string
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

// lifecycleProduct returns the product name for a synthetic "<product>-<version>" release whose
// product is in testidentification.LifecycleProducts, e.g. "quay-3.18" (-> "quay"). It returns ""
// for a bare OpenShift release like "4.21" or "4.10" that has no "-" separator, or for any release
// whose product is not opted into the lifecycle allowlist.
func lifecycleProduct(release string) string {
	product, _, found := strings.Cut(release, "-")
	if !found || !testidentification.LifecycleProducts.Has(product) {
		return ""
	}
	return product
}

// openshiftLifecycleTests returns the OpenShift install/upgrade testcase selection. useNew
// selects the modern install testcase names used from release 4.11 onward; false selects the
// legacy names used by earlier releases.
func openshiftLifecycleTests(useNew bool) lifecycleTestSelection {
	installExactNames := sets.New[string]()
	installPrefixes := sets.New(testidentification.OperatorInstallPrefix)
	healthInstallTestName := testidentification.InstallTestName
	infraTestName := testidentification.InfrastructureTestName
	if useNew {
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

// lifecycleTestsForRelease returns the testcase selection used by the install, upgrade, and
// release health endpoints for the given release. The selection is additive: OpenShift rows are
// always present. Synthetic "<product>-<version>" releases whose product is in
// testidentification.LifecycleProducts also append the "[sig-<product>] install/upgrade should
// succeed" testcases emitted by that product's deploy/upgrade CI steps alongside the OpenShift
// rows. Product CI runs on modern OpenShift hosts, and useNewInstallTest cannot parse a synthetic
// release like "quay-3.18", so product releases always use the modern OpenShift selection.
func lifecycleTestsForRelease(release string) lifecycleTestSelection {
	product := lifecycleProduct(release)
	if product == "" {
		return openshiftLifecycleTests(useNewInstallTest(release))
	}

	selection := openshiftLifecycleTests(true)

	sig := "[sig-" + product + "] "
	productInstallTestName := sig + testidentification.LifecycleInstallTestName
	productUpgradeTestName := sig + testidentification.LifecycleUpgradeTestName

	selection.InstallExactNames.Insert(productInstallTestName)
	selection.UpgradeExactNames.Insert(productUpgradeTestName)
	selection.ProductInstallTestName = productInstallTestName
	selection.ProductUpgradeTestName = productUpgradeTestName

	return selection
}

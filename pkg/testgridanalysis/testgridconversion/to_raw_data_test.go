package testgridconversion

import (
	"strings"
	"testing"

	testgridv1 "github.com/openshift/sippy/pkg/apis/testgrid/v1"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
	"github.com/stretchr/testify/assert"
)

func TestProcessJobDetails(t *testing.T) {
	// need to generate JobDetails with a mix of expected random and non random test names
	// validate the RawJobResult has the corrected names

	testNames := []string{
		"Operator results test operator install install_operatorname",
		testgridanalysisapi.OperatorUpgradePrefix + "upgrade_operatorname",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-nbqyx.Installing \"Red Hat Integration - 3scale\" operator in test-nbqyx Installs Red Hat Integration - 3scale operator in test-nbqyx and creates 3scale Backend Schema operand instance\"",
		"This test name should not be modified",
	}

	validationStrings := []string{
		"Operator results.operator conditions  install_operatorname",
		"Operator results.operator conditions  upgrade_operatorname",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-random.Installing \"Red Hat Integration - 3scale\" operator in test-random Installs Red Hat Integration - 3scale operator in test-random and creates 3scale Backend Schema operand instance\"",
		"This test name should not be modified",
	}

	result := processJobDetails(buildFakeJobDetails(testNames), 0, 1)

	assert.NotNil(t, result, "Nil response from processJobDetails")

	// check the keys of the map and validate they match our expectations
	assert.Equal(t, len(result.TestResults), len(testNames), "Unexpected test resulsts size %d", len(result.TestResults))

	for _, s := range validationStrings {
		assert.NotNil(t, result.TestResults[s], "Expected non nil test result for %s", s)
	}

}

func TestRegexExplicit(t *testing.T) {

	// explicit test cases and verifications
	testName1 := "\"Installing \"Red Hat Integration - 3scale\" operator in test-ieesa.Installing \"Red Hat Integration - 3scale\" operator in test-ieesa Installs Red Hat Integration - 3scale operator in test-ieesa and creates 3scale Backend Schema operand instance\""
	testName1Clean := "\"Installing \"Red Hat Integration - 3scale\" operator in test-random.Installing \"Red Hat Integration - 3scale\" operator in test-random Installs Red Hat Integration - 3scale operator in test-random and creates 3scale Backend Schema operand instance\""

	testName2 := "\"Installing \"Red Hat Integration - 3scale\" operator in test-jopkv.Installing \"Red Hat Integration - 3scale\" operator in test-jopkv \"after all\" hook for \"Installs Red Hat Integration - 3scale operator in test-jopkv and creates 3scale Backend Schema operand i (...)\""
	testName2Clean := "\"Installing \"Red Hat Integration - 3scale\" operator in test-random.Installing \"Red Hat Integration - 3scale\" operator in test-random \"after all\" hook for \"Installs Red Hat Integration - 3scale operator in test-random and creates 3scale Backend Schema operand i (...)\""

	testName1 = cleanTestName(testName1)
	assert.Equal(t, testName1, testName1Clean, "TestName1Clean did not match, %s", testName1)

	testName2 = cleanTestName(testName2)
	assert.Equal(t, testName2, testName2Clean, "TestName2Clean did not match, %s", testName2)
}

func TestRegexGeneric(t *testing.T) {

	// if we change the input data we might have to reconsider the hardcoded fixCount validation
	// for the example data we are working with when there is a random entry to be replaced
	// it shows up in 3 locations, this is verifying all 3 are getting updated
	expectedRandomReplacements := 3

	// get our list of tests
	testStrings := buildTestNameSamples()

	// get the count for the names we expect will get changes
	validateCount := len(testStrings)

	// add in the ones we don't expect to get changed after we have the count of original ones
	// intentional non matching replace with no trailing a-z after test-
	nonMatchString := "\"Installing \"Red Hat Integration - 3scale\" operator in test-.Installing \"Red Hat Integration - 3scale\" operator in test- Installs Red Hat Integration - 3scale operator in test- and creates 3scale Backend Schema operand instance\""
	testStrings = append(testStrings, nonMatchString)

	// will not trigger the first match for the starts with
	skipString := "\"Doesn'tStartWith Installing \"Red Hat Integration - 3scale\" operator in test-.Installing \"Red Hat Integration - 3scale\" operator in test- Installs Red Hat Integration - 3scale operator in test- and creates 3scale Backend Schema operand instance\""
	testStrings = append(testStrings, skipString)

	fixedNames := 0

	for _, s := range testStrings {

		sCleaned := cleanTestName(s)
		fixCount := strings.Count(sCleaned, matchRandomReplace)

		if s == nonMatchString {
			assert.Equal(t, 0, fixCount, "Expected fix count of 0 but received %d", fixCount)

		} else if sCleaned != skipString {
			assert.Equal(t, fixCount, expectedRandomReplacements, "Unexpected count for fixedName: %d for value: %s", fixCount, s)
		}

		if s != sCleaned {
			fixedNames++
		}
	}

	assert.Equal(t, fixedNames, validateCount, "Expected fixedNames count to be %d, but was %d", validateCount, fixedNames)
}

func buildFakeJobDetails(testNames []string) testgridv1.JobDetails {

	status1 := testgridv1.TestResult{
		Count: 1,
		Value: testgridv1.TestStatusFailure,
	}

	statuses := []testgridv1.TestResult{status1}
	tests := []testgridv1.Test{}

	for _, s := range testNames {

		test := testgridv1.Test{
			Name:     s,
			Statuses: statuses,
		}

		tests = append(tests, test)

	}

	jobDetails := &testgridv1.JobDetails{
		Name:        "mockName",
		Tests:       tests,
		Query:       "mockQuery",
		ChangeLists: []string{"mockChange"},
		Timestamps:  []int{1},
	}

	return *jobDetails
}

func buildTestNameSamples() []string {

	//pulled from sippydb
	testStrings := []string{
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-nbqyx.Installing \"Red Hat Integration - 3scale\" operator in test-nbqyx Installs Red Hat Integration - 3scale operator in test-nbqyx and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-xthdq.Installing \"Red Hat Integration - 3scale\" operator in test-xthdq Installs Red Hat Integration - 3scale operator in test-xthdq and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-ccfbb.Installing \"Red Hat Integration - 3scale\" operator in test-ccfbb Installs Red Hat Integration - 3scale operator in test-ccfbb and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-hisxw.Installing \"Red Hat Integration - 3scale\" operator in test-hisxw Installs Red Hat Integration - 3scale operator in test-hisxw and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-xidod.Installing \"Red Hat Integration - 3scale\" operator in test-xidod Installs Red Hat Integration - 3scale operator in test-xidod and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-cumkj.Installing \"Red Hat Integration - 3scale\" operator in test-cumkj Installs Red Hat Integration - 3scale operator in test-cumkj and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-cvpdv.Installing \"Red Hat Integration - 3scale\" operator in test-cvpdv Installs Red Hat Integration - 3scale operator in test-cvpdv and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-paogs.Installing \"Red Hat Integration - 3scale\" operator in test-paogs Installs Red Hat Integration - 3scale operator in test-paogs and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-iuiwr.Installing \"Red Hat Integration - 3scale\" operator in test-iuiwr Installs Red Hat Integration - 3scale operator in test-iuiwr and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-xxpcx.Installing \"Red Hat Integration - 3scale\" operator in test-xxpcx Installs Red Hat Integration - 3scale operator in test-xxpcx and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-erhuj.Installing \"Red Hat Integration - 3scale\" operator in test-erhuj Installs Red Hat Integration - 3scale operator in test-erhuj and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-hqofb.Installing \"Red Hat Integration - 3scale\" operator in test-hqofb Installs Red Hat Integration - 3scale operator in test-hqofb and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-oyexl.Installing \"Red Hat Integration - 3scale\" operator in test-oyexl Installs Red Hat Integration - 3scale operator in test-oyexl and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-clmvy.Installing \"Red Hat Integration - 3scale\" operator in test-clmvy Installs Red Hat Integration - 3scale operator in test-clmvy and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-yhfho.Installing \"Red Hat Integration - 3scale\" operator in test-yhfho Installs Red Hat Integration - 3scale operator in test-yhfho and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-scjqx.Installing \"Red Hat Integration - 3scale\" operator in test-scjqx Installs Red Hat Integration - 3scale operator in test-scjqx and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-fjwdk.Installing \"Red Hat Integration - 3scale\" operator in test-fjwdk Installs Red Hat Integration - 3scale operator in test-fjwdk and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-nbczq.Installing \"Red Hat Integration - 3scale\" operator in test-nbczq Installs Red Hat Integration - 3scale operator in test-nbczq and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-ygckj.Installing \"Red Hat Integration - 3scale\" operator in test-ygckj Installs Red Hat Integration - 3scale operator in test-ygckj and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-kcotw.Installing \"Red Hat Integration - 3scale\" operator in test-kcotw Installs Red Hat Integration - 3scale operator in test-kcotw and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-hbuiy.Installing \"Red Hat Integration - 3scale\" operator in test-hbuiy Installs Red Hat Integration - 3scale operator in test-hbuiy and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-occcv.Installing \"Red Hat Integration - 3scale\" operator in test-occcv Installs Red Hat Integration - 3scale operator in test-occcv and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-czrzs.Installing \"Red Hat Integration - 3scale\" operator in test-czrzs Installs Red Hat Integration - 3scale operator in test-czrzs and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-xsqye.Installing \"Red Hat Integration - 3scale\" operator in test-xsqye Installs Red Hat Integration - 3scale operator in test-xsqye and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-uvzro.Installing \"Red Hat Integration - 3scale\" operator in test-uvzro Installs Red Hat Integration - 3scale operator in test-uvzro and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-ccruk.Installing \"Red Hat Integration - 3scale\" operator in test-ccruk Installs Red Hat Integration - 3scale operator in test-ccruk and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-padvn.Installing \"Red Hat Integration - 3scale\" operator in test-padvn Installs Red Hat Integration - 3scale operator in test-padvn and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-ystpu.Installing \"Red Hat Integration - 3scale\" operator in test-ystpu Installs Red Hat Integration - 3scale operator in test-ystpu and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-zhbag.Installing \"Red Hat Integration - 3scale\" operator in test-zhbag Installs Red Hat Integration - 3scale operator in test-zhbag and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-wfxdh.Installing \"Red Hat Integration - 3scale\" operator in test-wfxdh Installs Red Hat Integration - 3scale operator in test-wfxdh and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-jubpn.Installing \"Red Hat Integration - 3scale\" operator in test-jubpn Installs Red Hat Integration - 3scale operator in test-jubpn and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-lxqfk.Installing \"Red Hat Integration - 3scale\" operator in test-lxqfk Installs Red Hat Integration - 3scale operator in test-lxqfk and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-jjngj.Installing \"Red Hat Integration - 3scale\" operator in test-jjngj Installs Red Hat Integration - 3scale operator in test-jjngj and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-xdnrt.Installing \"Red Hat Integration - 3scale\" operator in test-xdnrt Installs Red Hat Integration - 3scale operator in test-xdnrt and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-yqgfh.Installing \"Red Hat Integration - 3scale\" operator in test-yqgfh Installs Red Hat Integration - 3scale operator in test-yqgfh and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-imgqn.Installing \"Red Hat Integration - 3scale\" operator in test-imgqn Installs Red Hat Integration - 3scale operator in test-imgqn and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-pnnbb.Installing \"Red Hat Integration - 3scale\" operator in test-pnnbb Installs Red Hat Integration - 3scale operator in test-pnnbb and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-kaydg.Installing \"Red Hat Integration - 3scale\" operator in test-kaydg Installs Red Hat Integration - 3scale operator in test-kaydg and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-qtwho.Installing \"Red Hat Integration - 3scale\" operator in test-qtwho Installs Red Hat Integration - 3scale operator in test-qtwho and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-rtjdf.Installing \"Red Hat Integration - 3scale\" operator in test-rtjdf Installs Red Hat Integration - 3scale operator in test-rtjdf and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-lqskk.Installing \"Red Hat Integration - 3scale\" operator in test-lqskk Installs Red Hat Integration - 3scale operator in test-lqskk and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-mbgbv.Installing \"Red Hat Integration - 3scale\" operator in test-mbgbv Installs Red Hat Integration - 3scale operator in test-mbgbv and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-isoct.Installing \"Red Hat Integration - 3scale\" operator in test-isoct Installs Red Hat Integration - 3scale operator in test-isoct and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-gxqfx.Installing \"Red Hat Integration - 3scale\" operator in test-gxqfx Installs Red Hat Integration - 3scale operator in test-gxqfx and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-xonvn.Installing \"Red Hat Integration - 3scale\" operator in test-xonvn Installs Red Hat Integration - 3scale operator in test-xonvn and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-kbfgm.Installing \"Red Hat Integration - 3scale\" operator in test-kbfgm Installs Red Hat Integration - 3scale operator in test-kbfgm and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-pcrlo.Installing \"Red Hat Integration - 3scale\" operator in test-pcrlo Installs Red Hat Integration - 3scale operator in test-pcrlo and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-gaffb.Installing \"Red Hat Integration - 3scale\" operator in test-gaffb Installs Red Hat Integration - 3scale operator in test-gaffb and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-grrte.Installing \"Red Hat Integration - 3scale\" operator in test-grrte Installs Red Hat Integration - 3scale operator in test-grrte and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-asbpf.Installing \"Red Hat Integration - 3scale\" operator in test-asbpf Installs Red Hat Integration - 3scale operator in test-asbpf and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-uskeg.Installing \"Red Hat Integration - 3scale\" operator in test-uskeg Installs Red Hat Integration - 3scale operator in test-uskeg and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-yiaum.Installing \"Red Hat Integration - 3scale\" operator in test-yiaum Installs Red Hat Integration - 3scale operator in test-yiaum and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-tgold.Installing \"Red Hat Integration - 3scale\" operator in test-tgold Installs Red Hat Integration - 3scale operator in test-tgold and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-ljbup.Installing \"Red Hat Integration - 3scale\" operator in test-ljbup Installs Red Hat Integration - 3scale operator in test-ljbup and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-gaduh.Installing \"Red Hat Integration - 3scale\" operator in test-gaduh Installs Red Hat Integration - 3scale operator in test-gaduh and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-yxhky.Installing \"Red Hat Integration - 3scale\" operator in test-yxhky Installs Red Hat Integration - 3scale operator in test-yxhky and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-lyoti.Installing \"Red Hat Integration - 3scale\" operator in test-lyoti Installs Red Hat Integration - 3scale operator in test-lyoti and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-urbrd.Installing \"Red Hat Integration - 3scale\" operator in test-urbrd Installs Red Hat Integration - 3scale operator in test-urbrd and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-mmlcu.Installing \"Red Hat Integration - 3scale\" operator in test-mmlcu Installs Red Hat Integration - 3scale operator in test-mmlcu and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-mycjh.Installing \"Red Hat Integration - 3scale\" operator in test-mycjh Installs Red Hat Integration - 3scale operator in test-mycjh and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-wjjdh.Installing \"Red Hat Integration - 3scale\" operator in test-wjjdh Installs Red Hat Integration - 3scale operator in test-wjjdh and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-nsyin.Installing \"Red Hat Integration - 3scale\" operator in test-nsyin Installs Red Hat Integration - 3scale operator in test-nsyin and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-piiov.Installing \"Red Hat Integration - 3scale\" operator in test-piiov \"after all\" hook for \"Installs Red Hat Integration - 3scale operator in test-piiov and creates 3scale Backend Schema operand i (...)\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-piiov.Installing \"Red Hat Integration - 3scale\" operator in test-piiov Installs Red Hat Integration - 3scale operator in test-piiov and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-bbubq.Installing \"Red Hat Integration - 3scale\" operator in test-bbubq Installs Red Hat Integration - 3scale operator in test-bbubq and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-tzyar.Installing \"Red Hat Integration - 3scale\" operator in test-tzyar Installs Red Hat Integration - 3scale operator in test-tzyar and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-gclbt.Installing \"Red Hat Integration - 3scale\" operator in test-gclbt Installs Red Hat Integration - 3scale operator in test-gclbt and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-brnry.Installing \"Red Hat Integration - 3scale\" operator in test-brnry Installs Red Hat Integration - 3scale operator in test-brnry and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-jyuty.Installing \"Red Hat Integration - 3scale\" operator in test-jyuty Installs Red Hat Integration - 3scale operator in test-jyuty and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-rvkfa.Installing \"Red Hat Integration - 3scale\" operator in test-rvkfa \"after all\" hook for \"Installs Red Hat Integration - 3scale operator in test-rvkfa and creates 3scale Backend Schema operand i (...)\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-rvkfa.Installing \"Red Hat Integration - 3scale\" operator in test-rvkfa Installs Red Hat Integration - 3scale operator in test-rvkfa and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-zimgv.Installing \"Red Hat Integration - 3scale\" operator in test-zimgv Installs Red Hat Integration - 3scale operator in test-zimgv and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-kqdll.Installing \"Red Hat Integration - 3scale\" operator in test-kqdll Installs Red Hat Integration - 3scale operator in test-kqdll and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-djqhj.Installing \"Red Hat Integration - 3scale\" operator in test-djqhj Installs Red Hat Integration - 3scale operator in test-djqhj and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-muoas.Installing \"Red Hat Integration - 3scale\" operator in test-muoas Installs Red Hat Integration - 3scale operator in test-muoas and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-sbgqb.Installing \"Red Hat Integration - 3scale\" operator in test-sbgqb Installs Red Hat Integration - 3scale operator in test-sbgqb and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-tgtkz.Installing \"Red Hat Integration - 3scale\" operator in test-tgtkz Installs Red Hat Integration - 3scale operator in test-tgtkz and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-hodyk.Installing \"Red Hat Integration - 3scale\" operator in test-hodyk Installs Red Hat Integration - 3scale operator in test-hodyk and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-hfjgy.Installing \"Red Hat Integration - 3scale\" operator in test-hfjgy Installs Red Hat Integration - 3scale operator in test-hfjgy and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-halvk.Installing \"Red Hat Integration - 3scale\" operator in test-halvk Installs Red Hat Integration - 3scale operator in test-halvk and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-meozv.Installing \"Red Hat Integration - 3scale\" operator in test-meozv Installs Red Hat Integration - 3scale operator in test-meozv and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-wxifz.Installing \"Red Hat Integration - 3scale\" operator in test-wxifz Installs Red Hat Integration - 3scale operator in test-wxifz and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-krnjn.Installing \"Red Hat Integration - 3scale\" operator in test-krnjn \"after all\" hook for \"Installs Red Hat Integration - 3scale operator in test-krnjn and creates 3scale Backend Schema operand i (...)\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-krnjn.Installing \"Red Hat Integration - 3scale\" operator in test-krnjn Installs Red Hat Integration - 3scale operator in test-krnjn and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-cvymk.Installing \"Red Hat Integration - 3scale\" operator in test-cvymk Installs Red Hat Integration - 3scale operator in test-cvymk and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-ladol.Installing \"Red Hat Integration - 3scale\" operator in test-ladol Installs Red Hat Integration - 3scale operator in test-ladol and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-hfxlh.Installing \"Red Hat Integration - 3scale\" operator in test-hfxlh Installs Red Hat Integration - 3scale operator in test-hfxlh and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-mwugb.Installing \"Red Hat Integration - 3scale\" operator in test-mwugb Installs Red Hat Integration - 3scale operator in test-mwugb and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-jqrna.Installing \"Red Hat Integration - 3scale\" operator in test-jqrna Installs Red Hat Integration - 3scale operator in test-jqrna and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-qwbjb.Installing \"Red Hat Integration - 3scale\" operator in test-qwbjb Installs Red Hat Integration - 3scale operator in test-qwbjb and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-ljemb.Installing \"Red Hat Integration - 3scale\" operator in test-ljemb Installs Red Hat Integration - 3scale operator in test-ljemb and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-zaooq.Installing \"Red Hat Integration - 3scale\" operator in test-zaooq Installs Red Hat Integration - 3scale operator in test-zaooq and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-ayngh.Installing \"Red Hat Integration - 3scale\" operator in test-ayngh Installs Red Hat Integration - 3scale operator in test-ayngh and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-hgwnj.Installing \"Red Hat Integration - 3scale\" operator in test-hgwnj Installs Red Hat Integration - 3scale operator in test-hgwnj and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-zsabv.Installing \"Red Hat Integration - 3scale\" operator in test-zsabv Installs Red Hat Integration - 3scale operator in test-zsabv and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-uekhm.Installing \"Red Hat Integration - 3scale\" operator in test-uekhm Installs Red Hat Integration - 3scale operator in test-uekhm and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-jopkv.Installing \"Red Hat Integration - 3scale\" operator in test-jopkv \"after all\" hook for \"Installs Red Hat Integration - 3scale operator in test-jopkv and creates 3scale Backend Schema operand i (...)\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-jopkv.Installing \"Red Hat Integration - 3scale\" operator in test-jopkv Installs Red Hat Integration - 3scale operator in test-jopkv and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-dzlem.Installing \"Red Hat Integration - 3scale\" operator in test-dzlem Installs Red Hat Integration - 3scale operator in test-dzlem and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-bbahh.Installing \"Red Hat Integration - 3scale\" operator in test-bbahh Installs Red Hat Integration - 3scale operator in test-bbahh and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-wzymg.Installing \"Red Hat Integration - 3scale\" operator in test-wzymg Installs Red Hat Integration - 3scale operator in test-wzymg and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-hieha.Installing \"Red Hat Integration - 3scale\" operator in test-hieha Installs Red Hat Integration - 3scale operator in test-hieha and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-dcblg.Installing \"Red Hat Integration - 3scale\" operator in test-dcblg Installs Red Hat Integration - 3scale operator in test-dcblg and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-bvsmv.Installing \"Red Hat Integration - 3scale\" operator in test-bvsmv Installs Red Hat Integration - 3scale operator in test-bvsmv and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-iadyv.Installing \"Red Hat Integration - 3scale\" operator in test-iadyv Installs Red Hat Integration - 3scale operator in test-iadyv and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-hnant.Installing \"Red Hat Integration - 3scale\" operator in test-hnant Installs Red Hat Integration - 3scale operator in test-hnant and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-pttzf.Installing \"Red Hat Integration - 3scale\" operator in test-pttzf Installs Red Hat Integration - 3scale operator in test-pttzf and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-ydire.Installing \"Red Hat Integration - 3scale\" operator in test-ydire Installs Red Hat Integration - 3scale operator in test-ydire and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-hwgst.Installing \"Red Hat Integration - 3scale\" operator in test-hwgst Installs Red Hat Integration - 3scale operator in test-hwgst and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-liktk.Installing \"Red Hat Integration - 3scale\" operator in test-liktk Installs Red Hat Integration - 3scale operator in test-liktk and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-ywsnb.Installing \"Red Hat Integration - 3scale\" operator in test-ywsnb Installs Red Hat Integration - 3scale operator in test-ywsnb and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-ealpr.Installing \"Red Hat Integration - 3scale\" operator in test-ealpr Installs Red Hat Integration - 3scale operator in test-ealpr and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-aurux.Installing \"Red Hat Integration - 3scale\" operator in test-aurux Installs Red Hat Integration - 3scale operator in test-aurux and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-bvtmh.Installing \"Red Hat Integration - 3scale\" operator in test-bvtmh Installs Red Hat Integration - 3scale operator in test-bvtmh and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-addza.Installing \"Red Hat Integration - 3scale\" operator in test-addza Installs Red Hat Integration - 3scale operator in test-addza and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-ogcte.Installing \"Red Hat Integration - 3scale\" operator in test-ogcte Installs Red Hat Integration - 3scale operator in test-ogcte and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-bimqn.Installing \"Red Hat Integration - 3scale\" operator in test-bimqn Installs Red Hat Integration - 3scale operator in test-bimqn and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-mvegy.Installing \"Red Hat Integration - 3scale\" operator in test-mvegy Installs Red Hat Integration - 3scale operator in test-mvegy and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-qcmho.Installing \"Red Hat Integration - 3scale\" operator in test-qcmho Installs Red Hat Integration - 3scale operator in test-qcmho and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-renxp.Installing \"Red Hat Integration - 3scale\" operator in test-renxp Installs Red Hat Integration - 3scale operator in test-renxp and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-raoaz.Installing \"Red Hat Integration - 3scale\" operator in test-raoaz Installs Red Hat Integration - 3scale operator in test-raoaz and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-fzftk.Installing \"Red Hat Integration - 3scale\" operator in test-fzftk Installs Red Hat Integration - 3scale operator in test-fzftk and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-ozxfm.Installing \"Red Hat Integration - 3scale\" operator in test-ozxfm Installs Red Hat Integration - 3scale operator in test-ozxfm and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-ygggh.Installing \"Red Hat Integration - 3scale\" operator in test-ygggh Installs Red Hat Integration - 3scale operator in test-ygggh and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-vzrws.Installing \"Red Hat Integration - 3scale\" operator in test-vzrws Installs Red Hat Integration - 3scale operator in test-vzrws and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-rvnqe.Installing \"Red Hat Integration - 3scale\" operator in test-rvnqe Installs Red Hat Integration - 3scale operator in test-rvnqe and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-omddd.Installing \"Red Hat Integration - 3scale\" operator in test-omddd \"after all\" hook for \"Installs Red Hat Integration - 3scale operator in test-omddd and creates 3scale Backend Schema operand i (...)\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-omddd.Installing \"Red Hat Integration - 3scale\" operator in test-omddd Installs Red Hat Integration - 3scale operator in test-omddd and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-erjrn.Installing \"Red Hat Integration - 3scale\" operator in test-erjrn Installs Red Hat Integration - 3scale operator in test-erjrn and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-emhcc.Installing \"Red Hat Integration - 3scale\" operator in test-emhcc Installs Red Hat Integration - 3scale operator in test-emhcc and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-mftza.Installing \"Red Hat Integration - 3scale\" operator in test-mftza Installs Red Hat Integration - 3scale operator in test-mftza and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-gtcn.Installing \"Red Hat Integration - 3scale\" operator in test-gtcn Installs Red Hat Integration - 3scale operator in test-gtcn and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-cohep.Installing \"Red Hat Integration - 3scale\" operator in test-cohep Installs Red Hat Integration - 3scale operator in test-cohep and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-yvida.Installing \"Red Hat Integration - 3scale\" operator in test-yvida Installs Red Hat Integration - 3scale operator in test-yvida and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-gkjsp.Installing \"Red Hat Integration - 3scale\" operator in test-gkjsp Installs Red Hat Integration - 3scale operator in test-gkjsp and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-veawg.Installing \"Red Hat Integration - 3scale\" operator in test-veawg Installs Red Hat Integration - 3scale operator in test-veawg and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-nuvuh.Installing \"Red Hat Integration - 3scale\" operator in test-nuvuh Installs Red Hat Integration - 3scale operator in test-nuvuh and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-vfqji.Installing \"Red Hat Integration - 3scale\" operator in test-vfqji Installs Red Hat Integration - 3scale operator in test-vfqji and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-ocmsb.Installing \"Red Hat Integration - 3scale\" operator in test-ocmsb Installs Red Hat Integration - 3scale operator in test-ocmsb and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-rscpr.Installing \"Red Hat Integration - 3scale\" operator in test-rscpr Installs Red Hat Integration - 3scale operator in test-rscpr and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-lkzia.Installing \"Red Hat Integration - 3scale\" operator in test-lkzia Installs Red Hat Integration - 3scale operator in test-lkzia and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-ijcan.Installing \"Red Hat Integration - 3scale\" operator in test-ijcan Installs Red Hat Integration - 3scale operator in test-ijcan and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-bvxzu.Installing \"Red Hat Integration - 3scale\" operator in test-bvxzu Installs Red Hat Integration - 3scale operator in test-bvxzu and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-eiidb.Installing \"Red Hat Integration - 3scale\" operator in test-eiidb \"after all\" hook for \"Installs Red Hat Integration - 3scale operator in test-eiidb and creates 3scale Backend Schema operand i (...)\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-eiidb.Installing \"Red Hat Integration - 3scale\" operator in test-eiidb Installs Red Hat Integration - 3scale operator in test-eiidb and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-lkgzg.Installing \"Red Hat Integration - 3scale\" operator in test-lkgzg Installs Red Hat Integration - 3scale operator in test-lkgzg and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-slavo.Installing \"Red Hat Integration - 3scale\" operator in test-slavo Installs Red Hat Integration - 3scale operator in test-slavo and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-uhwdv.Installing \"Red Hat Integration - 3scale\" operator in test-uhwdv Installs Red Hat Integration - 3scale operator in test-uhwdv and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-hjhug.Installing \"Red Hat Integration - 3scale\" operator in test-hjhug Installs Red Hat Integration - 3scale operator in test-hjhug and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-hzsqm.Installing \"Red Hat Integration - 3scale\" operator in test-hzsqm Installs Red Hat Integration - 3scale operator in test-hzsqm and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-pjkdd.Installing \"Red Hat Integration - 3scale\" operator in test-pjkdd \"after all\" hook for \"Installs Red Hat Integration - 3scale operator in test-pjkdd and creates 3scale Backend Schema operand i (...)\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-pjkdd.Installing \"Red Hat Integration - 3scale\" operator in test-pjkdd Installs Red Hat Integration - 3scale operator in test-pjkdd and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-hkgbg.Installing \"Red Hat Integration - 3scale\" operator in test-hkgbg Installs Red Hat Integration - 3scale operator in test-hkgbg and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-vldyv.Installing \"Red Hat Integration - 3scale\" operator in test-vldyv Installs Red Hat Integration - 3scale operator in test-vldyv and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-zzuao.Installing \"Red Hat Integration - 3scale\" operator in test-zzuao Installs Red Hat Integration - 3scale operator in test-zzuao and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-plhfz.Installing \"Red Hat Integration - 3scale\" operator in test-plhfz Installs Red Hat Integration - 3scale operator in test-plhfz and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-agnvz.Installing \"Red Hat Integration - 3scale\" operator in test-agnvz \"after all\" hook for \"Installs Red Hat Integration - 3scale operator in test-agnvz and creates 3scale Backend Schema operand i (...)\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-agnvz.Installing \"Red Hat Integration - 3scale\" operator in test-agnvz Installs Red Hat Integration - 3scale operator in test-agnvz and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-aggkp.Installing \"Red Hat Integration - 3scale\" operator in test-aggkp Installs Red Hat Integration - 3scale operator in test-aggkp and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-qijlp.Installing \"Red Hat Integration - 3scale\" operator in test-qijlp Installs Red Hat Integration - 3scale operator in test-qijlp and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-xejfp.Installing \"Red Hat Integration - 3scale\" operator in test-xejfp Installs Red Hat Integration - 3scale operator in test-xejfp and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-luxlf.Installing \"Red Hat Integration - 3scale\" operator in test-luxlf \"after all\" hook for \"Installs Red Hat Integration - 3scale operator in test-luxlf and creates 3scale Backend Schema operand i (...)\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-luxlf.Installing \"Red Hat Integration - 3scale\" operator in test-luxlf Installs Red Hat Integration - 3scale operator in test-luxlf and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-qmygy.Installing \"Red Hat Integration - 3scale\" operator in test-qmygy Installs Red Hat Integration - 3scale operator in test-qmygy and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-gkpfd.Installing \"Red Hat Integration - 3scale\" operator in test-gkpfd Installs Red Hat Integration - 3scale operator in test-gkpfd and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-wlggb.Installing \"Red Hat Integration - 3scale\" operator in test-wlggb Installs Red Hat Integration - 3scale operator in test-wlggb and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-ieesa.Installing \"Red Hat Integration - 3scale\" operator in test-ieesa Installs Red Hat Integration - 3scale operator in test-ieesa and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-iuayo.Installing \"Red Hat Integration - 3scale\" operator in test-iuayo Installs Red Hat Integration - 3scale operator in test-iuayo and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-bnhgj.Installing \"Red Hat Integration - 3scale\" operator in test-bnhgj Installs Red Hat Integration - 3scale operator in test-bnhgj and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-ytxgk.Installing \"Red Hat Integration - 3scale\" operator in test-ytxgk Installs Red Hat Integration - 3scale operator in test-ytxgk and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-lmllt.Installing \"Red Hat Integration - 3scale\" operator in test-lmllt Installs Red Hat Integration - 3scale operator in test-lmllt and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-qleiu.Installing \"Red Hat Integration - 3scale\" operator in test-qleiu \"after all\" hook for \"Installs Red Hat Integration - 3scale operator in test-qleiu and creates 3scale Backend Schema operand i (...)\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-qleiu.Installing \"Red Hat Integration - 3scale\" operator in test-qleiu Installs Red Hat Integration - 3scale operator in test-qleiu and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-dzpz.Installing \"Red Hat Integration - 3scale\" operator in test-dzpz Installs Red Hat Integration - 3scale operator in test-dzpz and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-amdic.Installing \"Red Hat Integration - 3scale\" operator in test-amdic Installs Red Hat Integration - 3scale operator in test-amdic and creates 3scale Backend Schema operand instance\"",
		"\"Installing \"Red Hat Integration - 3scale\" operator in test-qbpdf.Installing \"Red Hat Integration - 3scale\" operator in test-qbpdf Installs Red Hat Integration - 3scale operator in test-qbpdf and creates 3scale Backend Schema operand instance\"",
	}

	return testStrings
}

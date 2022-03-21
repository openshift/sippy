package installhtml

import (
	"fmt"
	"strings"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/util/sets"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

func TestDetailTests(format ResponseFormat, curr, prev sippyprocessingv1.TestReport, testSubstrings []string) string {
	dataForTestsByVariant := getDataForTestsByVariant(
		curr, prev,
		isTestDetailRelatedTest(testSubstrings),
		neverMatch,
	)

	variants := sets.String{}
	for _, byVariant := range dataForTestsByVariant.testNameToVariantToTestResult {
		variants.Insert(sets.StringKeySet(byVariant).UnsortedList()...)
	}

	if format == JSON {
		return dataForTestsByVariant.getTableJSON("Details for Tests", "Test Details by Variant",
			variants.List(), noChange)
	}

	return dataForTestsByVariant.getTableHTML("Details for Tests", "TestDetailByVariant", "Test Details by Variant", variants.List(), noChange)
}

func TestDetailTestsFromDB(dbc *db.DB, format ResponseFormat, release string, testSubstrings []string) (string, error) {
	// TODO: use the new approach from install_by_operators.go
	dataForTestsByVariant, err := getDataForTestsByVariantFromDB(dbc, release, testSubstrings)
	if err != nil {
		return "", err
	}

	variants := sets.String{}
	for _, byVariant := range dataForTestsByVariant.testNameToVariantToTestResult {
		variants.Insert(sets.StringKeySet(byVariant).UnsortedList()...)
	}

	if format == JSON {
		return dataForTestsByVariant.getTableJSON("Details for Tests", "Test Details by Variant",
			variants.List(), noChange), nil
	}

	return dataForTestsByVariant.getTableHTML("Details for Tests", "TestDetailByVariant", "Test Details by Variant", variants.List(), noChange), nil
}

func summaryTestDetailRelatedTests(curr, prev sippyprocessingv1.TestReport, testSubstrings []string, numDays int, release string) string {
	// test name | test | pass rate | higher/lower | pass rate
	s := fmt.Sprintf(`
	<table class="table">
		<tr>
			<th colspan=5 class="text-center">
				<a class="text-dark" id="Tests" href="#Tests">Tests</a>
				<i class="fa fa-info-circle" title="Tests, sorted by passing rate.  The link will prepopulate a BZ template to be filled out and submitted to report a test against the test."></i>
			</th>
		</tr>
		<tr>
			<th colspan=2/><th class="text-center">Latest %d Days</th><th/><th class="text-center">Previous 7 Days</th>
		</tr>
		<tr>
			<th>Test Name</th><th>File a Test</th><th>Pass Rate</th><th/><th>Pass Rate</th>
		</tr>
	`, numDays)

	s += failingTestsRows(curr.ByTest, prev.ByTest, release, isTestDetailRelatedTest(testSubstrings))

	s += "</table>"

	return s
}

func isTestDetailRelatedTest(testSubstrings []string) func(sippyprocessingv1.TestResult) bool {
	return func(testResult sippyprocessingv1.TestResult) bool {
		for _, testSubString := range testSubstrings {
			if strings.Contains(testResult.Name, testSubString) {
				return true
			}
		}

		return false
	}
}

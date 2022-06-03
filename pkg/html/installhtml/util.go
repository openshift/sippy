package installhtml

import (
	"encoding/json"

	"github.com/openshift/sippy/pkg/apis/api"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/util/sets"
)

// TODO: this should get removed? Including the TestResult struct
type currPrevTestResult struct {
	curr sippyprocessingv1.TestResult
	prev *sippyprocessingv1.TestResult
}

func (r currPrevTestResult) toTest(name string) api.Test {
	test := api.Test{
		Name:                  name,
		CurrentPassPercentage: r.curr.PassPercentage,
		CurrentSuccesses:      r.curr.Successes,
		CurrentFailures:       r.curr.Failures,
		CurrentFlakes:         r.curr.Flakes,
		CurrentRuns:           r.curr.Successes + r.curr.Failures + r.curr.Flakes,
	}

	if r.prev != nil {
		test.PreviousPassPercentage = r.prev.PassPercentage
		test.PreviousFlakes = r.prev.Flakes
		test.PreviousFailures = r.prev.Failures
		test.PreviousSuccesses = r.prev.Successes
		test.PreviousRuns = r.prev.Successes + r.prev.Failures + r.prev.Flakes
	}

	return test
}

func (c *currPrevFailedTestResult) toCurrPrevTestResult() *currPrevTestResult {
	if c == nil {
		return nil
	}
	if c.prev == nil {
		return &currPrevTestResult{curr: c.curr.TestResultAcrossAllJobs}
	}
	return &currPrevTestResult{
		curr: c.curr.TestResultAcrossAllJobs,
		prev: &c.prev.TestResultAcrossAllJobs,
	}
}

type currPrevFailedTestResult struct {
	curr sippyprocessingv1.FailingTestResult
	prev *sippyprocessingv1.FailingTestResult
}

// TODO: this struct needs to go, it's way too convoluted and deep.
type testsByVariant struct {
	aggregateResultByTestName      map[string]*currPrevFailedTestResult
	testNameToVariantToTestResult  map[string]map[string]*currPrevTestResult // these are the other rows in the table.
	aggregationToOverallTestResult map[string]*currPrevTestResult            // this is the first row of the table, summarizing all data.  If empty or nil, no summary is given.
}

func getDataForTestsByVariantFromDB(dbc *db.DB, release string, testSubStrings []string) (testsByVariant, error) {
	ret := testsByVariant{
		aggregateResultByTestName:      map[string]*currPrevFailedTestResult{}, // not used in output, maybe we can skip
		testNameToVariantToTestResult:  map[string]map[string]*currPrevTestResult{},
		aggregationToOverallTestResult: map[string]*currPrevTestResult{}, // may not be used in output in our first use case
	}

	testReports, err := query.TestReportsByVariant(dbc, release, testSubStrings)
	if err != nil {
		return ret, err
	}

	// We don't need to populate the ret.aggregateResultByTestName as it is only used in calculating the old way,
	// and not when we output below.
	// We *may* not need to populate ret.aggregationToOverallTestResult either, as this is only needed sometimes below.
	// TODO: ^^ when? who calls it this way?

	// We have a pretty clean list of TestResults by variant from the dbc, but transform to the old datastructure
	// to re-use the response writing logic below.
	for _, tr := range testReports {
		if _, ok := ret.testNameToVariantToTestResult[tr.Name]; !ok {
			ret.testNameToVariantToTestResult[tr.Name] = map[string]*currPrevTestResult{}
		}
		ret.testNameToVariantToTestResult[tr.Name][tr.Variant] = &currPrevTestResult{
			curr: sippyprocessingv1.TestResult{
				Name:              tr.Name,
				Successes:         tr.CurrentSuccesses,
				Failures:          tr.CurrentFailures,
				Flakes:            tr.CurrentFlakes,
				PassPercentage:    tr.CurrentPassPercentage,
				BugList:           nil, // TODO
				AssociatedBugList: nil, // TODO
			},
			prev: &sippyprocessingv1.TestResult{
				Name:              tr.Name,
				Successes:         tr.PreviousSuccesses,
				Failures:          tr.PreviousFailures,
				Flakes:            tr.PreviousFlakes,
				PassPercentage:    tr.PreviousPassPercentage,
				BugList:           nil, // TODO
				AssociatedBugList: nil, // TODO
			},
		}
	}

	return ret, nil
}

//nolint:goconst
func (a testsByVariant) getTableJSON(
	title string,
	description string,
	aggregationNames []string, // these are the columns
	testNameToDisplayName func(string) string,
) string {
	summary := map[string]interface{}{
		"title":        title,
		"description":  description,
		"column_names": aggregationNames,
	}
	tests := make(map[string]map[string]api.Test)

	// now the overall install results by variant
	if len(a.aggregationToOverallTestResult) > 0 {
		results := make(map[string]api.Test)
		for _, variantName := range aggregationNames {
			data := a.aggregationToOverallTestResult[variantName].toTest(variantName)
			results[variantName] = data
		}
		tests["Overall"] = results
	}

	for _, testName := range sets.StringKeySet(a.testNameToVariantToTestResult).List() {
		testDisplayName := testNameToDisplayName(testName)
		variantResults := a.testNameToVariantToTestResult[testName]
		results := make(map[string]api.Test)
		for _, variantName := range aggregationNames {
			if data, ok := variantResults[variantName]; ok {
				results[variantName] = data.toTest(variantName)
			}
		}
		tests[testDisplayName] = results
	}

	summary["tests"] = tests
	result, err := json.Marshal(summary)
	if err != nil {
		panic(err)
	}

	return string(result)
}

func noChange(testName string) string {
	return testName
}

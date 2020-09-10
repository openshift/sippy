package testreportconversion

import sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"

// testResultsByPassPercentage sorts from lowest to highest pass percentage
type testResultsByPassPercentage []sippyprocessingv1.TestResult

func (a testResultsByPassPercentage) Len() int      { return len(a) }
func (a testResultsByPassPercentage) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a testResultsByPassPercentage) Less(i, j int) bool {
	return a[i].PassPercentage < a[j].PassPercentage
}

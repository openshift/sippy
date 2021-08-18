package releasehtml_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/html/htmltesthelpers"
	"github.com/openshift/sippy/pkg/html/releasehtml"
)

const release string = "4.9"

func TestPrintJobsReport(t *testing.T) {
	expectedContents := []string{release}

	testFunc := func(r *httptest.ResponseRecorder) {
		releasehtml.PrintJobsReport(r, release)
	}

	htmltesthelpers.AssertHTTPResponseContains(t, expectedContents, testFunc)
}

func TestPrintVariantsReport(t *testing.T) {
	variant := "aws"

	currWeek := &sippyprocessingv1.VariantResults{}
	prevWeek := &sippyprocessingv1.VariantResults{}

	expectedContents := []string{}

	testFunc := func(r *httptest.ResponseRecorder) {
		releasehtml.PrintVariantsReport(r, release, variant, currWeek, prevWeek, time.Now())
	}

	htmltesthelpers.AssertHTTPResponseContains(t, expectedContents, testFunc)
}

func TestWriteLandingPage(t *testing.T) {
	displayNames := []string{
		release,
	}

	testFunc := func(r *httptest.ResponseRecorder) {
		releasehtml.WriteLandingPage(r, displayNames)
	}

	htmltesthelpers.AssertHTTPResponseContains(t, displayNames, testFunc)
}

func TestPrintHTMLReport(t *testing.T) {
	req := &http.Request{}

	jobName := "periodic-ci-openshift-release-master-nightly-4.9-e2e-aws"

	report := htmltesthelpers.GetTestReport(jobName, "a-test", "4.9")
	twoDayReport := report
	prevReport := report
	numDays := 7
	jobTestCount := 10
	allReportNames := []string{release}

	expectedContents := []string{
		release,
		"StepMetrics",
		"All Multistage Jobs",
		"<td>e2e-aws",
		"<td>e2e-gcp",
		"Step Metrics For All",
		"<td>aws-specific",
		"<td>gcp-specific",
		"<td>openshift-e2e-test",
		"<td>ipi-install",
	}

	testFunc := func(r *httptest.ResponseRecorder) {
		releasehtml.PrintHTMLReport(r, req, report, twoDayReport, prevReport, numDays, jobTestCount, allReportNames)
	}

	htmltesthelpers.AssertHTTPResponseContains(t, expectedContents, testFunc)
}

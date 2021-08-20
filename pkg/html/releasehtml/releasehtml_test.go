package releasehtml_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/api/stepmetrics/fixtures"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/html/htmltesthelpers"
	"github.com/openshift/sippy/pkg/html/releasehtml"
)

func TestPrintVariantsReport(t *testing.T) {
	variant := "aws"

	currWeek := &sippyprocessingv1.VariantResults{}
	prevWeek := &sippyprocessingv1.VariantResults{}

	expectedContents := []string{}

	testFunc := func(r *httptest.ResponseRecorder) {
		releasehtml.PrintVariantsReport(r, fixtures.Release, variant, currWeek, prevWeek, time.Now())
	}

	htmltesthelpers.AssertHTTPResponseContains(t, expectedContents, testFunc)
}

func TestWriteLandingPage(t *testing.T) {
	displayNames := []string{
		fixtures.Release,
	}

	testFunc := func(r *httptest.ResponseRecorder) {
		releasehtml.WriteLandingPage(r, displayNames)
	}

	htmltesthelpers.AssertHTTPResponseContains(t, displayNames, testFunc)
}

func TestPrintHTMLReport(t *testing.T) {
	req := &http.Request{}

	report := fixtures.GetTestReport(fixtures.AwsJobName, "a-test", fixtures.Release)
	twoDayReport := report
	prevReport := report
	numDays := 7
	jobTestCount := 10
	allReportNames := []string{fixtures.Release}

	expectedContents := []string{
		fixtures.Release,
		"StepMetrics",
		"All Multistage Job Names",
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

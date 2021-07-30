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
	htmltesthelpers.WriteHTMLToFile(t.Name()+".html", testFunc)
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
	htmltesthelpers.WriteHTMLToFile(t.Name()+".html", testFunc)
}

func TestWriteLandingPage(t *testing.T) {
	displayNames := []string{
		release,
	}

	testFunc := func(r *httptest.ResponseRecorder) {
		releasehtml.WriteLandingPage(r, displayNames)
	}

	htmltesthelpers.AssertHTTPResponseContains(t, displayNames, testFunc)
	htmltesthelpers.WriteHTMLToFile(t.Name()+".html", testFunc)
}

func TestPrintHTMLReport(t *testing.T) {
	req := &http.Request{}

	report := sippyprocessingv1.TestReport{}
	twoDayReport := sippyprocessingv1.TestReport{}
	prevReport := sippyprocessingv1.TestReport{}
	numDays := 7
	jobTestCount := 10
	allReportNames := []string{release}

	testFunc := func(r *httptest.ResponseRecorder) {
		releasehtml.PrintHTMLReport(r, req, report, twoDayReport, prevReport, numDays, jobTestCount, allReportNames)
	}

	htmltesthelpers.AssertHTTPResponseContains(t, allReportNames, testFunc)
	htmltesthelpers.WriteHTMLToFile(t.Name()+".html", testFunc)
}

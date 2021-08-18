package installhtml_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/html/htmltesthelpers"
	"github.com/openshift/sippy/pkg/html/installhtml"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
)

const jobName string = "job-name"
const release string = "4.9"
const numDays int = 7

func TestPrintUpgradeHTMLReport(t *testing.T) {
	req := &http.Request{}

	report := getReport("failing-test-2")
	prevReport := report

	expectedContents := []string{}

	testFunc := func(r *httptest.ResponseRecorder) {
		installhtml.PrintUpgradeHTMLReport(r, req, report, prevReport, numDays, release)
	}

	htmltesthelpers.AssertHTTPResponseContains(t, expectedContents, testFunc)
}

func TestPrintInstallHTMLReport(t *testing.T) {
	req := &http.Request{}

	report := getReport(testgridanalysisapi.InstallTestName)
	prevReport := report

	expectedContents := []string{}

	testFunc := func(r *httptest.ResponseRecorder) {
		installhtml.PrintInstallHTMLReport(r, req, report, prevReport, numDays, release)
	}

	htmltesthelpers.AssertHTTPResponseContains(t, expectedContents, testFunc)
}

func TestPrintOperatorHealthHTMLReport(t *testing.T) {
	req := &http.Request{}

	report := getReport(testgridanalysisapi.FinalOperatorHealthTestName)
	prevReport := report

	expectedContents := []string{}

	testFunc := func(r *httptest.ResponseRecorder) {
		installhtml.PrintOperatorHealthHTMLReport(r, req, report, prevReport, numDays, release)

	}

	htmltesthelpers.AssertHTTPResponseContains(t, expectedContents, testFunc)
}

func TestPrintTestDetailHTMLReport(t *testing.T) {
	req := &http.Request{}

	report := getReport("failing-test-2")
	prevReport := report

	testSubstrings := []string{
		"substring-1",
		"substring-2",
	}

	expectedContents := []string{}

	testFunc := func(r *httptest.ResponseRecorder) {
		installhtml.PrintTestDetailHTMLReport(r, req, report, prevReport, testSubstrings, numDays, release)

	}

	htmltesthelpers.AssertHTTPResponseContains(t, expectedContents, testFunc)
}

func getReport(testName string) sippyprocessingv1.TestReport {
	return htmltesthelpers.GetTestReport(jobName, testName, release)
}

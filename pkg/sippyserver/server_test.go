package sippyserver_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"testing"
	"time"

	testgridv1 "github.com/openshift/sippy/pkg/apis/testgrid/v1"
	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/openshift/sippy/pkg/sippyserver"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridconversion"
	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"
)

const port int = 8080
const release string = "4.9"

func TestSippyserverSmokeTest(t *testing.T) {
	// This is a smoke test that starts up the Sippy server with some data and
	// then makes HTTP requests against it to ensure that we get the appropriate
	// status code in response.

	// The intent is to get a test suite that targets as much of the Sippy
	// servers' execution path as possible.  Future work can focus on asserting
	// that a specific behavior occurs given specific input.  For now, we just
	// examine HTTP response status codes for all of the paths with a few
	// different input cases.

	// Known release URL args
	knownReleaseURLArgs := map[string]string{
		"release": release,
	}

	// Unknown release URL args
	unknownReleaseURLArgs := map[string]string{
		"release": "unknown-release",
	}

	// No supplied URL args.
	emptyURLArgs := map[string]string{}

	// Used to cause HTTP Bad Request (400).
	malformedJobFilterRegex := `\K`

	// These are partial test cases grouped by path and HTTP status.
	groupedTestCases := byPathAndStatus{
		"/": byStatus{
			http.StatusOK: testCases{
				{
					args: knownReleaseURLArgs,
				},
				{
					args: emptyURLArgs,
				},
				{
					args: unknownReleaseURLArgs,
				},
			},
		},
		"/api/jobs": byStatus{
			http.StatusOK: testCases{
				{
					args: knownReleaseURLArgs,
				},
			},
			http.StatusBadRequest: testCases{
				{
					args: emptyURLArgs,
				},
			},
			http.StatusNotFound: testCases{
				{
					args: unknownReleaseURLArgs,
				},
			},
		},
		"/api/jobs/details": byStatus{
			http.StatusOK: testCases{
				{
					args: knownReleaseURLArgs,
				},
			},
			http.StatusBadRequest: testCases{
				{
					args: emptyURLArgs,
				},
			},
			http.StatusNotFound: testCases{
				{
					args: unknownReleaseURLArgs,
				},
			},
		},
		"/api/tests": byStatus{
			http.StatusOK: testCases{
				{
					args: knownReleaseURLArgs,
				},
			},
			http.StatusBadRequest: testCases{
				{
					args: emptyURLArgs,
				},
			},
			http.StatusNotFound: testCases{
				{
					args: unknownReleaseURLArgs,
				},
			},
		},
		"/canary": byStatus{
			http.StatusOK: testCases{
				{
					args: knownReleaseURLArgs,
				},
				{
					args: unknownReleaseURLArgs,
				},
				{
					args: emptyURLArgs,
				},
			},
		},
		"/detailed": byStatus{
			http.StatusOK: testCases{
				{
					args: emptyURLArgs,
				},
				{
					args: knownReleaseURLArgs,
				},
				{
					args: unknownReleaseURLArgs,
				},
				{
					args: map[string]string{
						"release":                 release,
						"startDay":                "0",
						"endDay":                  "7",
						"testSuccessThreshold":    "98.0",
						"minTestRuns":             "10",
						"failureClusterThreshold": "10",
						"jobTestCount":            "10",
						"jobFilter":               "periodic-ci-openshift-release-master-ci-4.9-e2e-aws",
					},
				},
			},
			http.StatusBadRequest: testCases{
				{
					args: map[string]string{
						"release":   release,
						"jobFilter": malformedJobFilterRegex,
					},
				},
			},
		},
		"/install": byStatus{
			http.StatusOK: testCases{
				{
					args: knownReleaseURLArgs,
				},
			},
			http.StatusNotFound: testCases{
				{
					args: emptyURLArgs,
				},
				{
					args: unknownReleaseURLArgs,
				},
			},
		},
		"/jobs": byStatus{
			http.StatusOK: testCases{
				{
					args: emptyURLArgs,
				},
				{
					args: knownReleaseURLArgs,
				},
			},
		},
		"/json": byStatus{
			http.StatusOK: testCases{
				{
					args: knownReleaseURLArgs,
				},
				{
					args: map[string]string{
						"release": "all",
					},
				},
			},
			http.StatusNotFound: testCases{
				{
					args: unknownReleaseURLArgs,
				},
				{
					args: emptyURLArgs,
				},
			},
		},
		"/operator-health": byStatus{
			http.StatusOK: testCases{
				{
					args: knownReleaseURLArgs,
				},
			},
			http.StatusNotFound: testCases{
				{
					args: emptyURLArgs,
				},
				{
					args: unknownReleaseURLArgs,
				},
			},
		},
		"/refresh": byStatus{
			http.StatusOK: testCases{
				{
					args: emptyURLArgs,
				},
				{
					args: unknownReleaseURLArgs,
				},
				{
					args: knownReleaseURLArgs,
				},
			},
		},
		"/testdetails": byStatus{
			http.StatusOK: testCases{
				{
					args: knownReleaseURLArgs,
				},
			},
			http.StatusNotFound: testCases{
				{
					args: unknownReleaseURLArgs,
				},
				{
					args: emptyURLArgs,
				},
			},
		},
		"/upgrade": byStatus{
			http.StatusOK: testCases{
				{
					args: knownReleaseURLArgs,
				},
			},
			http.StatusNotFound: testCases{
				{
					args: emptyURLArgs,
				},
				{
					args: unknownReleaseURLArgs,
				},
			},
		},
		"/variants": byStatus{
			http.StatusOK: testCases{
				{
					args: map[string]string{
						"release": release,
						"variant": "aws",
					},
				},
			},
			http.StatusBadRequest: testCases{
				{
					args: emptyURLArgs,
				},
				{
					// This 400s because it is missing a required arg (variant)
					args: knownReleaseURLArgs,
				},
			},
			http.StatusNotFound: testCases{
				{
					args: map[string]string{
						"release": release,
						"variant": "unknown-variant",
					},
				},
				{
					args: map[string]string{
						"release": "unknown-release",
						"variant": "aws",
					},
				},
			},
		},
	}

	// Get our TestGrid fixture.
	testGridJobDetails, timestamp := getTestGridData()

	// Configure the server and inject our TestGrid fixtures.
	sippyServer := configureSippyServer(testGridJobDetails, timestamp)

	// Start the server in a goroutine so we can run our tests.
	go sippyServer.Serve()

	// Ensure the server gets shut down.
	defer func() {
		if err := sippyServer.GetHTTPServer().Close(); err != nil {
			t.Fatal(err)
		}
	}()

	// Prepare and run our grouped test cases.
	for _, tc := range groupedTestCases.all() {
		// Run each testcase in a subtest so they may be run concurrently and have
		// expressive output.
		t.Run(tc.name(), func(t *testing.T) {
			// Get the URL we want to query as a string.
			path := tc.getURL().String()

			// Query the URL.
			//nolint:gosec // URLs are hard-coded in the test suite.
			resp, err := http.Get(path)

			// Check if we have any errors.
			if err != nil {
				t.Errorf("did not expect an error: %s on path %s", err, path)
			}

			// Discard the response body, and close the reader:
			// - Not closing it will cause leaking file descriptors.
			// - Closing it without reading the contents first will cause broken pipe
			// errors from the server.
			// In the future, we could write to a buffer and make assertions on the
			// response content.
			defer resp.Body.Close()
			if _, err := io.Copy(ioutil.Discard, resp.Body); err != nil {
				t.Errorf("could not read response: %s", err)
			}

			// Check our status code.
			if resp.StatusCode != tc.statusCode {
				t.Errorf("expected status code to be %d, got: %d", tc.statusCode, resp.StatusCode)
			}
		})
	}
}

// Collection of testCases
type testCases []testCase

// Group the test cases by HTTP status code
type byStatus map[int]testCases

// Group the test cases by the URL path, then by HTTP status code
type byPathAndStatus map[string]byStatus

func (b byPathAndStatus) all() testCases {
	testCasesToRun := testCases{}

	// Prepare our grouped test cases using the paths and expected status codes.
	for path, testCasesByStatus := range b {
		for status, testCases := range testCasesByStatus {
			for _, tc := range testCases {
				testCasesToRun = append(testCasesToRun, testCase{
					args:       tc.args,
					statusCode: status,
					path:       path,
				})
			}
		}
	}

	return testCasesToRun
}

type testCase struct {
	// The URL query params to send with our request. All values will be encoded.
	args map[string]string
	// The path to access
	path string
	// The expected HTTP status code
	statusCode int
}

func (tc testCase) name() string {
	return fmt.Sprintf("HTTP %d %s %s", tc.statusCode, http.StatusText(tc.statusCode), tc.getURL())
}

func (tc testCase) getURL() *url.URL {
	// Convert the path and URL query argument map into a valid URL using the
	// stdlib libraries.
	urlValues := url.Values{}

	for k, v := range tc.args {
		urlValues.Add(k, v)
	}

	return &url.URL{
		Scheme:   "http",
		Host:     fmt.Sprintf("localhost:%d", port),
		Path:     tc.path,
		RawQuery: urlValues.Encode(),
	}
}

func configureSippyServer(jobDetails []testgridv1.JobDetails, timestamp time.Time) *sippyserver.Server {
	loadingConfig := sippyserver.TestGridLoadingConfig{
		Loader: func(_ string, _ []string, _ *regexp.Regexp) ([]testgridv1.JobDetails, time.Time) {
			// Inject Test Grid fixtures so we don't have to read from disk.
			return jobDetails, timestamp
		},
		// We're not reading from disk, so this can be empty
		LocalData: "",
		// We're not actively trying to filter anything
		JobFilter: regexp.MustCompile(``),
	}

	analysisConfig := sippyserver.RawJobResultsAnalysisConfig{
		StartDay: 0,
		NumDays:  7,
	}

	displayConfig := sippyserver.DisplayDataConfig{
		MinTestRuns:             1,
		TestSuccessThreshold:    0.95,
		FailureClusterThreshold: 0,
	}

	dashboardCoordinates := []sippyserver.TestGridDashboardCoordinates{
		{
			ReportName: release,
			TestGridDashboardNames: []string{
				fmt.Sprintf("redhat-openshift-ocp-release-%s-broken", release),
				fmt.Sprintf("redhat-openshift-ocp-release-%s-blocking", release),
				fmt.Sprintf("redhat-openshift-ocp-release-%s-informing", release),
			},
			BugzillaRelease: release,
		},
	}

	listenAddr := fmt.Sprintf(":%d", port)

	// Configure the Sippy server.
	sippyServer := sippyserver.NewServer(
		loadingConfig,
		analysisConfig,
		displayConfig,
		dashboardCoordinates,
		listenAddr,
		testgridconversion.NewOpenshiftSyntheticTestManager(),
		testidentification.NewOpenshiftVariantManager(),
		buganalysis.NewNoOpBugCache(),
		nil,
		nil,
	)

	// Refresh data and generate reports.
	sippyServer.RefreshData()

	return sippyServer
}

func getTestGridData() ([]testgridv1.JobDetails, time.Time) {
	now := time.Now()

	jobName := "periodic-ci-openshift-release-master-ci-4.9-e2e-aws"

	jobDetails := []testgridv1.JobDetails{
		{
			Name:  jobName,
			Query: "origin-ci-test/logs/" + jobName,
			ChangeLists: []string{
				"0123456789",
			},
			Timestamps: []int{
				// The code under test calls time.Now(). If we do not subtract one
				// second from time.Now(), this test fixture will be ignored.
				int(now.Add(-1*time.Second).Unix() * 1000),
			},
			Tests: []testgridv1.Test{
				{
					Name: "passing-test",
					Statuses: []testgridv1.TestResult{
						{
							Count: 1,
							Value: testgridv1.TestStatusSuccess,
						},
					},
				},
				{
					Name: "failing-test",
					Statuses: []testgridv1.TestResult{
						{
							Count: 1,
							Value: testgridv1.TestStatusFailure,
						},
					},
				},
			},
		},
	}

	return jobDetails, now
}

package spotcheckjobs

import (
	"context"
	"fmt"
	"sync"

	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crstatus"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
)

var _ middleware.Middleware = &SpotCheckJobs{}

// NewSpotCheckJobsMiddleware creates middleware that handles analysis for
// spot-check jobs. These jobs run infrequently and are evaluated on a simple
// pass/fail basis: at least one successful run in the sample window means healthy.
//
// The spot-check test data (querying [sig-sippy] openshift-tests should work
// for spot-check tier jobs) is injected into the sample status by the generator's
// getTestStatus method. This middleware only handles the custom analysis logic.
func NewSpotCheckJobsMiddleware(reqOptions reqopts.RequestOptions) *SpotCheckJobs {
	return &SpotCheckJobs{
		reqOptions: reqOptions,
	}
}

type SpotCheckJobs struct {
	reqOptions reqopts.RequestOptions
}

func (s *SpotCheckJobs) Query(_ context.Context, _ *sync.WaitGroup, _ chan error) {}

func (s *SpotCheckJobs) QueryTestDetails(_ context.Context, _ *sync.WaitGroup, _ chan error) {}

func (s *SpotCheckJobs) PreAnalysis(_ crtest.Identification, _ *testdetails.TestComparison) error {
	return nil
}

func (s *SpotCheckJobs) PostAnalysis(_ crtest.Identification, _ *testdetails.TestComparison) error {
	return nil
}

func (s *SpotCheckJobs) PreTestDetailsAnalysis(_ crtest.KeyWithVariants, _ *crstatus.TestJobRunStatuses) error {
	return nil
}

// Analyze claims spot-check tests and determines their status. The heuristic is:
//   - Any successful run in the sample window = healthy (NotSignificant)
//   - A single failed run with no successes = pending retry (MissingSample), since an
//     external component will trigger a retry for failed spot-check jobs
//   - Exactly two failed runs with no successes = SignificantRegression
//   - Three or more failed runs with no successes = ExtremeRegression
//   - No runs at all = no data (MissingSample)
//
// Returns false for non-spot-check tests to defer to other analyzers.
func (s *SpotCheckJobs) Analyze(testKey crtest.Identification,
	testStats *testdetails.TestComparison) (bool, error) {

	if !crtest.IsSpotCheckTestID(testKey.TestID) {
		return false, nil
	}

	sampleName, _, _ := crtest.ParseSpotCheckTestID(testKey.TestID)
	sample := s.findSample(sampleName)
	sampleDays := 0
	if sample != nil {
		sampleDays = int(sample.End.Sub(sample.Start).Hours() / 24)
		// The counts come from the spot-check window, not the ordinary sample the stats were initialized with.
		start, end := sample.Start, sample.End
		if sample.Release.Name != "" {
			testStats.SampleStats.Release = sample.Release.Name
		}
		testStats.SampleStats.Start = &start
		testStats.SampleStats.End = &end
	}

	totalRuns := testStats.SampleStats.Total()
	successfulRuns := testStats.SampleStats.SuccessCount
	failedRuns := totalRuns - successfulRuns

	switch {
	case successfulRuns > 0:
		testStats.ReportStatus = crtest.NotSignificant
		testStats.Explanations = append(testStats.Explanations,
			fmt.Sprintf("Spot-check job passed %d out of %d runs in the %d-day sample window",
				successfulRuns, totalRuns, sampleDays))
	case failedRuns >= 3:
		testStats.ReportStatus = crtest.ExtremeRegression
		testStats.Explanations = append(testStats.Explanations,
			fmt.Sprintf("Spot-check job did not pass in the %d-day sample window (%d runs, 0 successes)",
				sampleDays, totalRuns))
	case failedRuns == 2:
		testStats.ReportStatus = crtest.SignificantRegression
		testStats.Explanations = append(testStats.Explanations,
			fmt.Sprintf("Spot-check job failed %d times in the %d-day sample window with no successes",
				failedRuns, sampleDays))
	case failedRuns == 1:
		testStats.ReportStatus = crtest.MissingSample
		testStats.Explanations = append(testStats.Explanations,
			fmt.Sprintf("Spot-check job failed once in the %d-day sample window; awaiting retry before flagging regression",
				sampleDays))
	default:
		testStats.ReportStatus = crtest.MissingSample
		testStats.Explanations = append(testStats.Explanations,
			fmt.Sprintf("No spot-check job runs found in the %d-day sample window", sampleDays))
	}

	testStats.Comparison = crtest.SpotCheck
	testStats.BaseStats = nil

	return true, nil
}

func (s *SpotCheckJobs) findSample(name string) *reqopts.SpotCheckJobSampleOpts {
	for i := range s.reqOptions.SpotCheckJobSamples {
		if s.reqOptions.SpotCheckJobSamples[i].Name == name {
			return &s.reqOptions.SpotCheckJobSamples[i]
		}
	}
	return nil
}

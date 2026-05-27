package spotcheckjobs

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider"
	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crstatus"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
	log "github.com/sirupsen/logrus"
)

var _ middleware.Middleware = &SpotCheckJobs{}

// spotCheckMapping defines the hardcoded component/capability for spot-check jobs
// until SpotCheckComponent/SpotCheckCapability variants exist in the variant registry.
type spotCheckMapping struct {
	substrings []string
	component  string
	capability string
}

var spotCheckMappings = []spotCheckMapping{
	{substrings: []string{"-cpu-partitioning"}, component: "Node", capability: "CPU Partitioning"},
	{substrings: []string{"-etcd-scaling"}, component: "etcd", capability: "Scaling"},
}

// NewSpotCheckJobsMiddleware creates middleware that injects synthetic test results for
// "rare" tier jobs (e.g. cpu-partitioning, etcd-scaling) that don't run in the standard
// junit-based test pipeline. These jobs are evaluated on a simple pass/fail basis:
// at least one successful run in the sample window means healthy.
func NewSpotCheckJobsMiddleware(
	provider dataprovider.DataProvider,
	reqOptions reqopts.RequestOptions,
) *SpotCheckJobs {
	return &SpotCheckJobs{
		dataProvider: provider,
		reqOptions:   reqOptions,
		log:          log.WithField("middleware", "SpotCheckJobs"),
	}
}

type SpotCheckJobs struct {
	dataProvider dataprovider.DataProvider
	reqOptions   reqopts.RequestOptions
	log          log.FieldLogger

	// sampleJobDetails is populated during QueryTestDetails and consumed by PreTestDetailsAnalysis.
	sampleJobDetails      map[string][]dataprovider.JobRunDetail
	sampleJobDetailsMutex sync.Mutex
}

// Query fetches aggregated spot-check job results from BigQuery, creates synthetic
// test statuses (one per variant group), and injects them into the sample status channel.
// Each synthetic test uses a binary pass/fail: >=1 successful run = pass.
func (s *SpotCheckJobs) Query(ctx context.Context, wg *sync.WaitGroup,
	allJobVariants crtest.JobVariants,
	_, sampleStatusCh chan map[string]crstatus.TestStatus, errCh chan error) {

	if s.reqOptions.SpotCheckSample == nil {
		return
	}

	for _, m := range spotCheckMappings {
		wg.Go(func() {
			select {
			case <-ctx.Done():
				s.log.Info("context canceled during spot check query")
				return
			default:
			}

			groups, err := s.dataProvider.QuerySpotCheckJobRuns(ctx,
				s.reqOptions, allJobVariants, m.substrings,
				s.reqOptions.SpotCheckSample.Start, s.reqOptions.SpotCheckSample.End)
			if err != nil {
				errCh <- fmt.Errorf("spot check query failed for %s/%s: %w", m.component, m.capability, err)
				return
			}

			sampleStatus := map[string]crstatus.TestStatus{}
			for _, group := range groups {
				testKey := crtest.KeyWithVariants{
					TestID:   syntheticTestID(m.component, m.capability),
					Variants: group.Variants,
				}
				keyStr := testKey.KeyOrDie()

				atLeastOnePass := group.SuccessfulRuns >= 1
				successCount := 0
				if atLeastOnePass {
					successCount = 1
				}

				sampleStatus[keyStr] = crstatus.TestStatus{
					TestName:     syntheticTestName(m.component, m.capability),
					Component:    m.component,
					Capabilities: []string{m.capability},
					Variants:     variantMapToSlice(group.Variants),
					Count: crtest.Count{
						TotalCount:   1,
						SuccessCount: successCount,
					},
				}
			}

			s.log.Infof("injecting %d spot-check synthetic test results for %s/%s", len(sampleStatus), m.component, m.capability)
			sampleStatusCh <- sampleStatus
		})
	}
}

// QueryTestDetails fetches individual job run details for spot-check synthetic tests,
// storing them for later use by PreTestDetailsAnalysis to populate the drill-down view.
func (s *SpotCheckJobs) QueryTestDetails(ctx context.Context, wg *sync.WaitGroup,
	errCh chan error, allJobVariants crtest.JobVariants) {

	if s.reqOptions.SpotCheckSample == nil {
		return
	}

	for _, testIDOpt := range s.reqOptions.TestIDOptions {
		if !isSpotCheckTestID(testIDOpt.TestID) {
			continue
		}

		m := mappingForTestID(testIDOpt.TestID)
		if m == nil {
			s.log.Warnf("no mapping found for spot-check test ID %s", testIDOpt.TestID)
			continue
		}

		wg.Go(func() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			details, err := s.dataProvider.QuerySpotCheckJobRunDetails(ctx,
				s.reqOptions, allJobVariants,
				testIDOpt.RequestedVariants, m.substrings,
				s.reqOptions.SpotCheckSample.Start, s.reqOptions.SpotCheckSample.End)
			if err != nil {
				errCh <- fmt.Errorf("spot check details query failed: %w", err)
				return
			}

			testKey := crtest.KeyWithVariants{
				TestID:   testIDOpt.TestID,
				Variants: testIDOpt.RequestedVariants,
			}

			s.sampleJobDetailsMutex.Lock()
			defer s.sampleJobDetailsMutex.Unlock()
			if s.sampleJobDetails == nil {
				s.sampleJobDetails = map[string][]dataprovider.JobRunDetail{}
			}
			s.sampleJobDetails[testKey.KeyOrDie()] = details
			s.log.Infof("loaded %d spot-check job run details for %s", len(details), testIDOpt.TestID)
		})
	}
}

func (s *SpotCheckJobs) PreAnalysis(_ crtest.Identification,
	_ *testdetails.TestComparison) error {
	return nil
}

// Analyze claims spot-check tests and applies a simple heuristic: any successful run in
// the sample window means the job is healthy (NotSignificant), zero successes means
// ExtremeRegression. Returns false for non-spot-check tests to defer to other analyzers.
func (s *SpotCheckJobs) Analyze(testKey crtest.Identification,
	testStats *testdetails.TestComparison) (bool, error) {

	if !isSpotCheckTestID(testKey.TestID) {
		return false, nil
	}

	sampleDays := int(s.reqOptions.SpotCheckSample.End.Sub(s.reqOptions.SpotCheckSample.Start).Hours() / 24)

	if testStats.SampleStats.SuccessCount > 0 {
		testStats.ReportStatus = crtest.NotSignificant
		testStats.Explanations = append(testStats.Explanations,
			fmt.Sprintf("Spot-check job passed at least once in the %d-day sample window", sampleDays))
	} else {
		testStats.ReportStatus = crtest.ExtremeRegression
		testStats.Explanations = append(testStats.Explanations,
			fmt.Sprintf("Spot-check job did not pass in the %d-day sample window (%d runs, 0 successes)",
				sampleDays, testStats.SampleStats.Total()))
	}

	testStats.Comparison = crtest.SpotCheck
	testStats.BaseStats = nil

	return true, nil
}

func (s *SpotCheckJobs) PostAnalysis(_ crtest.Identification, _ *testdetails.TestComparison) error {
	return nil
}

// PreTestDetailsAnalysis populates the test details drill-down view for spot-check tests
// by converting the cached job run details into TestJobRunRows for the sample side.
// There is no base side since spot-check tests have no historical baseline comparison.
func (s *SpotCheckJobs) PreTestDetailsAnalysis(testKey crtest.KeyWithVariants,
	status *crstatus.TestJobRunStatuses) error {

	if !isSpotCheckTestID(testKey.TestID) {
		return nil
	}

	s.sampleJobDetailsMutex.Lock()
	details := s.sampleJobDetails[testKey.KeyOrDie()]
	s.sampleJobDetailsMutex.Unlock()

	if status.SampleStatus == nil {
		status.SampleStatus = map[string][]crstatus.TestJobRunRows{}
	}

	for _, run := range details {
		successCount := 0
		if run.Success {
			successCount = 1
		}
		row := crstatus.TestJobRunRows{
			TestKey:      testKey,
			TestKeyStr:   testKey.KeyOrDie(),
			TestName:     syntheticTestNameFromID(testKey.TestID),
			ProwJob:      run.JobName,
			ProwJobRunID: run.RunID,
			ProwJobURL:   run.URL,
			StartTime:    run.StartTime,
			Count: crtest.Count{
				TotalCount:   1,
				SuccessCount: successCount,
			},
		}
		status.SampleStatus[run.JobName] = append(status.SampleStatus[run.JobName], row)
	}

	return nil
}

// mappingForTestID returns the spotCheckMapping that produced the given synthetic testID,
// or nil if no mapping matches.
func mappingForTestID(testID string) *spotCheckMapping {
	for i, m := range spotCheckMappings {
		if syntheticTestID(m.component, m.capability) == testID {
			return &spotCheckMappings[i]
		}
	}
	return nil
}

func isSpotCheckTestID(testID string) bool {
	return strings.HasPrefix(testID, "spotcheck:")
}

func syntheticTestID(component, capability string) string {
	return fmt.Sprintf("spotcheck:%s:%s",
		strings.ToLower(component),
		strings.ToLower(strings.ReplaceAll(capability, " ", "-")))
}

func syntheticTestName(component, capability string) string {
	return fmt.Sprintf("[spot-check] %s / %s job must pass at least once per sample window",
		component, capability)
}

func syntheticTestNameFromID(testID string) string {
	parts := strings.SplitN(testID, ":", 3)
	if len(parts) == 3 {
		return fmt.Sprintf("[spot-check] %s / %s job must pass at least once per sample window",
			parts[1], strings.ReplaceAll(parts[2], "-", " "))
	}
	return testID
}

func variantMapToSlice(m map[string]string) []string {
	result := make([]string, 0, len(m))
	for k, v := range m {
		result = append(result, k+":"+v)
	}
	return result
}

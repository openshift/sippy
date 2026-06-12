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

// NewSpotCheckJobsMiddleware creates middleware that injects synthetic test results for
// spot-check jobs (e.g. cpu-partitioning, etcd-scaling) that don't run in the standard
// junit-based test pipeline. These jobs are evaluated on a simple pass/fail basis:
// at least one successful run in the sample window means healthy.
//
// Jobs are identified by the SpotCheckComponent and SpotCheckCapability variants in the
// variant registry. The component/capability values control where the synthetic test
// results appear in the component readiness report.
//
// Multiple spot-check samples can be configured (e.g. spotcheck-30d, spotcheck-90d),
// each with its own time window and variant filters. Each sample produces a separate
// set of synthetic test results.
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

// Query fetches aggregated spot-check job results from BigQuery for each configured
// sample, creates synthetic test statuses (one per component/capability/variant group
// per sample), and injects them into the sample status channel.
func (s *SpotCheckJobs) Query(ctx context.Context, wg *sync.WaitGroup,
	allJobVariants crtest.JobVariants,
	_, sampleStatusCh chan map[string]crstatus.TestStatus, errCh chan error) {

	if len(s.reqOptions.SpotCheckJobSamples) == 0 {
		return
	}

	for _, sample := range s.reqOptions.SpotCheckJobSamples {
		wg.Go(func() {
			select {
			case <-ctx.Done():
				s.log.Info("context canceled during spot check query")
				return
			default:
			}

			groups, err := s.dataProvider.QuerySpotCheckJobRuns(ctx,
				s.reqOptions, allJobVariants, sample.IncludeVariants,
				sample.Start, sample.End)
			if err != nil {
				errCh <- fmt.Errorf("spot check query failed for %s: %w", sample.Name, err)
				return
			}

			sampleStatus := map[string]crstatus.TestStatus{}
			requestedVariants := map[string]string{}
			if len(s.reqOptions.TestIDOptions) > 0 {
				requestedVariants = s.reqOptions.TestIDOptions[0].RequestedVariants
			}
			for _, group := range groups {
				if group.Component == "" || group.Capability == "" {
					s.log.Warnf("skipping spot-check group with empty component/capability: %+v", group)
					continue
				}

				if !variantsMatch(group.Variants, requestedVariants) {
					continue
				}

				testKey := crtest.KeyWithVariants{
					TestID:   syntheticTestID(sample.Name, group.Component, group.Capability),
					Variants: group.Variants,
				}
				keyStr := testKey.KeyOrDie()

				sampleStatus[keyStr] = crstatus.TestStatus{
					TestName:     syntheticTestName(group.Component, group.Capability),
					Component:    group.Component,
					Capabilities: []string{group.Capability},
					Variants:     variantMapToSlice(group.Variants),
					Count: crtest.Count{
						TotalCount:   group.TotalRuns,
						SuccessCount: group.SuccessfulRuns,
					},
					LastFailure: group.LastFailure,
				}
			}

			s.log.Infof("injecting %d spot-check synthetic test results for sample %s", len(sampleStatus), sample.Name)
			sampleStatusCh <- sampleStatus
		})
	}
}

// QueryTestDetails fetches individual job run details for spot-check synthetic tests,
// storing them for later use by PreTestDetailsAnalysis to populate the drill-down view.
func (s *SpotCheckJobs) QueryTestDetails(ctx context.Context, wg *sync.WaitGroup,
	errCh chan error, allJobVariants crtest.JobVariants) {

	if len(s.reqOptions.SpotCheckJobSamples) == 0 {
		return
	}

	for _, testIDOpt := range s.reqOptions.TestIDOptions {
		if !isSpotCheckTestID(testIDOpt.TestID) {
			continue
		}

		sampleName, component, capability := componentCapabilityFromTestID(testIDOpt.TestID)
		if component == "" || capability == "" {
			s.log.Warnf("could not parse component/capability from spot-check test ID %s", testIDOpt.TestID)
			continue
		}

		sample := s.findSample(sampleName)
		if sample == nil {
			s.log.Warnf("no spot-check sample found for tier %s in test ID %s", sampleName, testIDOpt.TestID)
			continue
		}

		wg.Go(func() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			details, err := s.dataProvider.QuerySpotCheckJobRunDetails(ctx,
				s.reqOptions, allJobVariants, sample.IncludeVariants,
				testIDOpt.RequestedVariants, component, capability,
				sample.Start, sample.End)
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
			s.log.WithField("variants", testIDOpt.RequestedVariants).Infof("loaded %d spot-check job run details for %s", len(details), testIDOpt.TestID)
		})
	}
}

func (s *SpotCheckJobs) PreAnalysis(_ crtest.Identification,
	_ *testdetails.TestComparison) error {
	return nil
}

// Analyze claims spot-check tests and determines their status. The heuristic is:
//   - Any successful run in the sample window = healthy (NotSignificant)
//   - A single failed run with no successes = pending retry (MissingSample), since an
//     external component will trigger a retry for failed spot-check jobs
//   - Two or more failed runs with no successes = confirmed regression (ExtremeRegression)
//   - No runs at all = no data (MissingSample)
//
// Returns false for non-spot-check tests to defer to other analyzers.
func (s *SpotCheckJobs) Analyze(testKey crtest.Identification,
	testStats *testdetails.TestComparison) (bool, error) {

	if !isSpotCheckTestID(testKey.TestID) {
		return false, nil
	}

	sampleName, _, _ := componentCapabilityFromTestID(testKey.TestID)
	sample := s.findSample(sampleName)
	sampleDays := 0
	if sample != nil {
		sampleDays = int(sample.End.Sub(sample.Start).Hours() / 24)
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
		// No runs at all
		testStats.ReportStatus = crtest.MissingSample
		testStats.Explanations = append(testStats.Explanations,
			fmt.Sprintf("No spot-check job runs found in the %d-day sample window", sampleDays))
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

func (s *SpotCheckJobs) findSample(name string) *reqopts.SpotCheckJobSampleOpts {
	for i := range s.reqOptions.SpotCheckJobSamples {
		if s.reqOptions.SpotCheckJobSamples[i].Name == name {
			return &s.reqOptions.SpotCheckJobSamples[i]
		}
	}
	return nil
}

func isSpotCheckTestID(testID string) bool {
	return strings.HasPrefix(testID, "spotcheck-")
}

// componentCapabilityFromTestID extracts the sample name, component and capability from
// a synthetic spot-check test ID. The format is "spotcheck-30d:component:capability".
func componentCapabilityFromTestID(testID string) (string, string, string) {
	parts := strings.SplitN(testID, ":", 3)
	if len(parts) != 3 {
		return "", "", ""
	}
	component := parts[1]
	capability := strings.ReplaceAll(parts[2], "-", " ")
	return parts[0], component, capability
}

func syntheticTestID(sampleName, component, capability string) string {
	return fmt.Sprintf("%s:%s:%s",
		sampleName,
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

func variantsMatch(groupVariants, requestedVariants map[string]string) bool {
	for k, v := range requestedVariants {
		if gv, ok := groupVariants[k]; !ok || gv != v {
			return false
		}
	}
	return true
}

func variantMapToSlice(m map[string]string) []string {
	result := make([]string, 0, len(m))
	for k, v := range m {
		result = append(result, k+":"+v)
	}
	return result
}

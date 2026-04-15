package releasefallback

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider"
	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware"
	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crstatus"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
	apiCache "github.com/openshift/sippy/pkg/apis/cache"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/openshift/sippy/pkg/util/sets"
	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/api"
)

const (
	// consider fallback data good for 7 days
	fallbackQueryTimeRoundingOverride = 24 * 7 * time.Hour
)

var _ middleware.Middleware = &ReleaseFallback{}

func NewReleaseFallbackMiddleware(
	provider dataprovider.DataProvider,
	reqOptions reqopts.RequestOptions,
	releaseConfigs []v1.Release,
) *ReleaseFallback {
	return &ReleaseFallback{
		dataProvider:   provider,
		log:            log.WithField("middleware", "ReleaseFallback"),
		reqOptions:     reqOptions,
		releaseConfigs: releaseConfigs,
	}
}

// ReleaseFallback middleware allows us to use the best pass rate data from the past
// several releases for our basis instead of just the requested basis. This helps prevent
// minor gradual degredation of quality, and also simplifies the process of accepting
// intentional regressions shortly before release, as we'll then automatically use the data
// from prior releases.
//
// It is responsible for querying basis test status for those several releases, and
// then replacing any basis test stats with a better releases test stats, when appropriate.
// This is done when we have sufficient test coverage, and a better pass rate.
type ReleaseFallback struct {
	dataProvider               dataprovider.DataProvider
	cachedFallbackTestStatuses *FallbackReleases
	log                        log.FieldLogger
	reqOptions                 reqopts.RequestOptions

	// baseOverrideStatus maps test key, to job name, to the result rows for that job.
	// This is used in test details reports, and in the typical API case will only contain one
	// test ID, but when cache priming for a view, we may have multiple.
	baseOverrideStatus map[string]map[string][]crstatus.TestJobRunRows
	baseOverrideMutex  sync.Mutex // Mutex to protect the map
	releaseConfigs     []v1.Release
}

func (r *ReleaseFallback) Analyze(testID string, variants map[string]string, report *testdetails.TestComparison) error {
	return nil
}

func (r *ReleaseFallback) Query(ctx context.Context, wg *sync.WaitGroup, allJobVariants crtest.JobVariants,
	_, _ chan map[string]crstatus.TestStatus, errCh chan error) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			r.log.Infof("Context canceled while fetching fallback query status")
			return
		default:
			// TODO: should we pass the same wg through rather than using another?
			errs := r.getFallbackBaseQueryStatus(ctx, allJobVariants, r.reqOptions.BaseRelease.Name, r.reqOptions.BaseRelease.Start, r.reqOptions.BaseRelease.End)
			if len(errs) > 0 {
				for _, err := range errs {
					errCh <- err
				}
			}
		}
	}()
}

// PreAnalysis looks for a better pass rate across our fallback releases for the given test stats.
// It then swaps them out and leaves an explanation before handing back to the core for analysis.
func (r *ReleaseFallback) PreAnalysis(testKey crtest.Identification, testStats *testdetails.TestComparison) error {
	// Nothing to do for tests without a basis, i.e. new tests.
	if testStats.BaseStats == nil {
		return nil
	}
	testIDVariantsKey := crtest.KeyWithVariants{
		TestID:   testKey.TestID,
		Variants: testKey.Variants,
	}
	testIDBytes, _ := json.Marshal(testIDVariantsKey)
	testKeyStr := string(testIDBytes)

	if r.cachedFallbackTestStatuses == nil {
		// In the test details path, this map is not initialized and we have no work to do for pre analysis.
		// Fallback is treated as a separate second report entirely, rather than swapping out values on the fly,
		// as this allows us to return both the fallback report as well as the one they asked for, which is helpful
		// for user comparison.
		return nil
	}

	var priorRelease = testStats.BaseStats.Release
	var err error
	var swappedExplanation string
	for err == nil {
		var cachedReleaseTestStatuses ReleaseTestMap
		var cTestStatus crstatus.TestStatus
		ok := false
		priorRelease, err = utils.PreviousRelease(priorRelease, r.releaseConfigs)
		// if we fail to determine the previous release then stop
		if err != nil {
			break
		}

		// if we hit a missing release then stop
		if cachedReleaseTestStatuses, ok = r.cachedFallbackTestStatuses.Releases[priorRelease]; !ok {
			break
		}

		// it's ok if we don't have a testKeyStr for this release
		// we likely won't have it for earlier releases either, but we can keep going
		if cTestStatus, ok = cachedReleaseTestStatuses.Tests[testKeyStr]; ok {

			// what is our base total compared to the original base
			// this happens when jobs shift like sdn -> ovn
			// if we get below threshold that's a sign we are reducing our base signal
			if float64(cTestStatus.TotalCount)/float64(testStats.BaseStats.Total()) < .6 {
				return nil
			}
			basePassRate := testStats.BaseStats.PassRate(r.reqOptions.AdvancedOption.FlakeAsFailure)

			cTestStats := cTestStatus.ToTestStats(r.reqOptions.AdvancedOption.FlakeAsFailure)
			if cTestStats.SuccessRate > basePassRate {
				// We've found a better pass rate in a prior release with enough runs to qualify.
				// Adjust the stats and keep looking for an even better one.
				testStats.BaseStats = &testdetails.ReleaseStats{
					Release: priorRelease,
					Start:   cachedReleaseTestStatuses.Start,
					End:     cachedReleaseTestStatuses.End,
					Stats:   cTestStats,
				}
				swappedExplanation = fmt.Sprintf("Overrode base stats (%.4f) using release %s (%.4f)",
					basePassRate, testStats.BaseStats.Release, cTestStats.SuccessRate)
			}
		}
	}
	// Add an explanation for the user why we fell back for the final release data:
	if swappedExplanation != "" {
		testStats.Explanations = append(testStats.Explanations, swappedExplanation)
	}

	return nil
}

func (r *ReleaseFallback) PostAnalysis(testKey crtest.Identification, testStats *testdetails.TestComparison) error {
	return nil
}

func (r *ReleaseFallback) getFallbackBaseQueryStatus(ctx context.Context,
	allJobVariants crtest.JobVariants,
	release string, start, end time.Time) []error {
	generator := newFallbackTestQueryReleasesGenerator(r.dataProvider, r.reqOptions, allJobVariants, release, start, end, r.releaseConfigs)

	cachedFallbackTestStatuses, errs := api.GetDataFromCacheOrGenerate[*FallbackReleases](
		ctx, r.dataProvider.Cache(), r.reqOptions.CacheOption,
		api.NewCacheSpec(generator.getCacheKey(), "FallbackReleases~", &end),
		generator.getTestFallbackReleases,
		&FallbackReleases{})

	if len(errs) > 0 {
		return errs
	}

	r.cachedFallbackTestStatuses = cachedFallbackTestStatuses
	return nil
}

func (r *ReleaseFallback) QueryTestDetails(ctx context.Context, wg *sync.WaitGroup, errCh chan error, allJobVariants crtest.JobVariants) {
	r.log.Infof("Querying fallback override test statuses for %d test ID options", len(r.reqOptions.TestIDOptions))

	// Lookup all release dates, we're going to need them
	timeRanges, errs := r.dataProvider.QueryReleaseDates(ctx, r.reqOptions)
	for _, err := range errs {
		errCh <- err
	}
	if errs != nil {
		return
	}

	// we have an array of TestIdentificationOptions, each of which MAY have a BaseOverrideRelease specified.
	// This was determined from the main report path through this code.
	// We want to do one query per fallback release, for each test ID we fell back to that release for.
	// First we sort each release to map to the tests we fell back to that release for.

	releaseToTestIDOptions := map[string][]reqopts.TestIdentification{}
	for _, testIDOpts := range r.reqOptions.TestIDOptions {
		if testIDOpts.BaseOverrideRelease == "" {
			// no fallback for this regressed test, so this middleware has no work to do
			continue
		}
		if _, ok := releaseToTestIDOptions[testIDOpts.BaseOverrideRelease]; !ok {
			releaseToTestIDOptions[testIDOpts.BaseOverrideRelease] = []reqopts.TestIdentification{}
		}
		releaseToTestIDOptions[testIDOpts.BaseOverrideRelease] = append(releaseToTestIDOptions[testIDOpts.BaseOverrideRelease], testIDOpts)
	}
	r.baseOverrideStatus = map[string]map[string][]crstatus.TestJobRunRows{}

	// Now we'll do one concurrent query for each release that has some fallback tests:
	for release, testIDOpts := range releaseToTestIDOptions {
		r.log.Infof("Querying %d fallback override test statuses for release %s", len(testIDOpts), release)

		start, end, err := utils.FindStartEndTimesForRelease(timeRanges, release)
		if err != nil {
			errCh <- err
			return
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				r.log.Infof("Context canceled while fetching base job run test status")
				return
			default:
				fallbackReqOpts := r.reqOptions
				fallbackReqOpts.BaseRelease.Name = release
				fallbackReqOpts.BaseRelease.Start = *start
				fallbackReqOpts.BaseRelease.End = *end
				fallbackReqOpts.TestIDOptions = testIDOpts

				baseStatus, errs := r.dataProvider.QueryBaseJobRunTestStatus(ctx, fallbackReqOpts, allJobVariants)

				for _, err := range errs {
					errCh <- err
				}

				// Now that we've queried all the results for a fallback release, we need to chop them up into
				// per test -> job -> result rows.

				// We have a struct where the statuses are mapped by prowjob to all rows results for that prowjob,
				// with multiple tests intermingled in that layer.
				// Build out a new struct where these are split up by test ID.
				// split the status on test ID, and pass only that tests data in for reporting:
				r.baseOverrideMutex.Lock()
				for jobName, rows := range baseStatus {
					for _, row := range rows {
						testKeyStr := row.TestKeyStr
						if _, ok := r.baseOverrideStatus[testKeyStr]; !ok {
							r.log.Infof("added test key: " + testKeyStr)
							r.baseOverrideStatus[testKeyStr] = map[string][]crstatus.TestJobRunRows{}
						}
						if r.baseOverrideStatus[testKeyStr][jobName] == nil {
							r.baseOverrideStatus[testKeyStr][jobName] = []crstatus.TestJobRunRows{}
						}
						r.baseOverrideStatus[testKeyStr][jobName] =
							append(r.baseOverrideStatus[testKeyStr][jobName], row)
					}
				}

				r.baseOverrideMutex.Unlock()

				r.log.Infof("queried fallback base override job run test status: %d jobs, %d errors", len(r.baseOverrideStatus), len(errs))
			}
		}()
	}

}

func (r *ReleaseFallback) PreTestDetailsAnalysis(testKey crtest.KeyWithVariants, status *crstatus.TestJobRunStatuses) error {
	// Add our baseOverrideStatus to the report, unfortunate hack we have to live with for now.
	testKeyStr := testKey.KeyOrDie()
	if _, ok := r.baseOverrideStatus[testKeyStr]; ok {
		status.BaseOverrideStatus = r.baseOverrideStatus[testKeyStr]
	}
	return nil
}

func (r *ReleaseFallback) TestDetailsAnalyze(report *testdetails.Report) error {
	return nil
}

// fallbackTestQueryReleasesGenerator iterates the configured number of past releases, querying base status for
// each, which can then be used to return the best basis data from those past releases for comparison.
type fallbackTestQueryReleasesGenerator struct {
	dataProvider               dataprovider.DataProvider
	cacheOption                apiCache.RequestOptions
	allJobVariants             crtest.JobVariants
	BaseRelease                string
	BaseStart                  time.Time
	BaseEnd                    time.Time
	CachedFallbackTestStatuses FallbackReleases
	lock                       *sync.Mutex
	ReqOptions                 reqopts.RequestOptions
	releaseConfigs             []v1.Release
}

func newFallbackTestQueryReleasesGenerator(
	provider dataprovider.DataProvider,
	reqOptions reqopts.RequestOptions,
	allJobVariants crtest.JobVariants,
	release string, start, end time.Time,
	releaseConfigs []v1.Release,
) fallbackTestQueryReleasesGenerator {

	generator := fallbackTestQueryReleasesGenerator{
		dataProvider:   provider,
		cacheOption:    reqOptions.CacheOption,
		allJobVariants: allJobVariants,
		BaseRelease:    release,
		BaseStart:      start,
		BaseEnd:        end,
		lock:           &sync.Mutex{},
		ReqOptions:     reqOptions,
		releaseConfigs: releaseConfigs,
	}
	return generator
}

type fallbackTestQueryReleasesGeneratorCacheKey struct {
	BaseRelease string
	BaseStart   time.Time
	BaseEnd     time.Time
	// VariantDBGroupBy is the only field within VariantOption that is used here
	VariantDBGroupBy sets.String
	// CRTimeRoundingFactor is used by GetReleaseDatesFromBigQuery
	CRTimeRoundingFactor time.Duration
	// KeyTestNames affects the BuildComponentReportQuery results via filtering logic
	KeyTestNames []string
}

// getCacheKey creates a cache key using the generator properties that we want included for uniqueness in what
// we cache. This provides a safer option than using the generator previously which carries some public fields
// which would be serialized and thus cause unnecessary cache misses.
func (f *fallbackTestQueryReleasesGenerator) getCacheKey() fallbackTestQueryReleasesGeneratorCacheKey {
	return fallbackTestQueryReleasesGeneratorCacheKey{
		BaseRelease:          f.BaseRelease,
		BaseStart:            f.BaseStart,
		BaseEnd:              f.BaseEnd,
		VariantDBGroupBy:     f.ReqOptions.VariantOption.DBGroupBy,
		CRTimeRoundingFactor: f.ReqOptions.CacheOption.CRTimeRoundingFactor,
		KeyTestNames:         f.ReqOptions.AdvancedOption.KeyTestNames,
	}
}

func (f *fallbackTestQueryReleasesGenerator) getTestFallbackReleases(ctx context.Context) (*FallbackReleases, []error) {
	wg := sync.WaitGroup{}
	f.CachedFallbackTestStatuses = newFallbackReleases()
	timeRanges, errs := f.dataProvider.QueryReleaseDates(ctx, f.ReqOptions)

	if errs != nil {
		return nil, errs
	}

	// currently gets current base plus previous 3
	// current base is just for testing but use could be
	// extended to no longer require the base query
	selectedTimeRanges := calculateFallbackReleases(f.BaseRelease, timeRanges, f.releaseConfigs)

	for _, crRelease := range selectedTimeRanges {

		start := f.BaseStart
		end := f.BaseEnd

		// we want our base release validation to match the base release report dates
		if crRelease.Release != f.BaseRelease && crRelease.End != nil && crRelease.Start != nil {
			start = *crRelease.Start
			end = *crRelease.End
		}

		wg.Add(1)
		go func(queryRelease crtest.ReleaseTimeRange, queryStart, queryEnd time.Time) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				log.Infof("Context canceled while fetching fallback base query status")
				return
			default:
				stats, errs := f.getTestFallbackRelease(ctx, queryRelease.Release, queryStart, queryEnd)
				if len(errs) > 0 {
					log.Errorf("FallbackBaseQueryStatus for %s failed with: %v", queryRelease, errs)
					return
				}

				f.updateTestStatuses(queryRelease, stats.BaseStatus)
			}
		}(*crRelease, start, end)
	}
	wg.Wait()

	return &f.CachedFallbackTestStatuses, nil
}

func calculateFallbackReleases(startingRelease string, timeRanges []crtest.ReleaseTimeRange, releaseConfigs []v1.Release) []*crtest.ReleaseTimeRange {
	var selectedTimeRanges []*crtest.ReleaseTimeRange
	fallbackRelease := startingRelease

	// Get up to 3 fallback releases
	for i := 0; i < 3; i++ {
		var crRelease *crtest.ReleaseTimeRange

		var err error
		fallbackRelease, err = utils.PreviousRelease(fallbackRelease, releaseConfigs)
		if err != nil {
			log.WithError(err).Errorf("Failure determining fallback release for %s", fallbackRelease)
			break
		}

		for i := range timeRanges {
			if timeRanges[i].Release == fallbackRelease {
				crRelease = &timeRanges[i]
				break
			}
		}

		if crRelease != nil {
			selectedTimeRanges = append(selectedTimeRanges, crRelease)
		}
	}
	return selectedTimeRanges
}

func (f *fallbackTestQueryReleasesGenerator) updateTestStatuses(release crtest.ReleaseTimeRange, updateStatuses map[string]crstatus.TestStatus) {

	var testStatuses ReleaseTestMap
	var ok bool
	// since we  can be called for multiple releases and
	// we update the map below we need to block concurrent map writes
	f.lock.Lock()
	defer f.lock.Unlock()
	if testStatuses, ok = f.CachedFallbackTestStatuses.Releases[release.Release]; !ok {
		testStatuses = ReleaseTestMap{
			ReleaseTimeRange: release,
			Tests:            map[string]crstatus.TestStatus{},
		}
		f.CachedFallbackTestStatuses.Releases[release.Release] = testStatuses
	}

	for key, value := range updateStatuses {
		testStatuses.Tests[key] = value
	}
}

func (f *fallbackTestQueryReleasesGenerator) getTestFallbackRelease(ctx context.Context, release string, start, end time.Time) (crstatus.ReportTestStatus, []error) {
	fallbackReqOpts := f.ReqOptions
	fallbackReqOpts.BaseRelease.Name = release
	fallbackReqOpts.BaseRelease.Start = start
	fallbackReqOpts.BaseRelease.End = end

	baseStatus, errs := f.dataProvider.QueryBaseTestStatus(ctx, fallbackReqOpts, f.allJobVariants)
	if len(errs) > 0 {
		return crstatus.ReportTestStatus{}, errs
	}

	return crstatus.ReportTestStatus{BaseStatus: baseStatus}, nil
}

func newFallbackReleases() FallbackReleases {
	fb := FallbackReleases{
		Releases: map[string]ReleaseTestMap{},
	}
	return fb
}

type ReleaseTestMap struct {
	crtest.ReleaseTimeRange
	Tests map[string]crstatus.TestStatus
}

type FallbackReleases struct {
	Releases map[string]ReleaseTestMap
}

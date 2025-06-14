package releasefallback

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/openshift/sippy/pkg/api/componentreadiness/middleware"
	"github.com/openshift/sippy/pkg/api/componentreadiness/query"
	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/api"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/cache"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
)

const (
	// consider fallback data good for 7 days
	fallbackQueryTimeRoundingOverride = 24 * 7 * time.Hour
)

var _ middleware.Middleware = &ReleaseFallback{}

func NewReleaseFallbackMiddleware(client *bqcachedclient.Client,
	reqOptions crtype.RequestOptions,
) *ReleaseFallback {
	return &ReleaseFallback{
		client:     client,
		log:        log.WithField("middleware", "ReleaseFallback"),
		reqOptions: reqOptions,
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
	client                     *bqcachedclient.Client
	cachedFallbackTestStatuses *crtype.FallbackReleases
	log                        log.FieldLogger
	reqOptions                 crtype.RequestOptions

	// baseOverrideStatus maps test key, to job name, to the result rows for that job.
	// This is used in test details reports, and in the typical API case will only contain one
	// test ID, but when cache priming for a view, we may have multiple.
	baseOverrideStatus map[string]map[string][]crtype.TestJobRunRows
	baseOverrideMutex  sync.Mutex // Mutex to protect the map
}

func (r *ReleaseFallback) Analyze(testID string, variants map[string]string, report *crtype.ReportTestStats) error {
	return nil
}

func (r *ReleaseFallback) Query(ctx context.Context, wg *sync.WaitGroup, allJobVariants crtype.JobVariants,
	_, _ chan map[string]crtype.TestStatus, errCh chan error) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			r.log.Infof("Context canceled while fetching fallback query status")
			return
		default:
			// TODO: should we pass the same wg through rather than using another?
			errs := r.getFallbackBaseQueryStatus(ctx, allJobVariants, r.reqOptions.BaseRelease.Release, r.reqOptions.BaseRelease.Start, r.reqOptions.BaseRelease.End)
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
func (r *ReleaseFallback) PreAnalysis(testKey crtype.ReportTestIdentification, testStats *crtype.ReportTestStats) error {
	// Nothing to do for tests without a basis, i.e. new tests.
	if testStats.BaseStats == nil {
		return nil
	}
	testIDVariantsKey := crtype.TestWithVariantsKey{
		TestID:   testKey.TestID,
		Variants: testKey.Variants,
	}
	testIDBytes, _ := json.Marshal(testIDVariantsKey)
	testKeyStr := string(testIDBytes)

	if !r.reqOptions.AdvancedOption.IncludeMultiReleaseAnalysis {
		// nothing to do if this feature is disabled
		return nil
	}

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
		var cachedReleaseTestStatuses crtype.ReleaseTestMap
		var cTestStatus crtype.TestStatus
		ok := false
		priorRelease, err = utils.PreviousRelease(priorRelease)
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
				testStats.BaseStats = &crtype.TestDetailsReleaseStats{
					Release:              priorRelease,
					Start:                cachedReleaseTestStatuses.Start,
					End:                  cachedReleaseTestStatuses.End,
					TestDetailsTestStats: cTestStats,
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

func (r *ReleaseFallback) PostAnalysis(testKey crtype.ReportTestIdentification, testStats *crtype.ReportTestStats) error {
	return nil
}

func (r *ReleaseFallback) getFallbackBaseQueryStatus(ctx context.Context,
	allJobVariants crtype.JobVariants,
	release string, start, end time.Time) []error {
	generator := newFallbackTestQueryReleasesGenerator(r.client, r.reqOptions, allJobVariants, release, start, end)

	cachedFallbackTestStatuses, errs := api.GetDataFromCacheOrGenerate[*crtype.FallbackReleases](
		ctx, r.client.Cache, generator.cacheOption,
		api.GetPrefixedCacheKey("FallbackReleases~", generator),
		generator.getTestFallbackReleases,
		&crtype.FallbackReleases{})

	if len(errs) > 0 {
		return errs
	}

	r.cachedFallbackTestStatuses = cachedFallbackTestStatuses
	return nil
}

func (r *ReleaseFallback) QueryTestDetails(ctx context.Context, wg *sync.WaitGroup, errCh chan error, allJobVariants crtype.JobVariants) {
	r.log.Infof("Querying fallback override test statuses for %d test ID options", len(r.reqOptions.TestIDOptions))

	// Lookup all release dates, we're going to need them
	releases, errs := query.GetReleaseDatesFromBigQuery(ctx, r.client, r.reqOptions)
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

	releaseToTestIDOptions := map[string][]crtype.RequestTestIdentificationOptions{}
	for _, testIDOpts := range r.reqOptions.TestIDOptions {
		if testIDOpts.BaseOverrideRelease == "" {
			// no fallback for this regressed test, so this middleware has no work to do
			continue
		}
		if _, ok := releaseToTestIDOptions[testIDOpts.BaseOverrideRelease]; !ok {
			releaseToTestIDOptions[testIDOpts.BaseOverrideRelease] = []crtype.RequestTestIdentificationOptions{}
		}
		releaseToTestIDOptions[testIDOpts.BaseOverrideRelease] = append(releaseToTestIDOptions[testIDOpts.BaseOverrideRelease], testIDOpts)
	}
	r.baseOverrideStatus = map[string]map[string][]crtype.TestJobRunRows{}

	// Now we'll do one concurrent bigquery query for each release that has some fallback tests:
	for release, testIDOpts := range releaseToTestIDOptions {
		r.log.Infof("Querying %d fallback override test statuses for release %s", len(testIDOpts), release)

		start, end, err := utils.FindStartEndTimesForRelease(releases, release)
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
				var errs []error
				// We cannot inject our fallback data, rather we will query it, store it internally, and apply it during TransformTestDetails
				generator := query.NewBaseTestDetailsQueryGenerator(
					r.log.WithField("release", release),
					r.client,
					r.reqOptions,
					allJobVariants,
					release,
					*start,
					*end,
					testIDOpts,
				)

				jobRunTestStatus, errs := api.GetDataFromCacheOrGenerate[crtype.TestJobRunStatuses](
					ctx,
					r.client.Cache, r.reqOptions.CacheOption,
					api.GetPrefixedCacheKey("BaseJobRunTestStatus~", generator),
					generator.QueryTestStatus,
					crtype.TestJobRunStatuses{})

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
				for jobName, rows := range jobRunTestStatus.BaseStatus {
					for _, row := range rows {
						testKeyStr := row.TestKeyStr
						if _, ok := r.baseOverrideStatus[testKeyStr]; !ok {
							r.log.Infof("added test key: " + testKeyStr)
							r.baseOverrideStatus[testKeyStr] = map[string][]crtype.TestJobRunRows{}
						}
						if r.baseOverrideStatus[testKeyStr][jobName] == nil {
							r.baseOverrideStatus[testKeyStr][jobName] = []crtype.TestJobRunRows{}
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

func (r *ReleaseFallback) PreTestDetailsAnalysis(testKey crtype.TestWithVariantsKey, status *crtype.TestJobRunStatuses) error {
	// Add our baseOverrideStatus to the report, unfortunate hack we have to live with for now.
	testKeyStr := testKey.KeyOrDie()
	if _, ok := r.baseOverrideStatus[testKeyStr]; ok {
		status.BaseOverrideStatus = r.baseOverrideStatus[testKeyStr]
	}
	return nil
}

func (r *ReleaseFallback) TestDetailsAnalyze(report *crtype.ReportTestDetails) error {
	return nil
}

// fallbackTestQueryReleasesGenerator iterates the configured number of past releases, querying base status for
// each, which can then be used to return the best basis data from those past releases for comparison.
type fallbackTestQueryReleasesGenerator struct {
	client                     *bqcachedclient.Client
	cacheOption                cache.RequestOptions
	allJobVariants             crtype.JobVariants
	BaseRelease                string
	BaseStart                  time.Time
	BaseEnd                    time.Time
	CachedFallbackTestStatuses crtype.FallbackReleases
	lock                       *sync.Mutex
	ReqOptions                 crtype.RequestOptions
}

func newFallbackTestQueryReleasesGenerator(
	client *bqcachedclient.Client,
	reqOptions crtype.RequestOptions,
	allJobVariants crtype.JobVariants,
	release string, start, end time.Time) fallbackTestQueryReleasesGenerator {

	generator := fallbackTestQueryReleasesGenerator{
		client:         client,
		allJobVariants: allJobVariants,
		cacheOption: cache.RequestOptions{
			ForceRefresh: reqOptions.CacheOption.ForceRefresh,
			// increase the time that fallback queries are cached for
			CRTimeRoundingFactor: fallbackQueryTimeRoundingOverride,
		},
		BaseRelease: release,
		BaseStart:   start,
		BaseEnd:     end,
		lock:        &sync.Mutex{},
		ReqOptions:  reqOptions,
	}
	return generator

}

func (f *fallbackTestQueryReleasesGenerator) getTestFallbackReleases(ctx context.Context) (*crtype.FallbackReleases, []error) {
	wg := sync.WaitGroup{}
	f.CachedFallbackTestStatuses = newFallbackReleases()
	releases, errs := query.GetReleaseDatesFromBigQuery(ctx, f.client, f.ReqOptions)

	if errs != nil {
		return nil, errs
	}

	// currently gets current base plus previous 3
	// current base is just for testing but use could be
	// extended to no longer require the base query
	selectedReleases := calculateFallbackReleases(f.BaseRelease, releases)

	for _, crRelease := range selectedReleases {

		start := f.BaseStart
		end := f.BaseEnd

		// we want our base release validation to match the base release report dates
		if crRelease.Release != f.BaseRelease && crRelease.End != nil && crRelease.Start != nil {
			start = *crRelease.Start
			end = *crRelease.End
		}

		wg.Add(1)
		go func(queryRelease crtype.Release, queryStart, queryEnd time.Time) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				log.Infof("Context canceled while fetching fallback base query status")
				return
			default:
				stats, errs := f.getTestFallbackRelease(ctx, f.client, queryRelease.Release, queryStart, queryEnd)
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

func calculateFallbackReleases(startingRelease string, releases []crtype.Release) []*crtype.Release {
	var selectedReleases []*crtype.Release
	fallbackRelease := startingRelease

	// Get up to 3 fallback releases
	for i := 0; i < 3; i++ {
		var crRelease *crtype.Release

		var err error
		fallbackRelease, err = utils.PreviousRelease(fallbackRelease)
		if err != nil {
			log.WithError(err).Errorf("Failure determining fallback release for %s", fallbackRelease)
			break
		}

		for i := range releases {
			if releases[i].Release == fallbackRelease {
				crRelease = &releases[i]
				break
			}
		}

		if crRelease != nil {
			selectedReleases = append(selectedReleases, crRelease)
		}
	}
	return selectedReleases
}

func (f *fallbackTestQueryReleasesGenerator) updateTestStatuses(release crtype.Release, updateStatuses map[string]crtype.TestStatus) {

	var testStatuses crtype.ReleaseTestMap
	var ok bool
	// since we  can be called for multiple releases and
	// we update the map below we need to block concurrent map writes
	f.lock.Lock()
	defer f.lock.Unlock()
	if testStatuses, ok = f.CachedFallbackTestStatuses.Releases[release.Release]; !ok {
		testStatuses = crtype.ReleaseTestMap{Release: release, Tests: map[string]crtype.TestStatus{}}
		f.CachedFallbackTestStatuses.Releases[release.Release] = testStatuses
	}

	for key, value := range updateStatuses {
		testStatuses.Tests[key] = value
	}
}

func (f *fallbackTestQueryReleasesGenerator) getTestFallbackRelease(ctx context.Context, client *bqcachedclient.Client, release string, start, end time.Time) (crtype.ReportTestStatus, []error) {
	generator := newFallbackBaseQueryGenerator(client, f.ReqOptions, f.allJobVariants, release, start, end)
	cacheKey := api.GetPrefixedCacheKey("FallbackBaseTestStatus~", generator)
	testStatuses, errs := api.GetDataFromCacheOrGenerate[crtype.ReportTestStatus](ctx, f.client.Cache, generator.cacheOption, cacheKey, generator.getTestFallbackRelease, crtype.ReportTestStatus{})

	if len(errs) > 0 {
		return crtype.ReportTestStatus{}, errs
	}

	return testStatuses, nil
}

type fallbackTestQueryGenerator struct {
	client      *bqcachedclient.Client
	cacheOption cache.RequestOptions
	allVariants crtype.JobVariants
	BaseRelease string
	BaseStart   time.Time
	BaseEnd     time.Time
	ReqOptions  crtype.RequestOptions
}

func newFallbackBaseQueryGenerator(client *bqcachedclient.Client, reqOptions crtype.RequestOptions, allVariants crtype.JobVariants,
	baseRelease string, baseStart, baseEnd time.Time) fallbackTestQueryGenerator {
	generator := fallbackTestQueryGenerator{
		client:      client,
		allVariants: allVariants,
		ReqOptions:  reqOptions,
		cacheOption: cache.RequestOptions{
			ForceRefresh: reqOptions.CacheOption.ForceRefresh,
			// increase the time that base query is cached for since it shouldn't be changing
			CRTimeRoundingFactor: fallbackQueryTimeRoundingOverride,
		},
		BaseRelease: baseRelease,
		BaseStart:   baseStart,
		BaseEnd:     baseEnd,
	}
	return generator
}

func (f *fallbackTestQueryGenerator) getTestFallbackRelease(ctx context.Context) (crtype.ReportTestStatus, []error) {
	commonQuery, groupByQuery, queryParameters := query.BuildComponentReportQuery(
		f.client,
		f.ReqOptions,
		f.allVariants, f.ReqOptions.VariantOption.IncludeVariants,
		query.DefaultJunitTable, false, true)
	before := time.Now()
	log.Infof("Starting Fallback (%s) QueryTestStatus", f.BaseRelease)
	errs := []error{}
	baseString := commonQuery + ` AND branch = @BaseRelease`
	baseQuery := f.client.BQ.Query(baseString + groupByQuery)

	baseQuery.Parameters = append(baseQuery.Parameters, queryParameters...)
	baseQuery.Parameters = append(baseQuery.Parameters, []bigquery.QueryParameter{
		{
			Name:  "From",
			Value: f.BaseStart,
		},
		{
			Name:  "To",
			Value: f.BaseEnd,
		},
		{
			Name:  "BaseRelease",
			Value: f.BaseRelease,
		},
	}...)

	baseStatus, baseErrs := query.FetchTestStatusResults(ctx, baseQuery)

	if len(baseErrs) != 0 {
		errs = append(errs, baseErrs...)
	}

	log.Infof("Fallback (%s) QueryTestStatus completed in %s with %d base results from db", f.BaseRelease, time.Since(before), len(baseStatus))

	return crtype.ReportTestStatus{BaseStatus: baseStatus}, errs
}

func newFallbackReleases() crtype.FallbackReleases {
	fb := crtype.FallbackReleases{
		Releases: map[string]crtype.ReleaseTestMap{},
	}
	return fb
}

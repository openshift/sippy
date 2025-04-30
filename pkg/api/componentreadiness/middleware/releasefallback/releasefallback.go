package releasefallback

import (
	"context"
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

	baseOverrideStatus map[string][]crtype.JobRunTestStatusRow
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

// Transform iterates the base status looking for any statuses that had a better pass rate in the prior releases
// we queried earlier.
func (r *ReleaseFallback) Transform(status *crtype.ReportTestStatus) error {
	for testKeyStr, baseStats := range status.BaseStatus {
		newBaseStatus := r.matchBestBaseStats(testKeyStr, r.reqOptions.BaseRelease.Release, baseStats)
		if newBaseStatus.Release != nil && newBaseStatus.Release.Release != r.reqOptions.BaseRelease.Release {
			status.BaseStatus[testKeyStr] = newBaseStatus
		}
	}

	return nil
}

// matchBestBaseStats returns the testStatus, release and reportTestStatus
// that has the highest threshold across the basis release and previous releases included
// in fallback comparison.
func (r *ReleaseFallback) matchBestBaseStats(
	testKeyStr, baseRelease string,
	baseStats crtype.TestStatus) crtype.TestStatus {

	if !r.reqOptions.AdvancedOption.IncludeMultiReleaseAnalysis {
		return baseStats
	}

	if r.cachedFallbackTestStatuses == nil {
		r.log.Errorf("Invalid fallback test statuses")
		return baseStats
	}

	var priorRelease = baseRelease
	var err error
	for err == nil {
		var cachedReleaseTestStatuses crtype.ReleaseTestMap
		var cTestStats crtype.TestStatus
		ok := false
		priorRelease, err = utils.PreviousRelease(priorRelease)
		// if we fail to determine the previous release then stop
		if err != nil {
			return baseStats
		}

		// if we hit a missing release then stop
		if cachedReleaseTestStatuses, ok = r.cachedFallbackTestStatuses.Releases[priorRelease]; !ok {
			return baseStats
		}

		// it's ok if we don't have a testKeyStr for this release
		// we likely won't have it for earlier releases either, but we can keep going
		if cTestStats, ok = cachedReleaseTestStatuses.Tests[testKeyStr]; ok {

			// what is our base total compared to the original base
			// this happens when jobs shift like sdn -> ovn
			// if we get below threshold that's a sign we are reducing our base signal
			if float64(cTestStats.TotalCount)/float64(baseStats.TotalCount) < .6 {
				r.log.Debugf("Fallback base total: %d to low for fallback analysis compared to original: %d",
					cTestStats.TotalCount, baseStats.TotalCount)
				return baseStats
			}
			_, success, fail, flake := baseStats.GetTotalSuccessFailFlakeCounts()
			basePassRate := utils.CalculatePassRate(r.reqOptions, success, fail, flake)

			_, success, fail, flake = cTestStats.GetTotalSuccessFailFlakeCounts()
			cPassRate := utils.CalculatePassRate(r.reqOptions, success, fail, flake)
			if cPassRate > basePassRate {
				baseStats = cTestStats
				// If we swapped out base stats for better ones from a prior release, we need to communicate
				// this back to the core report generator so it can include the adjusted release/start/end dates in
				// the report, and ultimately the UI.
				baseStats.Release = &cachedReleaseTestStatuses.Release
				r.log.Debugf("Overrode base stats (%.4f) using release %s (%.4f) for test: %s - %s",
					basePassRate, baseStats.Release.Release, cPassRate, baseStats.TestName, testKeyStr)
			}
		}
	}

	return baseStats
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
	r.log.Infof("Querying fallback override test statuses")
	if r.reqOptions.BaseOverrideRelease.Release != "" && r.reqOptions.BaseOverrideRelease.Release != r.reqOptions.BaseRelease.Release {
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
					r.client,
					r.reqOptions,
					allJobVariants,
					r.reqOptions.BaseOverrideRelease.Release,
					r.reqOptions.BaseOverrideRelease.Start,
					r.reqOptions.BaseOverrideRelease.End,
				)

				jobRunTestStatus, errs := api.GetDataFromCacheOrGenerate[crtype.JobRunTestReportStatus](
					ctx,
					r.client.Cache, r.reqOptions.CacheOption,
					api.GetPrefixedCacheKey("BaseJobRunTestStatus~", generator),
					generator.QueryTestStatus,
					crtype.JobRunTestReportStatus{})

				for _, err := range errs {
					errCh <- err
				}
				r.baseOverrideStatus = jobRunTestStatus.BaseStatus

				r.log.Infof("queried fallback base override job run test status: %d jobs, %d errors", len(r.baseOverrideStatus), len(errs))
			}
		}()
	}

}

func (r *ReleaseFallback) TransformTestDetails(status *crtype.JobRunTestReportStatus) error {
	// Simply attach the override base stats to the status
	r.log.Infof("Transforming fallback test details")
	status.BaseOverrideStatus = r.baseOverrideStatus
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
	var selectedReleases []*crtype.Release
	fallbackRelease := f.BaseRelease

	// Get up to 3 fallback releases
	for i := 0; i < 3; i++ {
		var crRelease *crtype.Release

		fallbackRelease, err := utils.PreviousRelease(fallbackRelease)
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

	for _, crRelease := range selectedReleases {

		start := f.BaseStart
		end := f.BaseEnd

		// we want our base release validation to match the base release report dates
		if crRelease.Release != f.BaseRelease && crRelease.End != nil && crRelease.Start != nil {
			end = *crRelease.End
			start = *crRelease.Start
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

	testStatuses, errs := api.GetDataFromCacheOrGenerate[crtype.ReportTestStatus](ctx, f.client.Cache, generator.cacheOption, api.GetPrefixedCacheKey("FallbackBaseTestStatus~", generator), generator.getTestFallbackRelease, crtype.ReportTestStatus{})

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
	commonQuery, groupByQuery, queryParameters := query.BuildCommonTestStatusQuery(
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

package releasefallback

import (
	"context"
	"sync"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/openshift/sippy/pkg/api/componentreadiness"
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

//var _ middleware.Middleware = ReleaseFallback{}

func NewReleaseFallbackMiddleware(client *bqcachedclient.Client,
	cacheOption cache.RequestOptions,
	logger log.FieldLogger,
	c *componentreadiness.ComponentReportGenerator,
	allJobVariants crtype.JobVariants) *ReleaseFallback {
	return &ReleaseFallback{
		client:         client,
		cacheOption:    cacheOption,
		c:              c,
		log:            logger,
		allJobVariants: allJobVariants,
	}
}

// ReleaseFallback middleware allows us to use the best pass rate data from the past
// several releases for our basis instead of the requested basis.
//
// It is responsible for querying basis test status for those several releases, and
// then replacing any basis test stats with a better releases test stats, when appropriate.
// This is done when we have sufficient test coverage, and a better pass rate.
type ReleaseFallback struct {
	client                     *bqcachedclient.Client
	cachedFallbackTestStatuses *crtype.FallbackReleases
	log                        log.FieldLogger
	c                          *componentreadiness.ComponentReportGenerator
	allJobVariants             crtype.JobVariants
	cacheOption                cache.RequestOptions
}

func (r *ReleaseFallback) Query(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			r.log.Infof("Context canceled while fetching fallback query status")
			return
		default:
			r.getFallbackBaseQueryStatus(ctx, r.allJobVariants, r.c.BaseRelease.Release, r.c.BaseRelease.Start, r.c.BaseRelease.End)
		}
	}()
	return
}

func (r *ReleaseFallback) getFallbackBaseQueryStatus(ctx context.Context,
	allJobVariants crtype.JobVariants,
	release string, start, end time.Time) []error {
	generator := newFallbackTestQueryReleasesGenerator(r.client, r.cacheOption, r.c, allJobVariants, release, start, end)

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

// fallbackTestQueryReleasesGenerator iterates the configured number of past releases, querying base status for
// each, which can then be used to return the best basis data from those past releases for comparison.
type fallbackTestQueryReleasesGenerator struct {
	client                     *bqcachedclient.Client
	cacheOption                cache.RequestOptions
	allVariants                crtype.JobVariants
	BaseRelease                string
	BaseStart                  time.Time
	BaseEnd                    time.Time
	CachedFallbackTestStatuses crtype.FallbackReleases
	lock                       *sync.Mutex
	ComponentReportGenerator   *componentreadiness.ComponentReportGenerator
}

func newFallbackTestQueryReleasesGenerator(
	client *bqcachedclient.Client,
	cacheOption cache.RequestOptions,
	c *componentreadiness.ComponentReportGenerator,
	allVariants crtype.JobVariants,
	release string, start, end time.Time) fallbackTestQueryReleasesGenerator {

	generator := fallbackTestQueryReleasesGenerator{
		client:      client,
		allVariants: allVariants,
		cacheOption: cache.RequestOptions{
			ForceRefresh: cacheOption.ForceRefresh,
			// increase the time that fallback queries are cached for
			CRTimeRoundingFactor: fallbackQueryTimeRoundingOverride,
		},
		BaseRelease:              release,
		BaseStart:                start,
		BaseEnd:                  end,
		lock:                     &sync.Mutex{},
		ComponentReportGenerator: c,
	}
	return generator

}

func (f *fallbackTestQueryReleasesGenerator) getTestFallbackReleases(ctx context.Context) (*crtype.FallbackReleases, []error) {
	wg := sync.WaitGroup{}
	f.CachedFallbackTestStatuses = newFallbackReleases()
	releases, errs := componentreadiness.GetReleaseDatesFromBigQuery(ctx, f.client)

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

		fallbackRelease, err := componentreadiness.PreviousRelease(fallbackRelease)
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

func (f *fallbackTestQueryReleasesGenerator) getTestFallbackRelease(ctx context.Context,
	client *bqcachedclient.Client, cacheOption cache.RequestOptions,
	release string, start, end time.Time) (crtype.ReportTestStatus, []error) {
	generator := newFallbackBaseQueryGenerator(client, cacheOption, f.ComponentReportGenerator, f.allVariants, release, start, end)

	testStatuses, errs := api.GetDataFromCacheOrGenerate[crtype.ReportTestStatus](ctx, f.client.Cache, generator.cacheOption, api.GetPrefixedCacheKey("FallbackBaseTestStatus~", generator), generator.getTestFallbackRelease, crtype.ReportTestStatus{})

	if len(errs) > 0 {
		return crtype.ReportTestStatus{}, errs
	}

	return testStatuses, nil
}

type fallbackTestQueryGenerator struct {
	client                   *bqcachedclient.Client
	cacheOption              cache.RequestOptions
	allVariants              crtype.JobVariants
	BaseRelease              string
	BaseStart                time.Time
	BaseEnd                  time.Time
	ComponentReportGenerator *componentreadiness.ComponentReportGenerator
}

func newFallbackBaseQueryGenerator(client *bqcachedclient.Client, cacheOption cache.RequestOptions, c *componentreadiness.ComponentReportGenerator, allVariants crtype.JobVariants,
	baseRelease string, baseStart, baseEnd time.Time) fallbackTestQueryGenerator {
	generator := fallbackTestQueryGenerator{
		ComponentReportGenerator: c,
		client:                   client,
		allVariants:              allVariants,
		cacheOption: cache.RequestOptions{
			ForceRefresh: cacheOption.ForceRefresh,
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
	commonQuery, groupByQuery, queryParameters := componentreadiness.BuildCommonTestStatusQuery(
		f.ComponentReportGenerator,
		f.allVariants, f.ComponentReportGenerator.IncludeVariants,
		componentreadiness.DefaultJunitTable, false, true)
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

	baseStatus, baseErrs := fetchTestStatusResults(ctx, baseQuery)

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

package componentreadiness

import (
	"context"
	"sync"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/openshift/sippy/pkg/api"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/cache"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	log "github.com/sirupsen/logrus"
)

type baseQueryGenerator struct {
	client                   *bqcachedclient.Client
	cacheOption              cache.RequestOptions
	commonQuery              string
	groupByQuery             string
	queryParameters          []bigquery.QueryParameter
	ComponentReportGenerator *componentReportGenerator
}

func (b *baseQueryGenerator) queryTestStatus(ctx context.Context) (crtype.ReportTestStatus, []error) {
	before := time.Now()
	errs := []error{}
	baseString := b.commonQuery + ` AND branch = @BaseRelease`
	baseQuery := b.client.BQ.Query(baseString + b.groupByQuery)

	baseQuery.Parameters = append(baseQuery.Parameters, b.queryParameters...)
	baseQuery.Parameters = append(baseQuery.Parameters, []bigquery.QueryParameter{
		{
			Name:  "From",
			Value: b.ComponentReportGenerator.BaseRelease.Start,
		},
		{
			Name:  "To",
			Value: b.ComponentReportGenerator.BaseRelease.End,
		},
		{
			Name:  "BaseRelease",
			Value: b.ComponentReportGenerator.BaseRelease.Release,
		},
	}...)

	baseStatus, baseErrs := fetchTestStatus(ctx, baseQuery)

	if len(baseErrs) != 0 {
		errs = append(errs, baseErrs...)
	}

	log.Infof("Base QueryTestStatus completed in %s with %d base results from db", time.Since(before), len(baseStatus))

	return crtype.ReportTestStatus{BaseStatus: baseStatus}, errs
}

type sampleQueryGenerator struct {
	client                   *bqcachedclient.Client
	commonQuery              string
	groupByQuery             string
	queryParameters          []bigquery.QueryParameter
	ComponentReportGenerator *componentReportGenerator
}

// getSampleQueryStatus builds the sample query, executes it, and returns the sample test status.
func (c *componentReportGenerator) getSampleQueryStatus(
	ctx context.Context, allJobVariants crtype.JobVariants) (map[string]crtype.TestStatus, []error) {
	commonQuery, groupByQuery, queryParameters := c.getCommonTestStatusQuery(allJobVariants, true, false)
	generator := sampleQueryGenerator{
		client:                   c.client,
		commonQuery:              commonQuery,
		groupByQuery:             groupByQuery,
		queryParameters:          queryParameters,
		ComponentReportGenerator: c,
	}

	componentReportTestStatus, errs := api.GetDataFromCacheOrGenerate[crtype.ReportTestStatus](ctx,
		c.client.Cache, c.cacheOption,
		api.GetPrefixedCacheKey("SampleTestStatus~", generator),
		generator.queryTestStatus, crtype.ReportTestStatus{})

	if len(errs) > 0 {
		return nil, errs
	}

	return componentReportTestStatus.SampleStatus, nil
}

func (s *sampleQueryGenerator) queryTestStatus(ctx context.Context) (crtype.ReportTestStatus, []error) {
	before := time.Now()
	errs := []error{}
	sampleString := s.commonQuery + ` AND branch = @SampleRelease`
	if s.ComponentReportGenerator.SampleRelease.PullRequestOptions != nil {
		sampleString += `  AND org = @Org AND repo = @Repo AND pr_number = @PRNumber`
	}
	sampleQuery := s.client.BQ.Query(sampleString + s.groupByQuery)
	sampleQuery.Parameters = append(sampleQuery.Parameters, s.queryParameters...)
	sampleQuery.Parameters = append(sampleQuery.Parameters, []bigquery.QueryParameter{
		{
			Name:  "From",
			Value: s.ComponentReportGenerator.SampleRelease.Start,
		},
		{
			Name:  "To",
			Value: s.ComponentReportGenerator.SampleRelease.End,
		},
		{
			Name:  "SampleRelease",
			Value: s.ComponentReportGenerator.SampleRelease.Release,
		},
	}...)
	if s.ComponentReportGenerator.SampleRelease.PullRequestOptions != nil {
		sampleQuery.Parameters = append(sampleQuery.Parameters, []bigquery.QueryParameter{
			{
				Name:  "Org",
				Value: s.ComponentReportGenerator.SampleRelease.PullRequestOptions.Org,
			},
			{
				Name:  "Repo",
				Value: s.ComponentReportGenerator.SampleRelease.PullRequestOptions.Repo,
			},
			{
				Name:  "PRNumber",
				Value: s.ComponentReportGenerator.SampleRelease.PullRequestOptions.PRNumber,
			},
		}...)
	}

	sampleStatus, sampleErrs := fetchTestStatus(ctx, sampleQuery)

	if len(sampleErrs) != 0 {
		errs = append(errs, sampleErrs...)
	}

	log.Infof("Sample QueryTestStatus completed in %s with %d sample results db", time.Since(before), len(sampleStatus))

	return crtype.ReportTestStatus{SampleStatus: sampleStatus}, errs
}

type fallbackTestQueryReleasesGenerator struct {
	client                     *bqcachedclient.Client
	cacheOption                cache.RequestOptions
	commonQuery                string
	groupByQuery               string
	queryParameters            []bigquery.QueryParameter
	allJobVariants             crtype.JobVariants
	BaseRelease                string
	BaseStart                  time.Time
	BaseEnd                    time.Time
	CachedFallbackTestStatuses crtype.FallbackReleases
	lock                       *sync.Mutex
}

func (f *fallbackTestQueryReleasesGenerator) getTestFallbackReleases(ctx context.Context) (*crtype.FallbackReleases, []error) {
	wg := sync.WaitGroup{}
	f.CachedFallbackTestStatuses = newFallbackReleases()
	releases, errs := GetReleaseDatesFromBigQuery(ctx, f.client)

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

		fallbackRelease, err := previousRelease(fallbackRelease)
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

type fallbackTestQueryGenerator struct {
	client          *bqcachedclient.Client
	cacheOption     cache.RequestOptions
	commonQuery     string
	groupByQuery    string
	queryParameters []bigquery.QueryParameter
	allJobVariants  crtype.JobVariants
	BaseRelease     string
	BaseStart       time.Time
	BaseEnd         time.Time
}

func (f *fallbackTestQueryReleasesGenerator) getTestFallbackRelease(ctx context.Context, release string, start, end time.Time) (crtype.ReportTestStatus, []error) {
	generator := fallbackTestQueryGenerator{
		client: f.client,
		cacheOption: cache.RequestOptions{
			ForceRefresh: f.cacheOption.ForceRefresh,
			// increase the time that base query is cached for since it shouldn't be changing
			CRTimeRoundingFactor: fallbackQueryTimeRoundingOverride,
		},
		commonQuery:     f.commonQuery,
		groupByQuery:    f.groupByQuery,
		BaseRelease:     release,
		BaseStart:       start,
		BaseEnd:         end,
		queryParameters: f.queryParameters,
	}

	testStatuses, errs := api.GetDataFromCacheOrGenerate[crtype.ReportTestStatus](ctx, f.client.Cache, generator.cacheOption, api.GetPrefixedCacheKey("FallbackBaseTestStatus~", generator), generator.getTestFallbackRelease, crtype.ReportTestStatus{})

	if len(errs) > 0 {
		return crtype.ReportTestStatus{}, errs
	}

	return testStatuses, nil
}

func (f *fallbackTestQueryGenerator) getTestFallbackRelease(ctx context.Context) (crtype.ReportTestStatus, []error) {
	return f.getFallbackBaseQueryStatus(ctx, f.BaseRelease, f.BaseStart, f.BaseEnd)
}

func (f *fallbackTestQueryGenerator) getFallbackBaseQueryStatus(ctx context.Context, release string, start, end time.Time) (crtype.ReportTestStatus, []error) {
	before := time.Now()
	log.Infof("Starting Fallback (%s) QueryTestStatus", release)
	errs := []error{}
	baseString := f.commonQuery + ` AND branch = @BaseRelease`
	baseQuery := f.client.BQ.Query(baseString + f.groupByQuery)

	baseQuery.Parameters = append(baseQuery.Parameters, f.queryParameters...)
	baseQuery.Parameters = append(baseQuery.Parameters, []bigquery.QueryParameter{
		{
			Name:  "From",
			Value: start,
		},
		{
			Name:  "To",
			Value: end,
		},
		{
			Name:  "BaseRelease",
			Value: release,
		},
	}...)

	baseStatus, baseErrs := fetchTestStatus(ctx, baseQuery)

	if len(baseErrs) != 0 {
		errs = append(errs, baseErrs...)
	}

	log.Infof("Fallback (%s) QueryTestStatus completed in %s with %d base results from db", release, time.Since(before), len(baseStatus))

	return crtype.ReportTestStatus{BaseStatus: baseStatus}, errs
}

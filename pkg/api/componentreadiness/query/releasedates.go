package query

import (
	"context"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/util"
)

func GetReleaseDatesFromBigQuery(ctx context.Context, client *bigquery.Client, reqOptions reqopts.RequestOptions) ([]crtest.ReleaseTimeRange, []error) {
	queries := &releaseDateQuerier{client: client, reqOptions: reqOptions}
	return api.GetDataFromCacheOrGenerate[[]crtest.ReleaseTimeRange](ctx,
		client.Cache,
		cache.RequestOptions{},
		api.GetPrefixedCacheKey("CRReleaseDates~", reqOptions), // global singleton instance
		queries.QueryReleaseDates, []crtest.ReleaseTimeRange{})
}

type releaseDateQuerier struct {
	client     *bigquery.Client
	reqOptions reqopts.RequestOptions
}

func (c *releaseDateQuerier) QueryReleaseDates(ctx context.Context) ([]crtest.ReleaseTimeRange, []error) {
	releases, err := api.GetReleasesFromBigQuery(ctx, c.client)
	if err != nil {
		return nil, []error{err}
	}
	timeRanges := []crtest.ReleaseTimeRange{}
	for _, release := range releases {
		timeRange := crtest.ReleaseTimeRange{Release: release.Release}
		if release.GADate != nil {
			prior := util.AdjustReleaseTime(*release.GADate, true, "30", c.reqOptions.CacheOption.CRTimeRoundingFactor)
			timeRange.Start = &prior
			timeRange.End = release.GADate
		}
		timeRanges = append(timeRanges, timeRange)
	}
	return timeRanges, nil
}

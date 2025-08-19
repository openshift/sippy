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

func GetReleaseDatesFromBigQuery(ctx context.Context, client *bigquery.Client, reqOptions reqopts.RequestOptions) ([]crtest.Release, []error) {
	queries := &releaseDateQuerier{client: client, reqOptions: reqOptions}
	return api.GetDataFromCacheOrGenerate[[]crtest.Release](ctx,
		client.Cache,
		cache.RequestOptions{},
		api.GetPrefixedCacheKey("CRReleaseDates~", crtest.Release{}), // global singleton instance
		queries.QueryReleaseDates, []crtest.Release{})
}

type releaseDateQuerier struct {
	client     *bigquery.Client
	reqOptions reqopts.RequestOptions
}

func (c *releaseDateQuerier) QueryReleaseDates(ctx context.Context) ([]crtest.Release, []error) {
	releases, err := api.GetReleasesFromBigQuery(ctx, c.client)
	if err != nil {
		return nil, []error{err}
	}
	crReleases := []crtest.Release{}
	for _, release := range releases {
		crRelease := crtest.Release{Release: release.Release}
		if release.GADate != nil {
			prior := util.AdjustReleaseTime(*release.GADate, true, "30", c.reqOptions.CacheOption.CRTimeRoundingFactor)
			crRelease.Start = &prior
			crRelease.End = release.GADate
		}
		crReleases = append(crReleases, crRelease)
	}
	return crReleases, nil
}

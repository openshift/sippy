package query

import (
	"context"

	"github.com/openshift/sippy/pkg/api"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/util"
)

func GetReleaseDatesFromBigQuery(ctx context.Context, client *bigquery.Client, reqOptions reqopts.RequestOptions) ([]crtype.Release, []error) {
	queries := &releaseDateQuerier{client: client, reqOptions: reqOptions}
	return api.GetDataFromCacheOrGenerate[[]crtype.Release](ctx,
		client.Cache,
		cache.RequestOptions{},
		api.GetPrefixedCacheKey("CRReleaseDates~", reqOptions),
		queries.QueryReleaseDates, []crtype.Release{})
}

type releaseDateQuerier struct {
	client     *bigquery.Client
	reqOptions reqopts.RequestOptions
}

func (c *releaseDateQuerier) QueryReleaseDates(ctx context.Context) ([]crtype.Release, []error) {
	releases, err := api.GetReleasesFromBigQuery(ctx, c.client)
	if err != nil {
		return nil, []error{err}
	}
	crReleases := []crtype.Release{}
	for _, release := range releases {
		crRelease := crtype.Release{Release: release.Release}
		if release.GADate != nil {
			prior := util.AdjustReleaseTime(*release.GADate, true, "30", c.reqOptions.CacheOption.CRTimeRoundingFactor)
			crRelease.Start = &prior
			crRelease.End = release.GADate
		}
		crReleases = append(crReleases, crRelease)
	}
	return crReleases, nil
}

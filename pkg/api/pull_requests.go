package api

import (
	"context"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/filter"
)

func GetPullRequestsReportFromDB(ctx context.Context, dbc *db.DB, release string, filterOpts *filter.FilterOptions) ([]apitype.PullRequest, error) {
	return query.PullRequestReport(ctx, dbc, filterOpts, release)
}

func GetPayloadDiffPullRequests(ctx context.Context, dbc *db.DB, fromPayload, toPayload string) ([]models.ReleasePullRequest, error) {
	return query.GetPayloadDiff(ctx, dbc.DB, fromPayload, toPayload)
}

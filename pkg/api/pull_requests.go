package api

import (
	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/filter"
)

func GetPullRequestsReportFromDB(dbc *db.DB, release string, filterOpts *filter.FilterOptions) ([]apitype.PullRequest, error) {
	return query.PullRequestReport(dbc, filterOpts, release)
}

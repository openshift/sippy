package api

import (
	"context"
	"time"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/filter"
)

func GetRepositoriesReportFromDB(ctx context.Context, dbc *db.DB, release string, filterOpts *filter.FilterOptions, reportEnd time.Time) ([]apitype.Repository, error) {
	return query.RepositoryReport(ctx, dbc, filterOpts, release, reportEnd)
}

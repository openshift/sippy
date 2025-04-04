package api

import (
	"time"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/filter"
)

func GetRepositoriesReportFromDB(dbc *db.DB, release string, filterOpts *filter.FilterOptions, reportEnd time.Time) ([]apitype.Repository, error) {
	return query.RepositoryReport(dbc, filterOpts, release, reportEnd)
}

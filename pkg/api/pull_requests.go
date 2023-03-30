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

type PullRequestPayloadHistory struct {
	Release     string     `json:"release"`
	Stream      string     `json:"stream"`
	ReleaseTags [][]string `json:"release_tags"`
}

func GetPullRequestPayloadHistory(dbc *db.DB, pullRequestID int) ([]PullRequestPayloadHistory, error) {
	var url string
	q := dbc.DB.Table("prow_pull_requests").Where("id = ?", pullRequestID).Pluck("url", &url)
	if q.Error != nil {
		return nil, q.Error
	}

	results := make([]PullRequestPayloadHistory, 0)
	q = dbc.DB.Table("release_pull_requests pr").
		Select(`release,
		stream,
		architecture,
		url,
		json_agg(json_build_array(release_tag, phase)
	ORDER BY release_tag DESC) AS release_tags`).
		Where("url = ?", url).
		Joins("JOIN release_tag_pull_requests rtpr ON rtpr.release_pull_request_id = pr.id").
		Joins("INNER JOIN release_tags rt on rt.id = rtpr.release_tag_id").Group("release, stream, architecture, url")
	q = q.Scan(&results)

	return results, q.Error
}

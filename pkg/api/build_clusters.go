package api

import (
	"time"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/query"
)

func GetBuildClusterHealthReport(dbc *db.DB, start, boundary, end time.Time) ([]apitype.BuildClusterHealth, error) {
	results, err := query.BuildClusterHealth(dbc, start, boundary, end)
	return results, err
}

func GetBuildClusterHealthAnalysis(dbc *db.DB, period string) (map[string]apitype.BuildClusterHealthAnalysis, error) {
	results := make(map[string]apitype.BuildClusterHealthAnalysis, 0)

	health, err := query.BuildClusterAnalysis(dbc, period)
	if err != nil {
		return nil, err
	}

	var formatter string
	if period == PeriodDay {
		formatter = "2006-01-02"
	} else {
		formatter = "2006-01-02 15:00"
	}

	for _, item := range health {
		if _, ok := results[item.Cluster]; !ok {
			results[item.Cluster] = apitype.BuildClusterHealthAnalysis{
				ByPeriod: make(map[string]apitype.BuildClusterHealth),
			}
		}
		key := item.Period.UTC().Format(formatter)
		results[item.Cluster].ByPeriod[key] = apitype.BuildClusterHealth{
			CurrentRuns:           item.TotalRuns,
			CurrentPasses:         item.Passes,
			CurrentFails:          item.Failures,
			CurrentPassPercentage: item.PassPercentage,
		}
	}

	return results, nil
}

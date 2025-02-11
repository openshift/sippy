package query

import (
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/filter"
)

func GetFeatureGatesFromDB(dbc *gorm.DB, release string, filterOpts *filter.FilterOptions) ([]api.FeatureGate, error) {
	subQuery := dbc.Table("prow_test_report_7d_matview").
		Select("name, release, regexp_matches(name, '\\[(FeatureGate|OCPFeatureGate):([^\\]]+)\\]') AS match").
		Where("release = ?", release)

	query := dbc.Table("feature_gates AS fg").
		Select(`
			ROW_NUMBER() OVER (ORDER BY fg.feature_gate) AS id,
			fg.feature_gate,
			fg.release,
			COUNT(DISTINCT mt.name) AS unique_test_count,
			ARRAY_AGG(DISTINCT fg.feature_set || ':' || fg.topology) AS enabled
		`).
		Joins("LEFT JOIN (?) AS mt ON fg.feature_gate = mt.match[2]", subQuery).
		Where("fg.release = ? AND fg.status = 'enabled'", release).
		Group("fg.feature_gate, fg.release").
		Order("fg.feature_gate")

	table := dbc.Table("(?) AS results", query)

	q, err := filter.FilterableDBResult(table, filterOpts, api.FeatureGate{})
	if err != nil {
		return nil, err
	}

	results := make([]api.FeatureGate, 0)
	tx := q.Scan(&results)
	if tx.Error != nil {
		return nil, tx.Error
	}

	return results, nil
}

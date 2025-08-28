package query

import (
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/filter"
)

func GetFeatureGatesFromDB(dbc *gorm.DB, release string, filterOpts *filter.FilterOptions) ([]api.FeatureGate, error) {
	// Get tests by feature gate
	// Install related FG is special and is covered by install should succeed case.
	subQuery := dbc.Table("prow_test_report_7d_matview").
		Select(`name, release, regexp_matches(name, '\[(FeatureGate|OCPFeatureGate):([^\]]+)\]|install should succeed') AS match`).
		Where("release = ?", release)

	// Figure out the first release we ever saw a FG
	firstSeenQuery := dbc.Table("feature_gates").
		Select(`
			feature_gate,
			MIN(release) OVER (
				PARTITION BY feature_gate 
				ORDER BY string_to_array(release, '.')::int[] ASC
			) AS first_seen_in,
			CAST((string_to_array(MIN(release) OVER (PARTITION BY feature_gate), '.'))[1] AS INT) AS first_seen_in_major,
			CAST((string_to_array(MIN(release) OVER (PARTITION BY feature_gate), '.'))[2] AS INT) AS first_seen_in_minor
		`).
		Where("status = 'enabled'")

	query := dbc.Table("feature_gates AS fg").
		Select(`
			ROW_NUMBER() OVER (ORDER BY fg.feature_gate) AS id,
			fg.feature_gate,
			fg.release,
			fs.first_seen_in,
			fs.first_seen_in_major,
			fs.first_seen_in_minor,
			COUNT(DISTINCT mt.name) AS unique_test_count,
			ARRAY_AGG(DISTINCT fg.feature_set || ':' || fg.topology) AS enabled
		`).
		Joins("LEFT JOIN (?) AS mt ON fg.feature_gate = mt.match[2] OR (fg.feature_gate LIKE '%Install%' AND name LIKE '%install should succeed%')", subQuery).
		Joins("LEFT JOIN (?) AS fs ON fg.feature_gate = fs.feature_gate", firstSeenQuery).
		Where("fg.release = ? AND fg.status = 'enabled'", release).
		Group("fg.feature_gate, fg.release, fs.first_seen_in, fs.first_seen_in_major, fs.first_seen_in_minor").
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

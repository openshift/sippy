package query

import (
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/apis/api"
)

func GetFeatureGatesFromDB(dbc *gorm.DB, release string) ([]api.FeatureGate, error) {
	// Define the query with release filtering
	query := `WITH matched_tests AS (
				SELECT
					name,
					release,
					regexp_matches(name, '\[(FeatureGate|OCPFeatureGate):([^\]]+)\]') AS match
				FROM prow_test_report_7d_matview
				WHERE release = ? 
			)
			SELECT
				ROW_NUMBER() OVER (ORDER BY fg.feature_gate) AS id,
				fg.feature_gate,
				fg.release,
				COUNT(DISTINCT mt.name) AS unique_test_count,
				ARRAY_AGG(DISTINCT fg.feature_set || ':' || fg.topology) AS enabled
			FROM feature_gates fg
			LEFT JOIN matched_tests mt
				ON fg.feature_gate = mt.match[2]
			WHERE fg.release = ?
			AND fg.status = 'enabled'
			GROUP BY fg.feature_gate, fg.release
			ORDER BY fg.feature_gate;
`

	results := make([]api.FeatureGate, 0)
	// Execute the query, passing in the release filter
	if err := dbc.Raw(query, release, release).Scan(&results).Error; err != nil {
		return nil, err
	}

	return results, nil
}

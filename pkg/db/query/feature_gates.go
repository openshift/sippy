package query

import (
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/apis/api"
)

func GetFeatureGatesFromDB(dbc *gorm.DB, release string) ([]api.FeatureGate, error) {
	// Define the query with release filtering
	query := `
		SELECT
			ROW_NUMBER() OVER (ORDER BY release, match[1], match[2]) AS id,
			match[1] AS type,
			match[2] AS feature_gate,
			release,
			COUNT(DISTINCT name) AS unique_test_count
		FROM (
			SELECT 
				regexp_matches(name, '\[(FeatureGate|OCPFeatureGate):([^\]]+)\]', 'g') AS match,
				release,
				name
			FROM prow_test_report_7d_matview
			WHERE release = ?
		) subquery
		WHERE match IS NOT NULL
		GROUP BY match[1], match[2], release
		ORDER BY release, type, feature_gate
	`

	var results []api.FeatureGate
	// Execute the query, passing in the release filter
	if err := dbc.Raw(query, release).Scan(&results).Error; err != nil {
		return nil, err
	}

	return results, nil
}

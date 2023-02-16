package query

import (
	"fmt"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/testidentification"
	"github.com/openshift/sippy/pkg/util/sets"
)

// PlatformInfraSuccess takes a list of platforms and a period (default
// or twoDay), and returns a map containing keys for platform, and infra
// success percentage for that period.
func PlatformInfraSuccess(dbc *db.DB, platforms sets.String, period string) (map[string]float64, error) {
	results := make(map[string]float64)

	table := ""
	switch period {
	case "default":
		table = "prow_test_report_7d_matview"
	case "twoDay":
		table = "prow_test_report_2d_matview"
	default:
		return nil, fmt.Errorf("unknown period %s", period)
	}

	raw := dbc.DB.Table(table).
		Select("*, unnest(variants) as variant").
		Where("name = ?", testidentification.NewInfrastructureTestName)

	var sqlResults []struct {
		Variant        string
		PassPercentage float64
	}
	q := dbc.DB.Table("(?) as results", raw).
		Select(`
			variant,
			SUM(current_successes) * 100.0 / NULLIF(SUM(current_runs), 0) AS pass_percentage`).
		Where("variant in ?", platforms.List()).
		Group("variant").Scan(&sqlResults)

	for _, r := range sqlResults {
		results[r.Variant] = r.PassPercentage
	}

	return results, q.Error
}

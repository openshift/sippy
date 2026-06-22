package query

import (
	"database/sql"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/testidentification"
	"github.com/openshift/sippy/pkg/util/sets"
)

// PlatformInfraSuccess takes a list of platforms and a period (default
// or twoDay), and returns a map containing keys for platform, and infra
// success percentage for that period.
func PlatformInfraSuccess(dbc *db.DB, platforms sets.String, period string) (map[string]float64, error) {
	now := time.Now()
	results := make(map[string]float64)

	table := ""
	switch period {
	case "current":
		table = "prow_test_report_7d_matview"
	case "twoDay":
		table = "prow_test_report_2d_matview"
	default:
		return nil, fmt.Errorf("unknown period %s", period)
	}

	var sqlResults []struct {
		Variant        string
		PassPercentage float64
	}
	q := dbc.DB.Raw(fmt.Sprintf(`
		WITH target_variants AS (
			SELECT vc.id, v.variant
			FROM variant_combinations vc, unnest(vc.variants) AS v(variant)
			WHERE v.variant IN @platforms
		)
		SELECT tv.variant,
			SUM(m.current_successes) * 100.0 / NULLIF(SUM(m.current_runs), 0) AS pass_percentage
		FROM %s m
		JOIN target_variants tv ON m.variant_combination_id = tv.id
		WHERE m.name = @testname
		GROUP BY tv.variant`, table),
		sql.Named("platforms", platforms.List()),
		sql.Named("testname", testidentification.NewInfrastructureTestName),
	).Scan(&sqlResults)

	for _, r := range sqlResults {
		results[r.Variant] = r.PassPercentage
	}

	elapsed := time.Since(now)
	log.WithFields(log.Fields{
		"elapsed": elapsed,
	}).Info("PlatformInfraSuccess completed")
	return results, q.Error
}

package query

import (
	"fmt"
	"time"

	"cloud.google.com/go/civil"

	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/filter"
)

func GetFeatureGatesFromDB(dbc *db.DB, release string, filterOpts *filter.FilterOptions) ([]api.FeatureGate, error) {
	// Get test names that had actual runs in the last 7 days. We compare
	// prefix_sum_runs at the end vs start of the range to confirm the test ran
	// at least once, filtering out tests that merely have carried-forward rows.
	tomorrow := civil.DateOf(time.Now().UTC()).AddDays(1)
	dr := DateRange{Start: tomorrow.AddDays(-8), End: tomorrow}
	if err := resolveDateRanges(dbc, release, &dr); err != nil {
		return nil, err
	}
	lookupEnd := dr.End.AddDays(-1)
	lookupStart := dr.Start.AddDays(-1)

	activeTestExists := `EXISTS (
		SELECT 1 FROM test_cumulative_summaries e
		LEFT JOIN test_cumulative_summaries s
			ON s.test_id = e.test_id AND s.prow_job_id = e.prow_job_id
			AND s.suite_id = e.suite_id AND s.release = e.release
			AND s.date = ?
		WHERE e.test_id = t.id AND e.date = ? AND e.release = ?
			AND e.prefix_sum_runs > COALESCE(s.prefix_sum_runs, 0)
	)`

	byTag := dbc.DB.Table("tests t").
		Select("t.name, ? AS release, (m.match)[2] AS gate_name", release).
		Joins(`CROSS JOIN LATERAL regexp_matches(t.name, '\[(FeatureGate|OCPFeatureGate):([^\]]+)\]', 'g') AS m(match)`).
		Where("t.name LIKE ?", "%FeatureGate:%").
		Where(activeTestExists, lookupStart, lookupEnd, release)

	byInstall := dbc.DB.Table("tests t").
		Select("t.name, ? AS release, NULL::text AS gate_name", release).
		Where("t.name LIKE ?", "%install should succeed%").
		Where(activeTestExists, lookupStart, lookupEnd, release)

	subQuery := dbc.DB.Raw("? UNION ALL ?", byTag, byInstall)

	// Figure out the first release we ever saw a FG.
	firstSeenQuery := dbc.DB.Raw(`
		SELECT DISTINCT ON (feature_gate)
			feature_gate,
			release AS first_seen_in,
			CAST((string_to_array(release, '.'))[1] AS INT) AS first_seen_in_major,
			CAST((string_to_array(release, '.'))[2] AS INT) AS first_seen_in_minor
		FROM feature_gates
		WHERE status = 'enabled'
		ORDER BY feature_gate, string_to_array(release, '.')::int[] ASC
	`)

	fgQuery := dbc.DB.Table("feature_gates AS fg").
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
		Joins(`LEFT JOIN (?) AS mt ON fg.feature_gate = mt.gate_name
			OR (mt.gate_name IS NULL AND fg.feature_gate LIKE '%Install%' AND mt.name LIKE '%install should succeed%')`, subQuery).
		Joins("LEFT JOIN (?) AS fs ON fg.feature_gate = fs.feature_gate", firstSeenQuery).
		Where("fg.release = ? AND fg.status = 'enabled'", release).
		Group("fg.feature_gate, fg.release, fs.first_seen_in, fs.first_seen_in_major, fs.first_seen_in_minor").
		Order("fg.feature_gate")

	table := dbc.DB.Table("(?) AS results", fgQuery)

	q, err := filter.FilterableDBResult(table, filterOpts, api.FeatureGate{})
	if err != nil {
		return nil, fmt.Errorf("failed to apply filter: %w", err)
	}

	results := make([]api.FeatureGate, 0)
	tx := q.Scan(&results)
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to scan feature gates: %w", tx.Error)
	}

	return results, nil
}

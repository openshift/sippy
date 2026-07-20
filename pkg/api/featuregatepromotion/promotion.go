package featuregatepromotion

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/civil"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/sippy/pkg/db"
)

const (
	RequiredNumberOfTests              = 5
	RequiredNumberOfTestRunsPerVariant = 14
	RequiredPassRateOfTestsPerVariant  = 0.95
)

var (
	RequiredSelfManagedJobVariants = []JobVariant{
		{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
		{Cloud: "azure", Architecture: "amd64", Topology: "ha"},
		{Cloud: "gcp", Architecture: "amd64", Topology: "ha"},
		{Cloud: "vsphere", Architecture: "amd64", Topology: "ha"},
		{Cloud: "metal", Architecture: "amd64", Topology: "ha", NetworkStack: "ipv4"},
		{Cloud: "metal", Architecture: "amd64", Topology: "ha", NetworkStack: "ipv6"},
		{Cloud: "metal", Architecture: "amd64", Topology: "ha", NetworkStack: "dual"},
		{Cloud: "aws", Architecture: "amd64", Topology: "single"},
		{Cloud: "aws", Architecture: "amd64", Topology: "ha", OS: "rhel10", Optional: true},
	}

	OptionalSelfManagedPlatformVariants = []JobVariant{
		{Cloud: "nutanix", Architecture: "amd64", Topology: "ha"},
		{Cloud: "openstack", Architecture: "amd64", Topology: "ha"},
		{Cloud: "metal", Architecture: "amd64", Topology: "two-node-arbiter", NetworkStack: "ipv4"},
		{Cloud: "metal", Architecture: "amd64", Topology: "two-node-arbiter", NetworkStack: "ipv6"},
		{Cloud: "metal", Architecture: "amd64", Topology: "two-node-arbiter", NetworkStack: "dual"},
		{Cloud: "metal", Architecture: "amd64", Topology: "two-node-fencing", NetworkStack: "ipv4", JobTiers: "candidate,standard,informing,blocking"},
		{Cloud: "metal", Architecture: "amd64", Topology: "two-node-fencing", NetworkStack: "ipv6", JobTiers: "candidate,standard,informing,blocking"},
		{Cloud: "metal", Architecture: "amd64", Topology: "two-node-fencing", NetworkStack: "dual", JobTiers: "candidate,standard,informing,blocking"},
	}

	NonHypershiftPlatforms = regexp.MustCompile("(?i)nutanix|metal|vsphere|openstack|azure|gcp")

	RequiredHypershiftJobVariants = []JobVariant{
		{Cloud: "aws", Architecture: "amd64", Topology: "external"},
	}
)

// GetPromotionStatus computes the promotion readiness for a feature gate.
func GetPromotionStatus(dbc *db.DB, release, featureGate string) (*PromotionStatus, error) {
	topologies, err := getGateTopologies(dbc, release, featureGate)
	if err != nil {
		return nil, fmt.Errorf("getting gate topologies: %w", err)
	}

	variantsToCheck := determineVariantsToCheck(featureGate, topologies)

	isInstallGate := strings.Contains(featureGate, "Install")

	var rows []testQueryRow
	if isInstallGate {
		rows, err = queryJobBasedResults(dbc, release, featureGate)
	} else {
		rows, err = queryTestBasedResults(dbc, release, featureGate)
	}
	if err != nil {
		return nil, fmt.Errorf("querying test results: %w", err)
	}

	result := buildPromotionStatus(featureGate, release, variantsToCheck, rows)
	return result, nil
}

// getGateTopologies returns the set of topologies where this feature gate is enabled.
func getGateTopologies(dbc *db.DB, release, featureGate string) (sets.Set[string], error) {
	var topologies []string
	tx := dbc.DB.Table("feature_gates").
		Select("DISTINCT topology").
		Where("release = ? AND feature_gate = ? AND status = 'enabled'", release, featureGate).
		Pluck("topology", &topologies)
	if tx.Error != nil {
		return nil, tx.Error
	}
	return sets.New(topologies...), nil
}

// determineVariantsToCheck selects which variant combos to check based on the
// feature gate name and the topologies it is enabled for.
func determineVariantsToCheck(featureGate string, topologies sets.Set[string]) []JobVariant {
	var variants []JobVariant

	if topologies.Has("Hypershift") && !NonHypershiftPlatforms.MatchString(featureGate) {
		variants = append(variants, FilterVariants(featureGate, RequiredHypershiftJobVariants)...)
	}

	if topologies.Has("SelfManagedHA") {
		platformVariants := FilterVariants(featureGate, OptionalSelfManagedPlatformVariants, RequiredSelfManagedJobVariants)
		if len(platformVariants) == 0 {
			platformVariants = RequiredSelfManagedJobVariants
		}
		variants = append(variants, platformVariants...)
	}

	return variants
}

// FilterVariants returns only the variants whose cloud, architecture, or topology
// appear in the feature gate name (case-insensitive). If no variants match, returns nil.
func FilterVariants(featureGate string, variantLists ...[]JobVariant) []JobVariant {
	var filtered []JobVariant
	normalized := strings.ToLower(featureGate)

	for _, variants := range variantLists {
		for _, v := range variants {
			cloud := strings.ReplaceAll(strings.ToLower(v.Cloud), "-ipi", "")
			arch := strings.ToLower(v.Architecture)
			topology := strings.ToLower(v.Topology)

			if strings.Contains(normalized, cloud) ||
				strings.Contains(normalized, arch) ||
				MatchTwoNodeFeatureGates(normalized, topology) {
				filtered = append(filtered, v)
			}
		}
	}
	return filtered
}

// MatchTwoNodeFeatureGates checks for Arbiter, DualReplica, or Fencing feature gates
// that have special topologies.
func MatchTwoNodeFeatureGates(featureGate, topology string) bool {
	if (strings.Contains(featureGate, "dualreplica") || strings.Contains(featureGate, "fencing")) &&
		strings.Contains(topology, "fencing") {
		return true
	}
	return false
}

// TopologyDisplayName converts internal topology names to display names.
func TopologyDisplayName(topology string) string {
	if topology == "external" {
		return "hypershift"
	}
	return topology
}

// ValidateJobTiers checks that a JobVariant's JobTiers field is valid.
func ValidateJobTiers(v JobVariant) error {
	if v.JobTiers == "" {
		return nil
	}

	validTiers := sets.New("standard", "informing", "blocking", "candidate")
	hasValid := false
	for tier := range strings.SplitSeq(v.JobTiers, ",") {
		tier = strings.TrimSpace(tier)
		if tier != "" {
			hasValid = true
			if !validTiers.Has(tier) {
				return fmt.Errorf("invalid JobTier %q in variant %+v - must be one of: standard, informing, blocking, candidate", tier, v)
			}
		}
	}
	if !hasValid {
		return fmt.Errorf("JobTiers string %q contains no valid tier names in variant %+v", v.JobTiers, v)
	}
	return nil
}

// JobTiersForVariant returns the set of job tiers to query for a variant.
func JobTiersForVariant(v JobVariant) []string {
	if v.JobTiers == "" {
		return []string{"standard", "informing", "blocking", "candidate"}
	}
	tierSet := sets.New[string]()
	for tier := range strings.SplitSeq(v.JobTiers, ",") {
		if trimmed := strings.TrimSpace(tier); trimmed != "" {
			tierSet.Insert(trimmed)
		}
	}
	if tierSet.Len() == 0 {
		return []string{"standard", "informing", "blocking", "candidate"}
	}
	return sets.List(tierSet)
}

// queryTestBasedResults runs a single SQL query to get test results for all
// required variants at once, using the prefix sum approach.
func queryTestBasedResults(dbc *db.DB, release, featureGate string) ([]testQueryRow, error) {
	tomorrow := civil.DateOf(time.Now().UTC()).AddDays(1)
	sample := dateRange{Start: tomorrow.AddDays(-8), End: tomorrow}
	base := dateRange{Start: tomorrow.AddDays(-15), End: sample.Start}

	if err := clampDateRanges(dbc, release, &sample, &base); err != nil {
		return nil, err
	}

	end := sample.End.AddDays(-1)
	boundary := sample.Start.AddDays(-1)
	start := base.Start.AddDays(-1)

	testPattern := fmt.Sprintf("%%FeatureGate:%s]%%", featureGate)

	query := `
		SELECT
			t.name AS test_name,
			(SELECT v FROM unnest(vc.variants) AS v WHERE v LIKE 'Platform:%' LIMIT 1) AS platform,
			(SELECT v FROM unnest(vc.variants) AS v WHERE v LIKE 'Architecture:%' LIMIT 1) AS architecture,
			(SELECT v FROM unnest(vc.variants) AS v WHERE v LIKE 'Topology:%' LIMIT 1) AS topology,
			(SELECT v FROM unnest(vc.variants) AS v WHERE v LIKE 'NetworkStack:%' LIMIT 1) AS network_stack,
			(SELECT v FROM unnest(vc.variants) AS v WHERE v LIKE 'OS:%' LIMIT 1) AS os,
			(SELECT v FROM unnest(vc.variants) AS v WHERE v LIKE 'JobTier:%' LIMIT 1) AS job_tier,
			SUM(COALESCE(e.prefix_sum_successes - COALESCE(m.prefix_sum_successes, 0), 0))::int AS current_successes,
			SUM(COALESCE(e.prefix_sum_failures  - COALESCE(m.prefix_sum_failures,  0), 0))::int AS current_failures,
			SUM(COALESCE(e.prefix_sum_flakes    - COALESCE(m.prefix_sum_flakes,    0), 0))::int AS current_flakes,
			SUM(COALESCE(e.prefix_sum_runs      - COALESCE(m.prefix_sum_runs,      0), 0))::int AS current_runs,
			SUM(COALESCE(m.prefix_sum_successes - COALESCE(s.prefix_sum_successes, 0), 0))::int AS previous_successes,
			SUM(COALESCE(m.prefix_sum_failures  - COALESCE(s.prefix_sum_failures,  0), 0))::int AS previous_failures,
			SUM(COALESCE(m.prefix_sum_flakes    - COALESCE(s.prefix_sum_flakes,    0), 0))::int AS previous_flakes,
			SUM(COALESCE(m.prefix_sum_runs      - COALESCE(s.prefix_sum_runs,      0), 0))::int AS previous_runs
		FROM test_cumulative_summaries e
		JOIN prow_jobs pj ON e.prow_job_id = pj.id AND pj.variant_combination_id IS NOT NULL
		JOIN variant_combinations vc ON pj.variant_combination_id = vc.id
		JOIN tests t ON t.id = e.test_id
		LEFT JOIN test_cumulative_summaries m
			ON m.test_id = e.test_id AND m.prow_job_id = e.prow_job_id
			AND m.suite_id = e.suite_id AND m.release = e.release AND m.date = ?
		LEFT JOIN test_cumulative_summaries s
			ON s.test_id = e.test_id AND s.prow_job_id = e.prow_job_id
			AND s.suite_id = e.suite_id AND s.release = e.release AND s.date = ?
		WHERE e.date = ? AND e.release = ?
			AND t.name LIKE ?
			AND NOT EXISTS (SELECT 1 FROM unnest(vc.variants) AS v WHERE v = 'never-stable')
			AND NOT EXISTS (SELECT 1 FROM unnest(vc.variants) AS v WHERE v = 'aggregated')
		GROUP BY t.name,
			(SELECT v FROM unnest(vc.variants) AS v WHERE v LIKE 'Platform:%' LIMIT 1),
			(SELECT v FROM unnest(vc.variants) AS v WHERE v LIKE 'Architecture:%' LIMIT 1),
			(SELECT v FROM unnest(vc.variants) AS v WHERE v LIKE 'Topology:%' LIMIT 1),
			(SELECT v FROM unnest(vc.variants) AS v WHERE v LIKE 'NetworkStack:%' LIMIT 1),
			(SELECT v FROM unnest(vc.variants) AS v WHERE v LIKE 'OS:%' LIMIT 1),
			(SELECT v FROM unnest(vc.variants) AS v WHERE v LIKE 'JobTier:%' LIMIT 1)
	`

	var rows []testQueryRow
	tx := dbc.DB.Raw(query, boundary, start, end, release, testPattern).Scan(&rows)
	if tx.Error != nil {
		return nil, fmt.Errorf("querying test results: %w", tx.Error)
	}

	// Strip the "Key:" prefixes from variant dimension values
	for i := range rows {
		rows[i].Platform = stripPrefix(rows[i].Platform, "Platform:")
		rows[i].Architecture = stripPrefix(rows[i].Architecture, "Architecture:")
		rows[i].Topology = stripPrefix(rows[i].Topology, "Topology:")
		rows[i].NetworkStack = stripPrefix(rows[i].NetworkStack, "NetworkStack:")
		rows[i].OS = stripPrefix(rows[i].OS, "OS:")
		rows[i].JobTier = stripPrefix(rows[i].JobTier, "JobTier:")
	}

	log.WithFields(log.Fields{
		"release":      release,
		"feature_gate": featureGate,
		"row_count":    len(rows),
	}).Debug("promotion readiness query complete")

	return rows, nil
}

// queryJobBasedResults queries job pass rates for Install-type feature gates.
func queryJobBasedResults(dbc *db.DB, release, featureGate string) ([]testQueryRow, error) {
	tomorrow := civil.DateOf(time.Now().UTC()).AddDays(1)
	sample := dateRange{Start: tomorrow.AddDays(-8), End: tomorrow}
	base := dateRange{Start: tomorrow.AddDays(-15), End: sample.Start}

	if err := clampDateRanges(dbc, release, &sample, &base); err != nil {
		return nil, err
	}

	capabilityVariant := fmt.Sprintf("Capability:%s", featureGate)

	// For Install gates, query job-level pass rates using prow_job_runs.
	// We look at jobs tagged with the Capability variant and compute
	// pass rates per variant combo over the current and previous windows.
	query := `
		WITH job_runs AS (
			SELECT
				pj.name AS job_name,
				(SELECT v FROM unnest(vc.variants) AS v WHERE v LIKE 'Platform:%' LIMIT 1) AS platform,
				(SELECT v FROM unnest(vc.variants) AS v WHERE v LIKE 'Architecture:%' LIMIT 1) AS architecture,
				(SELECT v FROM unnest(vc.variants) AS v WHERE v LIKE 'Topology:%' LIMIT 1) AS topology,
				(SELECT v FROM unnest(vc.variants) AS v WHERE v LIKE 'NetworkStack:%' LIMIT 1) AS network_stack,
				(SELECT v FROM unnest(vc.variants) AS v WHERE v LIKE 'OS:%' LIMIT 1) AS os,
				pjr.overall_result,
				pjr.created_at
			FROM prow_job_runs pjr
			JOIN prow_jobs pj ON pjr.prow_job_id = pj.id
			JOIN variant_combinations vc ON pj.variant_combination_id = vc.id
			WHERE pj.release = ?
				AND EXISTS (SELECT 1 FROM unnest(vc.variants) AS v WHERE v = ?)
				AND NOT EXISTS (SELECT 1 FROM unnest(vc.variants) AS v WHERE v = 'never-stable')
				AND NOT EXISTS (SELECT 1 FROM unnest(vc.variants) AS v WHERE v = 'aggregated')
				AND pjr.created_at >= ?
		)
		SELECT
			job_name AS test_name,
			platform,
			architecture,
			topology,
			network_stack,
			os,
			'' AS job_tier,
			SUM(CASE WHEN created_at >= ? THEN 1 ELSE 0 END)::int AS current_runs,
			SUM(CASE WHEN created_at >= ? AND overall_result = 'S' THEN 1 ELSE 0 END)::int AS current_successes,
			0 AS current_failures,
			0 AS current_flakes,
			SUM(CASE WHEN created_at < ? THEN 1 ELSE 0 END)::int AS previous_runs,
			SUM(CASE WHEN created_at < ? AND overall_result = 'S' THEN 1 ELSE 0 END)::int AS previous_successes,
			0 AS previous_failures,
			0 AS previous_flakes
		FROM job_runs
		GROUP BY job_name, platform, architecture, topology, network_stack, os
	`

	baseStart := base.Start
	sampleStart := sample.Start

	var rows []testQueryRow
	tx := dbc.DB.Raw(query,
		release, capabilityVariant, baseStart,
		sampleStart, sampleStart,
		sampleStart, sampleStart,
	).Scan(&rows)
	if tx.Error != nil {
		return nil, fmt.Errorf("querying job results: %w", tx.Error)
	}

	for i := range rows {
		rows[i].Platform = stripPrefix(rows[i].Platform, "Platform:")
		rows[i].Architecture = stripPrefix(rows[i].Architecture, "Architecture:")
		rows[i].Topology = stripPrefix(rows[i].Topology, "Topology:")
		rows[i].NetworkStack = stripPrefix(rows[i].NetworkStack, "NetworkStack:")
		rows[i].OS = stripPrefix(rows[i].OS, "OS:")
	}

	log.WithFields(log.Fields{
		"release":      release,
		"feature_gate": featureGate,
		"row_count":    len(rows),
	}).Debug("job-based promotion readiness query complete")

	return rows, nil
}

// buildPromotionStatus assembles the PromotionStatus from query results.
func buildPromotionStatus(featureGate, release string, variantsToCheck []JobVariant, rows []testQueryRow) *PromotionStatus {
	status := &PromotionStatus{
		FeatureGate: featureGate,
		Release:     release,
		Sufficient:  true,
	}

	sort.Sort(OrderedJobVariants(variantsToCheck))

	for _, jv := range variantsToCheck {
		vr := buildVariantResult(jv, rows)
		status.Variants = append(status.Variants, vr)

		status.Warnings = append(status.Warnings, vr.Warnings...)
		if !jv.Optional {
			status.Errors = append(status.Errors, vr.Errors...)
		} else {
			status.Warnings = append(status.Warnings, vr.Errors...)
		}
	}

	for _, vr := range status.Variants {
		if !vr.Sufficient && !vr.Optional {
			status.Sufficient = false
			break
		}
	}

	return status
}

// buildVariantResult processes query rows for a single variant combo.
func buildVariantResult(jv JobVariant, allRows []testQueryRow) VariantResult {
	vr := VariantResult{
		Cloud:        jv.Cloud,
		Architecture: jv.Architecture,
		Topology:     jv.Topology,
		NetworkStack: jv.NetworkStack,
		OS:           jv.OS,
		Optional:     jv.Optional,
		Sufficient:   true,
	}

	allowedTiers := sets.New(JobTiersForVariant(jv)...)

	// Collect rows matching this variant, accumulating across job tiers
	testMap := map[string]*TestResult{}
	hasCandidateTierResults := false

	for _, row := range allRows {
		if !matchesVariant(row, jv) {
			continue
		}
		if row.JobTier != "" && !allowedTiers.Has(row.JobTier) {
			continue
		}

		if row.JobTier == "candidate" {
			hasCandidateTierResults = true
		}

		tr, ok := testMap[row.TestName]
		if !ok {
			tr = &TestResult{TestName: row.TestName}
			testMap[row.TestName] = tr
		}

		// Apply lookback: use 7-day window if sufficient, else extend to 14 days
		if row.CurrentRuns >= RequiredNumberOfTestRunsPerVariant {
			tr.TotalRuns += row.CurrentRuns
			tr.SuccessfulRuns += row.CurrentSuccesses
			tr.FailedRuns += row.CurrentFailures
			tr.FlakedRuns += row.CurrentFlakes
		} else {
			tr.TotalRuns += row.CurrentRuns + row.PreviousRuns
			tr.SuccessfulRuns += row.CurrentSuccesses + row.PreviousSuccesses
			tr.FailedRuns += row.CurrentFailures + row.PreviousFailures
			tr.FlakedRuns += row.CurrentFlakes + row.PreviousFlakes
		}
	}

	// Build sorted test results
	testNames := sets.KeySet(testMap)
	for _, name := range sets.List(testNames) {
		tr := testMap[name]
		if tr.TotalRuns > 0 {
			tr.PassPercent = float32(tr.SuccessfulRuns) / float32(tr.TotalRuns)
		}
		tr.Sufficient = tr.TotalRuns >= RequiredNumberOfTestRunsPerVariant &&
			tr.PassPercent >= RequiredPassRateOfTestsPerVariant
		vr.TestResults = append(vr.TestResults, *tr)
	}

	// Validate
	if hasCandidateTierResults {
		vr.Warnings = append(vr.Warnings,
			fmt.Sprintf("variant %s includes test data from candidate-tier jobs which are not covered by Component Readiness", variantLabel(jv)))
	}

	if len(vr.TestResults) < RequiredNumberOfTests {
		vr.Sufficient = false
		vr.Errors = append(vr.Errors,
			fmt.Sprintf("only %d tests found, need at least %d for %q on %s",
				len(vr.TestResults), RequiredNumberOfTests, vr.Cloud, variantLabel(jv)))
	}

	for _, tr := range vr.TestResults {
		if tr.TotalRuns < RequiredNumberOfTestRunsPerVariant {
			vr.Sufficient = false
			vr.Errors = append(vr.Errors,
				fmt.Sprintf("%q only has %d runs, need at least %d on %s",
					tr.TestName, tr.TotalRuns, RequiredNumberOfTestRunsPerVariant, variantLabel(jv)))
		}
		if tr.TotalRuns > 0 && tr.PassPercent < RequiredPassRateOfTestsPerVariant {
			vr.Sufficient = false
			vr.Errors = append(vr.Errors,
				fmt.Sprintf("%q only passed %d%%, need at least %d%% on %s",
					tr.TestName, int(tr.PassPercent*100), int(RequiredPassRateOfTestsPerVariant*100), variantLabel(jv)))
		}
	}

	return vr
}

// matchesVariant checks if a query row matches a required variant combo.
func matchesVariant(row testQueryRow, jv JobVariant) bool {
	if !strings.EqualFold(row.Platform, jv.Cloud) {
		return false
	}
	if !strings.EqualFold(row.Architecture, jv.Architecture) {
		return false
	}
	if !strings.EqualFold(row.Topology, jv.Topology) {
		return false
	}
	if jv.NetworkStack != "" && !strings.EqualFold(row.NetworkStack, jv.NetworkStack) {
		return false
	}
	if jv.OS != "" && !strings.EqualFold(row.OS, jv.OS) {
		return false
	}
	return true
}

func variantLabel(jv JobVariant) string {
	parts := []string{TopologyDisplayName(jv.Topology), jv.Cloud, jv.Architecture}
	if jv.NetworkStack != "" {
		parts = append(parts, jv.NetworkStack)
	}
	if jv.OS != "" {
		parts = append(parts, "OS:"+jv.OS)
	}
	return strings.Join(parts, "/")
}

func stripPrefix(s, prefix string) string {
	return strings.TrimPrefix(s, prefix)
}

// OrderedJobVariants implements sort.Interface for consistent variant ordering.
type OrderedJobVariants []JobVariant

func (a OrderedJobVariants) Len() int      { return len(a) }
func (a OrderedJobVariants) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a OrderedJobVariants) Less(i, j int) bool {
	if c := strings.Compare(a[i].Topology, a[j].Topology); c != 0 {
		return c < 0
	}
	if c := strings.Compare(a[i].Cloud, a[j].Cloud); c != 0 {
		return c < 0
	}
	if c := strings.Compare(a[i].Architecture, a[j].Architecture); c != 0 {
		return c < 0
	}
	networkOrder := map[string]string{"": "0", "ipv4": "1", "ipv6": "2", "dual": "3"}
	if c := strings.Compare(networkOrder[a[i].NetworkStack], networkOrder[a[j].NetworkStack]); c != 0 {
		return c < 0
	}
	if c := strings.Compare(a[i].OS, a[j].OS); c != 0 {
		return c < 0
	}
	return strings.Compare(a[i].JobTiers, a[j].JobTiers) < 0
}

// dateRange is a local copy to avoid depending on the query package directly.
type dateRange struct {
	Start civil.Date
	End   civil.Date
}

func clampDateRanges(dbc *db.DB, release string, ranges ...*dateRange) error {
	var maxDate *civil.Date
	row := dbc.DB.Table("test_cumulative_summaries").
		Select("MAX(date)").
		Where("release = ?", release).
		Row()
	if err := row.Scan(&maxDate); err != nil {
		return fmt.Errorf("resolving max date for release %s: %w", release, err)
	}
	if maxDate == nil {
		return nil
	}
	clampTo := maxDate.AddDays(1)
	for _, dr := range ranges {
		if dr.End.After(clampTo) {
			dr.End = clampTo
		}
		if dr.Start.After(clampTo) {
			dr.Start = clampTo
		}
	}
	return nil
}

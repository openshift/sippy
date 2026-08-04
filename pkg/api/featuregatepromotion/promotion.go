package featuregatepromotion

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/civil"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"

	sippyapi "github.com/openshift/sippy/pkg/api"
	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/query"
)

const (
	RequiredNumberOfTests              = 5
	RequiredNumberOfTestRunsPerVariant = 14
	RequiredPassRateOfTestsPerVariant  = 0.95
)

var featureGateRegex = regexp.MustCompile(`\[OCPFeatureGate:([^\]]+)\]`)

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
// It queries test data using the same filters and query path as the /api/tests
// endpoint, ensuring the evaluated data matches what HATEOAS links point to.
func GetPromotionStatus(ctx context.Context, dbc *db.DB, cacheClient cache.Cache, release, featureGate string) (*PromotionStatus, error) {
	topologies, err := getGateTopologies(dbc, release, featureGate)
	if err != nil {
		return nil, fmt.Errorf("getting gate topologies: %w", err)
	}

	variantsToCheck := determineVariantsToCheck(featureGate, topologies)

	gateFilter := GateTestFilter(featureGate)
	annotationTests, err := sippyapi.QueryTestResults(ctx, dbc, cacheClient, release, &gateFilter)
	if err != nil {
		return nil, fmt.Errorf("querying annotation test results: %w", err)
	}

	allTests := annotationTests

	if strings.Contains(featureGate, "Install") {
		installFilter := InstallTestFilter(featureGate)
		capabilityTests, err := sippyapi.QueryTestResults(ctx, dbc, cacheClient, release, &installFilter)
		if err != nil {
			return nil, fmt.Errorf("querying capability test results: %w", err)
		}
		allTests = append(allTests, capabilityTests...)
	}

	log.WithFields(log.Fields{
		"release":      release,
		"feature_gate": featureGate,
		"test_count":   len(allTests),
	}).Debug("promotion readiness query complete")

	status := buildPromotionStatus(featureGate, release, variantsToCheck, allTests)

	regressedTestNames, err := getCapabilityRegressionTestNames(dbc, release, featureGate)
	if err != nil {
		return nil, fmt.Errorf("querying capability regression test names: %w", err)
	}

	if len(regressedTestNames) > 0 {
		promotedGates, err := getPromotedGateNames(dbc, release)
		if err != nil {
			return nil, fmt.Errorf("querying promoted gate names: %w", err)
		}

		failingCount := 0
		for _, name := range regressedTestNames {
			if isFailureDueToUnpromotedGate(name, promotedGates) {
				continue
			}
			failingCount++
		}

		if failingCount > 0 {
			status.Sufficient = false
			status.Errors = append(status.Errors,
				fmt.Sprintf("%d tests in jobs owned by this feature gate have a pass rate below 92%%", failingCount))
		}
	}

	return status, nil
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

// getPromotedGateNames returns the set of feature gate names that are promoted
// to the Default feature set in either SelfManagedHA or Hypershift topologies.
func getPromotedGateNames(dbc *db.DB, release string) (sets.Set[string], error) {
	var names []string
	tx := dbc.DB.Table("feature_gates").
		Select("DISTINCT feature_gate").
		Where("release = ? AND status = 'enabled' AND feature_set = 'Default' AND topology IN ('SelfManagedHA', 'Hypershift')", release).
		Pluck("feature_gate", &names)
	if tx.Error != nil {
		return nil, tx.Error
	}
	return sets.New(names...), nil
}

// isFailureDueToUnpromotedGate returns true if a test name contains an
// [OCPFeatureGate:X] annotation where X is not yet promoted to Default.
// Such failures are expected and should not count against promotion readiness.
func isFailureDueToUnpromotedGate(testName string, promotedGates sets.Set[string]) bool {
	matches := featureGateRegex.FindAllStringSubmatch(testName, -1)
	if len(matches) == 0 {
		return false
	}
	for _, match := range matches {
		gateName := match[1]
		if !promotedGates.Has(gateName) {
			return true
		}
	}
	return false
}

// getCapabilityRegressionTestNames returns test names with a working percentage
// below 92% on jobs owned by this feature gate (via Capability variant).
// This uses a lightweight query rather than the full /api/tests pipeline.
func getCapabilityRegressionTestNames(dbc *db.DB, release, featureGate string) ([]string, error) {
	capabilityVariant := fmt.Sprintf("Capability:%s", featureGate)

	tomorrow := civil.DateOf(time.Now().UTC()).AddDays(1)
	dr := query.DateRange{Start: tomorrow.AddDays(-8), End: tomorrow}
	if err := query.ResolveDateRanges(dbc, release, &dr); err != nil {
		return nil, err
	}
	end := dr.End.AddDays(-1)
	start := dr.Start.AddDays(-1)

	excludedNames := []string{"install should succeed", "openshift-tests should work", "infrastructure should work"}

	type testPassRate struct {
		Name              string
		WorkingPercentage float64
	}

	var results []testPassRate
	tx := dbc.DB.Raw(`
		SELECT t.name,
			CASE WHEN SUM(COALESCE(e.prefix_sum_runs - COALESCE(s.prefix_sum_runs, 0), 0)) = 0 THEN 0
			ELSE (SUM(COALESCE(e.prefix_sum_successes - COALESCE(s.prefix_sum_successes, 0), 0))
				+ SUM(COALESCE(e.prefix_sum_flakes - COALESCE(s.prefix_sum_flakes, 0), 0)))
				* 100.0
				/ SUM(COALESCE(e.prefix_sum_runs - COALESCE(s.prefix_sum_runs, 0), 0))
			END AS working_percentage
		FROM test_cumulative_summaries e
		JOIN prow_jobs pj ON e.prow_job_id = pj.id
		JOIN tests t ON e.test_id = t.id
		LEFT JOIN test_cumulative_summaries s
			ON s.test_id = e.test_id AND s.prow_job_id = e.prow_job_id
			AND s.suite_id = e.suite_id AND s.lifecycle = e.lifecycle
			AND s.release = e.release AND s.date = ?
		WHERE e.date = ? AND e.release = ?
			AND ? = ANY(pj.variants)
			AND NOT EXISTS (
				SELECT 1 FROM variant_combinations
				WHERE 'never-stable' = ANY(variants)
				AND id = pj.variant_combination_id
			)
			AND t.name NOT LIKE '%' || ? || '%'
			AND t.name NOT LIKE '%' || ? || '%'
			AND t.name NOT LIKE '%' || ? || '%'
		GROUP BY t.name
		HAVING SUM(COALESCE(e.prefix_sum_runs - COALESCE(s.prefix_sum_runs, 0), 0)) >= 1
			AND (SUM(COALESCE(e.prefix_sum_successes - COALESCE(s.prefix_sum_successes, 0), 0))
				+ SUM(COALESCE(e.prefix_sum_flakes - COALESCE(s.prefix_sum_flakes, 0), 0)))
				* 100.0
				/ SUM(COALESCE(e.prefix_sum_runs - COALESCE(s.prefix_sum_runs, 0), 0)) < 92
	`, start, end, release, capabilityVariant,
		excludedNames[0], excludedNames[1], excludedNames[2]).
		Scan(&results)
	if tx.Error != nil {
		return nil, tx.Error
	}

	names := make([]string, len(results))
	for i, r := range results {
		names[i] = r.Name
	}
	return names, nil
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

// buildPromotionStatus assembles the PromotionStatus from test results returned
// by the shared query function (same data as /api/tests).
func buildPromotionStatus(featureGate, release string, variantsToCheck []JobVariant, tests []apitype.Test) *PromotionStatus {
	status := &PromotionStatus{
		FeatureGate: featureGate,
		Release:     release,
		Sufficient:  true,
	}

	sort.Sort(OrderedJobVariants(variantsToCheck))

	for _, jv := range variantsToCheck {
		vr := buildVariantResult(jv, tests)
		vr.Variants["Capability"] = featureGate
		status.ResultsByVariant = append(status.ResultsByVariant, vr)

		status.Warnings = append(status.Warnings, vr.Warnings...)
		if !jv.Optional {
			status.Errors = append(status.Errors, vr.Errors...)
		} else {
			status.Warnings = append(status.Warnings, vr.Errors...)
		}
	}

	sort.Strings(status.Warnings)
	sort.Strings(status.Errors)

	for _, vr := range status.ResultsByVariant {
		if !vr.Sufficient && !vr.Optional {
			status.Sufficient = false
			break
		}
	}

	return status
}

// buildVariantResult processes test results for a single variant combo.
func buildVariantResult(jv JobVariant, allTests []apitype.Test) VariantResult {
	variants := map[string]string{
		"Platform":     jv.Cloud,
		"Architecture": jv.Architecture,
		"Topology":     jv.Topology,
	}
	if jv.NetworkStack != "" {
		variants["NetworkStack"] = jv.NetworkStack
	}
	if jv.OS != "" {
		variants["OS"] = jv.OS
	}

	vr := VariantResult{
		Variants:   variants,
		Optional:   jv.Optional,
		Sufficient: true,
	}

	allowedTiers := sets.New(JobTiersForVariant(jv)...)

	// Collect tests matching this variant, accumulating across job tiers
	testMap := map[string]*TestResult{}
	hasCandidateTierResults := false

	for _, test := range allTests {
		parsed := parseVariants(test.Variants)
		if !matchesParsedVariant(parsed, jv) {
			continue
		}
		if jobTier := parsed["JobTier"]; jobTier != "" && !allowedTiers.Has(jobTier) {
			continue
		}

		if parsed["JobTier"] == "candidate" {
			hasCandidateTierResults = true
		}

		tr, ok := testMap[test.Name]
		if !ok {
			tr = &TestResult{TestName: test.Name}
			testMap[test.Name] = tr
		}

		// Apply lookback: use 7-day window if sufficient, else extend to 14 days
		if test.CurrentRuns >= RequiredNumberOfTestRunsPerVariant {
			tr.TotalRuns += test.CurrentRuns
			tr.SuccessfulRuns += test.CurrentSuccesses
			tr.FailedRuns += test.CurrentFailures
			tr.FlakedRuns += test.CurrentFlakes
		} else {
			tr.TotalRuns += test.CurrentRuns + test.PreviousRuns
			tr.SuccessfulRuns += test.CurrentSuccesses + test.PreviousSuccesses
			tr.FailedRuns += test.CurrentFailures + test.PreviousFailures
			tr.FlakedRuns += test.CurrentFlakes + test.PreviousFlakes
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
			fmt.Sprintf("only %d tests found, need at least %d on %s",
				len(vr.TestResults), RequiredNumberOfTests, variantLabel(jv)))
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

// parseVariants extracts variant key-value pairs from a pq.StringArray.
// Each entry is formatted as "Key:value" (e.g., "Platform:aws").
func parseVariants(variants []string) map[string]string {
	parsed := make(map[string]string, len(variants))
	for _, v := range variants {
		if idx := strings.IndexByte(v, ':'); idx > 0 {
			parsed[v[:idx]] = v[idx+1:]
		}
	}
	return parsed
}

// matchesParsedVariant checks if parsed variant dimensions match a required variant combo.
func matchesParsedVariant(parsed map[string]string, jv JobVariant) bool {
	if !strings.EqualFold(parsed["Platform"], jv.Cloud) {
		return false
	}
	if !strings.EqualFold(parsed["Architecture"], jv.Architecture) {
		return false
	}
	if !strings.EqualFold(parsed["Topology"], jv.Topology) {
		return false
	}
	if jv.NetworkStack != "" && !strings.EqualFold(parsed["NetworkStack"], jv.NetworkStack) {
		return false
	}
	if jv.OS != "" && !strings.EqualFold(parsed["OS"], jv.OS) {
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

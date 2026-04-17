package report

import (
	"context"
	"strings"
	"testing"
	"time"

	componentreadiness "github.com/openshift/sippy/pkg/api/componentreadiness"
	pgprovider "github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider/postgres"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/openshift/sippy/pkg/util/sets"
	"github.com/openshift/sippy/test/e2e/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupProvider(t *testing.T) (*pgprovider.PostgresProvider, reqopts.RequestOptions) {
	dbc := util.CreateE2EPostgresConnection(t)
	provider := pgprovider.NewPostgresProvider(dbc, nil)

	now := time.Now().UTC().Truncate(time.Hour)

	reqOptions := reqopts.RequestOptions{
		BaseRelease: reqopts.Release{
			Name:  "4.21",
			Start: now.Add(-60 * 24 * time.Hour),
			End:   now.Add(-30 * 24 * time.Hour),
		},
		SampleRelease: reqopts.Release{
			Name:  "4.22",
			Start: now.Add(-3 * 24 * time.Hour),
			End:   now,
		},
		VariantOption: reqopts.Variants{
			ColumnGroupBy: sets.NewString("Network", "Platform", "Topology"),
			DBGroupBy:     sets.NewString("Architecture", "FeatureSet", "Installer", "Network", "Platform", "Suite", "Topology", "Upgrade", "LayeredProduct"),
		},
		AdvancedOption: reqopts.Advanced{
			Confidence:                  95,
			PityFactor:                  5,
			MinimumFailure:              3,
			PassRateRequiredNewTests:    90,
			IncludeMultiReleaseAnalysis: true,
		},
		CacheOption: cache.RequestOptions{},
	}

	return provider, reqOptions
}

func TestReportStatuses(t *testing.T) {
	provider, reqOptions := setupProvider(t)
	ctx := context.Background()

	report, errs := componentreadiness.GetComponentReport(ctx, provider, nil, reqOptions, "")
	require.Empty(t, errs, "GetComponentReport returned errors: %v", errs)
	require.NotEmpty(t, report.Rows, "report should have rows")

	// Collect regressed test statuses keyed by testID+column platform.
	type cellKey struct {
		testID   string
		platform string
	}
	regressedStatuses := map[cellKey]crtest.Status{}

	for _, row := range report.Rows {
		for _, col := range row.Columns {
			if col.Status <= crtest.SignificantRegression {
				for _, rt := range col.RegressedTests {
					key := cellKey{testID: rt.TestID, platform: col.ColumnIdentification.Variants["Platform"]}
					if existing, ok := regressedStatuses[key]; !ok || rt.ReportStatus < existing {
						regressedStatuses[key] = rt.ReportStatus
					}
				}
			}
		}
	}

	// Verify structural integrity
	for _, row := range report.Rows {
		assert.NotEmpty(t, row.RowIdentification.Component, "every row should have a component")
		for _, col := range row.Columns {
			assert.NotEmpty(t, col.ColumnIdentification.Variants, "column should have variants")
			assert.Contains(t, col.ColumnIdentification.Variants, "Platform")
			assert.Contains(t, col.ColumnIdentification.Variants, "Network")
			assert.Contains(t, col.ColumnIdentification.Variants, "Topology")
		}
	}

	// Collect all cell statuses across the grid
	statusCounts := map[crtest.Status]int{}
	for _, row := range report.Rows {
		for _, col := range row.Columns {
			statusCounts[col.Status]++
		}
	}

	t.Logf("Status distribution: %v", statusCounts)

	assert.Contains(t, statusCounts, crtest.NotSignificant, "should have NotSignificant cells")
	assert.Contains(t, statusCounts, crtest.MissingSample, "should have MissingSample cells")

	hasRegression := statusCounts[crtest.SignificantRegression] > 0 || statusCounts[crtest.ExtremeRegression] > 0
	assert.True(t, hasRegression, "should have at least one regression cell")

	// Verify specific regressed test statuses per platform
	assert.Equal(t, crtest.ExtremeRegression, regressedStatuses[cellKey{"test-extreme-regression", "aws"}],
		"extreme regression on aws should have ExtremeRegression status")
	assert.Equal(t, crtest.SignificantRegression, regressedStatuses[cellKey{"test-significant-regression", "aws"}],
		"significant regression on aws should have SignificantRegression status")
}

func TestTestDetails(t *testing.T) {
	provider, reqOptions := setupProvider(t)
	ctx := context.Background()

	releases, err := provider.QueryReleases(ctx)
	require.NoError(t, err)

	// Generate report to find a regressed test
	report, errs := componentreadiness.GetComponentReport(ctx, provider, nil, reqOptions, "")
	require.Empty(t, errs)

	var testID reqopts.TestIdentification
	found := false
	for _, row := range report.Rows {
		for _, col := range row.Columns {
			for _, rt := range col.RegressedTests {
				testID = reqopts.TestIdentification{
					Component:         rt.RowIdentification.Component,
					Capability:        rt.RowIdentification.Capability,
					TestID:            rt.RowIdentification.TestID,
					RequestedVariants: rt.ColumnIdentification.Variants,
				}
				found = true
				break
			}
			if found {
				break
			}
		}
		if found {
			break
		}
	}
	require.True(t, found, "should find at least one regressed test")

	detailReqOpts := reqOptions
	detailReqOpts.TestIDOptions = []reqopts.TestIdentification{testID}

	details, detailErrs := componentreadiness.GetTestDetails(ctx, provider, nil, detailReqOpts, releases, "")
	require.Empty(t, detailErrs, "GetTestDetails returned errors: %v", detailErrs)

	assert.Equal(t, testID.TestID, details.Identification.RowIdentification.TestID)
	assert.NotEmpty(t, details.Analyses, "details should have analyses")

	// With postgres provider, test details should have actual run data
	for _, analysis := range details.Analyses {
		total := analysis.SampleStats.SuccessCount + analysis.SampleStats.FailureCount + analysis.SampleStats.FlakeCount
		assert.Greater(t, total, 0, "sample stats should have run data")
	}
}

func TestFallback(t *testing.T) {
	provider, reqOptions := setupProvider(t)
	ctx := context.Background()

	report, errs := componentreadiness.GetComponentReport(ctx, provider, nil, reqOptions, "")
	require.Empty(t, errs, "GetComponentReport returned errors: %v", errs)

	type regressedInfo struct {
		status       crtest.Status
		explanations []string
	}
	regressedByID := map[string]regressedInfo{}
	for _, row := range report.Rows {
		for _, col := range row.Columns {
			for _, rt := range col.RegressedTests {
				regressedByID[rt.TestID] = regressedInfo{
					status:       rt.ReportStatus,
					explanations: rt.Explanations,
				}
			}
		}
	}

	if info, ok := regressedByID["test-fallback-improves"]; ok {
		t.Logf("test-fallback-improves: status=%d, explanations=%v", info.status, info.explanations)
		hasOverride := false
		for _, exp := range info.explanations {
			if strings.Contains(exp, "Overrode base stats") && strings.Contains(exp, "4.20") {
				hasOverride = true
			}
		}
		assert.True(t, hasOverride, "fallback-improves should mention override to 4.20 in explanations")
	} else {
		t.Error("test-fallback-improves should be in regressed tests")
	}

	if info, ok := regressedByID["test-fallback-double"]; ok {
		t.Logf("test-fallback-double: status=%d, explanations=%v", info.status, info.explanations)
		hasOverride := false
		for _, exp := range info.explanations {
			if strings.Contains(exp, "Overrode base stats") && strings.Contains(exp, "4.19") {
				hasOverride = true
			}
		}
		assert.True(t, hasOverride, "fallback-double should mention override to 4.19 in explanations")
	} else {
		t.Error("test-fallback-double should be in regressed tests")
	}
}

func TestFallbackInsufficientRuns(t *testing.T) {
	provider, reqOptions := setupProvider(t)
	ctx := context.Background()

	report, errs := componentreadiness.GetComponentReport(ctx, provider, nil, reqOptions, "")
	require.Empty(t, errs)

	found := false
	for _, row := range report.Rows {
		for _, col := range row.Columns {
			for _, rt := range col.RegressedTests {
				if rt.TestID == "test-fallback-insufficient-runs" {
					found = true
					for _, exp := range rt.Explanations {
						assert.NotContains(t, exp, "Overrode base stats",
							"insufficient-runs test should NOT have fallback override explanation")
					}
				}
			}
		}
	}
	assert.True(t, found, "test-fallback-insufficient-runs should be in the report as a regression")
}

func TestMissingBasis(t *testing.T) {
	provider, reqOptions := setupProvider(t)
	ctx := context.Background()

	report, errs := componentreadiness.GetComponentReport(ctx, provider, nil, reqOptions, "")
	require.Empty(t, errs)

	hasMissingBasis := false
	hasMissingSample := false
	for _, row := range report.Rows {
		for _, col := range row.Columns {
			if col.Status == crtest.MissingBasis {
				hasMissingBasis = true
			}
			if col.Status == crtest.MissingSample {
				hasMissingSample = true
			}
		}
	}

	assert.True(t, hasMissingBasis, "report should have at least one MissingBasis cell (new test with good pass rate)")
	assert.True(t, hasMissingSample, "report should have at least one MissingSample cell")
}

func TestNewTestPassRateRegression(t *testing.T) {
	provider, reqOptions := setupProvider(t)
	ctx := context.Background()

	report, errs := componentreadiness.GetComponentReport(ctx, provider, nil, reqOptions, "")
	require.Empty(t, errs)

	// The new flaky test (80% pass rate) should be flagged as a regression
	// via buildPassRateTestStats since it's below the 90% PassRateRequiredNewTests threshold.
	found := false
	for _, row := range report.Rows {
		for _, col := range row.Columns {
			for _, rt := range col.RegressedTests {
				if rt.TestID == "test-new-test-pass-rate-fail" {
					found = true
					assert.Equal(t, crtest.ExtremeRegression, rt.ReportStatus,
						"new test with 70%% pass rate should be an extreme regression (below 80%% extreme threshold)")
					assert.Equal(t, crtest.PassRate, rt.Comparison,
						"new test regression should use pass rate comparison, not fisher exact")
				}
			}
		}
	}
	assert.True(t, found, "test-new-test-pass-rate-fail should appear as a regressed test")
}

func TestTestDetailsForFallbackTest(t *testing.T) {
	provider, reqOptions := setupProvider(t)
	ctx := context.Background()

	releases, err := provider.QueryReleases(ctx)
	require.NoError(t, err)

	// test-fallback-improves has data in 4.21 (worse) and 4.20 (better),
	// so the report should override the base to 4.20.
	report, errs := componentreadiness.GetComponentReport(ctx, provider, nil, reqOptions, "")
	require.Empty(t, errs)

	var testID reqopts.TestIdentification
	found := false
	for _, row := range report.Rows {
		for _, col := range row.Columns {
			for _, rt := range col.RegressedTests {
				if rt.TestID == "test-fallback-improves" {
					testID = reqopts.TestIdentification{
						Component:           rt.RowIdentification.Component,
						Capability:          rt.RowIdentification.Capability,
						TestID:              rt.RowIdentification.TestID,
						RequestedVariants:   rt.ColumnIdentification.Variants,
						BaseOverrideRelease: "4.20",
					}
					found = true
				}
			}
		}
	}
	require.True(t, found, "test-fallback-improves should be in the report")

	detailReqOpts := reqOptions
	detailReqOpts.TestIDOptions = []reqopts.TestIdentification{testID}

	details, detailErrs := componentreadiness.GetTestDetails(ctx, provider, nil, detailReqOpts, releases, "")
	require.Empty(t, detailErrs, "GetTestDetails returned errors: %v", detailErrs)

	assert.Equal(t, "test-fallback-improves", details.Identification.RowIdentification.TestID)
	assert.NotEmpty(t, details.Analyses, "details should have analyses")

	// The fallback path should produce an analysis with base override data
	require.GreaterOrEqual(t, len(details.Analyses), 1)
	for _, analysis := range details.Analyses {
		total := analysis.SampleStats.SuccessCount + analysis.SampleStats.FailureCount + analysis.SampleStats.FlakeCount
		assert.Greater(t, total, 0, "sample stats should have run data")
	}
}

func TestTestDetailsForNewTest(t *testing.T) {
	provider, reqOptions := setupProvider(t)
	ctx := context.Background()

	releases, err := provider.QueryReleases(ctx)
	require.NoError(t, err)

	// test-new-test-pass-rate-fail only exists in 4.22 sample, no base data.
	report, errs := componentreadiness.GetComponentReport(ctx, provider, nil, reqOptions, "")
	require.Empty(t, errs)

	var testID reqopts.TestIdentification
	found := false
	for _, row := range report.Rows {
		for _, col := range row.Columns {
			for _, rt := range col.RegressedTests {
				if rt.TestID == "test-new-test-pass-rate-fail" {
					testID = reqopts.TestIdentification{
						Component:         rt.RowIdentification.Component,
						Capability:        rt.RowIdentification.Capability,
						TestID:            rt.RowIdentification.TestID,
						RequestedVariants: rt.ColumnIdentification.Variants,
					}
					found = true
				}
			}
		}
	}
	require.True(t, found, "test-new-test-pass-rate-fail should be in the report")

	detailReqOpts := reqOptions
	detailReqOpts.TestIDOptions = []reqopts.TestIdentification{testID}

	details, detailErrs := componentreadiness.GetTestDetails(ctx, provider, nil, detailReqOpts, releases, "")
	require.Empty(t, detailErrs, "GetTestDetails returned errors: %v", detailErrs)

	assert.Equal(t, "test-new-test-pass-rate-fail", details.Identification.RowIdentification.TestID)
	assert.NotEmpty(t, details.Analyses, "details should have analyses")

	// New test should have sample data but no base data
	for _, analysis := range details.Analyses {
		sampleTotal := analysis.SampleStats.SuccessCount + analysis.SampleStats.FailureCount + analysis.SampleStats.FlakeCount
		assert.Greater(t, sampleTotal, 0, "sample stats should have run data")
		if analysis.BaseStats != nil {
			baseTotal := analysis.BaseStats.SuccessCount + analysis.BaseStats.FailureCount + analysis.BaseStats.FlakeCount
			assert.Equal(t, 0, baseTotal, "base stats should have no run data for a new test")
		}
	}
}

func TestReportDevMode(t *testing.T) {
	provider, reqOptions := setupProvider(t)
	ctx := context.Background()

	t.Setenv("DEV_MODE", "1")

	report, errs := componentreadiness.GetComponentReport(ctx, provider, nil, reqOptions, "")
	require.Empty(t, errs, "GetComponentReport in DEV_MODE returned errors: %v", errs)
	assert.NotEmpty(t, report.Rows, "DEV_MODE report should still have rows")
}

func TestReportWithIncludeVariants(t *testing.T) {
	provider, reqOptions := setupProvider(t)
	ctx := context.Background()

	reqOptions.VariantOption.IncludeVariants = map[string][]string{
		"Platform": {"aws"},
	}

	report, errs := componentreadiness.GetComponentReport(ctx, provider, nil, reqOptions, "")
	require.Empty(t, errs, "GetComponentReport with IncludeVariants returned errors: %v", errs)
	require.NotEmpty(t, report.Rows, "filtered report should have rows")

	for _, row := range report.Rows {
		for _, col := range row.Columns {
			assert.Equal(t, "aws", col.ColumnIdentification.Variants["Platform"],
				"all columns should be aws when filtering by Platform:aws")
		}
	}
}

func TestSignificantImprovement(t *testing.T) {
	provider, reqOptions := setupProvider(t)
	ctx := context.Background()

	report, errs := componentreadiness.GetComponentReport(ctx, provider, nil, reqOptions, "")
	require.Empty(t, errs)

	hasImprovement := false
	for _, row := range report.Rows {
		for _, col := range row.Columns {
			if col.Status == crtest.SignificantImprovement {
				hasImprovement = true
			}
		}
	}
	assert.True(t, hasImprovement, "report should have at least one SignificantImprovement cell")
}

func TestJobVariants(t *testing.T) {
	provider, _ := setupProvider(t)
	ctx := context.Background()

	variants, errs := componentreadiness.GetJobVariants(ctx, provider)
	require.Empty(t, errs)
	require.NotEmpty(t, variants.Variants)

	assert.Contains(t, variants.Variants, "Platform")
	assert.Contains(t, variants.Variants, "Architecture")
	assert.Contains(t, variants.Variants, "Network")
	assert.Contains(t, variants.Variants, "Topology")
	assert.Contains(t, variants.Variants, "FeatureSet")
}

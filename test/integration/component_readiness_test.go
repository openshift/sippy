package integration

import (
	"context"
	"math/big"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"

	componentreadiness "github.com/openshift/sippy/pkg/api/componentreadiness"
	"github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider/postgres"
	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crstatus"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	intutil "github.com/openshift/sippy/test/integration/util"
)

// --- Seed data helpers ---

func crTestDB(t *testing.T) *db.DB {
	t.Helper()
	return intutil.NewTestDB(t, pgContainer)
}

// createVariantCombination, createProwJobWithVC, createTestOwnership, and
// createCumulativeSummary below are thin delegates to the shared intutil fixture helpers,
// kept as file-local wrappers (same names/signatures as before) so the ~250 call sites in
// this file didn't need to change. The actual row-creation logic lives once in
// test/integration/util/fixtures.go, shared with test/integration/tests_report_test.go.

func createVariantCombination(t *testing.T, dbc *db.DB, variants []string) models.VariantCombination {
	t.Helper()
	return intutil.CreateVariantCombination(t, dbc, variants)
}

func createProwJobWithVC(t *testing.T, dbc *db.DB, name, release string, vc models.VariantCombination) models.ProwJob {
	t.Helper()
	return intutil.CreateProwJobWithOptions(t, dbc, name, release, nil, intutil.WithVariantCombination(vc))
}

func createTestOwnership(t *testing.T, dbc *db.DB, testID uint, suiteID *uint, uniqueID, component string, caps []string) models.TestOwnership {
	t.Helper()
	return intutil.CreateTestOwnership(t, dbc, testID, suiteID, uniqueID, component,
		intutil.WithTestOwnershipJiraComponent(component), intutil.WithTestOwnershipCapabilities(caps))
}

type cumulativeSummaryOpts struct {
	prefixSumFailures    int64
	prefixMaxLastFailure *time.Time
	prefixMaxLastSuccess *time.Time
	lifecycle            string
}

type cumulativeSummaryOption func(*cumulativeSummaryOpts)

func withLastFailure(t time.Time) cumulativeSummaryOption {
	return func(o *cumulativeSummaryOpts) { o.prefixMaxLastFailure = &t }
}

func withLastSuccess(t time.Time) cumulativeSummaryOption {
	return func(o *cumulativeSummaryOpts) { o.prefixMaxLastSuccess = &t }
}

func withFailures(failures int64) cumulativeSummaryOption {
	return func(o *cumulativeSummaryOpts) { o.prefixSumFailures = failures }
}

func withLifecycle(lifecycle string) cumulativeSummaryOption {
	return func(o *cumulativeSummaryOpts) { o.lifecycle = lifecycle }
}

func createCumulativeSummary(t *testing.T, dbc *db.DB, date civil.Date, release string, testID, prowJobID, suiteID uint, runs, successes, flakes int64, options ...cumulativeSummaryOption) {
	t.Helper()
	var o cumulativeSummaryOpts
	for _, fn := range options {
		fn(&o)
	}
	intutilOpts := []intutil.CumulativeSummaryOption{intutil.WithCumulativeSummaryFailures(o.prefixSumFailures)}
	if o.lifecycle != "" {
		intutilOpts = append(intutilOpts, intutil.WithCumulativeSummaryLifecycle(o.lifecycle))
	}
	if o.prefixMaxLastFailure != nil {
		intutilOpts = append(intutilOpts, intutil.WithCumulativeSummaryLastFailure(*o.prefixMaxLastFailure))
	}
	if o.prefixMaxLastSuccess != nil {
		intutilOpts = append(intutilOpts, intutil.WithCumulativeSummaryLastSuccess(*o.prefixMaxLastSuccess))
	}
	intutil.CreateCumulativeSummary(t, dbc, date, release, testID, prowJobID, suiteID, runs, successes, flakes, intutilOpts...)
}

func createGARawData(t *testing.T, dbc *db.DB, release string, windowDays int, testID, prowJobID, suiteID uint, runs, passes, flakes int64) { //nolint:unparam
	t.Helper()
	ga := models.ProwGARawTestDatum{
		Release:    release,
		WindowDays: windowDays,
		TestID:     testID,
		ProwJobID:  prowJobID,
		SuiteID:    suiteID,
		Runs:       runs,
		Passes:     passes,
		Flakes:     flakes,
	}
	require.NoError(t, dbc.DB.Create(&ga).Error)
}

func createReleaseDefinition(t *testing.T, dbc *db.DB, release string, gaDate *civil.Date) {
	t.Helper()
	rd := models.ReleaseDefinition{
		Release: release,
		GADate:  gaDate,
	}
	require.NoError(t, dbc.DB.Create(&rd).Error)
}

func createProwJobRunForCR(t *testing.T, dbc *db.DB, prowJobID uint, release string, timestamp time.Time) models.ProwJobRun {
	t.Helper()
	run := models.ProwJobRun{
		ProwJobID:      prowJobID,
		ProwJobRelease: release,
		Timestamp:      timestamp,
		Succeeded:      true,
	}
	require.NoError(t, dbc.DB.Create(&run).Error)
	return run
}

func createProwJobRunTest(t *testing.T, dbc *db.DB, runID, prowJobID, testID uint, suiteID *uint, status int, release string, timestamp time.Time) {
	t.Helper()
	pjrt := models.ProwJobRunTest{
		ProwJobRunID:        runID,
		ProwJobID:           prowJobID,
		ProwJobRunTimestamp: timestamp,
		ProwJobRunRelease:   release,
		TestID:              testID,
		SuiteID:             suiteID,
		Status:              status,
	}
	require.NoError(t, dbc.DB.Create(&pjrt).Error)
}

func setJobRunLabels(t *testing.T, dbc *db.DB, runID uint, labels []string) {
	t.Helper()
	require.NoError(t, dbc.DB.Model(&models.ProwJobRun{}).Where("id = ?", runID).
		Update("labels", pq.StringArray(labels)).Error)
}

func createTestOwnershipFull(t *testing.T, dbc *db.DB, testID uint, suiteID *uint, uniqueID, component string, caps []string, jiraComponentID *uint) models.TestOwnership {
	t.Helper()
	return intutil.CreateTestOwnership(t, dbc, testID, suiteID, uniqueID, component,
		intutil.WithTestOwnershipJiraComponent(component), intutil.WithTestOwnershipCapabilities(caps), intutil.WithTestOwnershipJiraComponentID(jiraComponentID))
}

func uintPtr(v uint) *uint { return &v }

func defaultReqOptions(release string) reqopts.RequestOptions {
	return reqopts.RequestOptions{
		SampleRelease: reqopts.Release{
			Name:  release,
			Start: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
		},
		BaseRelease: reqopts.Release{
			Name:  release,
			Start: time.Date(2024, 5, 15, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		},
		VariantOption: reqopts.Variants{
			DBGroupBy:     sets.New[string]("Platform", "Network"),
			ColumnGroupBy: sets.New[string]("Platform"),
		},
		AdvancedOption: reqopts.Advanced{
			MinimumFailure: 1,
		},
	}
}

// crSeedData holds the shared seed data used by multiple tests.
type crSeedData struct {
	vcAWS   models.VariantCombination
	vcGCP   models.VariantCombination
	vcAWS2  models.VariantCombination // same DBGroupBy dims as vcAWS, different non-grouped dim
	jobAWS  models.ProwJob
	jobGCP  models.ProwJob
	jobAWS2 models.ProwJob
	test1   models.Test
	test2   models.Test
	test3   models.Test
	suite   models.Suite
	tow1    models.TestOwnership
	tow2    models.TestOwnership
	tow3    models.TestOwnership
}

// seedCRData creates a standard dataset for prefix-sum tests using release "4.16":
// - 3 variant combinations: aws+ovn, gcp+sdn, aws+sdn (same Platform=aws as vcAWS but different Network)
// - 3 prow jobs, one per VC
// - 3 tests across 2 components, with test ownerships
// - Cumulative summaries for a known date range
func seedCRData(t *testing.T, dbc *db.DB) crSeedData {
	t.Helper()
	release := "4.16"

	vcAWS := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	vcGCP := createVariantCombination(t, dbc, []string{"Platform:gcp", "Network:sdn"})
	vcAWS2 := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:sdn"})

	jobAWS := createProwJobWithVC(t, dbc, "periodic-e2e-aws-ovn", release, vcAWS)
	jobGCP := createProwJobWithVC(t, dbc, "periodic-e2e-gcp-sdn", release, vcGCP)
	jobAWS2 := createProwJobWithVC(t, dbc, "periodic-e2e-aws-sdn", release, vcAWS2)

	test1 := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] PVC should work")
	test2 := intutil.CreateTest(t, dbc, "openshift-tests:[sig-network] Services should serve")
	test3 := intutil.CreateTest(t, dbc, "openshift-tests:[sig-auth] RBAC should restrict")

	suite := intutil.CreateSuite(t, dbc, "openshift-tests")

	tow1 := createTestOwnership(t, dbc, test1.ID, &suite.ID, "openshift-tests:aaa", "Storage", []string{"PersistentVolumes", "IPv4"})
	tow2 := createTestOwnership(t, dbc, test2.ID, &suite.ID, "openshift-tests:bbb", "Networking", []string{"Services", "IPv4"})
	tow3 := createTestOwnership(t, dbc, test3.ID, &suite.ID, "openshift-tests:ccc", "Authentication", []string{"RBAC"})

	// Prefix sums: the query computes counts = end_date_value - start_date_value.
	// Date range [2024-06-01, 2024-06-15): lookupEnd = 2024-06-14, lookupStart = 2024-05-31.
	// We create rows at 2024-05-31 (start-1) and 2024-06-14 (end-1).
	startMinus1 := civil.Date{Year: 2024, Month: 5, Day: 31}
	endMinus1 := civil.Date{Year: 2024, Month: 6, Day: 14}

	// test1 on jobAWS: runs=10, successes=8, flakes=1 -> failures=1
	createCumulativeSummary(t, dbc, startMinus1, release, test1.ID, jobAWS.ID, suite.ID, 100, 90, 5)
	createCumulativeSummary(t, dbc, endMinus1, release, test1.ID, jobAWS.ID, suite.ID, 110, 98, 6)

	// test2 on jobGCP: runs=20, successes=15, flakes=2 -> failures=3
	createCumulativeSummary(t, dbc, startMinus1, release, test2.ID, jobGCP.ID, suite.ID, 50, 40, 3)
	createCumulativeSummary(t, dbc, endMinus1, release, test2.ID, jobGCP.ID, suite.ID, 70, 55, 5)

	// test1 on jobAWS2: runs=5, successes=4, flakes=0 -> failures=1
	createCumulativeSummary(t, dbc, startMinus1, release, test1.ID, jobAWS2.ID, suite.ID, 30, 25, 2)
	createCumulativeSummary(t, dbc, endMinus1, release, test1.ID, jobAWS2.ID, suite.ID, 35, 29, 2)

	// test3 on jobAWS: runs=8, successes=8, flakes=0 -> failures=0 (used for placeholder test)
	createCumulativeSummary(t, dbc, startMinus1, release, test3.ID, jobAWS.ID, suite.ID, 40, 40, 0)
	createCumulativeSummary(t, dbc, endMinus1, release, test3.ID, jobAWS.ID, suite.ID, 48, 48, 0)

	return crSeedData{
		vcAWS: vcAWS, vcGCP: vcGCP, vcAWS2: vcAWS2,
		jobAWS: jobAWS, jobGCP: jobGCP, jobAWS2: jobAWS2,
		test1: test1, test2: test2, test3: test3,
		suite: suite,
		tow1:  tow1, tow2: tow2, tow3: tow3,
	}
}

func TestQuerySampleTestStatus(t *testing.T) {
	t.Run("basic aggregation", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"
		seed := seedCRData(t, dbc)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		includeVariants := map[string][]string{
			"Platform": {"aws", "gcp"},
			"Network":  {"ovn", "sdn"},
		}

		result, errs := provider.QuerySampleTestStatus(context.Background(), opts, includeVariants,
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)
		require.NotEmpty(t, result)

		// test1 on jobAWS (Platform:aws, Network:ovn): runs=10, successes=8, flakes=1
		awsOvnKey := crtest.KeyWithVariants{
			TestID:   seed.tow1.UniqueID,
			Variants: map[string]string{"Platform": "aws", "Network": "ovn"},
		}
		ts, ok := result[awsOvnKey.Encode()]
		require.True(t, ok, "expected key %s in results", awsOvnKey.Encode())
		assert.Equal(t, 10, ts.TotalCount)
		assert.Equal(t, 8, ts.SuccessCount)
		assert.Equal(t, 1, ts.FlakeCount)
		assert.Equal(t, "Storage", ts.Component)

		// test2 on jobGCP (Platform:gcp, Network:sdn): runs=20, successes=15, flakes=2
		gcpSdnKey := crtest.KeyWithVariants{
			TestID:   seed.tow2.UniqueID,
			Variants: map[string]string{"Platform": "gcp", "Network": "sdn"},
		}
		ts2, ok := result[gcpSdnKey.Encode()]
		require.True(t, ok, "expected key %s in results", gcpSdnKey.Encode())
		assert.Equal(t, 20, ts2.TotalCount)
		assert.Equal(t, 15, ts2.SuccessCount)
		assert.Equal(t, 2, ts2.FlakeCount)
		assert.Equal(t, "Networking", ts2.Component)

		// test1 on jobAWS2 (Platform:aws, Network:sdn): runs=5, successes=4, flakes=0
		awsSdnKey := crtest.KeyWithVariants{
			TestID:   seed.tow1.UniqueID,
			Variants: map[string]string{"Platform": "aws", "Network": "sdn"},
		}
		ts3, ok := result[awsSdnKey.Encode()]
		require.True(t, ok, "expected key %s in results", awsSdnKey.Encode())
		assert.Equal(t, 5, ts3.TotalCount)
		assert.Equal(t, 4, ts3.SuccessCount)
		assert.Equal(t, 0, ts3.FlakeCount)
	})

	t.Run("variant filter", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"
		seedCRData(t, dbc)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		includeVariants := map[string][]string{
			"Platform": {"aws"},
		}

		result, errs := provider.QuerySampleTestStatus(context.Background(), opts, includeVariants,
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)

		for _, ts := range result {
			if ts.Variants != nil {
				platform, ok := ts.Variants["Platform"]
				if ok {
					assert.Equal(t, "aws", platform, "only aws variants should be returned")
				}
			}
		}

		for key := range result {
			assert.NotContains(t, key, "gcp", "gcp variant should be filtered out")
		}
	})

	t.Run("variant grouping collapses same DBGroupBy dims", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		// Two VCs that share Platform:aws but differ on a non-grouped variant (Topology)
		vc1 := createVariantCombination(t, dbc, []string{"Platform:aws", "Topology:ha"})
		vc2 := createVariantCombination(t, dbc, []string{"Platform:aws", "Topology:single"})

		job1 := createProwJobWithVC(t, dbc, "periodic-e2e-aws-ha", release, vc1)
		job2 := createProwJobWithVC(t, dbc, "periodic-e2e-aws-single", release, vc2)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] PVC test")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests")
		createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:pvc", "Storage", []string{"PVC"})

		startMinus1 := civil.Date{Year: 2024, Month: 5, Day: 31}
		endMinus1 := civil.Date{Year: 2024, Month: 6, Day: 14}

		// job1: runs=10, successes=8, flakes=1
		createCumulativeSummary(t, dbc, startMinus1, release, test.ID, job1.ID, suite.ID, 100, 90, 5)
		createCumulativeSummary(t, dbc, endMinus1, release, test.ID, job1.ID, suite.ID, 110, 98, 6)

		// job2: runs=6, successes=5, flakes=0
		createCumulativeSummary(t, dbc, startMinus1, release, test.ID, job2.ID, suite.ID, 50, 45, 2)
		createCumulativeSummary(t, dbc, endMinus1, release, test.ID, job2.ID, suite.ID, 56, 50, 2)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		opts.VariantOption.DBGroupBy = sets.New[string]("Platform")

		result, errs := provider.QuerySampleTestStatus(context.Background(), opts,
			map[string][]string{"Platform": {"aws"}},
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)

		// Both jobs collapse to the same group (Platform:aws), aggregating counts
		key := crtest.KeyWithVariants{
			TestID:   "openshift-tests:pvc",
			Variants: map[string]string{"Platform": "aws"},
		}
		ts, ok := result[key.Encode()]
		require.True(t, ok, "expected collapsed group key")
		assert.Equal(t, 16, ts.TotalCount, "runs should aggregate: 10 + 6")
		assert.Equal(t, 13, ts.SuccessCount, "successes should aggregate: 8 + 5")
		assert.Equal(t, 1, ts.FlakeCount, "flakes should aggregate: 1 + 0")
	})

	t.Run("minimum failure threshold", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"
		seed := seedCRData(t, dbc)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		opts.AdvancedOption.MinimumFailure = 2
		includeVariants := map[string][]string{
			"Platform": {"aws", "gcp"},
			"Network":  {"ovn", "sdn"},
		}

		result, errs := provider.QuerySampleTestStatus(context.Background(), opts, includeVariants,
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)

		// test2/jobGCP has 3 failures (>= 2), should appear via failure query
		gcpKey := crtest.KeyWithVariants{
			TestID:   seed.tow2.UniqueID,
			Variants: map[string]string{"Platform": "gcp", "Network": "sdn"},
		}
		ts, ok := result[gcpKey.Encode()]
		require.True(t, ok, "test2/gcp with 3 failures should pass MinimumFailure=2")
		assert.Equal(t, 20, ts.TotalCount)

		// test1/jobAWS has 1 failure (< 2), should NOT appear in failure results
		// but a placeholder for its component should exist
		awsOvnKey := crtest.KeyWithVariants{
			TestID:   seed.tow1.UniqueID,
			Variants: map[string]string{"Platform": "aws", "Network": "ovn"},
		}
		_, hasDirect := result[awsOvnKey.Encode()]
		assert.False(t, hasDirect, "test1/aws-ovn with 1 failure should not pass MinimumFailure=2 filter")
	})

	t.Run("no data for release", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.99"

		// Create at least one VC so the query doesn't short-circuit on empty variantLookup
		vc := createVariantCombination(t, dbc, []string{"Platform:aws"})
		createProwJobWithVC(t, dbc, "periodic-e2e-aws-empty", release, vc)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)

		result, errs := provider.QuerySampleTestStatus(context.Background(), opts,
			map[string][]string{"Platform": {"aws"}},
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)
		assert.Empty(t, result)
	})

	t.Run("RequestedVariants drill-down", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"
		seed := seedCRData(t, dbc)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		opts.TestIDOptions = []reqopts.TestIdentification{{
			RequestedVariants: map[string]string{"Platform": "aws", "Network": "ovn"},
		}}
		includeVariants := map[string][]string{
			"Platform": {"aws", "gcp"},
			"Network":  {"ovn", "sdn"},
		}

		result, errs := provider.QuerySampleTestStatus(context.Background(), opts, includeVariants,
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)

		// Only aws+ovn VC should match
		for _, ts := range result {
			if ts.Variants != nil {
				if p, ok := ts.Variants["Platform"]; ok {
					assert.Equal(t, "aws", p)
				}
				if n, ok := ts.Variants["Network"]; ok {
					assert.Equal(t, "ovn", n)
				}
			}
		}

		// test1 on aws+ovn should be present
		awsOvnKey := crtest.KeyWithVariants{
			TestID:   seed.tow1.UniqueID,
			Variants: map[string]string{"Platform": "aws", "Network": "ovn"},
		}
		_, ok := result[awsOvnKey.Encode()]
		assert.True(t, ok, "test1 on aws+ovn should be present with RequestedVariants drill-down")
	})

	t.Run("TestID drill-down", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"
		seed := seedCRData(t, dbc)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		opts.TestIDOptions = []reqopts.TestIdentification{{
			TestID: seed.tow1.UniqueID, // "openshift-tests:aaa"
		}}
		includeVariants := map[string][]string{
			"Platform": {"aws", "gcp"},
			"Network":  {"ovn", "sdn"},
		}

		result, errs := provider.QuerySampleTestStatus(context.Background(), opts, includeVariants,
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)

		// Only test1 entries should appear (excluding grid placeholders)
		for _, ts := range result {
			if isPlaceholderKey(ts.TestID) {
				continue
			}
			assert.Equal(t, seed.tow1.UniqueID, ts.TestID,
				"only test1 entries should appear with TestID drill-down")
		}
	})

	t.Run("Capability drill-down", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"
		seedCRData(t, dbc)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		opts.TestIDOptions = []reqopts.TestIdentification{{
			Capability: "RBAC",
		}}
		includeVariants := map[string][]string{
			"Platform": {"aws", "gcp"},
			"Network":  {"ovn", "sdn"},
		}

		result, errs := provider.QuerySampleTestStatus(context.Background(), opts, includeVariants,
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)

		// test3 has RBAC capability but 0 failures, so with MinimumFailure=1
		// it only appears as a placeholder. Check that no non-RBAC tests are returned.
		for _, ts := range result {
			if ts.TestID != "" && !isPlaceholderKey(ts.TestID) {
				assert.Contains(t, ts.Capabilities, "RBAC",
					"only tests with RBAC capability should appear")
			}
		}
	})

	t.Run("release isolation", func(t *testing.T) {
		dbc := crTestDB(t)
		sampleRelease := "4.17"
		distractorRelease := "4.16"

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		sampleJob := createProwJobWithVC(t, dbc, "periodic-e2e-aws-sample", sampleRelease, vc)
		distractorJob := createProwJobWithVC(t, dbc, "periodic-e2e-aws-distractor", distractorRelease, vc)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] release isolation")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-ri")
		tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:ri-test", "Storage", []string{"PVC"})

		startMinus1 := civil.Date{Year: 2024, Month: 5, Day: 31}
		endMinus1 := civil.Date{Year: 2024, Month: 6, Day: 14}

		// Sample release: runs=10, successes=8, flakes=1
		createCumulativeSummary(t, dbc, startMinus1, sampleRelease, test.ID, sampleJob.ID, suite.ID, 100, 90, 5)
		createCumulativeSummary(t, dbc, endMinus1, sampleRelease, test.ID, sampleJob.ID, suite.ID, 110, 98, 6)

		// Distractor release: much larger counts that would be obvious if leaked
		createCumulativeSummary(t, dbc, startMinus1, distractorRelease, test.ID, distractorJob.ID, suite.ID, 1000, 900, 50)
		createCumulativeSummary(t, dbc, endMinus1, distractorRelease, test.ID, distractorJob.ID, suite.ID, 2000, 1800, 100)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(sampleRelease)

		result, errs := provider.QuerySampleTestStatus(context.Background(), opts,
			map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}},
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)

		key := crtest.KeyWithVariants{
			TestID:   tow.UniqueID,
			Variants: map[string]string{"Platform": "aws", "Network": "ovn"},
		}
		ts, ok := result[key.Encode()]
		require.True(t, ok, "expected result for sample release")
		assert.Equal(t, 10, ts.TotalCount, "should only include data from the queried release, not the distractor")
	})

	t.Run("obsolete ownership excluded", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		job := createProwJobWithVC(t, dbc, "periodic-e2e-aws-obs", release, vc)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] obsolete test")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-obs")

		obsOwnership := models.TestOwnership{
			TestID:                test.ID,
			SuiteID:               &suite.ID,
			UniqueID:              "openshift-tests:obsolete",
			Name:                  "openshift-tests:obsolete",
			Component:             "Storage",
			Capabilities:          pq.StringArray{"PVC"},
			StaffApprovedObsolete: true,
		}
		require.NoError(t, dbc.DB.Create(&obsOwnership).Error)

		startMinus1 := civil.Date{Year: 2024, Month: 5, Day: 31}
		endMinus1 := civil.Date{Year: 2024, Month: 6, Day: 14}
		createCumulativeSummary(t, dbc, startMinus1, release, test.ID, job.ID, suite.ID, 100, 90, 5)
		createCumulativeSummary(t, dbc, endMinus1, release, test.ID, job.ID, suite.ID, 110, 98, 6)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)

		result, errs := provider.QuerySampleTestStatus(context.Background(), opts,
			map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}},
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)

		for _, ts := range result {
			assert.NotEqual(t, "openshift-tests:obsolete", ts.TestID,
				"obsolete test ownership should be excluded from results")
		}
	})

	t.Run("suite isolation", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		job := createProwJobWithVC(t, dbc, "periodic-e2e-aws-suite", release, vc)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] multi-suite test")
		suiteA := intutil.CreateSuite(t, dbc, "suite-a")
		suiteB := intutil.CreateSuite(t, dbc, "suite-b")

		towA := createTestOwnership(t, dbc, test.ID, &suiteA.ID, "suite-a:multi", "Storage", []string{"PVC"})
		towB := createTestOwnership(t, dbc, test.ID, &suiteB.ID, "suite-b:multi", "Storage", []string{"PVC"})

		startMinus1 := civil.Date{Year: 2024, Month: 5, Day: 31}
		endMinus1 := civil.Date{Year: 2024, Month: 6, Day: 14}

		// suite-a: runs=10, successes=8, flakes=1
		createCumulativeSummary(t, dbc, startMinus1, release, test.ID, job.ID, suiteA.ID, 100, 90, 5)
		createCumulativeSummary(t, dbc, endMinus1, release, test.ID, job.ID, suiteA.ID, 110, 98, 6)

		// suite-b: runs=20, successes=15, flakes=2
		createCumulativeSummary(t, dbc, startMinus1, release, test.ID, job.ID, suiteB.ID, 50, 40, 3)
		createCumulativeSummary(t, dbc, endMinus1, release, test.ID, job.ID, suiteB.ID, 70, 55, 5)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)

		result, errs := provider.QuerySampleTestStatus(context.Background(), opts,
			map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}},
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)

		keyA := crtest.KeyWithVariants{
			TestID:   towA.UniqueID,
			Variants: map[string]string{"Platform": "aws", "Network": "ovn"},
		}
		keyB := crtest.KeyWithVariants{
			TestID:   towB.UniqueID,
			Variants: map[string]string{"Platform": "aws", "Network": "ovn"},
		}

		tsA, ok := result[keyA.Encode()]
		require.True(t, ok, "suite-a result should be present")
		assert.Equal(t, 10, tsA.TotalCount)
		assert.Equal(t, "suite-a", tsA.TestSuite)

		tsB, ok := result[keyB.Encode()]
		require.True(t, ok, "suite-b result should be present")
		assert.Equal(t, 20, tsB.TotalCount)
		assert.Equal(t, "suite-b", tsB.TestSuite)
	})

	t.Run("sample query uses compare-side variants in cross-compare view", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vcHA := createVariantCombination(t, dbc, []string{"Platform:aws", "Topology:ha"})
		vcSingle := createVariantCombination(t, dbc, []string{"Platform:aws", "Topology:single"})

		jobHA := createProwJobWithVC(t, dbc, "periodic-e2e-aws-ha", release, vcHA)
		jobSingle := createProwJobWithVC(t, dbc, "periodic-e2e-aws-single", release, vcSingle)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] cross-compare test")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-cc")
		createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:cc-test", "Storage", []string{"PVC"})

		startMinus1 := civil.Date{Year: 2024, Month: 5, Day: 31}
		endMinus1 := civil.Date{Year: 2024, Month: 6, Day: 14}

		// HA: runs=10, successes=8, flakes=1
		createCumulativeSummary(t, dbc, startMinus1, release, test.ID, jobHA.ID, suite.ID, 100, 90, 5)
		createCumulativeSummary(t, dbc, endMinus1, release, test.ID, jobHA.ID, suite.ID, 110, 98, 6)

		// Single: runs=20, successes=15, flakes=2
		createCumulativeSummary(t, dbc, startMinus1, release, test.ID, jobSingle.ID, suite.ID, 50, 40, 3)
		createCumulativeSummary(t, dbc, endMinus1, release, test.ID, jobSingle.ID, suite.ID, 70, 55, 5)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		opts.VariantOption.DBGroupBy = sets.New[string]("Platform", "Topology")
		opts.VariantOption.ColumnGroupBy = sets.New[string]("Platform")
		// Base-side: Topology:ha, but cross-compare swaps to Topology:single for sample
		opts.VariantOption.VariantCrossCompare = []string{"Topology"}
		opts.VariantOption.CompareVariants = map[string][]string{"Topology": {"single"}}

		// includeVariants has the base-side value
		includeVariants := map[string][]string{
			"Platform": {"aws"},
			"Topology": {"ha"},
		}

		result, errs := provider.QuerySampleTestStatus(context.Background(), opts, includeVariants,
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)

		nonPlaceholders := filterPlaceholders(result)
		require.NotEmpty(t, nonPlaceholders, "should return results for cross-compare")
		for _, ts := range nonPlaceholders {
			assert.Equal(t, "single", ts.Variants["Topology"],
				"should return sample-side (single) data, not base-side (ha)")
			assert.Equal(t, 20, ts.TotalCount)
		}
	})

	t.Run("most recent failure date is included in results", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		job := createProwJobWithVC(t, dbc, "periodic-e2e-aws-lf", release, vc)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] last failure test")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-lf")
		tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:lf-test", "Storage", []string{"PVC"})

		startMinus1 := civil.Date{Year: 2024, Month: 5, Day: 31}
		endMinus1 := civil.Date{Year: 2024, Month: 6, Day: 14}

		failTime := time.Date(2024, 6, 10, 18, 0, 0, 0, time.UTC)

		// runs=10, successes=8, flakes=1 -> failures=1
		// The end-of-window row carries the prefix_max_last_failure timestamp.
		createCumulativeSummary(t, dbc, startMinus1, release, test.ID, job.ID, suite.ID, 100, 90, 5)
		createCumulativeSummary(t, dbc, endMinus1, release, test.ID, job.ID, suite.ID, 110, 98, 6, withLastFailure(failTime))

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)

		result, errs := provider.QuerySampleTestStatus(context.Background(), opts,
			map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}},
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)

		key := crtest.KeyWithVariants{
			TestID:   tow.UniqueID,
			Variants: map[string]string{"Platform": "aws", "Network": "ovn"},
		}
		ts, ok := result[key.Encode()]
		require.True(t, ok, "expected result for test with failures")
		assert.False(t, ts.LastFailure.IsZero(), "LastFailure should be populated when prefix_max_last_failure is set")
		assert.Equal(t, failTime, ts.LastFailure,
			"LastFailure should match prefix_max_last_failure: got %v, want %v", ts.LastFailure, failTime)
	})

	t.Run("last failure picks MAX across multiple jobs in same variant group", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vc1 := createVariantCombination(t, dbc, []string{"Platform:aws", "Topology:ha"})
		vc2 := createVariantCombination(t, dbc, []string{"Platform:aws", "Topology:single"})
		job1 := createProwJobWithVC(t, dbc, "periodic-e2e-aws-ha-lf", release, vc1)
		job2 := createProwJobWithVC(t, dbc, "periodic-e2e-aws-single-lf", release, vc2)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] multi-job last failure")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-mj-lf")
		tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:mj-lf-test", "Storage", []string{"PVC"})

		startMinus1 := civil.Date{Year: 2024, Month: 5, Day: 31}
		endMinus1 := civil.Date{Year: 2024, Month: 6, Day: 14}

		earlierFailure := time.Date(2024, 6, 5, 12, 0, 0, 0, time.UTC)
		laterFailure := time.Date(2024, 6, 12, 9, 0, 0, 0, time.UTC)

		// job1: runs=6, successes=4, flakes=1 -> failures=1, earlier failure timestamp
		createCumulativeSummary(t, dbc, startMinus1, release, test.ID, job1.ID, suite.ID, 50, 45, 2)
		createCumulativeSummary(t, dbc, endMinus1, release, test.ID, job1.ID, suite.ID, 56, 49, 3, withLastFailure(earlierFailure))

		// job2: runs=8, successes=6, flakes=1 -> failures=1, later failure timestamp
		createCumulativeSummary(t, dbc, startMinus1, release, test.ID, job2.ID, suite.ID, 30, 25, 1)
		createCumulativeSummary(t, dbc, endMinus1, release, test.ID, job2.ID, suite.ID, 38, 31, 2, withLastFailure(laterFailure))

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		opts.VariantOption.DBGroupBy = sets.New[string]("Platform")

		result, errs := provider.QuerySampleTestStatus(context.Background(), opts,
			map[string][]string{"Platform": {"aws"}},
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)

		key := crtest.KeyWithVariants{
			TestID:   tow.UniqueID,
			Variants: map[string]string{"Platform": "aws"},
		}
		ts, ok := result[key.Encode()]
		require.True(t, ok, "expected collapsed group result")
		assert.Equal(t, 14, ts.TotalCount, "runs should aggregate: 6 + 8")
		assert.Equal(t, laterFailure, ts.LastFailure,
			"LastFailure should be MAX across jobs: got %v, want %v", ts.LastFailure, laterFailure)
	})

	t.Run("last failure picks non-NULL when mixed with NULL across jobs", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vc1 := createVariantCombination(t, dbc, []string{"Platform:aws", "Topology:ha"})
		vc2 := createVariantCombination(t, dbc, []string{"Platform:aws", "Topology:single"})
		job1 := createProwJobWithVC(t, dbc, "periodic-e2e-aws-ha-mixed", release, vc1)
		job2 := createProwJobWithVC(t, dbc, "periodic-e2e-aws-single-mixed", release, vc2)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] mixed null last failure")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-mix-lf")
		tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:mix-lf-test", "Storage", []string{"PVC"})

		startMinus1 := civil.Date{Year: 2024, Month: 5, Day: 31}
		endMinus1 := civil.Date{Year: 2024, Month: 6, Day: 14}

		failTime := time.Date(2024, 6, 8, 15, 0, 0, 0, time.UTC)

		// job1: has a failure timestamp
		createCumulativeSummary(t, dbc, startMinus1, release, test.ID, job1.ID, suite.ID, 50, 45, 2)
		createCumulativeSummary(t, dbc, endMinus1, release, test.ID, job1.ID, suite.ID, 56, 49, 3, withLastFailure(failTime))

		// job2: no failure timestamp (NULL prefix_max_last_failure)
		createCumulativeSummary(t, dbc, startMinus1, release, test.ID, job2.ID, suite.ID, 30, 25, 1)
		createCumulativeSummary(t, dbc, endMinus1, release, test.ID, job2.ID, suite.ID, 38, 31, 2)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		opts.VariantOption.DBGroupBy = sets.New[string]("Platform")

		result, errs := provider.QuerySampleTestStatus(context.Background(), opts,
			map[string][]string{"Platform": {"aws"}},
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)

		key := crtest.KeyWithVariants{
			TestID:   tow.UniqueID,
			Variants: map[string]string{"Platform": "aws"},
		}
		ts, ok := result[key.Encode()]
		require.True(t, ok, "expected collapsed group result")
		assert.Equal(t, failTime, ts.LastFailure,
			"LastFailure should be the non-NULL value: got %v, want %v", ts.LastFailure, failTime)
	})

	t.Run("results limited to available data when requested range exceeds it", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		job := createProwJobWithVC(t, dbc, "periodic-e2e-aws-clamp", release, vc)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] clamping test")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-clamp")
		tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:clamp-test", "Storage", []string{"PVC"})

		startMinus1 := civil.Date{Year: 2024, Month: 5, Day: 31}
		// Data only goes up to June 10, not June 14 (which the default range expects)
		clampedEnd := civil.Date{Year: 2024, Month: 6, Day: 10}

		createCumulativeSummary(t, dbc, startMinus1, release, test.ID, job.ID, suite.ID, 100, 90, 5)
		createCumulativeSummary(t, dbc, clampedEnd, release, test.ID, job.ID, suite.ID, 107, 95, 6)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)

		result, errs := provider.QuerySampleTestStatus(context.Background(), opts,
			map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}},
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)

		key := crtest.KeyWithVariants{
			TestID:   tow.UniqueID,
			Variants: map[string]string{"Platform": "aws", "Network": "ovn"},
		}
		ts, ok := result[key.Encode()]
		require.True(t, ok, "expected result even with clamped date range")
		assert.Equal(t, 7, ts.TotalCount, "counts should reflect only the available data period")
	})

	t.Run("tests without a suite appear with empty suite name", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		job := createProwJobWithVC(t, dbc, "periodic-e2e-aws-nosuite", release, vc)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] no suite test")
		tow := createTestOwnership(t, dbc, test.ID, nil, "openshift-tests:nosuite-test", "Storage", []string{"PVC"})

		startMinus1 := civil.Date{Year: 2024, Month: 5, Day: 31}
		endMinus1 := civil.Date{Year: 2024, Month: 6, Day: 14}

		createCumulativeSummary(t, dbc, startMinus1, release, test.ID, job.ID, 0, 100, 90, 5)
		createCumulativeSummary(t, dbc, endMinus1, release, test.ID, job.ID, 0, 110, 98, 6)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)

		result, errs := provider.QuerySampleTestStatus(context.Background(), opts,
			map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}},
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)

		key := crtest.KeyWithVariants{
			TestID:   tow.UniqueID,
			Variants: map[string]string{"Platform": "aws", "Network": "ovn"},
		}
		ts, ok := result[key.Encode()]
		require.True(t, ok, "test with NULL suite_id should appear in results")
		assert.Equal(t, 10, ts.TotalCount)
		assert.Equal(t, "", ts.TestSuite, "suite name should be empty when test has no suite")
	})

	t.Run("test with no data before reporting period returns full period counts", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		job := createProwJobWithVC(t, dbc, "periodic-e2e-aws-coalesce", release, vc)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] coalesce test")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-co")
		tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:coalesce-test", "Storage", []string{"PVC"})

		endMinus1 := civil.Date{Year: 2024, Month: 6, Day: 14}
		createCumulativeSummary(t, dbc, endMinus1, release, test.ID, job.ID, suite.ID, 50, 40, 3)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)

		result, errs := provider.QuerySampleTestStatus(context.Background(), opts,
			map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}},
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)

		key := crtest.KeyWithVariants{
			TestID:   tow.UniqueID,
			Variants: map[string]string{"Platform": "aws", "Network": "ovn"},
		}
		ts, ok := result[key.Encode()]
		require.True(t, ok, "test should appear even with no prior data")
		assert.Equal(t, 50, ts.TotalCount)
		assert.Equal(t, 40, ts.SuccessCount)
		assert.Equal(t, 3, ts.FlakeCount)
	})

	t.Run("failure counts preserved when test also qualifies as placeholder", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		job := createProwJobWithVC(t, dbc, "periodic-e2e-aws-merge", release, vc)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] merge test")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-merge")
		tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:merge-test", "Storage", []string{"PVC"})

		startMinus1 := civil.Date{Year: 2024, Month: 5, Day: 31}
		endMinus1 := civil.Date{Year: 2024, Month: 6, Day: 14}

		// runs=10, successes=7, flakes=1 -> failures=2
		createCumulativeSummary(t, dbc, startMinus1, release, test.ID, job.ID, suite.ID, 100, 90, 5)
		createCumulativeSummary(t, dbc, endMinus1, release, test.ID, job.ID, suite.ID, 110, 97, 6)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		opts.AdvancedOption.MinimumFailure = 1

		result, errs := provider.QuerySampleTestStatus(context.Background(), opts,
			map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}},
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)

		key := crtest.KeyWithVariants{
			TestID:   tow.UniqueID,
			Variants: map[string]string{"Platform": "aws", "Network": "ovn"},
		}
		ts, ok := result[key.Encode()]
		require.True(t, ok, "test should appear via failure query")
		assert.Equal(t, 10, ts.TotalCount, "failure counts should be preserved, not zeroed by placeholder")
		assert.Equal(t, 7, ts.SuccessCount)
		assert.Equal(t, 1, ts.FlakeCount)
	})

	t.Run("combined drill-down", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"
		seed := seedCRData(t, dbc)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		opts.TestIDOptions = []reqopts.TestIdentification{{
			TestID:            seed.tow1.UniqueID,
			Capability:        "PersistentVolumes",
			RequestedVariants: map[string]string{"Platform": "aws", "Network": "ovn"},
		}}
		includeVariants := map[string][]string{
			"Platform": {"aws", "gcp"},
			"Network":  {"ovn", "sdn"},
		}

		result, errs := provider.QuerySampleTestStatus(context.Background(), opts, includeVariants,
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)

		// Should get exactly test1 on aws+ovn
		nonPlaceholder := filterPlaceholders(result)
		require.Len(t, nonPlaceholder, 1, "combined drill-down should yield exactly one non-placeholder result")

		ts := nonPlaceholder[0]
		assert.Equal(t, seed.tow1.UniqueID, ts.TestID)
		assert.Equal(t, "aws", ts.Variants["Platform"])
		assert.Equal(t, "ovn", ts.Variants["Network"])
		assert.Equal(t, 10, ts.TotalCount)
	})
}

func TestQueryBaseTestStatus_PrefixSum(t *testing.T) {
	dbc := crTestDB(t)
	release := "4.16"
	seed := seedCRData(t, dbc)

	// Seed base period data: BaseRelease = [2024-05-15, 2024-06-01)
	// QueryBaseTestStatus constructs: baseRange = {Start:2024-05-15, End:2024-06-02}
	// lookupStart = Start.AddDays(-1) = 2024-05-14
	// lookupEnd = End.AddDays(-1) = 2024-06-01
	baseLookupStart := civil.Date{Year: 2024, Month: 5, Day: 14}
	baseLookupEnd := civil.Date{Year: 2024, Month: 6, Day: 1}

	createCumulativeSummary(t, dbc, baseLookupStart, release, seed.test1.ID, seed.jobAWS.ID, seed.suite.ID, 80, 75, 3)
	createCumulativeSummary(t, dbc, baseLookupEnd, release, seed.test1.ID, seed.jobAWS.ID, seed.suite.ID, 100, 90, 5)
	// base_runs = 100-80=20, base_successes = 90-75=15, base_flakes = 5-3=2

	provider := postgres.NewPostgresProvider(dbc, nil)
	opts := defaultReqOptions(release)
	includeVariants := map[string][]string{
		"Platform": {"aws"},
		"Network":  {"ovn"},
	}
	opts.VariantOption.IncludeVariants = includeVariants

	result, errs := provider.QueryBaseTestStatus(context.Background(), opts)
	require.Empty(t, errs)

	awsOvnKey := crtest.KeyWithVariants{
		TestID:   seed.tow1.UniqueID,
		Variants: map[string]string{"Platform": "aws", "Network": "ovn"},
	}
	ts, ok := result[awsOvnKey.Encode()]
	require.True(t, ok, "expected base test status for test1 on aws+ovn")
	assert.Equal(t, 20, ts.TotalCount)
	assert.Equal(t, 15, ts.SuccessCount)
	assert.Equal(t, 2, ts.FlakeCount)
}

func TestQueryTestStatus_DifferentBaseAndSampleReleases(t *testing.T) {
	dbc := crTestDB(t)
	baseRelease := "4.16"
	sampleRelease := "4.17"

	vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	baseJob := createProwJobWithVC(t, dbc, "periodic-e2e-aws-base", baseRelease, vc)
	sampleJob := createProwJobWithVC(t, dbc, "periodic-e2e-aws-sample", sampleRelease, vc)

	test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] cross-release test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests-xr")
	tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:xr-test", "Storage", []string{"PVC"})

	// Base period: [2024-05-15, 2024-06-01) -> lookupStart=2024-05-14, lookupEnd=2024-06-01
	// runs=20, successes=15, flakes=2
	baseLookupStart := civil.Date{Year: 2024, Month: 5, Day: 14}
	baseLookupEnd := civil.Date{Year: 2024, Month: 6, Day: 1}
	createCumulativeSummary(t, dbc, baseLookupStart, baseRelease, test.ID, baseJob.ID, suite.ID, 80, 75, 3)
	createCumulativeSummary(t, dbc, baseLookupEnd, baseRelease, test.ID, baseJob.ID, suite.ID, 100, 90, 5)

	// Sample period: [2024-06-01, 2024-06-15) -> lookupStart=2024-05-31, lookupEnd=2024-06-14
	// runs=10, successes=8, flakes=1
	sampleLookupStart := civil.Date{Year: 2024, Month: 5, Day: 31}
	sampleLookupEnd := civil.Date{Year: 2024, Month: 6, Day: 14}
	createCumulativeSummary(t, dbc, sampleLookupStart, sampleRelease, test.ID, sampleJob.ID, suite.ID, 100, 90, 5)
	createCumulativeSummary(t, dbc, sampleLookupEnd, sampleRelease, test.ID, sampleJob.ID, suite.ID, 110, 98, 6)

	provider := postgres.NewPostgresProvider(dbc, nil)
	opts := reqopts.RequestOptions{
		BaseRelease: reqopts.Release{
			Name:  baseRelease,
			Start: time.Date(2024, 5, 15, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		},
		SampleRelease: reqopts.Release{
			Name:  sampleRelease,
			Start: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
		},
		VariantOption: reqopts.Variants{
			DBGroupBy:       sets.New[string]("Platform", "Network"),
			ColumnGroupBy:   sets.New[string]("Platform"),
			IncludeVariants: map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}},
		},
		AdvancedOption: reqopts.Advanced{
			MinimumFailure: 1,
		},
	}

	key := crtest.KeyWithVariants{
		TestID:   tow.UniqueID,
		Variants: map[string]string{"Platform": "aws", "Network": "ovn"},
	}

	baseResult, errs := provider.QueryBaseTestStatus(context.Background(), opts)
	require.Empty(t, errs)

	baseTS, ok := baseResult[key.Encode()]
	require.True(t, ok, "expected base result")
	assert.Equal(t, 20, baseTS.TotalCount, "base should reflect baseRelease data only")
	assert.Equal(t, 15, baseTS.SuccessCount)

	sampleResult, errs := provider.QuerySampleTestStatus(context.Background(), opts,
		opts.VariantOption.IncludeVariants,
		opts.SampleRelease.Start, opts.SampleRelease.End)
	require.Empty(t, errs)

	sampleTS, ok := sampleResult[key.Encode()]
	require.True(t, ok, "expected sample result")
	assert.Equal(t, 10, sampleTS.TotalCount, "sample should reflect sampleRelease data only")
	assert.Equal(t, 8, sampleTS.SuccessCount)
}

func TestQueryBaseTestStatus_GA(t *testing.T) {
	dbc := crTestDB(t)
	release := "4.15"

	// GA date in the past
	gaDate := civil.Date{Year: 2024, Month: 3, Day: 1}
	createReleaseDefinition(t, dbc, release, &gaDate)

	gaEnd := utils.GAWindowEnd(gaDate)
	windowDays := 30
	gaStart := gaDate.AddDays(-windowDays)

	vcAWS := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	jobAWS := createProwJobWithVC(t, dbc, "periodic-ga-aws-ovn", release, vcAWS)
	test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] GA PVC test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests-ga")
	createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:ga-pvc", "Storage", []string{"PVC"})

	// GA raw data: 50 runs, 45 passes, 2 flakes
	createGARawData(t, dbc, release, windowDays, test.ID, jobAWS.ID, suite.ID, 50, 45, 2)

	provider := postgres.NewPostgresProvider(dbc, nil)
	opts := defaultReqOptions(release)
	opts.BaseRelease = reqopts.Release{
		Name:  release,
		Start: gaStart.In(time.UTC),
		End:   gaEnd.AddDays(-1).In(time.UTC),
	}
	opts.VariantOption.IncludeVariants = map[string][]string{
		"Platform": {"aws"},
		"Network":  {"ovn"},
	}

	result, errs := provider.QueryBaseTestStatus(context.Background(), opts)
	require.Empty(t, errs)

	key := crtest.KeyWithVariants{
		TestID:   "openshift-tests:ga-pvc",
		Variants: map[string]string{"Platform": "aws", "Network": "ovn"},
	}
	ts, ok := result[key.Encode()]
	require.True(t, ok, "expected GA base test status")
	assert.Equal(t, 50, ts.TotalCount)
	assert.Equal(t, 45, ts.SuccessCount)
	assert.Equal(t, 2, ts.FlakeCount)
	assert.True(t, ts.LastFailure.IsZero(), "GA base path should not populate LastFailure")

	t.Run("GA drill-down with TestID", func(t *testing.T) {
		opts2 := opts
		opts2.TestIDOptions = []reqopts.TestIdentification{{
			TestID: "openshift-tests:ga-pvc",
		}}

		result2, errs := provider.QueryBaseTestStatus(context.Background(), opts2)
		require.Empty(t, errs)

		ts2, ok := result2[key.Encode()]
		require.True(t, ok, "TestID drill-down should return GA data for matching test")
		assert.Equal(t, 50, ts2.TotalCount)
	})

	t.Run("GA drill-down with non-matching TestID", func(t *testing.T) {
		opts3 := opts
		opts3.TestIDOptions = []reqopts.TestIdentification{{
			TestID: "openshift-tests:nonexistent",
		}}

		result3, errs := provider.QueryBaseTestStatus(context.Background(), opts3)
		require.Empty(t, errs)

		nonPlaceholders := filterPlaceholders(result3)
		assert.Empty(t, nonPlaceholders, "non-matching TestID should yield no results")
	})

	t.Run("release without GA date uses cumulative data", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.14"

		createReleaseDefinition(t, dbc, release, nil)

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		job := createProwJobWithVC(t, dbc, "periodic-ga-nil-aws", release, vc)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] GA nil date test")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-ga-nil")
		createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:ga-nil", "Storage", []string{"PVC"})

		// Seed prefix-sum data for base period
		baseLookupStart := civil.Date{Year: 2024, Month: 5, Day: 14}
		baseLookupEnd := civil.Date{Year: 2024, Month: 6, Day: 1}
		createCumulativeSummary(t, dbc, baseLookupStart, release, test.ID, job.ID, suite.ID, 80, 75, 3)
		createCumulativeSummary(t, dbc, baseLookupEnd, release, test.ID, job.ID, suite.ID, 100, 90, 5)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		opts.VariantOption.IncludeVariants = map[string][]string{
			"Platform": {"aws"},
			"Network":  {"ovn"},
		}

		result, errs := provider.QueryBaseTestStatus(context.Background(), opts)
		require.Empty(t, errs)

		key := crtest.KeyWithVariants{
			TestID:   "openshift-tests:ga-nil",
			Variants: map[string]string{"Platform": "aws", "Network": "ovn"},
		}
		ts, ok := result[key.Encode()]
		require.True(t, ok, "release without GA date should still return data")
		assert.Equal(t, 20, ts.TotalCount)
	})

	t.Run("release with future GA date uses cumulative data", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.20"

		futureGA := civil.Date{Year: 2030, Month: 1, Day: 1}
		createReleaseDefinition(t, dbc, release, &futureGA)

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		job := createProwJobWithVC(t, dbc, "periodic-ga-future-aws", release, vc)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] GA future date test")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-ga-future")
		createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:ga-future", "Storage", []string{"PVC"})

		baseLookupStart := civil.Date{Year: 2024, Month: 5, Day: 14}
		baseLookupEnd := civil.Date{Year: 2024, Month: 6, Day: 1}
		createCumulativeSummary(t, dbc, baseLookupStart, release, test.ID, job.ID, suite.ID, 80, 75, 3)
		createCumulativeSummary(t, dbc, baseLookupEnd, release, test.ID, job.ID, suite.ID, 100, 90, 5)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		opts.VariantOption.IncludeVariants = map[string][]string{
			"Platform": {"aws"},
			"Network":  {"ovn"},
		}

		result, errs := provider.QueryBaseTestStatus(context.Background(), opts)
		require.Empty(t, errs)

		key := crtest.KeyWithVariants{
			TestID:   "openshift-tests:ga-future",
			Variants: map[string]string{"Platform": "aws", "Network": "ovn"},
		}
		ts, ok := result[key.Encode()]
		require.True(t, ok, "release with future GA date should still return data")
		assert.Equal(t, 20, ts.TotalCount)
	})

	t.Run("release with non-standard GA window duration uses cumulative data", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.13"

		// Set GADate such that the window is 15 days (not in GAWindows: [1, 30, 90])
		gaDate := civil.Date{Year: 2024, Month: 6, Day: 1}
		createReleaseDefinition(t, dbc, release, &gaDate)

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		job := createProwJobWithVC(t, dbc, "periodic-ga-nonstandard-aws", release, vc)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] GA nonstandard window test")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-ga-ns")
		createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:ga-nonstandard", "Storage", []string{"PVC"})

		// Base period: May 17 to June 2 (= GAWindowEnd for June 1)
		// windowDays = gaCivil.DaysSince(Start) = June1 - May17 = 15 (not in GAWindows)
		baseLookupStart := civil.Date{Year: 2024, Month: 5, Day: 16}
		baseLookupEnd := civil.Date{Year: 2024, Month: 6, Day: 1}
		createCumulativeSummary(t, dbc, baseLookupStart, release, test.ID, job.ID, suite.ID, 80, 75, 3)
		createCumulativeSummary(t, dbc, baseLookupEnd, release, test.ID, job.ID, suite.ID, 95, 88, 4)

		provider := postgres.NewPostgresProvider(dbc, nil)
		gaEnd := utils.GAWindowEnd(gaDate)
		opts := defaultReqOptions(release)
		opts.BaseRelease = reqopts.Release{
			Name:  release,
			Start: time.Date(2024, 5, 17, 0, 0, 0, 0, time.UTC),
			End:   gaEnd.AddDays(-1).In(time.UTC),
		}
		opts.VariantOption.IncludeVariants = map[string][]string{
			"Platform": {"aws"},
			"Network":  {"ovn"},
		}

		result, errs := provider.QueryBaseTestStatus(context.Background(), opts)
		require.Empty(t, errs)

		key := crtest.KeyWithVariants{
			TestID:   "openshift-tests:ga-nonstandard",
			Variants: map[string]string{"Platform": "aws", "Network": "ovn"},
		}
		ts, ok := result[key.Encode()]
		require.True(t, ok, "release with non-standard window should still return data")
		assert.Equal(t, 15, ts.TotalCount)
	})
}

func TestQuerySampleJobRunTestStatus(t *testing.T) {
	dbc := crTestDB(t)
	release := "4.16"

	vcAWS := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	vcGCP := createVariantCombination(t, dbc, []string{"Platform:gcp", "Network:sdn"})
	jobAWS := createProwJobWithVC(t, dbc, "periodic-jr-aws", release, vcAWS)
	jobGCP := createProwJobWithVC(t, dbc, "periodic-jr-gcp", release, vcGCP)

	test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] JR PVC test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests-jr")
	tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:jr-pvc", "Storage", []string{"PVC"})

	ts1 := time.Date(2024, 6, 5, 12, 0, 0, 0, time.UTC)
	ts2 := time.Date(2024, 6, 6, 12, 0, 0, 0, time.UTC)
	ts3 := time.Date(2024, 6, 7, 12, 0, 0, 0, time.UTC)

	run1 := createProwJobRunForCR(t, dbc, jobAWS.ID, release, ts1)
	run2 := createProwJobRunForCR(t, dbc, jobAWS.ID, release, ts2)
	run3 := createProwJobRunForCR(t, dbc, jobGCP.ID, release, ts3)

	// status 1 = pass, 12 = fail, 13 = flake
	createProwJobRunTest(t, dbc, run1.ID, jobAWS.ID, test.ID, &suite.ID, 1, release, ts1)
	createProwJobRunTest(t, dbc, run2.ID, jobAWS.ID, test.ID, &suite.ID, 12, release, ts2)
	createProwJobRunTest(t, dbc, run3.ID, jobGCP.ID, test.ID, &suite.ID, 13, release, ts3)

	provider := postgres.NewPostgresProvider(dbc, nil)
	opts := defaultReqOptions(release)
	opts.TestIDOptions = []reqopts.TestIdentification{{
		TestID: tow.UniqueID,
	}}

	t.Run("returns per-job-run test results", func(t *testing.T) {
		result, errs := provider.QuerySampleJobRunTestStatus(context.Background(), opts,
			map[string][]string{"Platform": {"aws", "gcp"}, "Network": {"ovn", "sdn"}},
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)

		totalSummaries := 0
		totalJobRuns := 0
		for _, summaries := range result {
			totalSummaries += len(summaries)
			for _, s := range summaries {
				totalJobRuns += len(s.JobRuns)
			}
		}
		assert.Equal(t, 2, totalSummaries, "should have 2 summaries (one per job)")
		assert.Equal(t, 3, totalJobRuns, "should have 3 individual job runs across summaries")

		for _, summaries := range result {
			for _, s := range summaries {
				assert.Equal(t, tow.UniqueID, s.TestKey.TestID)
			}
		}
	})

	t.Run("RequestedVariants filtering", func(t *testing.T) {
		opts2 := opts
		opts2.TestIDOptions = []reqopts.TestIdentification{{
			TestID:            tow.UniqueID,
			RequestedVariants: map[string]string{"Platform": "aws"},
		}}

		result, errs := provider.QuerySampleJobRunTestStatus(context.Background(), opts2,
			map[string][]string{"Platform": {"aws", "gcp"}, "Network": {"ovn", "sdn"}},
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)

		totalSummaries := 0
		totalJobRuns := 0
		for _, summaries := range result {
			totalSummaries += len(summaries)
			for _, s := range summaries {
				totalJobRuns += len(s.JobRuns)
			}
		}
		assert.Equal(t, 1, totalSummaries, "RequestedVariants=aws should produce 1 summary")
		assert.Equal(t, 2, totalJobRuns, "RequestedVariants=aws should include 2 aws runs")
	})

	t.Run("IncludeVariants filtering", func(t *testing.T) {
		result, errs := provider.QuerySampleJobRunTestStatus(context.Background(), opts,
			map[string][]string{"Platform": {"gcp"}},
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)

		totalRows := 0
		for _, rows := range result {
			totalRows += len(rows)
		}
		assert.Equal(t, 1, totalRows, "IncludeVariants=gcp should only return gcp run")
	})

	t.Run("infrastructure failure runs excluded from results", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		job := createProwJobWithVC(t, dbc, "periodic-jr-infra", release, vc)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] infra exclusion test")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-infra")
		tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:infra-test", "Storage", []string{"PVC"})

		normalTS := time.Date(2024, 6, 5, 12, 0, 0, 0, time.UTC)
		infraTS := time.Date(2024, 6, 6, 12, 0, 0, 0, time.UTC)

		normalRun := createProwJobRunForCR(t, dbc, job.ID, release, normalTS)
		infraRun := createProwJobRunForCR(t, dbc, job.ID, release, infraTS)
		setJobRunLabels(t, dbc, infraRun.ID, []string{"InfraFailure"})

		createProwJobRunTest(t, dbc, normalRun.ID, job.ID, test.ID, &suite.ID, 1, release, normalTS)
		createProwJobRunTest(t, dbc, infraRun.ID, job.ID, test.ID, &suite.ID, 12, release, infraTS)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		opts.TestIDOptions = []reqopts.TestIdentification{{TestID: tow.UniqueID}}

		result, errs := provider.QuerySampleJobRunTestStatus(context.Background(), opts,
			map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}},
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)

		totalRows := 0
		for _, rows := range result {
			totalRows += len(rows)
		}
		assert.Equal(t, 1, totalRows, "infrastructure failure run should be excluded")
	})

	t.Run("Jira component ID included in test details", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		job := createProwJobWithVC(t, dbc, "periodic-jr-jira", release, vc)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] jira component test")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-jira")
		tow := createTestOwnershipFull(t, dbc, test.ID, &suite.ID, "openshift-tests:jira-test", "Storage", []string{"PVC"}, uintPtr(42))

		ts := time.Date(2024, 6, 5, 12, 0, 0, 0, time.UTC)
		run := createProwJobRunForCR(t, dbc, job.ID, release, ts)
		createProwJobRunTest(t, dbc, run.ID, job.ID, test.ID, &suite.ID, 1, release, ts)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		opts.TestIDOptions = []reqopts.TestIdentification{{TestID: tow.UniqueID}}

		result, errs := provider.QuerySampleJobRunTestStatus(context.Background(), opts,
			map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}},
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)

		for _, rows := range result {
			for _, row := range rows {
				require.NotNil(t, row.JiraComponentID, "JiraComponentID should be populated")
				expected := new(big.Rat).SetUint64(42)
				assert.Equal(t, 0, row.JiraComponentID.Cmp(expected),
					"JiraComponentID should be big.Rat(42), got %s", row.JiraComponentID.RatString())
			}
		}
	})

	t.Run("release isolation", func(t *testing.T) {
		dbc := crTestDB(t)
		sampleRelease := "4.17"
		distractorRelease := "4.16"

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		sampleJob := createProwJobWithVC(t, dbc, "periodic-jr-sample", sampleRelease, vc)
		distractorJob := createProwJobWithVC(t, dbc, "periodic-jr-distractor", distractorRelease, vc)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] JR release isolation")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-jr-ri")
		tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:jr-ri", "Storage", []string{"PVC"})

		ts1 := time.Date(2024, 6, 5, 12, 0, 0, 0, time.UTC)
		ts2 := time.Date(2024, 6, 6, 12, 0, 0, 0, time.UTC)

		sampleRun := createProwJobRunForCR(t, dbc, sampleJob.ID, sampleRelease, ts1)
		distractorRun := createProwJobRunForCR(t, dbc, distractorJob.ID, distractorRelease, ts2)

		createProwJobRunTest(t, dbc, sampleRun.ID, sampleJob.ID, test.ID, &suite.ID, 1, sampleRelease, ts1)
		createProwJobRunTest(t, dbc, distractorRun.ID, distractorJob.ID, test.ID, &suite.ID, 12, distractorRelease, ts2)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(sampleRelease)
		opts.TestIDOptions = []reqopts.TestIdentification{{TestID: tow.UniqueID}}

		result, errs := provider.QuerySampleJobRunTestStatus(context.Background(), opts,
			map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}},
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)

		totalRows := 0
		for _, rows := range result {
			totalRows += len(rows)
		}
		assert.Equal(t, 1, totalRows, "should only include job runs from the queried release")
	})
}

func TestQueryBaseJobRunTestStatus(t *testing.T) {
	dbc := crTestDB(t)
	release := "4.16"

	vcAWS := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	jobAWS := createProwJobWithVC(t, dbc, "periodic-base-jr-aws", release, vcAWS)

	test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-network] Base JR test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests-base-jr")
	tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:base-jr", "Networking", []string{"Services"})

	ts1 := time.Date(2024, 5, 20, 12, 0, 0, 0, time.UTC)
	ts2 := time.Date(2024, 5, 25, 12, 0, 0, 0, time.UTC)

	run1 := createProwJobRunForCR(t, dbc, jobAWS.ID, release, ts1)
	run2 := createProwJobRunForCR(t, dbc, jobAWS.ID, release, ts2)

	createProwJobRunTest(t, dbc, run1.ID, jobAWS.ID, test.ID, &suite.ID, 1, release, ts1)
	createProwJobRunTest(t, dbc, run2.ID, jobAWS.ID, test.ID, &suite.ID, 12, release, ts2)

	provider := postgres.NewPostgresProvider(dbc, nil)
	opts := defaultReqOptions(release)
	opts.TestIDOptions = []reqopts.TestIdentification{{
		TestID: tow.UniqueID,
	}}
	opts.VariantOption.IncludeVariants = map[string][]string{
		"Platform": {"aws"},
		"Network":  {"ovn"},
	}

	result, errs := provider.QueryBaseJobRunTestStatus(context.Background(), opts)
	require.Empty(t, errs)

	totalSummaries := 0
	totalJobRuns := 0
	for _, summaries := range result {
		totalSummaries += len(summaries)
		for _, s := range summaries {
			totalJobRuns += len(s.JobRuns)
		}
	}
	assert.Equal(t, 1, totalSummaries, "should have 1 summary for the single job")
	assert.Equal(t, 2, totalJobRuns, "should have 2 individual job runs in the summary")

	for _, summaries := range result {
		for _, s := range summaries {
			assert.Equal(t, tow.UniqueID, s.TestKey.TestID)
		}
	}
}

func TestQueryBaseJobRunTestStatus_AggregateFallback(t *testing.T) {
	t.Run("prefix-sum fallback when per-run data is absent", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		job := createProwJobWithVC(t, dbc, "periodic-agg-aws", release, vc)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] agg fallback test")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-agg")
		tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:agg-fallback", "Storage", []string{"PVC"})

		// Base release: [2024-05-15, 2024-06-01)
		// lookupStart = 2024-05-14, lookupEnd = 2024-06-01
		baseLookupStart := civil.Date{Year: 2024, Month: 5, Day: 14}
		baseLookupEnd := civil.Date{Year: 2024, Month: 6, Day: 1}
		createCumulativeSummary(t, dbc, baseLookupStart, release, test.ID, job.ID, suite.ID, 80, 75, 3)
		createCumulativeSummary(t, dbc, baseLookupEnd, release, test.ID, job.ID, suite.ID, 100, 90, 5)
		// expected: runs=20, successes=15, flakes=2

		// No prow_job_run_tests rows created for this release

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		opts.TestIDOptions = []reqopts.TestIdentification{{
			TestID:            tow.UniqueID,
			RequestedVariants: map[string]string{"Platform": "aws", "Network": "ovn"},
		}}
		opts.VariantOption.IncludeVariants = map[string][]string{
			"Platform": {"aws"},
			"Network":  {"ovn"},
		}

		result, errs := provider.QueryBaseJobRunTestStatus(context.Background(), opts)
		require.Empty(t, errs)

		totalRows := 0
		for _, rows := range result {
			totalRows += len(rows)
		}
		require.Equal(t, 1, totalRows, "should have 1 aggregate entry for the one job")

		for _, rows := range result {
			for _, row := range rows {
				assert.Equal(t, tow.UniqueID, row.TestKey.TestID)
				assert.Equal(t, 20, row.Stats.Total(), "aggregate total should be prefix-sum delta")
				assert.Equal(t, 15, row.Stats.SuccessCount, "aggregate successes should be prefix-sum delta")
				assert.Equal(t, 2, row.Stats.FlakeCount, "aggregate flakes should be prefix-sum delta")
			}
		}
	})

	t.Run("GA fallback when per-run data is absent", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.15"

		gaDate := civil.Date{Year: 2024, Month: 3, Day: 1}
		createReleaseDefinition(t, dbc, release, &gaDate)

		gaEnd := utils.GAWindowEnd(gaDate)
		windowDays := 30
		gaStart := gaDate.AddDays(-windowDays)

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		job := createProwJobWithVC(t, dbc, "periodic-ga-agg-aws", release, vc)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] GA agg fallback test")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-ga-agg")
		tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:ga-agg-fallback", "Storage", []string{"PVC"})

		createGARawData(t, dbc, release, windowDays, test.ID, job.ID, suite.ID, 50, 45, 2)

		// No prow_job_run_tests rows

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		opts.BaseRelease = reqopts.Release{
			Name:  release,
			Start: gaStart.In(time.UTC),
			End:   gaEnd.AddDays(-1).In(time.UTC),
		}
		opts.TestIDOptions = []reqopts.TestIdentification{{
			TestID:            tow.UniqueID,
			RequestedVariants: map[string]string{"Platform": "aws", "Network": "ovn"},
		}}
		opts.VariantOption.IncludeVariants = map[string][]string{
			"Platform": {"aws"},
			"Network":  {"ovn"},
		}

		result, errs := provider.QueryBaseJobRunTestStatus(context.Background(), opts)
		require.Empty(t, errs)

		totalRows := 0
		for _, rows := range result {
			totalRows += len(rows)
		}
		require.Equal(t, 1, totalRows, "should have 1 aggregate entry from GA data")

		for _, rows := range result {
			for _, row := range rows {
				assert.Equal(t, tow.UniqueID, row.TestKey.TestID)
				assert.Equal(t, 50, row.Stats.Total())
				assert.Equal(t, 45, row.Stats.SuccessCount)
				assert.Equal(t, 2, row.Stats.FlakeCount)
			}
		}
	})

	t.Run("per-run data takes precedence over aggregate", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		job := createProwJobWithVC(t, dbc, "periodic-prec-aws", release, vc)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] precedence test")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-prec")
		tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:precedence", "Storage", []string{"PVC"})

		// Create cumulative summaries (aggregate data)
		baseLookupStart := civil.Date{Year: 2024, Month: 5, Day: 14}
		baseLookupEnd := civil.Date{Year: 2024, Month: 6, Day: 1}
		createCumulativeSummary(t, dbc, baseLookupStart, release, test.ID, job.ID, suite.ID, 80, 75, 3)
		createCumulativeSummary(t, dbc, baseLookupEnd, release, test.ID, job.ID, suite.ID, 100, 90, 5)

		// Also create per-run data (prow_job_run_tests)
		ts1 := time.Date(2024, 5, 20, 12, 0, 0, 0, time.UTC)
		ts2 := time.Date(2024, 5, 25, 12, 0, 0, 0, time.UTC)
		run1 := createProwJobRunForCR(t, dbc, job.ID, release, ts1)
		run2 := createProwJobRunForCR(t, dbc, job.ID, release, ts2)
		createProwJobRunTest(t, dbc, run1.ID, job.ID, test.ID, &suite.ID, 1, release, ts1)
		createProwJobRunTest(t, dbc, run2.ID, job.ID, test.ID, &suite.ID, 12, release, ts2)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		opts.TestIDOptions = []reqopts.TestIdentification{{TestID: tow.UniqueID}}
		opts.VariantOption.IncludeVariants = map[string][]string{
			"Platform": {"aws"},
			"Network":  {"ovn"},
		}

		result, errs := provider.QueryBaseJobRunTestStatus(context.Background(), opts)
		require.Empty(t, errs)

		totalSummaries := 0
		for _, summaries := range result {
			totalSummaries += len(summaries)
		}
		assert.Equal(t, 1, totalSummaries, "per-run data should produce 1 summary per test key")

		for _, summaries := range result {
			for _, summary := range summaries {
				assert.Equal(t, 2, summary.Stats.Total(), "summary should aggregate both per-run rows")
				assert.Len(t, summary.JobRuns, 2, "summary should contain 2 individual job runs")
			}
		}
	})

	t.Run("variant filtering applied in fallback", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vcAWS := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		vcGCP := createVariantCombination(t, dbc, []string{"Platform:gcp", "Network:sdn"})
		jobAWS := createProwJobWithVC(t, dbc, "periodic-vf-aws", release, vcAWS)
		jobGCP := createProwJobWithVC(t, dbc, "periodic-vf-gcp", release, vcGCP)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] variant filter test")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-vf")
		tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:variant-filter", "Storage", []string{"PVC"})

		baseLookupStart := civil.Date{Year: 2024, Month: 5, Day: 14}
		baseLookupEnd := civil.Date{Year: 2024, Month: 6, Day: 1}

		createCumulativeSummary(t, dbc, baseLookupStart, release, test.ID, jobAWS.ID, suite.ID, 80, 75, 3)
		createCumulativeSummary(t, dbc, baseLookupEnd, release, test.ID, jobAWS.ID, suite.ID, 100, 90, 5)
		createCumulativeSummary(t, dbc, baseLookupStart, release, test.ID, jobGCP.ID, suite.ID, 40, 35, 1)
		createCumulativeSummary(t, dbc, baseLookupEnd, release, test.ID, jobGCP.ID, suite.ID, 60, 50, 3)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		opts.TestIDOptions = []reqopts.TestIdentification{{
			TestID:            tow.UniqueID,
			RequestedVariants: map[string]string{"Platform": "aws", "Network": "ovn"},
		}}
		opts.VariantOption.IncludeVariants = map[string][]string{
			"Platform": {"aws"},
			"Network":  {"ovn"},
		}

		result, errs := provider.QueryBaseJobRunTestStatus(context.Background(), opts)
		require.Empty(t, errs)

		totalRows := 0
		for _, rows := range result {
			totalRows += len(rows)
		}
		assert.Equal(t, 1, totalRows, "only aws+ovn job should be returned")

		for _, rows := range result {
			for _, row := range rows {
				assert.Equal(t, 20, row.Stats.Total(), "should only contain aws job data")
			}
		}
	})

	t.Run("RequestedVariants filtering applied in fallback", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vcAWSOvn := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		vcAWSSdn := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:sdn"})
		jobOvn := createProwJobWithVC(t, dbc, "periodic-rv-ovn", release, vcAWSOvn)
		jobSdn := createProwJobWithVC(t, dbc, "periodic-rv-sdn", release, vcAWSSdn)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] requested variants test")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-rv")
		tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:req-variants", "Storage", []string{"PVC"})

		baseLookupStart := civil.Date{Year: 2024, Month: 5, Day: 14}
		baseLookupEnd := civil.Date{Year: 2024, Month: 6, Day: 1}

		createCumulativeSummary(t, dbc, baseLookupStart, release, test.ID, jobOvn.ID, suite.ID, 80, 75, 3)
		createCumulativeSummary(t, dbc, baseLookupEnd, release, test.ID, jobOvn.ID, suite.ID, 100, 90, 5)
		createCumulativeSummary(t, dbc, baseLookupStart, release, test.ID, jobSdn.ID, suite.ID, 40, 35, 1)
		createCumulativeSummary(t, dbc, baseLookupEnd, release, test.ID, jobSdn.ID, suite.ID, 60, 50, 3)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		opts.TestIDOptions = []reqopts.TestIdentification{{
			TestID:            tow.UniqueID,
			RequestedVariants: map[string]string{"Platform": "aws", "Network": "ovn"},
		}}
		opts.VariantOption.IncludeVariants = map[string][]string{
			"Platform": {"aws"},
			"Network":  {"ovn", "sdn"},
		}

		result, errs := provider.QueryBaseJobRunTestStatus(context.Background(), opts)
		require.Empty(t, errs)

		totalRows := 0
		for _, rows := range result {
			totalRows += len(rows)
		}
		assert.Equal(t, 1, totalRows, "RequestedVariants should narrow to ovn only")

		for _, rows := range result {
			for _, row := range rows {
				assert.Equal(t, "ovn", row.TestKey.Variants["Network"],
					"only ovn variant should be returned")
			}
		}
	})

	t.Run("multiple jobs aggregated separately", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		job1 := createProwJobWithVC(t, dbc, "periodic-multi-aws-1", release, vc)
		job2 := createProwJobWithVC(t, dbc, "periodic-multi-aws-2", release, vc)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] multi job test")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-mj")
		tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:multi-job", "Storage", []string{"PVC"})

		baseLookupStart := civil.Date{Year: 2024, Month: 5, Day: 14}
		baseLookupEnd := civil.Date{Year: 2024, Month: 6, Day: 1}

		createCumulativeSummary(t, dbc, baseLookupStart, release, test.ID, job1.ID, suite.ID, 80, 75, 3)
		createCumulativeSummary(t, dbc, baseLookupEnd, release, test.ID, job1.ID, suite.ID, 100, 90, 5)
		createCumulativeSummary(t, dbc, baseLookupStart, release, test.ID, job2.ID, suite.ID, 40, 35, 1)
		createCumulativeSummary(t, dbc, baseLookupEnd, release, test.ID, job2.ID, suite.ID, 60, 50, 3)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		opts.TestIDOptions = []reqopts.TestIdentification{{
			TestID:            tow.UniqueID,
			RequestedVariants: map[string]string{"Platform": "aws", "Network": "ovn"},
		}}
		opts.VariantOption.IncludeVariants = map[string][]string{
			"Platform": {"aws"},
			"Network":  {"ovn"},
		}

		result, errs := provider.QueryBaseJobRunTestStatus(context.Background(), opts)
		require.Empty(t, errs)

		assert.Len(t, result, 2, "should have 2 separate job entries")

		normalizedJob1 := utils.NormalizeProwJobName("periodic-multi-aws-1")
		normalizedJob2 := utils.NormalizeProwJobName("periodic-multi-aws-2")

		rows1, ok := result[normalizedJob1]
		require.True(t, ok, "should have entry for job1")
		assert.Len(t, rows1, 1)
		assert.Equal(t, 20, rows1[0].Stats.Total())

		rows2, ok := result[normalizedJob2]
		require.True(t, ok, "should have entry for job2")
		assert.Len(t, rows2, 1)
		assert.Equal(t, 20, rows2[0].Stats.Total())
	})

	t.Run("aggregate fallback rows have no individual run data", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		job := createProwJobWithVC(t, dbc, "periodic-flag-aws", release, vc)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] aggregate no-run-id test")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-flag")
		tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:flag-test", "Storage", []string{"PVC"})

		baseLookupStart := civil.Date{Year: 2024, Month: 5, Day: 14}
		baseLookupEnd := civil.Date{Year: 2024, Month: 6, Day: 1}
		createCumulativeSummary(t, dbc, baseLookupStart, release, test.ID, job.ID, suite.ID, 80, 75, 3)
		createCumulativeSummary(t, dbc, baseLookupEnd, release, test.ID, job.ID, suite.ID, 100, 90, 5)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		opts.TestIDOptions = []reqopts.TestIdentification{{
			TestID:            tow.UniqueID,
			RequestedVariants: map[string]string{"Platform": "aws", "Network": "ovn"},
		}}
		opts.VariantOption.IncludeVariants = map[string][]string{
			"Platform": {"aws"},
			"Network":  {"ovn"},
		}

		result, errs := provider.QueryBaseJobRunTestStatus(context.Background(), opts)
		require.Empty(t, errs)
		require.Len(t, result, 1, "should have 1 job entry")

		for _, summaries := range result {
			for _, summary := range summaries {
				assert.Empty(t, summary.JobRuns, "aggregate rows should have no individual job runs")
			}
		}
	})

	t.Run("per-run rows have individual run data", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		job := createProwJobWithVC(t, dbc, "periodic-notflag-aws", release, vc)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] per-run has-run-id test")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-notflag")
		tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:notflag-test", "Storage", []string{"PVC"})

		ts := time.Date(2024, 5, 20, 12, 0, 0, 0, time.UTC)
		run := createProwJobRunForCR(t, dbc, job.ID, release, ts)
		createProwJobRunTest(t, dbc, run.ID, job.ID, test.ID, &suite.ID, 1, release, ts)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		opts.TestIDOptions = []reqopts.TestIdentification{{TestID: tow.UniqueID}}
		opts.VariantOption.IncludeVariants = map[string][]string{
			"Platform": {"aws"},
			"Network":  {"ovn"},
		}

		result, errs := provider.QueryBaseJobRunTestStatus(context.Background(), opts)
		require.Empty(t, errs)
		require.Len(t, result, 1)

		for _, summaries := range result {
			for _, summary := range summaries {
				assert.NotEmpty(t, summary.JobRuns, "per-run summaries must have individual job runs")
			}
		}
	})

	t.Run("no-suite test ownership uses suite_id zero fallback", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		job := createProwJobWithVC(t, dbc, "periodic-nosuite-aws", release, vc)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] no suite test")
		tow := createTestOwnership(t, dbc, test.ID, nil, "openshift-tests:no-suite", "Storage", []string{"PVC"})

		baseLookupStart := civil.Date{Year: 2024, Month: 5, Day: 14}
		baseLookupEnd := civil.Date{Year: 2024, Month: 6, Day: 1}
		createCumulativeSummary(t, dbc, baseLookupStart, release, test.ID, job.ID, 0, 80, 75, 3)
		createCumulativeSummary(t, dbc, baseLookupEnd, release, test.ID, job.ID, 0, 100, 90, 5)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		opts.TestIDOptions = []reqopts.TestIdentification{{
			TestID:            tow.UniqueID,
			RequestedVariants: map[string]string{"Platform": "aws", "Network": "ovn"},
		}}
		opts.VariantOption.IncludeVariants = map[string][]string{
			"Platform": {"aws"},
			"Network":  {"ovn"},
		}

		result, errs := provider.QueryBaseJobRunTestStatus(context.Background(), opts)
		require.Empty(t, errs)

		totalRows := 0
		for _, rows := range result {
			totalRows += len(rows)
		}
		require.Equal(t, 1, totalRows, "should match cumulative summary with suite_id=0 when ownership has nil suite")

		for _, rows := range result {
			for _, row := range rows {
				assert.Equal(t, 20, row.Stats.Total())
				assert.Equal(t, 15, row.Stats.SuccessCount)
				assert.Equal(t, 2, row.Stats.FlakeCount)
			}
		}
	})

	t.Run("zero-delta rows excluded by HAVING filter", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		jobActive := createProwJobWithVC(t, dbc, "periodic-having-active", release, vc)
		jobIdle := createProwJobWithVC(t, dbc, "periodic-having-idle", release, vc)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] having filter test")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-having")
		tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:having-filter", "Storage", []string{"PVC"})

		baseLookupStart := civil.Date{Year: 2024, Month: 5, Day: 14}
		baseLookupEnd := civil.Date{Year: 2024, Month: 6, Day: 1}

		// Active job: has activity in the window (delta > 0)
		createCumulativeSummary(t, dbc, baseLookupStart, release, test.ID, jobActive.ID, suite.ID, 80, 75, 3)
		createCumulativeSummary(t, dbc, baseLookupEnd, release, test.ID, jobActive.ID, suite.ID, 100, 90, 5)

		// Idle job: identical prefix sums at start and end (delta = 0, no runs in window)
		createCumulativeSummary(t, dbc, baseLookupStart, release, test.ID, jobIdle.ID, suite.ID, 50, 45, 2)
		createCumulativeSummary(t, dbc, baseLookupEnd, release, test.ID, jobIdle.ID, suite.ID, 50, 45, 2)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		opts.TestIDOptions = []reqopts.TestIdentification{{
			TestID:            tow.UniqueID,
			RequestedVariants: map[string]string{"Platform": "aws", "Network": "ovn"},
		}}
		opts.VariantOption.IncludeVariants = map[string][]string{
			"Platform": {"aws"},
			"Network":  {"ovn"},
		}

		result, errs := provider.QueryBaseJobRunTestStatus(context.Background(), opts)
		require.Empty(t, errs)

		totalRows := 0
		for _, rows := range result {
			totalRows += len(rows)
		}
		assert.Equal(t, 1, totalRows, "only the active job should be returned; idle job with zero delta should be filtered out")

		for _, rows := range result {
			for _, row := range rows {
				assert.Equal(t, 20, row.Stats.Total(), "only the active job's delta should appear")
			}
		}
	})

	t.Run("test metadata populated in fallback results", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		job := createProwJobWithVC(t, dbc, "periodic-meta-aws", release, vc)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] metadata fallback test")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-meta")
		tow := createTestOwnershipFull(t, dbc, test.ID, &suite.ID, "openshift-tests:meta-fallback", "Storage", []string{"PVC"}, uintPtr(42))

		baseLookupStart := civil.Date{Year: 2024, Month: 5, Day: 14}
		baseLookupEnd := civil.Date{Year: 2024, Month: 6, Day: 1}
		createCumulativeSummary(t, dbc, baseLookupStart, release, test.ID, job.ID, suite.ID, 80, 75, 3)
		createCumulativeSummary(t, dbc, baseLookupEnd, release, test.ID, job.ID, suite.ID, 100, 90, 5)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		opts.TestIDOptions = []reqopts.TestIdentification{{
			TestID:            tow.UniqueID,
			RequestedVariants: map[string]string{"Platform": "aws", "Network": "ovn"},
		}}
		opts.VariantOption.IncludeVariants = map[string][]string{
			"Platform": {"aws"},
			"Network":  {"ovn"},
		}

		result, errs := provider.QueryBaseJobRunTestStatus(context.Background(), opts)
		require.Empty(t, errs)

		totalRows := 0
		for _, rows := range result {
			totalRows += len(rows)
		}
		require.Equal(t, 1, totalRows, "should have 1 aggregate entry")

		for _, rows := range result {
			for _, row := range rows {
				assert.Equal(t, test.Name, row.TestName, "TestName should be populated")
				assert.Equal(t, "Storage", row.JiraComponent, "JiraComponent should be populated")
				require.NotNil(t, row.JiraComponentID, "JiraComponentID should be populated")
				expected := new(big.Rat).SetUint64(42)
				assert.Equal(t, 0, row.JiraComponentID.Cmp(expected),
					"JiraComponentID should be 42, got %s", row.JiraComponentID.RatString())
			}
		}
	})
}

func TestQueryJobVariants(t *testing.T) {
	dbc := crTestDB(t)

	jobs := []models.ProwJob{
		{Name: "periodic-e2e-aws-ovn-ha", Release: "4.16", Variants: pq.StringArray{"Platform:aws", "Network:ovn", "Topology:ha"}},
		{Name: "periodic-e2e-gcp-sdn-single", Release: "4.16", Variants: pq.StringArray{"Platform:gcp", "Network:sdn", "Topology:single"}},
		{Name: "periodic-e2e-aws-sdn-ha", Release: "4.16", Variants: pq.StringArray{"Platform:aws", "Network:sdn", "Topology:ha"}},
	}
	for i := range jobs {
		require.NoError(t, dbc.DB.Create(&jobs[i]).Error)
	}

	provider := postgres.NewPostgresProvider(dbc, nil)

	result, errs := provider.QueryJobVariants(context.Background(), reqopts.RequestOptions{})
	require.Empty(t, errs)

	// Check Platform values
	platforms, ok := result.Variants["Platform"]
	require.True(t, ok, "should have Platform key")
	assert.Equal(t, []string{"aws", "gcp"}, platforms, "Platform values should be sorted and deduplicated")

	// Check Network values
	networks, ok := result.Variants["Network"]
	require.True(t, ok, "should have Network key")
	assert.Equal(t, []string{"ovn", "sdn"}, networks)

	// Check Topology values
	topologies, ok := result.Variants["Topology"]
	require.True(t, ok, "should have Topology key")
	assert.Equal(t, []string{"ha", "single"}, topologies)
}

func TestQueryJobRuns(t *testing.T) {
	t.Run("only periodic, release, and aggregator jobs included", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vcAWS := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})

		periodicJob := createProwJobWithVC(t, dbc, "periodic-ci-aws-test", release, vcAWS)
		releaseJob := createProwJobWithVC(t, dbc, "release-ci-aws-test", release, vcAWS)
		pullJob := createProwJobWithVC(t, dbc, "pull-ci-aws-test", release, vcAWS)

		ts1 := time.Date(2024, 6, 5, 12, 0, 0, 0, time.UTC)
		ts2 := time.Date(2024, 6, 6, 12, 0, 0, 0, time.UTC)
		ts3 := time.Date(2024, 6, 7, 12, 0, 0, 0, time.UTC)

		// periodic: 2 runs, 1 success
		createProwJobRunForCR(t, dbc, periodicJob.ID, release, ts1)
		r2 := createProwJobRunForCR(t, dbc, periodicJob.ID, release, ts2)
		require.NoError(t, dbc.DB.Model(&r2).Updates(map[string]any{"succeeded": false, "failed": true}).Error)

		// release: 1 run, 1 success
		createProwJobRunForCR(t, dbc, releaseJob.ID, release, ts1)

		// pull: 1 run (should be excluded by prefix filter)
		createProwJobRunForCR(t, dbc, pullJob.ID, release, ts3)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)

		result, err := provider.QueryJobRuns(context.Background(), opts, release,
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.NoError(t, err)

		assert.Contains(t, result, "periodic-ci-aws-test")
		assert.Contains(t, result, "release-ci-aws-test")
		assert.NotContains(t, result, "pull-ci-aws-test", "pull request jobs should be excluded")

		periodic := result["periodic-ci-aws-test"]
		assert.Equal(t, 2, periodic.TotalRuns)
		assert.Equal(t, 1, periodic.SuccessfulRuns)
		assert.InDelta(t, 50.0, periodic.PassRate, 0.01)

		releaseStats := result["release-ci-aws-test"]
		assert.Equal(t, 1, releaseStats.TotalRuns)
		assert.Equal(t, 1, releaseStats.SuccessfulRuns)
		assert.InDelta(t, 100.0, releaseStats.PassRate, 0.01)
	})

	t.Run("variant filtering", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vcAWS := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		vcGCP := createVariantCombination(t, dbc, []string{"Platform:gcp", "Network:sdn"})

		awsJob := createProwJobWithVC(t, dbc, "periodic-ci-aws-vf", release, vcAWS)
		gcpJob := createProwJobWithVC(t, dbc, "periodic-ci-gcp-vf", release, vcGCP)

		ts := time.Date(2024, 6, 5, 12, 0, 0, 0, time.UTC)
		createProwJobRunForCR(t, dbc, awsJob.ID, release, ts)
		createProwJobRunForCR(t, dbc, gcpJob.ID, release, ts)

		provider := postgres.NewPostgresProvider(dbc, nil)
		opts := defaultReqOptions(release)
		opts.VariantOption.IncludeVariants = map[string][]string{
			"Platform": {"aws"},
		}

		result, err := provider.QueryJobRuns(context.Background(), opts, release,
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.NoError(t, err)

		assert.Contains(t, result, "periodic-ci-aws-vf")
		assert.NotContains(t, result, "periodic-ci-gcp-vf", "gcp job should be filtered out by IncludeVariants")
	})
}

func TestQueryUniqueVariantValues(t *testing.T) {
	dbc := crTestDB(t)

	jobs := []models.ProwJob{
		{Name: "periodic-uv-aws", Variants: pq.StringArray{"Platform:aws", "Network:ovn", "Architecture:amd64"}},
		{Name: "periodic-uv-gcp", Variants: pq.StringArray{"Platform:gcp", "Network:sdn", "Architecture:arm64"}},
	}
	for i := range jobs {
		require.NoError(t, dbc.DB.Create(&jobs[i]).Error)
	}

	provider := postgres.NewPostgresProvider(dbc, nil)

	t.Run("nested returns key names", func(t *testing.T) {
		result, err := provider.QueryUniqueVariantValues(context.Background(), reqopts.RequestOptions{}, "", true)
		require.NoError(t, err)
		assert.Equal(t, []string{"Architecture", "Network", "Platform"}, result)
	})

	t.Run("field maps to variant key", func(t *testing.T) {
		result, err := provider.QueryUniqueVariantValues(context.Background(), reqopts.RequestOptions{}, "platform", false)
		require.NoError(t, err)
		assert.Equal(t, []string{"aws", "gcp"}, result)
	})

	t.Run("arch field maps to Architecture", func(t *testing.T) {
		result, err := provider.QueryUniqueVariantValues(context.Background(), reqopts.RequestOptions{}, "arch", false)
		require.NoError(t, err)
		assert.Equal(t, []string{"amd64", "arm64"}, result)
	})

	t.Run("unknown field returns empty", func(t *testing.T) {
		result, err := provider.QueryUniqueVariantValues(context.Background(), reqopts.RequestOptions{}, "unknown", false)
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

func TestQueryJobVariantValues(t *testing.T) {
	dbc := crTestDB(t)

	job1 := models.ProwJob{Name: "periodic-jvv-aws", Variants: pq.StringArray{"Platform:aws", "Network:ovn", "Topology:ha"}}
	job2 := models.ProwJob{Name: "periodic-jvv-gcp", Variants: pq.StringArray{"Platform:gcp", "Network:sdn", "Topology:single"}}
	require.NoError(t, dbc.DB.Create(&job1).Error)
	require.NoError(t, dbc.DB.Create(&job2).Error)

	provider := postgres.NewPostgresProvider(dbc, nil)

	t.Run("returns all variants for given jobs", func(t *testing.T) {
		result, err := provider.QueryJobVariantValues(context.Background(), reqopts.RequestOptions{},
			[]string{"periodic-jvv-aws", "periodic-jvv-gcp"}, nil)
		require.NoError(t, err)

		assert.Equal(t, map[string]string{"Platform": "aws", "Network": "ovn", "Topology": "ha"}, result["periodic-jvv-aws"])
		assert.Equal(t, map[string]string{"Platform": "gcp", "Network": "sdn", "Topology": "single"}, result["periodic-jvv-gcp"])
	})

	t.Run("variantKeys filter", func(t *testing.T) {
		result, err := provider.QueryJobVariantValues(context.Background(), reqopts.RequestOptions{},
			[]string{"periodic-jvv-aws"}, []string{"Platform"})
		require.NoError(t, err)

		assert.Equal(t, map[string]string{"Platform": "aws"}, result["periodic-jvv-aws"])
	})

	t.Run("empty jobNames returns empty map", func(t *testing.T) {
		result, err := provider.QueryJobVariantValues(context.Background(), reqopts.RequestOptions{}, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

func TestLookupJobVariants(t *testing.T) {
	dbc := crTestDB(t)

	job := models.ProwJob{Name: "periodic-ljv-aws", Variants: pq.StringArray{"Platform:aws", "Network:ovn"}}
	require.NoError(t, dbc.DB.Create(&job).Error)

	provider := postgres.NewPostgresProvider(dbc, nil)
	result, err := provider.LookupJobVariants(context.Background(), reqopts.RequestOptions{}, "periodic-ljv-aws")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"Platform": "aws", "Network": "ovn"}, result)
}

func TestQueryReleases(t *testing.T) {
	dbc := crTestDB(t)

	earlier := civil.Date{Year: 2023, Month: 6, Day: 1}
	later := civil.Date{Year: 2024, Month: 6, Day: 1}
	gaDate := civil.Date{Year: 2024, Month: 3, Day: 1}

	rd1 := models.ReleaseDefinition{
		Release:              "4.15",
		Status:               "GA",
		GADate:               &gaDate,
		PreviousRelease:      "4.14",
		Product:              "OCP",
		DevelopmentStartDate: &earlier,
	}
	rd2 := models.ReleaseDefinition{
		Release:              "4.16",
		Status:               "Development",
		PreviousRelease:      "4.15",
		Product:              "OCP",
		DevelopmentStartDate: &later,
	}
	require.NoError(t, dbc.DB.Create(&rd1).Error)
	require.NoError(t, dbc.DB.Create(&rd2).Error)

	provider := postgres.NewPostgresProvider(dbc, nil)
	releases, err := provider.QueryReleases(context.Background())
	require.NoError(t, err)
	require.Len(t, releases, 2)

	// Ordered by DevelopmentStartDate DESC, so 4.16 first
	assert.Equal(t, "4.16", releases[0].Release)
	assert.Equal(t, "Development", releases[0].Status)
	assert.Nil(t, releases[0].GADate)
	assert.Equal(t, "4.15", releases[0].PreviousRelease)
	assert.Equal(t, "OCP", releases[0].Product)

	assert.Equal(t, "4.15", releases[1].Release)
	assert.Equal(t, "GA", releases[1].Status)
	require.NotNil(t, releases[1].GADate)
	assert.Equal(t, gaDate, *releases[1].GADate)
}

func TestQueryReleaseDates(t *testing.T) {
	dbc := crTestDB(t)

	gaDate := civil.Date{Year: 2024, Month: 3, Day: 1}
	devStart := civil.Date{Year: 2023, Month: 6, Day: 1}
	devStartNoGA := civil.Date{Year: 2024, Month: 6, Day: 1}

	rdWithGA := models.ReleaseDefinition{
		Release:              "4.15",
		GADate:               &gaDate,
		DevelopmentStartDate: &devStart,
		Product:              "OCP",
	}
	rdWithoutGA := models.ReleaseDefinition{
		Release:              "4.16",
		DevelopmentStartDate: &devStartNoGA,
		Product:              "OCP",
	}
	require.NoError(t, dbc.DB.Create(&rdWithGA).Error)
	require.NoError(t, dbc.DB.Create(&rdWithoutGA).Error)

	provider := postgres.NewPostgresProvider(dbc, nil)
	opts := reqopts.RequestOptions{}
	timeRanges, errs := provider.QueryReleaseDates(context.Background(), opts)
	require.Empty(t, errs)
	require.Len(t, timeRanges, 2)

	var withGA, withoutGA *crtest.ReleaseTimeRange
	for i := range timeRanges {
		switch timeRanges[i].Release {
		case "4.15":
			withGA = &timeRanges[i]
		case "4.16":
			withoutGA = &timeRanges[i]
		}
	}

	require.NotNil(t, withGA, "release 4.15 should be in results")
	require.NotNil(t, withGA.Start, "release with GADate should have Start")
	require.NotNil(t, withGA.End, "release with GADate should have End")
	assert.Equal(t, gaDate.In(time.UTC), *withGA.End, "End should be the GA date")

	require.NotNil(t, withoutGA, "release 4.16 should be in results")
	assert.Nil(t, withoutGA.Start, "release without GADate should have nil Start")
	assert.Nil(t, withoutGA.End, "release without GADate should have nil End")
}

func TestGAPathAggregatesMultipleJobs(t *testing.T) {
	dbc := crTestDB(t)
	release := "4.15"

	gaDate := civil.Date{Year: 2024, Month: 3, Day: 1}
	createReleaseDefinition(t, dbc, release, &gaDate)

	gaEnd := utils.GAWindowEnd(gaDate)
	windowDays := 30
	gaStart := gaDate.AddDays(-windowDays)

	vc1 := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	vc2 := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:sdn"})
	job1 := createProwJobWithVC(t, dbc, "periodic-ga-multi-1", release, vc1)
	job2 := createProwJobWithVC(t, dbc, "periodic-ga-multi-2", release, vc2)

	test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] GA multi-job test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests-ga-mj")
	createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:ga-multi", "Storage", []string{"PVC"})

	// Two jobs contribute GA data for the same test on the same variant group (when grouped by Platform only)
	createGARawData(t, dbc, release, windowDays, test.ID, job1.ID, suite.ID, 30, 25, 2)
	createGARawData(t, dbc, release, windowDays, test.ID, job2.ID, suite.ID, 20, 18, 1)

	provider := postgres.NewPostgresProvider(dbc, nil)
	opts := defaultReqOptions(release)
	opts.BaseRelease = reqopts.Release{
		Name:  release,
		Start: gaStart.In(time.UTC),
		End:   gaEnd.AddDays(-1).In(time.UTC),
	}
	opts.VariantOption.DBGroupBy = sets.New[string]("Platform")
	opts.VariantOption.ColumnGroupBy = sets.New[string]("Platform")
	opts.VariantOption.IncludeVariants = map[string][]string{
		"Platform": {"aws"},
	}

	result, errs := provider.QueryBaseTestStatus(context.Background(), opts)
	require.Empty(t, errs)

	key := crtest.KeyWithVariants{
		TestID:   "openshift-tests:ga-multi",
		Variants: map[string]string{"Platform": "aws"},
	}
	ts, ok := result[key.Encode()]
	require.True(t, ok, "GA path should return aggregated data from multiple jobs")
	assert.Equal(t, 50, ts.TotalCount, "runs should aggregate: 30 + 20")
	assert.Equal(t, 43, ts.SuccessCount, "successes should aggregate: 25 + 18")
	assert.Equal(t, 3, ts.FlakeCount, "flakes should aggregate: 2 + 1")
}

func TestMultipleTestsInSameComponent(t *testing.T) {
	dbc := crTestDB(t)
	release := "4.16"

	vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	job := createProwJobWithVC(t, dbc, "periodic-e2e-aws-multi", release, vc)

	testA := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] PVC create")
	testB := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] PVC expand")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests-multi")

	towA := createTestOwnership(t, dbc, testA.ID, &suite.ID, "openshift-tests:pvc-create", "Storage", []string{"PVC"})
	towB := createTestOwnership(t, dbc, testB.ID, &suite.ID, "openshift-tests:pvc-expand", "Storage", []string{"PVC"})

	startMinus1 := civil.Date{Year: 2024, Month: 5, Day: 31}
	endMinus1 := civil.Date{Year: 2024, Month: 6, Day: 14}

	// testA: runs=10, successes=9, flakes=0 -> failures=1 (90% pass rate)
	createCumulativeSummary(t, dbc, startMinus1, release, testA.ID, job.ID, suite.ID, 100, 90, 5)
	createCumulativeSummary(t, dbc, endMinus1, release, testA.ID, job.ID, suite.ID, 110, 99, 5)

	// testB: runs=10, successes=5, flakes=0 -> failures=5 (50% pass rate)
	createCumulativeSummary(t, dbc, startMinus1, release, testB.ID, job.ID, suite.ID, 50, 40, 2)
	createCumulativeSummary(t, dbc, endMinus1, release, testB.ID, job.ID, suite.ID, 60, 45, 2)

	provider := postgres.NewPostgresProvider(dbc, nil)
	opts := defaultReqOptions(release)

	result, errs := provider.QuerySampleTestStatus(context.Background(), opts,
		map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}},
		opts.SampleRelease.Start, opts.SampleRelease.End)
	require.Empty(t, errs)

	keyA := crtest.KeyWithVariants{
		TestID:   towA.UniqueID,
		Variants: map[string]string{"Platform": "aws", "Network": "ovn"},
	}
	keyB := crtest.KeyWithVariants{
		TestID:   towB.UniqueID,
		Variants: map[string]string{"Platform": "aws", "Network": "ovn"},
	}

	tsA, ok := result[keyA.Encode()]
	require.True(t, ok, "testA should appear independently")
	assert.Equal(t, 10, tsA.TotalCount)
	assert.Equal(t, 9, tsA.SuccessCount)
	assert.Equal(t, "Storage", tsA.Component)

	tsB, ok := result[keyB.Encode()]
	require.True(t, ok, "testB should appear independently")
	assert.Equal(t, 10, tsB.TotalCount)
	assert.Equal(t, 5, tsB.SuccessCount)
	assert.Equal(t, "Storage", tsB.Component)
}

func TestBaseAggregatesMultipleJobsInSameVariantGroup(t *testing.T) {
	dbc := crTestDB(t)
	release := "4.16"

	vc1 := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	vc2 := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:sdn"})
	job1 := createProwJobWithVC(t, dbc, "periodic-base-agg-1", release, vc1)
	job2 := createProwJobWithVC(t, dbc, "periodic-base-agg-2", release, vc2)

	test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] base agg test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests-bagg")
	createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:base-agg", "Storage", []string{"PVC"})

	baseLookupStart := civil.Date{Year: 2024, Month: 5, Day: 14}
	baseLookupEnd := civil.Date{Year: 2024, Month: 6, Day: 1}

	// job1: delta runs=20, successes=15, flakes=2
	createCumulativeSummary(t, dbc, baseLookupStart, release, test.ID, job1.ID, suite.ID, 80, 75, 3)
	createCumulativeSummary(t, dbc, baseLookupEnd, release, test.ID, job1.ID, suite.ID, 100, 90, 5)

	// job2: delta runs=10, successes=8, flakes=1
	createCumulativeSummary(t, dbc, baseLookupStart, release, test.ID, job2.ID, suite.ID, 40, 35, 1)
	createCumulativeSummary(t, dbc, baseLookupEnd, release, test.ID, job2.ID, suite.ID, 50, 43, 2)

	provider := postgres.NewPostgresProvider(dbc, nil)
	opts := defaultReqOptions(release)
	opts.VariantOption.DBGroupBy = sets.New[string]("Platform")
	opts.VariantOption.ColumnGroupBy = sets.New[string]("Platform")
	opts.VariantOption.IncludeVariants = map[string][]string{
		"Platform": {"aws"},
	}

	result, errs := provider.QueryBaseTestStatus(context.Background(), opts)
	require.Empty(t, errs)

	key := crtest.KeyWithVariants{
		TestID:   "openshift-tests:base-agg",
		Variants: map[string]string{"Platform": "aws"},
	}
	ts, ok := result[key.Encode()]
	require.True(t, ok, "base should aggregate data from multiple jobs in same variant group")
	assert.Equal(t, 30, ts.TotalCount, "runs should aggregate: 20 + 10")
	assert.Equal(t, 23, ts.SuccessCount, "successes should aggregate: 15 + 8")
	assert.Equal(t, 3, ts.FlakeCount, "flakes should aggregate: 2 + 1")
}

func TestTestDetailStatusMapping(t *testing.T) {
	dbc := crTestDB(t)
	release := "4.16"

	vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	job := createProwJobWithVC(t, dbc, "periodic-status-map", release, vc)

	test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] status mapping test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests-sm")
	tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:status-map", "Storage", []string{"PVC"})

	passTS := time.Date(2024, 6, 5, 12, 0, 0, 0, time.UTC)
	failTS := time.Date(2024, 6, 6, 12, 0, 0, 0, time.UTC)
	flakeTS := time.Date(2024, 6, 7, 12, 0, 0, 0, time.UTC)

	passRun := createProwJobRunForCR(t, dbc, job.ID, release, passTS)
	failRun := createProwJobRunForCR(t, dbc, job.ID, release, failTS)
	flakeRun := createProwJobRunForCR(t, dbc, job.ID, release, flakeTS)

	createProwJobRunTest(t, dbc, passRun.ID, job.ID, test.ID, &suite.ID, 1, release, passTS)
	createProwJobRunTest(t, dbc, failRun.ID, job.ID, test.ID, &suite.ID, 12, release, failTS)
	createProwJobRunTest(t, dbc, flakeRun.ID, job.ID, test.ID, &suite.ID, 13, release, flakeTS)

	provider := postgres.NewPostgresProvider(dbc, nil)
	opts := defaultReqOptions(release)
	opts.TestIDOptions = []reqopts.TestIdentification{{TestID: tow.UniqueID}}

	result, errs := provider.QuerySampleJobRunTestStatus(context.Background(), opts,
		map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}},
		opts.SampleRelease.Start, opts.SampleRelease.End)
	require.Empty(t, errs)

	var summaries []crstatus.TestDetailsSummary
	for _, jobSummaries := range result {
		summaries = append(summaries, jobSummaries...)
	}
	require.Len(t, summaries, 1, "all runs for same test+job should summarize into 1 entry")

	summary := summaries[0]
	assert.Equal(t, 2, summary.Stats.SuccessCount, "pass + flake(success_val=1) = 2 successes")
	assert.Equal(t, 1, summary.Stats.FailureCount, "1 failure run")
	assert.Equal(t, 1, summary.Stats.FlakeCount, "1 flake run")
	require.Len(t, summary.JobRuns, 3, "should have 3 individual job runs")

	passDetail := summary.JobRuns[0]
	assert.Equal(t, 1, passDetail.TotalCount)
	assert.Equal(t, 1, passDetail.SuccessCount, "pass: SuccessCount should be 1")
	assert.Equal(t, 0, passDetail.FlakeCount, "pass: FlakeCount should be 0")

	failDetail := summary.JobRuns[1]
	assert.Equal(t, 1, failDetail.TotalCount)
	assert.Equal(t, 0, failDetail.SuccessCount, "fail: SuccessCount should be 0")
	assert.Equal(t, 0, failDetail.FlakeCount, "fail: FlakeCount should be 0")

	flakeDetail := summary.JobRuns[2]
	assert.Equal(t, 1, flakeDetail.TotalCount)
	assert.Equal(t, 1, flakeDetail.SuccessCount, "flake: SuccessCount should be 1")
	assert.Equal(t, 1, flakeDetail.FlakeCount, "flake: FlakeCount should be 1")
}

func TestJobNameNormalizationMergesResults(t *testing.T) {
	dbc := crTestDB(t)
	release := "4.16"

	vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	// Two jobs whose names differ only by version number, which normalizes to the same key
	job416 := createProwJobWithVC(t, dbc, "periodic-ci-4.16-e2e-aws", release, vc)
	job417 := createProwJobWithVC(t, dbc, "periodic-ci-4.17-e2e-aws", release, vc)

	test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] normalization test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests-norm")
	tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:norm-test", "Storage", []string{"PVC"})

	ts1 := time.Date(2024, 6, 5, 12, 0, 0, 0, time.UTC)
	ts2 := time.Date(2024, 6, 6, 12, 0, 0, 0, time.UTC)

	run1 := createProwJobRunForCR(t, dbc, job416.ID, release, ts1)
	run2 := createProwJobRunForCR(t, dbc, job417.ID, release, ts2)

	createProwJobRunTest(t, dbc, run1.ID, job416.ID, test.ID, &suite.ID, 1, release, ts1)
	createProwJobRunTest(t, dbc, run2.ID, job417.ID, test.ID, &suite.ID, 12, release, ts2)

	provider := postgres.NewPostgresProvider(dbc, nil)
	opts := defaultReqOptions(release)
	opts.TestIDOptions = []reqopts.TestIdentification{{TestID: tow.UniqueID}}

	result, errs := provider.QuerySampleJobRunTestStatus(context.Background(), opts,
		map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}},
		opts.SampleRelease.Start, opts.SampleRelease.End)
	require.Empty(t, errs)

	// Both "periodic-ci-4.16-e2e-aws" and "periodic-ci-4.17-e2e-aws" should normalize
	// to "periodic-ci-X.X-e2e-aws" and merge under that single key
	normalizedKey := utils.NormalizeProwJobName("periodic-ci-4.16-e2e-aws")
	summaries, ok := result[normalizedKey]
	require.True(t, ok, "both job runs should merge under normalized name %q", normalizedKey)
	require.Len(t, summaries, 1, "both runs should summarize into 1 entry for the same test key")
	assert.Len(t, summaries[0].JobRuns, 2, "both individual runs should appear in JobRuns")
}

func TestTestExistsInBaseButNotSample(t *testing.T) {
	dbc := crTestDB(t)
	baseRelease := "4.16"
	sampleRelease := "4.17"

	vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	baseJob := createProwJobWithVC(t, dbc, "periodic-e2e-aws-base-only", baseRelease, vc)
	sampleJob := createProwJobWithVC(t, dbc, "periodic-e2e-aws-sample-only", sampleRelease, vc)

	baseOnlyTest := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] base-only test")
	sampleOnlyTest := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] sample-only test")
	sharedTest := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] shared test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests-missing")

	towBaseOnly := createTestOwnership(t, dbc, baseOnlyTest.ID, &suite.ID, "openshift-tests:base-only", "Storage", []string{"PVC"})
	towSampleOnly := createTestOwnership(t, dbc, sampleOnlyTest.ID, &suite.ID, "openshift-tests:sample-only", "Storage", []string{"PVC"})
	towShared := createTestOwnership(t, dbc, sharedTest.ID, &suite.ID, "openshift-tests:shared", "Storage", []string{"PVC"})

	baseLookupStart := civil.Date{Year: 2024, Month: 5, Day: 14}
	baseLookupEnd := civil.Date{Year: 2024, Month: 6, Day: 1}
	sampleLookupStart := civil.Date{Year: 2024, Month: 5, Day: 31}
	sampleLookupEnd := civil.Date{Year: 2024, Month: 6, Day: 14}

	// baseOnlyTest has base data but no sample data
	createCumulativeSummary(t, dbc, baseLookupStart, baseRelease, baseOnlyTest.ID, baseJob.ID, suite.ID, 80, 75, 3)
	createCumulativeSummary(t, dbc, baseLookupEnd, baseRelease, baseOnlyTest.ID, baseJob.ID, suite.ID, 100, 90, 5)

	// sampleOnlyTest has sample data but no base data
	createCumulativeSummary(t, dbc, sampleLookupStart, sampleRelease, sampleOnlyTest.ID, sampleJob.ID, suite.ID, 50, 40, 2)
	createCumulativeSummary(t, dbc, sampleLookupEnd, sampleRelease, sampleOnlyTest.ID, sampleJob.ID, suite.ID, 60, 48, 3)

	// sharedTest has data in both
	createCumulativeSummary(t, dbc, baseLookupStart, baseRelease, sharedTest.ID, baseJob.ID, suite.ID, 80, 75, 3)
	createCumulativeSummary(t, dbc, baseLookupEnd, baseRelease, sharedTest.ID, baseJob.ID, suite.ID, 100, 90, 5)
	createCumulativeSummary(t, dbc, sampleLookupStart, sampleRelease, sharedTest.ID, sampleJob.ID, suite.ID, 100, 90, 5)
	createCumulativeSummary(t, dbc, sampleLookupEnd, sampleRelease, sharedTest.ID, sampleJob.ID, suite.ID, 110, 98, 6)

	provider := postgres.NewPostgresProvider(dbc, nil)
	includeVariants := map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}}
	opts := reqopts.RequestOptions{
		BaseRelease: reqopts.Release{
			Name:  baseRelease,
			Start: time.Date(2024, 5, 15, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		},
		SampleRelease: reqopts.Release{
			Name:  sampleRelease,
			Start: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
		},
		VariantOption: reqopts.Variants{
			DBGroupBy:       sets.New[string]("Platform", "Network"),
			ColumnGroupBy:   sets.New[string]("Platform"),
			IncludeVariants: includeVariants,
		},
		AdvancedOption: reqopts.Advanced{MinimumFailure: 1},
	}

	variantMap := map[string]string{"Platform": "aws", "Network": "ovn"}

	baseResult, errs := provider.QueryBaseTestStatus(context.Background(), opts)
	require.Empty(t, errs)

	sampleResult, errs := provider.QuerySampleTestStatus(context.Background(), opts,
		includeVariants, opts.SampleRelease.Start, opts.SampleRelease.End)
	require.Empty(t, errs)

	// baseOnlyTest: should be in base results, absent from sample
	baseOnlyKey := crtest.KeyWithVariants{TestID: towBaseOnly.UniqueID, Variants: variantMap}.Encode()
	_, inBase := baseResult[baseOnlyKey]
	assert.True(t, inBase, "base-only test should appear in base results")
	nonPlaceholderSample := filterPlaceholders(sampleResult)
	for _, ts := range nonPlaceholderSample {
		assert.NotEqual(t, towBaseOnly.UniqueID, ts.TestID,
			"base-only test should not appear in sample results")
	}

	// sampleOnlyTest: should be in sample results, absent from base
	sampleOnlyKey := crtest.KeyWithVariants{TestID: towSampleOnly.UniqueID, Variants: variantMap}.Encode()
	_, inSample := sampleResult[sampleOnlyKey]
	assert.True(t, inSample, "sample-only test should appear in sample results")
	nonPlaceholderBase := filterPlaceholders(baseResult)
	for _, ts := range nonPlaceholderBase {
		assert.NotEqual(t, towSampleOnly.UniqueID, ts.TestID,
			"sample-only test should not appear in base results")
	}

	// sharedTest: should be in both
	sharedKey := crtest.KeyWithVariants{TestID: towShared.UniqueID, Variants: variantMap}.Encode()
	_, inBase = baseResult[sharedKey]
	_, inSample = sampleResult[sharedKey]
	assert.True(t, inBase, "shared test should appear in base results")
	assert.True(t, inSample, "shared test should appear in sample results")
}

func TestSingleDayPeriod(t *testing.T) {
	dbc := crTestDB(t)
	release := "4.16"

	vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	job := createProwJobWithVC(t, dbc, "periodic-e2e-aws-1day", release, vc)

	test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] single day test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests-1day")
	tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:1day-test", "Storage", []string{"PVC"})

	// Single-day range: [2024-06-10, 2024-06-11)
	// lookupStart = 2024-06-09, lookupEnd = 2024-06-10
	lookupStart := civil.Date{Year: 2024, Month: 6, Day: 9}
	lookupEnd := civil.Date{Year: 2024, Month: 6, Day: 10}

	// delta: runs=5, successes=2, flakes=1 → failures=2 (passes MinimumFailure=1)
	createCumulativeSummary(t, dbc, lookupStart, release, test.ID, job.ID, suite.ID, 100, 90, 5)
	createCumulativeSummary(t, dbc, lookupEnd, release, test.ID, job.ID, suite.ID, 105, 92, 6)

	provider := postgres.NewPostgresProvider(dbc, nil)
	opts := defaultReqOptions(release)
	opts.SampleRelease.Start = time.Date(2024, 6, 10, 0, 0, 0, 0, time.UTC)
	opts.SampleRelease.End = time.Date(2024, 6, 11, 0, 0, 0, 0, time.UTC)

	result, errs := provider.QuerySampleTestStatus(context.Background(), opts,
		map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}},
		opts.SampleRelease.Start, opts.SampleRelease.End)
	require.Empty(t, errs)

	key := crtest.KeyWithVariants{
		TestID:   tow.UniqueID,
		Variants: map[string]string{"Platform": "aws", "Network": "ovn"},
	}
	ts, ok := result[key.Encode()]
	require.True(t, ok, "single-day period should return results")
	assert.Equal(t, 5, ts.TotalCount, "delta for single day: 105-100=5")
	assert.Equal(t, 2, ts.SuccessCount, "delta for single day: 92-90=2")
	assert.Equal(t, 1, ts.FlakeCount, "delta for single day: 6-5=1")
}

func TestMinimumFailureWithCapabilityFilter(t *testing.T) {
	dbc := crTestDB(t)
	release := "4.16"

	vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	job := createProwJobWithVC(t, dbc, "periodic-e2e-aws-mfcap", release, vc)

	testHigh := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] high failure PVC test")
	testLow := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] low failure PVC test")
	testOther := intutil.CreateTest(t, dbc, "openshift-tests:[sig-network] high failure network test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests-mfcap")

	// testHigh and testLow share PVC capability; testOther has Services capability
	createTestOwnership(t, dbc, testHigh.ID, &suite.ID, "openshift-tests:high-pvc", "Storage", []string{"PVC"})
	towLow := createTestOwnership(t, dbc, testLow.ID, &suite.ID, "openshift-tests:low-pvc", "Storage", []string{"PVC"})
	createTestOwnership(t, dbc, testOther.ID, &suite.ID, "openshift-tests:high-net", "Networking", []string{"Services"})

	startMinus1 := civil.Date{Year: 2024, Month: 5, Day: 31}
	endMinus1 := civil.Date{Year: 2024, Month: 6, Day: 14}

	// testHigh: runs=10, successes=5, flakes=0 -> failures=5
	createCumulativeSummary(t, dbc, startMinus1, release, testHigh.ID, job.ID, suite.ID, 100, 90, 5)
	createCumulativeSummary(t, dbc, endMinus1, release, testHigh.ID, job.ID, suite.ID, 110, 95, 5)

	// testLow: runs=10, successes=9, flakes=0 -> failures=1
	createCumulativeSummary(t, dbc, startMinus1, release, testLow.ID, job.ID, suite.ID, 50, 45, 2)
	createCumulativeSummary(t, dbc, endMinus1, release, testLow.ID, job.ID, suite.ID, 60, 54, 2)

	// testOther: runs=10, successes=5, flakes=0 -> failures=5 (but different capability)
	createCumulativeSummary(t, dbc, startMinus1, release, testOther.ID, job.ID, suite.ID, 200, 180, 10)
	createCumulativeSummary(t, dbc, endMinus1, release, testOther.ID, job.ID, suite.ID, 210, 185, 10)

	provider := postgres.NewPostgresProvider(dbc, nil)
	opts := defaultReqOptions(release)
	opts.AdvancedOption.MinimumFailure = 3
	opts.TestIDOptions = []reqopts.TestIdentification{{
		Capability: "PVC",
	}}

	result, errs := provider.QuerySampleTestStatus(context.Background(), opts,
		map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}},
		opts.SampleRelease.Start, opts.SampleRelease.End)
	require.Empty(t, errs)

	nonPlaceholders := filterPlaceholders(result)
	// testHigh has PVC capability and 5 failures >= 3: should appear
	// testLow has PVC capability but 1 failure < 3: should NOT appear
	// testOther has 5 failures >= 3 but Services capability, not PVC: should NOT appear
	for _, ts := range nonPlaceholders {
		assert.NotEqual(t, towLow.UniqueID, ts.TestID,
			"low-failure PVC test should not pass MinimumFailure=3")
		assert.NotEqual(t, "openshift-tests:high-net", ts.TestID,
			"non-PVC test should not appear with PVC capability filter")
	}
}

func TestDrillDownBySecondaryCapability(t *testing.T) {
	dbc := crTestDB(t)
	release := "4.16"

	vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	job := createProwJobWithVC(t, dbc, "periodic-e2e-aws-cap2", release, vc)

	// testShared has both PVC and IPv4 capabilities
	testShared := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] shared cap test")
	// testIPv4Only has only IPv4
	testIPv4Only := intutil.CreateTest(t, dbc, "openshift-tests:[sig-network] ipv4 only test")
	// testPVCOnly has only PVC
	testPVCOnly := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] pvc only test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests-cap2")

	createTestOwnership(t, dbc, testShared.ID, &suite.ID, "openshift-tests:shared-cap", "Storage", []string{"PVC", "IPv4"})
	createTestOwnership(t, dbc, testIPv4Only.ID, &suite.ID, "openshift-tests:ipv4-only", "Networking", []string{"IPv4"})
	createTestOwnership(t, dbc, testPVCOnly.ID, &suite.ID, "openshift-tests:pvc-only", "Storage", []string{"PVC"})

	startMinus1 := civil.Date{Year: 2024, Month: 5, Day: 31}
	endMinus1 := civil.Date{Year: 2024, Month: 6, Day: 14}

	for _, testModel := range []models.Test{testShared, testIPv4Only, testPVCOnly} {
		createCumulativeSummary(t, dbc, startMinus1, release, testModel.ID, job.ID, suite.ID, 100, 90, 5)
		createCumulativeSummary(t, dbc, endMinus1, release, testModel.ID, job.ID, suite.ID, 110, 98, 6)
	}

	provider := postgres.NewPostgresProvider(dbc, nil)
	opts := defaultReqOptions(release)
	opts.TestIDOptions = []reqopts.TestIdentification{{
		Capability: "IPv4",
	}}

	result, errs := provider.QuerySampleTestStatus(context.Background(), opts,
		map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}},
		opts.SampleRelease.Start, opts.SampleRelease.End)
	require.Empty(t, errs)

	nonPlaceholders := filterPlaceholders(result)
	for _, ts := range nonPlaceholders {
		assert.Contains(t, ts.Capabilities, "IPv4",
			"only tests with IPv4 capability should appear, got %v for %s", ts.Capabilities, ts.TestID)
	}

	// testPVCOnly should not appear (no IPv4 capability)
	for _, ts := range nonPlaceholders {
		assert.NotEqual(t, "openshift-tests:pvc-only", ts.TestID,
			"test with only PVC capability should not appear in IPv4 drill-down")
	}
}

func TestMixedLifecycleRowsProduceCorrectCounts(t *testing.T) {
	dbc := crTestDB(t)
	release := "4.16"

	vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	job := createProwJobWithVC(t, dbc, "periodic-e2e-aws-lifecycle", release, vc)
	test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] PVC lifecycle test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests")
	tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:lifecycle", "Storage", []string{"PersistentVolumes"})

	startMinus1 := civil.Date{Year: 2024, Month: 5, Day: 31}
	endMinus1 := civil.Date{Year: 2024, Month: 6, Day: 14}

	// Blocking: runs=10, successes=8, flakes=1
	createCumulativeSummary(t, dbc, startMinus1, release, test.ID, job.ID, suite.ID, 100, 90, 5, withLifecycle("blocking"))
	createCumulativeSummary(t, dbc, endMinus1, release, test.ID, job.ID, suite.ID, 110, 98, 6, withLifecycle("blocking"))

	// Informing: runs=20, successes=15, flakes=2
	createCumulativeSummary(t, dbc, startMinus1, release, test.ID, job.ID, suite.ID, 50, 40, 3, withLifecycle("informing"))
	createCumulativeSummary(t, dbc, endMinus1, release, test.ID, job.ID, suite.ID, 70, 55, 5, withLifecycle("informing"))

	provider := postgres.NewPostgresProvider(dbc, nil)
	opts := defaultReqOptions(release)
	includeVariants := map[string][]string{
		"Platform": {"aws"},
		"Network":  {"ovn"},
	}

	result, errs := provider.QuerySampleTestStatus(context.Background(), opts, includeVariants,
		opts.SampleRelease.Start, opts.SampleRelease.End)
	require.Empty(t, errs)
	require.NotEmpty(t, result)

	key := crtest.KeyWithVariants{
		TestID:   tow.UniqueID,
		Variants: map[string]string{"Platform": "aws", "Network": "ovn"},
	}
	ts, ok := result[key.Encode()]
	require.True(t, ok, "expected key %s in results", key.Encode())

	// Correct: blocking (10) + informing (20) = 30 runs
	// Without lifecycle join fix, cross-product would inflate to 40
	assert.Equal(t, 30, ts.TotalCount)
	assert.Equal(t, 23, ts.SuccessCount) // blocking 8 + informing 15
	assert.Equal(t, 3, ts.FlakeCount)    // blocking 1 + informing 2
}

func TestLifecycleFilterExcludesInformingFromSample(t *testing.T) {
	dbc := crTestDB(t)
	release := "4.16"

	vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	job := createProwJobWithVC(t, dbc, "periodic-e2e-aws-lifecycle-filter", release, vc)
	test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-network] informing filter test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests")
	tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:lifecycle-filter", "Networking", []string{"Connectivity"})

	// defaultReqOptions uses:
	//   sample: 2024-06-01 to 2024-06-15 -> lookup dates: 2024-05-31 (start-1), 2024-06-14 (end-1)
	//   base:   2024-05-15 to 2024-06-01 -> lookup dates: 2024-05-14 (start-1), 2024-06-01 (end+1-1)
	// Create prefix-sum rows at all four lookup dates.
	baseLookupStart := civil.Date{Year: 2024, Month: 5, Day: 14}
	baseLookupEnd := civil.Date{Year: 2024, Month: 6, Day: 1}
	sampleLookupStart := civil.Date{Year: 2024, Month: 5, Day: 31}
	sampleLookupEnd := civil.Date{Year: 2024, Month: 6, Day: 14}

	// Blocking: base period adds 5 runs (3 success, 1 flake), sample period adds 10 runs (8 success, 1 flake)
	createCumulativeSummary(t, dbc, baseLookupStart, release, test.ID, job.ID, suite.ID, 80, 70, 4, withLifecycle("blocking"))
	createCumulativeSummary(t, dbc, sampleLookupStart, release, test.ID, job.ID, suite.ID, 85, 73, 5, withLifecycle("blocking"))
	createCumulativeSummary(t, dbc, baseLookupEnd, release, test.ID, job.ID, suite.ID, 85, 73, 5, withLifecycle("blocking"))
	createCumulativeSummary(t, dbc, sampleLookupEnd, release, test.ID, job.ID, suite.ID, 95, 81, 6, withLifecycle("blocking"))

	// Informing: base period adds 8 runs (6 success, 1 flake), sample period adds 20 runs (15 success, 2 flakes)
	createCumulativeSummary(t, dbc, baseLookupStart, release, test.ID, job.ID, suite.ID, 30, 24, 2, withLifecycle("informing"))
	createCumulativeSummary(t, dbc, sampleLookupStart, release, test.ID, job.ID, suite.ID, 38, 30, 3, withLifecycle("informing"))
	createCumulativeSummary(t, dbc, baseLookupEnd, release, test.ID, job.ID, suite.ID, 38, 30, 3, withLifecycle("informing"))
	createCumulativeSummary(t, dbc, sampleLookupEnd, release, test.ID, job.ID, suite.ID, 58, 45, 5, withLifecycle("informing"))

	provider := postgres.NewPostgresProvider(dbc, nil)
	includeVariants := map[string][]string{
		"Platform": {"aws"},
		"Network":  {"ovn"},
	}

	key := crtest.KeyWithVariants{
		TestID:   tow.UniqueID,
		Variants: map[string]string{"Platform": "aws", "Network": "ovn"},
	}

	t.Run("sample with lifecycle=blocking excludes informing", func(t *testing.T) {
		opts := defaultReqOptions(release)
		opts.Lifecycles = []string{"blocking"}

		result, errs := provider.QuerySampleTestStatus(context.Background(), opts, includeVariants,
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)
		require.NotEmpty(t, result)

		ts, ok := result[key.Encode()]
		require.True(t, ok, "expected key %s in results", key.Encode())

		// Only blocking sample counts: 10 runs, 8 successes, 1 flake
		assert.Equal(t, 10, ts.TotalCount)
		assert.Equal(t, 8, ts.SuccessCount)
		assert.Equal(t, 1, ts.FlakeCount)
	})

	t.Run("base query does not filter by lifecycle", func(t *testing.T) {
		opts := defaultReqOptions(release)
		opts.Lifecycles = []string{"blocking"}

		result, errs := provider.QueryBaseTestStatus(context.Background(), opts)
		require.Empty(t, errs)
		require.NotEmpty(t, result)

		ts, ok := result[key.Encode()]
		require.True(t, ok, "expected key %s in results", key.Encode())

		// Base includes both blocking (5 runs) and informing (8 runs) = 13 runs
		assert.Equal(t, 13, ts.TotalCount)
		assert.Equal(t, 9, ts.SuccessCount) // blocking 3 + informing 6
		assert.Equal(t, 2, ts.FlakeCount)   // blocking 1 + informing 1
	})

	t.Run("sample without lifecycle filter includes all", func(t *testing.T) {
		opts := defaultReqOptions(release)

		result, errs := provider.QuerySampleTestStatus(context.Background(), opts, includeVariants,
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)
		require.NotEmpty(t, result)

		ts, ok := result[key.Encode()]
		require.True(t, ok, "expected key %s in results", key.Encode())

		// Both blocking (10) + informing (20) = 30 runs
		assert.Equal(t, 30, ts.TotalCount)
		assert.Equal(t, 23, ts.SuccessCount) // blocking 8 + informing 15
		assert.Equal(t, 3, ts.FlakeCount)    // blocking 1 + informing 2
	})

	t.Run("sample with lifecycle=informing returns only informing", func(t *testing.T) {
		opts := defaultReqOptions(release)
		opts.Lifecycles = []string{"informing"}

		result, errs := provider.QuerySampleTestStatus(context.Background(), opts, includeVariants,
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)
		require.NotEmpty(t, result)

		ts, ok := result[key.Encode()]
		require.True(t, ok, "expected key %s in results", key.Encode())

		// Only informing sample counts: 20 runs, 15 successes, 2 flakes
		assert.Equal(t, 20, ts.TotalCount)
		assert.Equal(t, 15, ts.SuccessCount)
		assert.Equal(t, 2, ts.FlakeCount)
	})

	t.Run("sample with both lifecycles returns all", func(t *testing.T) {
		opts := defaultReqOptions(release)
		opts.Lifecycles = []string{"blocking", "informing"}

		result, errs := provider.QuerySampleTestStatus(context.Background(), opts, includeVariants,
			opts.SampleRelease.Start, opts.SampleRelease.End)
		require.Empty(t, errs)
		require.NotEmpty(t, result)

		ts, ok := result[key.Encode()]
		require.True(t, ok, "expected key %s in results", key.Encode())

		// Both blocking (10) + informing (20) = 30 runs
		assert.Equal(t, 30, ts.TotalCount)
		assert.Equal(t, 23, ts.SuccessCount)
		assert.Equal(t, 3, ts.FlakeCount)
	})
}

func TestInformingOnlyTestExcludedFromSamplePlaceholders(t *testing.T) {
	dbc := crTestDB(t)
	release := "4.16"

	vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	job := createProwJobWithVC(t, dbc, "periodic-e2e-aws-placeholder-lifecycle", release, vc)

	testBoth := intutil.CreateTest(t, dbc, "openshift-tests:[sig-network] test with both lifecycles")
	testInformingOnly := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] informing-only test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests")

	towBoth := createTestOwnership(t, dbc, testBoth.ID, &suite.ID, "openshift-tests:both-lifecycle", "Networking", []string{"Connectivity"})
	createTestOwnership(t, dbc, testInformingOnly.ID, &suite.ID, "openshift-tests:informing-only", "Storage", []string{"PVC"})

	startMinus1 := civil.Date{Year: 2024, Month: 5, Day: 31}
	endMinus1 := civil.Date{Year: 2024, Month: 6, Day: 14}

	// testBoth: blocking data in sample period (10 runs, 8 successes, 1 flake)
	createCumulativeSummary(t, dbc, startMinus1, release, testBoth.ID, job.ID, suite.ID, 80, 70, 4, withLifecycle("blocking"))
	createCumulativeSummary(t, dbc, endMinus1, release, testBoth.ID, job.ID, suite.ID, 90, 78, 5, withLifecycle("blocking"))
	// testBoth: informing data
	createCumulativeSummary(t, dbc, startMinus1, release, testBoth.ID, job.ID, suite.ID, 30, 24, 2, withLifecycle("informing"))
	createCumulativeSummary(t, dbc, endMinus1, release, testBoth.ID, job.ID, suite.ID, 50, 39, 4, withLifecycle("informing"))

	// testInformingOnly: ONLY informing data in sample period (20 runs)
	createCumulativeSummary(t, dbc, startMinus1, release, testInformingOnly.ID, job.ID, suite.ID, 30, 24, 2, withLifecycle("informing"))
	createCumulativeSummary(t, dbc, endMinus1, release, testInformingOnly.ID, job.ID, suite.ID, 50, 39, 4, withLifecycle("informing"))

	provider := postgres.NewPostgresProvider(dbc, nil)
	includeVariants := map[string][]string{
		"Platform": {"aws"},
		"Network":  {"ovn"},
	}

	opts := defaultReqOptions(release)
	opts.Lifecycles = []string{"blocking"}

	result, errs := provider.QuerySampleTestStatus(context.Background(), opts, includeVariants,
		opts.SampleRelease.Start, opts.SampleRelease.End)
	require.Empty(t, errs)

	keyBoth := crtest.KeyWithVariants{
		TestID:   towBoth.UniqueID,
		Variants: map[string]string{"Platform": "aws", "Network": "ovn"},
	}
	ts, ok := result[keyBoth.Encode()]
	require.True(t, ok, "test with blocking data should be present")
	assert.Equal(t, 10, ts.TotalCount)
	assert.Equal(t, 8, ts.SuccessCount)
	assert.Equal(t, 1, ts.FlakeCount)

	// testInformingOnly should be absent entirely, including placeholders.
	// With lifecycle=blocking, this test has no data, so its component (Storage)
	// should not appear in the grid.
	for encodedKey, ts := range result {
		if ts.Component == "Storage" {
			t.Errorf("informing-only test's component should not appear in results, found key %s with component Storage", encodedKey)
		}
	}
}

func TestCrossCompareWithLifecycleFilter(t *testing.T) {
	dbc := crTestDB(t)
	release := "4.16"

	vcHA := createVariantCombination(t, dbc, []string{"Platform:aws", "Topology:ha"})
	vcSingle := createVariantCombination(t, dbc, []string{"Platform:aws", "Topology:single"})

	jobHA := createProwJobWithVC(t, dbc, "periodic-e2e-aws-ha-lifecycle", release, vcHA)
	jobSingle := createProwJobWithVC(t, dbc, "periodic-e2e-aws-single-lifecycle", release, vcSingle)

	test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] cross-compare lifecycle test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests-ccl")
	createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:cc-lifecycle", "Storage", []string{"PVC"})

	startMinus1 := civil.Date{Year: 2024, Month: 5, Day: 31}
	endMinus1 := civil.Date{Year: 2024, Month: 6, Day: 14}

	// HA job: blocking 10 runs, informing 5 runs in sample period
	createCumulativeSummary(t, dbc, startMinus1, release, test.ID, jobHA.ID, suite.ID, 100, 90, 5, withLifecycle("blocking"))
	createCumulativeSummary(t, dbc, endMinus1, release, test.ID, jobHA.ID, suite.ID, 110, 98, 6, withLifecycle("blocking"))
	createCumulativeSummary(t, dbc, startMinus1, release, test.ID, jobHA.ID, suite.ID, 40, 35, 2, withLifecycle("informing"))
	createCumulativeSummary(t, dbc, endMinus1, release, test.ID, jobHA.ID, suite.ID, 45, 39, 3, withLifecycle("informing"))

	// Single job: blocking 20 runs (15 successes, 2 flakes), informing 8 runs in sample period
	createCumulativeSummary(t, dbc, startMinus1, release, test.ID, jobSingle.ID, suite.ID, 50, 40, 3, withLifecycle("blocking"))
	createCumulativeSummary(t, dbc, endMinus1, release, test.ID, jobSingle.ID, suite.ID, 70, 55, 5, withLifecycle("blocking"))
	createCumulativeSummary(t, dbc, startMinus1, release, test.ID, jobSingle.ID, suite.ID, 20, 18, 1, withLifecycle("informing"))
	createCumulativeSummary(t, dbc, endMinus1, release, test.ID, jobSingle.ID, suite.ID, 28, 25, 2, withLifecycle("informing"))

	provider := postgres.NewPostgresProvider(dbc, nil)
	opts := defaultReqOptions(release)
	opts.Lifecycles = []string{"blocking"}
	opts.VariantOption.DBGroupBy = sets.New[string]("Platform", "Topology")
	opts.VariantOption.ColumnGroupBy = sets.New[string]("Platform")
	opts.VariantOption.VariantCrossCompare = []string{"Topology"}
	opts.VariantOption.CompareVariants = map[string][]string{"Topology": {"single"}}

	includeVariants := map[string][]string{
		"Platform": {"aws"},
		"Topology": {"ha"},
	}

	result, errs := provider.QuerySampleTestStatus(context.Background(), opts, includeVariants,
		opts.SampleRelease.Start, opts.SampleRelease.End)
	require.Empty(t, errs)

	nonPlaceholders := filterPlaceholders(result)
	require.NotEmpty(t, nonPlaceholders, "should return results for cross-compare with lifecycle filter")
	for _, ts := range nonPlaceholders {
		assert.Equal(t, "single", ts.Variants["Topology"],
			"should return sample-side (single) data, not base-side (ha)")
		// Only blocking counts from the single job: 20 runs, 15 successes, 2 flakes
		assert.Equal(t, 20, ts.TotalCount)
		assert.Equal(t, 15, ts.SuccessCount)
		assert.Equal(t, 2, ts.FlakeCount)
	}
}

// --- Test Details Report Tests ---

func testDetailsReqOptions(testID string, variants map[string]string) reqopts.RequestOptions {
	opts := defaultReqOptions("4.16")
	opts.TestIDOptions = []reqopts.TestIdentification{{
		TestID:            testID,
		RequestedVariants: variants,
	}}
	opts.VariantOption.IncludeVariants = map[string][]string{}
	for k, v := range variants {
		opts.VariantOption.IncludeVariants[k] = []string{v}
	}
	return opts
}

func TestTestDetailsReport_AggregateBaseStats(t *testing.T) {
	dbc := crTestDB(t)
	release := "4.16"

	vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	job := createProwJobWithVC(t, dbc, "periodic-td-agg-aws", release, vc)

	test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] aggregate base stats test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests-td-agg")
	tow := createTestOwnershipFull(t, dbc, test.ID, &suite.ID, "openshift-tests:td-agg", "Storage", []string{"PVC"}, uintPtr(42))

	// Base: aggregate data only (cumulative summaries, no prow_job_run_tests)
	baseLookupStart := civil.Date{Year: 2024, Month: 5, Day: 14}
	baseLookupEnd := civil.Date{Year: 2024, Month: 6, Day: 1}
	// runs=20, successes=15, flakes=2
	createCumulativeSummary(t, dbc, baseLookupStart, release, test.ID, job.ID, suite.ID, 80, 75, 3)
	createCumulativeSummary(t, dbc, baseLookupEnd, release, test.ID, job.ID, suite.ID, 100, 90, 5)

	// Sample: per-run data (3 passes + 1 failure)
	sampleTS1 := time.Date(2024, 6, 5, 12, 0, 0, 0, time.UTC)
	sampleTS2 := time.Date(2024, 6, 6, 12, 0, 0, 0, time.UTC)
	sampleTS3 := time.Date(2024, 6, 7, 12, 0, 0, 0, time.UTC)
	sampleTS4 := time.Date(2024, 6, 8, 12, 0, 0, 0, time.UTC)

	run1 := createProwJobRunForCR(t, dbc, job.ID, release, sampleTS1)
	run2 := createProwJobRunForCR(t, dbc, job.ID, release, sampleTS2)
	run3 := createProwJobRunForCR(t, dbc, job.ID, release, sampleTS3)
	run4 := createProwJobRunForCR(t, dbc, job.ID, release, sampleTS4)

	createProwJobRunTest(t, dbc, run1.ID, job.ID, test.ID, &suite.ID, 1, release, sampleTS1)
	createProwJobRunTest(t, dbc, run2.ID, job.ID, test.ID, &suite.ID, 1, release, sampleTS2)
	createProwJobRunTest(t, dbc, run3.ID, job.ID, test.ID, &suite.ID, 1, release, sampleTS3)
	createProwJobRunTest(t, dbc, run4.ID, job.ID, test.ID, &suite.ID, 12, release, sampleTS4)

	variants := map[string]string{"Platform": "aws", "Network": "ovn"}
	opts := testDetailsReqOptions(tow.UniqueID, variants)

	provider := postgres.NewPostgresProvider(dbc, nil)
	generator := componentreadiness.NewComponentReportGenerator(provider, opts, dbc, nil, "")
	report, errs := generator.GenerateTestDetailsReport(context.Background())
	require.Empty(t, errs)

	require.Len(t, report.Analyses, 1)
	analysis := report.Analyses[0]

	require.NotNil(t, analysis.BaseStats, "base stats should be populated from aggregate data")
	assert.Equal(t, 20, analysis.BaseStats.Total(), "base total should be prefix-sum delta")
	assert.Equal(t, 15, analysis.BaseStats.SuccessCount)

	assert.Equal(t, 4, analysis.SampleStats.Total(), "sample should have 4 runs")
	assert.Equal(t, 3, analysis.SampleStats.SuccessCount)
	assert.Equal(t, 1, analysis.SampleStats.FailureCount)

	require.Len(t, analysis.JobStats, 1)
	jobStat := analysis.JobStats[0]
	assert.Empty(t, jobStat.BaseJobRunStats, "aggregate base should have no individual runs")
	assert.Len(t, jobStat.SampleJobRunStats, 4, "sample should have 4 individual runs")

	assert.Equal(t, "Storage", report.JiraComponent)
	require.NotNil(t, report.JiraComponentID)
	assert.Equal(t, 0, report.JiraComponentID.Cmp(new(big.Rat).SetUint64(42)))
	assert.Equal(t, test.Name, report.TestName)
}

func TestTestDetailsReport_LastFailureTracking(t *testing.T) {
	t.Run("last failure populated from sample failures", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		job := createProwJobWithVC(t, dbc, "periodic-td-lf-aws", release, vc)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] last failure tracking test")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-td-lf")
		tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:td-lf", "Storage", []string{"PVC"})

		// Base: aggregate data
		baseLookupStart := civil.Date{Year: 2024, Month: 5, Day: 14}
		baseLookupEnd := civil.Date{Year: 2024, Month: 6, Day: 1}
		createCumulativeSummary(t, dbc, baseLookupStart, release, test.ID, job.ID, suite.ID, 80, 75, 3)
		createCumulativeSummary(t, dbc, baseLookupEnd, release, test.ID, job.ID, suite.ID, 100, 90, 5)

		// Sample: 2 passes and 1 failure at known timestamps
		passTS1 := time.Date(2024, 6, 5, 12, 0, 0, 0, time.UTC)
		failTS := time.Date(2024, 6, 8, 14, 30, 0, 0, time.UTC)
		passTS2 := time.Date(2024, 6, 10, 12, 0, 0, 0, time.UTC)

		runPass1 := createProwJobRunForCR(t, dbc, job.ID, release, passTS1)
		runFail := createProwJobRunForCR(t, dbc, job.ID, release, failTS)
		runPass2 := createProwJobRunForCR(t, dbc, job.ID, release, passTS2)

		createProwJobRunTest(t, dbc, runPass1.ID, job.ID, test.ID, &suite.ID, 1, release, passTS1)
		createProwJobRunTest(t, dbc, runFail.ID, job.ID, test.ID, &suite.ID, 12, release, failTS)
		createProwJobRunTest(t, dbc, runPass2.ID, job.ID, test.ID, &suite.ID, 1, release, passTS2)

		variants := map[string]string{"Platform": "aws", "Network": "ovn"}
		opts := testDetailsReqOptions(tow.UniqueID, variants)

		provider := postgres.NewPostgresProvider(dbc, nil)
		generator := componentreadiness.NewComponentReportGenerator(provider, opts, dbc, nil, "")
		report, errs := generator.GenerateTestDetailsReport(context.Background())
		require.Empty(t, errs)

		require.Len(t, report.Analyses, 1)
		require.NotNil(t, report.Analyses[0].LastFailure, "LastFailure should be set when sample has failures")
		assert.Equal(t, failTS, *report.Analyses[0].LastFailure,
			"LastFailure should equal the failing run's timestamp: got %v, want %v",
			*report.Analyses[0].LastFailure, failTS)
	})

	t.Run("last failure nil when all sample runs pass", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		job := createProwJobWithVC(t, dbc, "periodic-td-lf-pass-aws", release, vc)

		test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] last failure nil test")
		suite := intutil.CreateSuite(t, dbc, "openshift-tests-td-lf-pass")
		tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:td-lf-pass", "Storage", []string{"PVC"})

		// Base: aggregate data
		baseLookupStart := civil.Date{Year: 2024, Month: 5, Day: 14}
		baseLookupEnd := civil.Date{Year: 2024, Month: 6, Day: 1}
		createCumulativeSummary(t, dbc, baseLookupStart, release, test.ID, job.ID, suite.ID, 80, 75, 3)
		createCumulativeSummary(t, dbc, baseLookupEnd, release, test.ID, job.ID, suite.ID, 100, 90, 5)

		// Sample: all passes
		passTS1 := time.Date(2024, 6, 5, 12, 0, 0, 0, time.UTC)
		passTS2 := time.Date(2024, 6, 8, 12, 0, 0, 0, time.UTC)

		runPass1 := createProwJobRunForCR(t, dbc, job.ID, release, passTS1)
		runPass2 := createProwJobRunForCR(t, dbc, job.ID, release, passTS2)

		createProwJobRunTest(t, dbc, runPass1.ID, job.ID, test.ID, &suite.ID, 1, release, passTS1)
		createProwJobRunTest(t, dbc, runPass2.ID, job.ID, test.ID, &suite.ID, 1, release, passTS2)

		variants := map[string]string{"Platform": "aws", "Network": "ovn"}
		opts := testDetailsReqOptions(tow.UniqueID, variants)

		provider := postgres.NewPostgresProvider(dbc, nil)
		generator := componentreadiness.NewComponentReportGenerator(provider, opts, dbc, nil, "")
		report, errs := generator.GenerateTestDetailsReport(context.Background())
		require.Empty(t, errs)

		require.Len(t, report.Analyses, 1)
		assert.Nil(t, report.Analyses[0].LastFailure, "LastFailure should be nil when all sample runs pass")
	})
}

func TestTestDetailsReport_FlakeAsFailure(t *testing.T) {
	dbc := crTestDB(t)
	release := "4.16"

	vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	job := createProwJobWithVC(t, dbc, "periodic-td-faf-aws", release, vc)

	test := intutil.CreateTest(t, dbc, "openshift-tests:[sig-storage] flake as failure test")
	suite := intutil.CreateSuite(t, dbc, "openshift-tests-td-faf")
	tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:td-faf", "Storage", []string{"PVC"})

	// Base: per-run data with flakes (5 passes, 1 failure, 2 flakes)
	baseTS1 := time.Date(2024, 5, 16, 12, 0, 0, 0, time.UTC)
	baseTS2 := time.Date(2024, 5, 17, 12, 0, 0, 0, time.UTC)
	baseTS3 := time.Date(2024, 5, 18, 12, 0, 0, 0, time.UTC)
	baseTS4 := time.Date(2024, 5, 19, 12, 0, 0, 0, time.UTC)
	baseTS5 := time.Date(2024, 5, 20, 12, 0, 0, 0, time.UTC)
	baseTS6 := time.Date(2024, 5, 21, 12, 0, 0, 0, time.UTC)
	baseTS7 := time.Date(2024, 5, 22, 12, 0, 0, 0, time.UTC)
	baseTS8 := time.Date(2024, 5, 23, 12, 0, 0, 0, time.UTC)

	for _, ts := range []time.Time{baseTS1, baseTS2, baseTS3, baseTS4, baseTS5} {
		run := createProwJobRunForCR(t, dbc, job.ID, release, ts)
		createProwJobRunTest(t, dbc, run.ID, job.ID, test.ID, &suite.ID, 1, release, ts)
	}
	baseFailRun := createProwJobRunForCR(t, dbc, job.ID, release, baseTS6)
	createProwJobRunTest(t, dbc, baseFailRun.ID, job.ID, test.ID, &suite.ID, 12, release, baseTS6)
	for _, ts := range []time.Time{baseTS7, baseTS8} {
		run := createProwJobRunForCR(t, dbc, job.ID, release, ts)
		createProwJobRunTest(t, dbc, run.ID, job.ID, test.ID, &suite.ID, 13, release, ts)
	}

	// Sample: per-run data with flakes (4 passes, 1 failure, 1 flake)
	sampleTS1 := time.Date(2024, 6, 5, 12, 0, 0, 0, time.UTC)
	sampleTS2 := time.Date(2024, 6, 6, 12, 0, 0, 0, time.UTC)
	sampleTS3 := time.Date(2024, 6, 7, 12, 0, 0, 0, time.UTC)
	sampleTS4 := time.Date(2024, 6, 8, 12, 0, 0, 0, time.UTC)
	sampleTS5 := time.Date(2024, 6, 9, 12, 0, 0, 0, time.UTC)
	sampleTS6 := time.Date(2024, 6, 10, 12, 0, 0, 0, time.UTC)

	for _, ts := range []time.Time{sampleTS1, sampleTS2, sampleTS3, sampleTS4} {
		run := createProwJobRunForCR(t, dbc, job.ID, release, ts)
		createProwJobRunTest(t, dbc, run.ID, job.ID, test.ID, &suite.ID, 1, release, ts)
	}
	sampleFailRun := createProwJobRunForCR(t, dbc, job.ID, release, sampleTS5)
	createProwJobRunTest(t, dbc, sampleFailRun.ID, job.ID, test.ID, &suite.ID, 12, release, sampleTS5)
	sampleFlakeRun := createProwJobRunForCR(t, dbc, job.ID, release, sampleTS6)
	createProwJobRunTest(t, dbc, sampleFlakeRun.ID, job.ID, test.ID, &suite.ID, 13, release, sampleTS6)

	variants := map[string]string{"Platform": "aws", "Network": "ovn"}

	// Run with FlakeAsFailure=false
	optsFalse := testDetailsReqOptions(tow.UniqueID, variants)
	optsFalse.AdvancedOption.FlakeAsFailure = false

	provider := postgres.NewPostgresProvider(dbc, nil)
	genFalse := componentreadiness.NewComponentReportGenerator(provider, optsFalse, dbc, nil, "")
	reportFalse, errs := genFalse.GenerateTestDetailsReport(context.Background())
	require.Empty(t, errs)

	// Run with FlakeAsFailure=true
	optsTrue := testDetailsReqOptions(tow.UniqueID, variants)
	optsTrue.AdvancedOption.FlakeAsFailure = true

	genTrue := componentreadiness.NewComponentReportGenerator(provider, optsTrue, dbc, nil, "")
	reportTrue, errs := genTrue.GenerateTestDetailsReport(context.Background())
	require.Empty(t, errs)

	require.Len(t, reportFalse.Analyses, 1)
	require.Len(t, reportTrue.Analyses, 1)

	sampleFalse := reportFalse.Analyses[0].SampleStats
	sampleTrue := reportTrue.Analyses[0].SampleStats

	// Raw counts should be the same regardless of FlakeAsFailure
	assert.Equal(t, sampleFalse.FailureCount, sampleTrue.FailureCount, "raw FailureCount should be the same")
	assert.Equal(t, sampleFalse.FlakeCount, sampleTrue.FlakeCount, "raw FlakeCount should be the same")
	assert.Equal(t, sampleFalse.SuccessCount, sampleTrue.SuccessCount, "raw SuccessCount should be the same")

	// SuccessRate should differ: FlakeAsFailure=true counts flakes as failures, lowering the rate
	assert.Greater(t, sampleFalse.SuccessRate, sampleTrue.SuccessRate,
		"SuccessRate with FlakeAsFailure=false (%f) should be higher than with true (%f)",
		sampleFalse.SuccessRate, sampleTrue.SuccessRate)
}

// --- Helpers ---

func isPlaceholderKey(testID string) bool {
	return len(testID) > 5 && testID[:5] == "grid:"
}

func filterPlaceholders(result map[string]crstatus.TestStatus) []crstatus.TestStatus {
	var nonPlaceholder []crstatus.TestStatus
	for _, ts := range result {
		if !isPlaceholderKey(ts.TestID) {
			nonPlaceholder = append(nonPlaceholder, ts)
		}
	}
	return nonPlaceholder
}

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

func createVariantCombination(t *testing.T, dbc *db.DB, variants []string) models.VariantCombination {
	t.Helper()
	vc := models.VariantCombination{Variants: pq.StringArray(variants)}
	require.NoError(t, dbc.DB.Create(&vc).Error)
	return vc
}

func createProwJobWithVC(t *testing.T, dbc *db.DB, name, release string, vc models.VariantCombination) models.ProwJob {
	t.Helper()
	job := models.ProwJob{
		Name:                 name,
		Release:              release,
		Variants:             vc.Variants,
		VariantCombinationID: &vc.ID,
	}
	require.NoError(t, dbc.DB.Create(&job).Error)
	return job
}

func createTest(t *testing.T, dbc *db.DB, name string) models.Test {
	t.Helper()
	test := models.Test{Name: name}
	require.NoError(t, dbc.DB.Create(&test).Error)
	return test
}

func createSuite(t *testing.T, dbc *db.DB, name string) models.Suite {
	t.Helper()
	suite := models.Suite{Name: name}
	require.NoError(t, dbc.DB.Create(&suite).Error)
	return suite
}

func createTestOwnership(t *testing.T, dbc *db.DB, testID uint, suiteID *uint, uniqueID, component string, caps []string) models.TestOwnership {
	t.Helper()
	tow := models.TestOwnership{
		TestID:       testID,
		SuiteID:      suiteID,
		UniqueID:     uniqueID,
		Name:         uniqueID,
		Component:    component,
		Capabilities: pq.StringArray(caps),
	}
	require.NoError(t, dbc.DB.Create(&tow).Error)
	return tow
}

func createCumulativeSummary(t *testing.T, dbc *db.DB, date civil.Date, release string, testID, prowJobID, suiteID uint, runs, successes, flakes int64) {
	t.Helper()
	tcs := models.TestCumulativeSummary{
		Date:               date,
		Release:            release,
		TestID:             testID,
		ProwJobID:          prowJobID,
		SuiteID:            suiteID,
		PrefixSumRuns:      runs,
		PrefixSumSuccesses: successes,
		PrefixSumFlakes:    flakes,
	}
	require.NoError(t, dbc.DB.Create(&tcs).Error)
}

func createGARawData(t *testing.T, dbc *db.DB, release string, windowDays int, testID, prowJobID, suiteID uint, runs, passes, flakes int64) {
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

func createReleaseDefinition(t *testing.T, dbc *db.DB, release string, gaDate *time.Time) {
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
	tow := models.TestOwnership{
		TestID:          testID,
		SuiteID:         suiteID,
		UniqueID:        uniqueID,
		Name:            uniqueID,
		Component:       component,
		Capabilities:    pq.StringArray(caps),
		JiraComponentID: jiraComponentID,
	}
	require.NoError(t, dbc.DB.Create(&tow).Error)
	return tow
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

	test1 := createTest(t, dbc, "openshift-tests:[sig-storage] PVC should work")
	test2 := createTest(t, dbc, "openshift-tests:[sig-network] Services should serve")
	test3 := createTest(t, dbc, "openshift-tests:[sig-auth] RBAC should restrict")

	suite := createSuite(t, dbc, "openshift-tests")

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

		test := createTest(t, dbc, "openshift-tests:[sig-storage] PVC test")
		suite := createSuite(t, dbc, "openshift-tests")
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

		test := createTest(t, dbc, "openshift-tests:[sig-storage] release isolation")
		suite := createSuite(t, dbc, "openshift-tests-ri")
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

		test := createTest(t, dbc, "openshift-tests:[sig-storage] obsolete test")
		suite := createSuite(t, dbc, "openshift-tests-obs")

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

		test := createTest(t, dbc, "openshift-tests:[sig-storage] multi-suite test")
		suiteA := createSuite(t, dbc, "suite-a")
		suiteB := createSuite(t, dbc, "suite-b")

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

		test := createTest(t, dbc, "openshift-tests:[sig-storage] cross-compare test")
		suite := createSuite(t, dbc, "openshift-tests-cc")
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

		test := createTest(t, dbc, "openshift-tests:[sig-storage] last failure test")
		suite := createSuite(t, dbc, "openshift-tests-lf")
		tow := createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:lf-test", "Storage", []string{"PVC"})

		startMinus1 := civil.Date{Year: 2024, Month: 5, Day: 31}
		endMinus1 := civil.Date{Year: 2024, Month: 6, Day: 14}

		// runs=10, successes=8, flakes=1 -> failures=1
		createCumulativeSummary(t, dbc, startMinus1, release, test.ID, job.ID, suite.ID, 100, 90, 5)
		createCumulativeSummary(t, dbc, endMinus1, release, test.ID, job.ID, suite.ID, 110, 98, 6)

		failTime1 := time.Date(2024, 6, 5, 12, 0, 0, 0, time.UTC)
		failTime2 := time.Date(2024, 6, 10, 18, 0, 0, 0, time.UTC)
		run1 := createProwJobRunForCR(t, dbc, job.ID, release, failTime1)
		run2 := createProwJobRunForCR(t, dbc, job.ID, release, failTime2)
		createProwJobRunTest(t, dbc, run1.ID, job.ID, test.ID, &suite.ID, 12, release, failTime1) // failure
		createProwJobRunTest(t, dbc, run2.ID, job.ID, test.ID, &suite.ID, 12, release, failTime2) // later failure

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
		assert.False(t, ts.LastFailure.IsZero(), "LastFailure should be populated when failures exist")
		assert.True(t, ts.LastFailure.Equal(failTime2),
			"LastFailure should be the most recent failure: got %v, want %v", ts.LastFailure, failTime2)
	})

	t.Run("results limited to available data when requested range exceeds it", func(t *testing.T) {
		dbc := crTestDB(t)
		release := "4.16"

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		job := createProwJobWithVC(t, dbc, "periodic-e2e-aws-clamp", release, vc)

		test := createTest(t, dbc, "openshift-tests:[sig-storage] clamping test")
		suite := createSuite(t, dbc, "openshift-tests-clamp")
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

		test := createTest(t, dbc, "openshift-tests:[sig-storage] no suite test")
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

		test := createTest(t, dbc, "openshift-tests:[sig-storage] coalesce test")
		suite := createSuite(t, dbc, "openshift-tests-co")
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

		test := createTest(t, dbc, "openshift-tests:[sig-storage] merge test")
		suite := createSuite(t, dbc, "openshift-tests-merge")
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

	test := createTest(t, dbc, "openshift-tests:[sig-storage] cross-release test")
	suite := createSuite(t, dbc, "openshift-tests-xr")
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
	gaDate := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	createReleaseDefinition(t, dbc, release, &gaDate)

	gaCivil := civil.DateOf(gaDate)
	gaEnd := utils.GAWindowEnd(gaCivil)
	windowDays := 30
	gaStart := gaCivil.AddDays(-windowDays)

	vcAWS := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	jobAWS := createProwJobWithVC(t, dbc, "periodic-ga-aws-ovn", release, vcAWS)
	test := createTest(t, dbc, "openshift-tests:[sig-storage] GA PVC test")
	suite := createSuite(t, dbc, "openshift-tests-ga")
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

		test := createTest(t, dbc, "openshift-tests:[sig-storage] GA nil date test")
		suite := createSuite(t, dbc, "openshift-tests-ga-nil")
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

		futureGA := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
		createReleaseDefinition(t, dbc, release, &futureGA)

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		job := createProwJobWithVC(t, dbc, "periodic-ga-future-aws", release, vc)

		test := createTest(t, dbc, "openshift-tests:[sig-storage] GA future date test")
		suite := createSuite(t, dbc, "openshift-tests-ga-future")
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
		gaDate := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		createReleaseDefinition(t, dbc, release, &gaDate)

		vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
		job := createProwJobWithVC(t, dbc, "periodic-ga-nonstandard-aws", release, vc)

		test := createTest(t, dbc, "openshift-tests:[sig-storage] GA nonstandard window test")
		suite := createSuite(t, dbc, "openshift-tests-ga-ns")
		createTestOwnership(t, dbc, test.ID, &suite.ID, "openshift-tests:ga-nonstandard", "Storage", []string{"PVC"})

		// Base period: May 17 to June 2 (= GAWindowEnd for June 1)
		// windowDays = gaCivil.DaysSince(Start) = June1 - May17 = 15 (not in GAWindows)
		baseLookupStart := civil.Date{Year: 2024, Month: 5, Day: 16}
		baseLookupEnd := civil.Date{Year: 2024, Month: 6, Day: 1}
		createCumulativeSummary(t, dbc, baseLookupStart, release, test.ID, job.ID, suite.ID, 80, 75, 3)
		createCumulativeSummary(t, dbc, baseLookupEnd, release, test.ID, job.ID, suite.ID, 95, 88, 4)

		provider := postgres.NewPostgresProvider(dbc, nil)
		gaCivil := civil.DateOf(gaDate)
		gaEnd := utils.GAWindowEnd(gaCivil)
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

	test := createTest(t, dbc, "openshift-tests:[sig-storage] JR PVC test")
	suite := createSuite(t, dbc, "openshift-tests-jr")
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

		totalRows := 0
		for _, rows := range result {
			totalRows += len(rows)
		}
		assert.Equal(t, 3, totalRows, "should have 3 job run test rows")

		// Verify that all rows reference the correct test
		for _, rows := range result {
			for _, row := range rows {
				assert.Equal(t, tow.UniqueID, row.TestKey.TestID)
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

		totalRows := 0
		for _, rows := range result {
			totalRows += len(rows)
		}
		assert.Equal(t, 2, totalRows, "RequestedVariants=aws should exclude gcp run")
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

		test := createTest(t, dbc, "openshift-tests:[sig-storage] infra exclusion test")
		suite := createSuite(t, dbc, "openshift-tests-infra")
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

		test := createTest(t, dbc, "openshift-tests:[sig-storage] jira component test")
		suite := createSuite(t, dbc, "openshift-tests-jira")
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

		test := createTest(t, dbc, "openshift-tests:[sig-storage] JR release isolation")
		suite := createSuite(t, dbc, "openshift-tests-jr-ri")
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

	test := createTest(t, dbc, "openshift-tests:[sig-network] Base JR test")
	suite := createSuite(t, dbc, "openshift-tests-base-jr")
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

	totalRows := 0
	for _, rows := range result {
		totalRows += len(rows)
	}
	assert.Equal(t, 2, totalRows, "should have 2 base job run test rows")

	for _, rows := range result {
		for _, row := range rows {
			assert.Equal(t, tow.UniqueID, row.TestKey.TestID)
		}
	}
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

	earlier := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)
	later := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	gaDate := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)

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
	assert.True(t, releases[1].GADate.Equal(gaDate))
}

func TestQueryReleaseDates(t *testing.T) {
	dbc := crTestDB(t)

	gaDate := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	devStart := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)
	devStartNoGA := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

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
	assert.True(t, withGA.End.Equal(gaDate), "End should be the GA date")

	require.NotNil(t, withoutGA, "release 4.16 should be in results")
	assert.Nil(t, withoutGA.Start, "release without GADate should have nil Start")
	assert.Nil(t, withoutGA.End, "release without GADate should have nil End")
}

func TestGAPathAggregatesMultipleJobs(t *testing.T) {
	dbc := crTestDB(t)
	release := "4.15"

	gaDate := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	createReleaseDefinition(t, dbc, release, &gaDate)

	gaCivil := civil.DateOf(gaDate)
	gaEnd := utils.GAWindowEnd(gaCivil)
	windowDays := 30
	gaStart := gaCivil.AddDays(-windowDays)

	vc1 := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	vc2 := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:sdn"})
	job1 := createProwJobWithVC(t, dbc, "periodic-ga-multi-1", release, vc1)
	job2 := createProwJobWithVC(t, dbc, "periodic-ga-multi-2", release, vc2)

	test := createTest(t, dbc, "openshift-tests:[sig-storage] GA multi-job test")
	suite := createSuite(t, dbc, "openshift-tests-ga-mj")
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

	testA := createTest(t, dbc, "openshift-tests:[sig-storage] PVC create")
	testB := createTest(t, dbc, "openshift-tests:[sig-storage] PVC expand")
	suite := createSuite(t, dbc, "openshift-tests-multi")

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

	test := createTest(t, dbc, "openshift-tests:[sig-storage] base agg test")
	suite := createSuite(t, dbc, "openshift-tests-bagg")
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

	test := createTest(t, dbc, "openshift-tests:[sig-storage] status mapping test")
	suite := createSuite(t, dbc, "openshift-tests-sm")
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

	var rows []crstatus.TestJobRunRows
	for _, jobRows := range result {
		rows = append(rows, jobRows...)
	}
	require.Len(t, rows, 3)

	// Results are ordered by pjr.timestamp: pass (June 5), fail (June 6), flake (June 7)
	passRow := rows[0]
	assert.Equal(t, 1, passRow.TotalCount)
	assert.Equal(t, 1, passRow.SuccessCount, "pass: SuccessCount should be 1")
	assert.Equal(t, 0, passRow.FlakeCount, "pass: FlakeCount should be 0")

	failRow := rows[1]
	assert.Equal(t, 1, failRow.TotalCount)
	assert.Equal(t, 0, failRow.SuccessCount, "fail: SuccessCount should be 0")
	assert.Equal(t, 0, failRow.FlakeCount, "fail: FlakeCount should be 0")

	flakeRow := rows[2]
	assert.Equal(t, 1, flakeRow.TotalCount)
	assert.Equal(t, 1, flakeRow.SuccessCount, "flake: SuccessCount should be 1")
	assert.Equal(t, 1, flakeRow.FlakeCount, "flake: FlakeCount should be 1")
}

func TestJobNameNormalizationMergesResults(t *testing.T) {
	dbc := crTestDB(t)
	release := "4.16"

	vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	// Two jobs whose names differ only by version number, which normalizes to the same key
	job416 := createProwJobWithVC(t, dbc, "periodic-ci-4.16-e2e-aws", release, vc)
	job417 := createProwJobWithVC(t, dbc, "periodic-ci-4.17-e2e-aws", release, vc)

	test := createTest(t, dbc, "openshift-tests:[sig-storage] normalization test")
	suite := createSuite(t, dbc, "openshift-tests-norm")
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
	rows, ok := result[normalizedKey]
	require.True(t, ok, "both job runs should merge under normalized name %q", normalizedKey)
	assert.Len(t, rows, 2, "both runs should appear under the normalized key")
}

func TestTestExistsInBaseButNotSample(t *testing.T) {
	dbc := crTestDB(t)
	baseRelease := "4.16"
	sampleRelease := "4.17"

	vc := createVariantCombination(t, dbc, []string{"Platform:aws", "Network:ovn"})
	baseJob := createProwJobWithVC(t, dbc, "periodic-e2e-aws-base-only", baseRelease, vc)
	sampleJob := createProwJobWithVC(t, dbc, "periodic-e2e-aws-sample-only", sampleRelease, vc)

	baseOnlyTest := createTest(t, dbc, "openshift-tests:[sig-storage] base-only test")
	sampleOnlyTest := createTest(t, dbc, "openshift-tests:[sig-storage] sample-only test")
	sharedTest := createTest(t, dbc, "openshift-tests:[sig-storage] shared test")
	suite := createSuite(t, dbc, "openshift-tests-missing")

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

	test := createTest(t, dbc, "openshift-tests:[sig-storage] single day test")
	suite := createSuite(t, dbc, "openshift-tests-1day")
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

	testHigh := createTest(t, dbc, "openshift-tests:[sig-storage] high failure PVC test")
	testLow := createTest(t, dbc, "openshift-tests:[sig-storage] low failure PVC test")
	testOther := createTest(t, dbc, "openshift-tests:[sig-network] high failure network test")
	suite := createSuite(t, dbc, "openshift-tests-mfcap")

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
	testShared := createTest(t, dbc, "openshift-tests:[sig-storage] shared cap test")
	// testIPv4Only has only IPv4
	testIPv4Only := createTest(t, dbc, "openshift-tests:[sig-network] ipv4 only test")
	// testPVCOnly has only PVC
	testPVCOnly := createTest(t, dbc, "openshift-tests:[sig-storage] pvc only test")
	suite := createSuite(t, dbc, "openshift-tests-cap2")

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

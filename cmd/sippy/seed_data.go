package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"

	componentreadiness "github.com/openshift/sippy/pkg/api/componentreadiness"
	pgprovider "github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider/postgres"
	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/cache"
	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/models/jobrunscan"
	"github.com/openshift/sippy/pkg/flags"
	"github.com/openshift/sippy/pkg/sippyserver"
	"github.com/openshift/sippy/pkg/util/sets"
)

type SeedDataFlags struct {
	DBFlags      *flags.PostgresFlags
	InitDatabase bool
}

func NewSeedDataFlags() *SeedDataFlags {
	return &SeedDataFlags{
		DBFlags: flags.NewPostgresDatabaseFlags(),
	}
}

func (f *SeedDataFlags) BindFlags(fs *pflag.FlagSet) {
	f.DBFlags.BindFlags(fs)
	fs.BoolVar(&f.InitDatabase, "init-database", false, "Initialize the DB schema before seeding data")
}

func NewSeedDataCommand() *cobra.Command {
	f := NewSeedDataFlags()

	cmd := &cobra.Command{
		Use:   "seed-data",
		Short: "Populate test data in the database",
		Long: `Populate test data in the database for development purposes.

Creates deterministic Component Readiness data covering all CR statuses
(NotSignificant, SignificantRegression, ExtremeRegression, MissingSample,
MissingBasis, BasisOnly, SignificantImprovement, BelowMinFailure) and
fallback scenarios. Use with 'sippy serve --data-provider postgres'.

Drop and recreate the database to re-seed (e.g. docker compose down -v).
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.Contains(f.DBFlags.DSN, "amazonaws.com") {
				return fmt.Errorf("refusing to seed synthetic data into a production database")
			}

			dbc, err := f.DBFlags.GetDBClient()
			if err != nil {
				return errors.WithMessage(err, "could not connect to database")
			}

			if f.InitDatabase {
				log.Info("Initializing database schema...")
				t := f.DBFlags.GetPinnedTime()
				if err := dbc.UpdateSchema(t); err != nil {
					return errors.WithMessage(err, "could not migrate database")
				}
				log.Info("Database schema initialized successfully")
			}

			log.Info("Starting to seed test data...")
			return seedSyntheticData(dbc)
		},
	}

	f.BindFlags(cmd.Flags())

	return cmd
}

// --- Synthetic data seeding ---

// syntheticJobDef defines a job with its full 9-key variant map.
type syntheticJobDef struct {
	nameTemplate string
	variants     map[string]string
}

// syntheticTestSpec defines a test with deterministic pass/fail counts per release per job.
type syntheticTestSpec struct {
	testID       string
	testName     string
	component    string
	capabilities []string
	// Each entry maps a job name template -> per-release counts.
	// The job template determines which variants the test runs with.
	jobCounts map[string]map[string]testCount // jobTemplate -> release -> counts
}

type testCount struct {
	total   int
	success int
	flake   int
}

var syntheticReleases = []string{"4.22", "4.21", "4.20", "4.19"}

var syntheticJobs = []syntheticJobDef{
	{
		nameTemplate: "periodic-ci-openshift-release-master-ci-%s-upgrade-from-stable-4.21-e2e-aws-ovn-upgrade",
		variants: map[string]string{
			"Platform": "aws", "Architecture": "amd64", "Network": "ovn",
			"Topology": "ha", "Installer": "ipi", "FeatureSet": "default",
			"Suite": "unknown", "Upgrade": "minor", "LayeredProduct": "none",
		},
	},
	{
		nameTemplate: "periodic-ci-openshift-release-master-ci-%s-e2e-aws-ovn-amd64",
		variants: map[string]string{
			"Platform": "aws", "Architecture": "amd64", "Network": "ovn",
			"Topology": "ha", "Installer": "ipi", "FeatureSet": "default",
			"Suite": "parallel", "Upgrade": "none", "LayeredProduct": "none",
		},
	},
	{
		nameTemplate: "periodic-ci-openshift-release-master-ci-%s-e2e-aws-ovn-arm64",
		variants: map[string]string{
			"Platform": "aws", "Architecture": "arm64", "Network": "ovn",
			"Topology": "ha", "Installer": "ipi", "FeatureSet": "default",
			"Suite": "parallel", "Upgrade": "none", "LayeredProduct": "none",
		},
	},
	{
		nameTemplate: "periodic-ci-openshift-release-master-ci-%s-e2e-aws-ovn-techpreview-serial",
		variants: map[string]string{
			"Platform": "aws", "Architecture": "amd64", "Network": "ovn",
			"Topology": "ha", "Installer": "ipi", "FeatureSet": "techpreview",
			"Suite": "serial", "Upgrade": "none", "LayeredProduct": "none",
		},
	},
	{
		nameTemplate: "periodic-ci-openshift-release-master-ci-%s-e2e-gcp-ovn-amd64",
		variants: map[string]string{
			"Platform": "gcp", "Architecture": "amd64", "Network": "ovn",
			"Topology": "ha", "Installer": "ipi", "FeatureSet": "default",
			"Suite": "parallel", "Upgrade": "none", "LayeredProduct": "none",
		},
	},
	{
		nameTemplate: "periodic-ci-openshift-release-master-ci-%s-e2e-gcp-ovn-upgrade-micro",
		variants: map[string]string{
			"Platform": "gcp", "Architecture": "amd64", "Network": "ovn",
			"Topology": "ha", "Installer": "ipi", "FeatureSet": "default",
			"Suite": "unknown", "Upgrade": "micro", "LayeredProduct": "none",
		},
	},
}

// Job template constants for referencing specific jobs in test specs.
const awsAmd64Parallel = "periodic-ci-openshift-release-master-ci-%s-e2e-aws-ovn-amd64"
const awsArm64Parallel = "periodic-ci-openshift-release-master-ci-%s-e2e-aws-ovn-arm64"
const gcpAmd64Parallel = "periodic-ci-openshift-release-master-ci-%s-e2e-gcp-ovn-amd64"

// allJobTemplates returns name templates from syntheticJobs for use in test specs
// that should run on every job (e.g. install tests).
func allJobTemplates() []string {
	templates := make([]string, len(syntheticJobs))
	for i, j := range syntheticJobs {
		templates[i] = j.nameTemplate
	}
	return templates
}

// allJobCounts builds a jobCounts map that assigns the given per-release counts
// to every synthetic job. Used for tests like install indicators that run everywhere.
func allJobCounts(releaseCounts map[string]testCount) map[string]map[string]testCount {
	result := make(map[string]map[string]testCount, len(syntheticJobs))
	for _, tpl := range allJobTemplates() {
		result[tpl] = releaseCounts
	}
	return result
}

var syntheticTests = []syntheticTestSpec{
	// --- NotSignificant: appears in 3 jobs across 2 platforms ---
	{
		testID: "test-not-significant", testName: "[sig-arch] Check build pods use all cpu cores",
		component: "comp-NotSignificant", capabilities: []string{"cap1"},
		jobCounts: map[string]map[string]testCount{
			awsAmd64Parallel: {"4.21": {100, 95, 0}, "4.22": {100, 93, 0}},
			awsArm64Parallel: {"4.21": {80, 76, 0}, "4.22": {80, 75, 0}},
			gcpAmd64Parallel: {"4.21": {100, 97, 0}, "4.22": {100, 95, 0}},
		},
	},

	// --- SignificantRegression: regressed on aws/amd64, fine elsewhere ---
	{
		testID: "test-significant-regression", testName: "[sig-network] Services should serve endpoints on same port and different protocol",
		component: "comp-SignificantRegression", capabilities: []string{"cap1"},
		jobCounts: map[string]map[string]testCount{
			awsAmd64Parallel: {"4.21": {200, 190, 0}, "4.22": {200, 170, 0}},
			awsArm64Parallel: {"4.21": {180, 171, 0}, "4.22": {180, 168, 0}},
			gcpAmd64Parallel: {"4.21": {200, 190, 0}, "4.22": {200, 188, 0}},
		},
	},

	// --- ExtremeRegression: extreme on aws/amd64, significant on others ---
	{
		testID: "test-extreme-regression", testName: "[sig-etcd] etcd leader changes are not excessive",
		component: "comp-ExtremeRegression", capabilities: []string{"cap1"},
		jobCounts: map[string]map[string]testCount{
			awsAmd64Parallel: {"4.21": {200, 190, 0}, "4.22": {200, 140, 0}},
			awsArm64Parallel: {"4.21": {200, 190, 0}, "4.22": {200, 170, 0}},
			gcpAmd64Parallel: {"4.21": {200, 190, 0}, "4.22": {200, 170, 0}},
		},
	},

	// --- MissingSample: test in base, 0 sample runs ---
	{
		testID: "test-missing-sample", testName: "[sig-storage] CSI volumes should be mountable",
		component: "comp-MissingSample", capabilities: []string{"cap1"},
		jobCounts: map[string]map[string]testCount{
			awsAmd64Parallel: {"4.21": {100, 95, 0}, "4.22": {0, 0, 0}},
		},
	},

	// --- MissingBasis: test only in sample ---
	{
		testID: "test-missing-basis", testName: "[sig-node] New pod lifecycle test",
		component: "comp-MissingBasis", capabilities: []string{"cap1"},
		jobCounts: map[string]map[string]testCount{
			awsAmd64Parallel: {"4.22": {100, 95, 0}},
		},
	},

	// --- NewTestPassRateRegression: new test only in sample, below PassRateRequiredNewTests threshold ---
	{
		testID: "test-new-test-pass-rate-fail", testName: "[sig-node] New flaky pod readiness test",
		component: "comp-NewTestPassRate", capabilities: []string{"cap1"},
		jobCounts: map[string]map[string]testCount{
			awsAmd64Parallel: {"4.22": {100, 70, 0}},
		},
	},

	// --- BasisOnly: test in base, absent from sample ---
	{
		testID: "test-basis-only", testName: "[sig-apps] Removed deployment test",
		component: "comp-BasisOnly", capabilities: []string{"cap1"},
		jobCounts: map[string]map[string]testCount{
			awsAmd64Parallel: {"4.21": {100, 95, 0}},
		},
	},

	// --- SignificantImprovement: 80% -> 95% ---
	{
		testID: "test-significant-improvement", testName: "[sig-cli] oc adm should handle upgrades gracefully",
		component: "comp-SignificantImprovement", capabilities: []string{"cap1"},
		jobCounts: map[string]map[string]testCount{
			awsAmd64Parallel: {"4.21": {200, 160, 0}, "4.22": {200, 190, 0}},
		},
	},

	// --- BelowMinFailure: only 2 failures, below MinimumFailure=3 ---
	{
		testID: "test-below-min-failure", testName: "[sig-auth] RBAC should allow access with valid token",
		component: "comp-BelowMinFailure", capabilities: []string{"cap1"},
		jobCounts: map[string]map[string]testCount{
			awsAmd64Parallel: {"4.21": {100, 100, 0}, "4.22": {100, 98, 0}},
		},
	},

	// --- Fallback: 4.21 worse, 4.20 better -> swaps to 4.20 ---
	{
		testID: "test-fallback-improves", testName: "[sig-instrumentation] Metrics should report accurate cpu usage",
		component: "comp-FallbackImproves", capabilities: []string{"cap1"},
		jobCounts: map[string]map[string]testCount{
			awsAmd64Parallel: {
				"4.21": {200, 180, 0},
				"4.20": {200, 194, 0},
				"4.22": {200, 160, 0},
			},
		},
	},

	// --- Double fallback: 4.21->4.20->4.19 ---
	{
		testID: "test-fallback-double", testName: "[sig-scheduling] Scheduler should spread pods evenly",
		component: "comp-FallbackDouble", capabilities: []string{"cap1"},
		jobCounts: map[string]map[string]testCount{
			awsAmd64Parallel: {
				"4.21": {200, 180, 0},
				"4.20": {200, 186, 0},
				"4.19": {200, 194, 0},
				"4.22": {200, 160, 0},
			},
		},
	},

	// --- Fallback insufficient runs: 4.20 has <60% of 4.21 count ---
	{
		testID: "test-fallback-insufficient-runs", testName: "[sig-network] DNS should resolve cluster services",
		component: "comp-FallbackInsufficient", capabilities: []string{"cap1"},
		jobCounts: map[string]map[string]testCount{
			awsAmd64Parallel: {
				"4.21": {1000, 940, 0},
				"4.20": {100, 99, 0},
				"4.22": {1000, 850, 0},
			},
		},
	},

	// --- Install / health indicator tests: run on every job, every release ---
	{
		testID: "test-install-overall", testName: "install should succeed: overall",
		component: "comp-Install", capabilities: []string{"install"},
		jobCounts: allJobCounts(map[string]testCount{
			"4.22": {100, 95, 0}, "4.21": {100, 96, 0}, "4.20": {100, 97, 0}, "4.19": {100, 97, 0},
		}),
	},
	{
		testID: "test-install-config", testName: "install should succeed: configuration",
		component: "comp-Install", capabilities: []string{"install"},
		jobCounts: allJobCounts(map[string]testCount{
			"4.22": {100, 97, 0}, "4.21": {100, 98, 0}, "4.20": {100, 98, 0}, "4.19": {100, 98, 0},
		}),
	},
	{
		testID: "test-install-bootstrap", testName: "install should succeed: cluster bootstrap",
		component: "comp-Install", capabilities: []string{"install"},
		jobCounts: allJobCounts(map[string]testCount{
			"4.22": {100, 96, 0}, "4.21": {100, 97, 0}, "4.20": {100, 97, 0}, "4.19": {100, 97, 0},
		}),
	},
	{
		testID: "test-install-other", testName: "install should succeed: other",
		component: "comp-Install", capabilities: []string{"install"},
		jobCounts: allJobCounts(map[string]testCount{
			"4.22": {100, 98, 0}, "4.21": {100, 99, 0}, "4.20": {100, 99, 0}, "4.19": {100, 99, 0},
		}),
	},
	{
		testID: "test-install-infra", testName: "install should succeed: infrastructure",
		component: "comp-Install", capabilities: []string{"install"},
		jobCounts: allJobCounts(map[string]testCount{
			"4.22": {100, 96, 0}, "4.21": {100, 97, 0}, "4.20": {100, 97, 0}, "4.19": {100, 97, 0},
		}),
	},
	{
		testID: "test-upgrade", testName: "[sig-sippy] upgrade should work",
		component: "comp-Install", capabilities: []string{"upgrade"},
		jobCounts: allJobCounts(map[string]testCount{
			"4.22": {100, 94, 0}, "4.21": {100, 95, 0}, "4.20": {100, 96, 0}, "4.19": {100, 96, 0},
		}),
	},
	{
		testID: "test-openshift-tests", testName: "[sig-sippy] openshift-tests should work",
		component: "comp-Install", capabilities: []string{"tests"},
		jobCounts: allJobCounts(map[string]testCount{
			"4.22": {100, 90, 0}, "4.21": {100, 92, 0}, "4.20": {100, 93, 0}, "4.19": {100, 93, 0},
		}),
	},
}

// releaseTimeWindow returns the start/end times for a release's test data.
func releaseTimeWindow(release string) (start, end time.Time) {
	now := time.Now().UTC().Truncate(time.Hour)
	switch release {
	case "4.22":
		return now.Add(-3 * 24 * time.Hour), now
	case "4.21":
		return now.Add(-60 * 24 * time.Hour), now.Add(-30 * 24 * time.Hour)
	case "4.20":
		return now.Add(-120 * 24 * time.Hour), now.Add(-90 * 24 * time.Hour)
	case "4.19":
		return now.Add(-180 * 24 * time.Hour), now.Add(-150 * 24 * time.Hour)
	default:
		return now.Add(-14 * 24 * time.Hour), now
	}
}

func seedSyntheticData(dbc *db.DB) error {
	// Check if data already exists
	var count int64
	if err := dbc.DB.Model(&models.ProwJob{}).Count(&count).Error; err != nil {
		return fmt.Errorf("failed to check for existing data: %w", err)
	}
	if count > 0 {
		log.Infof("Database already contains %d ProwJobs, skipping seed. Use --init-database to reset.", count)
		return nil
	}

	if err := createTestSuite(dbc, "synthetic"); err != nil {
		return errors.WithMessage(err, "failed to create test suite")
	}
	log.Info("Created test suite 'synthetic'")

	if err := seedProwJobs(dbc); err != nil {
		return err
	}

	if err := seedTestsAndOwnerships(dbc); err != nil {
		return err
	}

	totalRuns, totalResults, err := seedJobRunsAndResults(dbc)
	if err != nil {
		return err
	}

	if err := createLabelsAndSymptoms(dbc); err != nil {
		return errors.WithMessage(err, "failed to create labels and symptoms")
	}

	if err := writeSyntheticViewsFile(); err != nil {
		return errors.WithMessage(err, "failed to write views file")
	}

	log.Info("Refreshing materialized views...")
	sippyserver.RefreshData(dbc, nil, false)

	log.Info("Syncing regressions...")
	if err := syncRegressions(dbc); err != nil {
		return errors.WithMessage(err, "failed to sync regressions")
	}

	log.Infof("Seeded synthetic data: %d ProwJobRuns, %d test results across %d releases",
		totalRuns, totalResults, len(syntheticReleases))
	return nil
}

func seedProwJobs(dbc *db.DB) error {
	for _, release := range syntheticReleases {
		for _, job := range syntheticJobs {
			name := fmt.Sprintf(job.nameTemplate, release)
			variants := variantMapToArray(job.variants)
			prowJob := models.ProwJob{
				Kind:     models.ProwKind("periodic"),
				Name:     name,
				Release:  release,
				Variants: variants,
			}
			var existing models.ProwJob
			if err := dbc.DB.Where("name = ?", name).FirstOrCreate(&existing, prowJob).Error; err != nil {
				return fmt.Errorf("failed to create ProwJob %s: %w", name, err)
			}
		}
	}
	log.Infof("Created ProwJobs for %d releases x %d jobs", len(syntheticReleases), len(syntheticJobs))
	return nil
}

type testInfo struct {
	name         string
	uniqueID     string
	component    string
	capabilities []string
}

func seedTestsAndOwnerships(dbc *db.DB) error {
	var suite models.Suite
	if err := dbc.DB.Where("name = ?", "synthetic").First(&suite).Error; err != nil {
		return fmt.Errorf("failed to find suite: %w", err)
	}

	seenTests := map[string]testInfo{}
	for _, ts := range syntheticTests {
		if _, ok := seenTests[ts.testName]; !ok {
			seenTests[ts.testName] = testInfo{
				name:         ts.testName,
				uniqueID:     ts.testID,
				component:    ts.component,
				capabilities: ts.capabilities,
			}
		}
	}

	for _, info := range seenTests {
		testModel := models.Test{Name: info.name}
		var existingTest models.Test
		if err := dbc.DB.Where("name = ?", info.name).FirstOrCreate(&existingTest, testModel).Error; err != nil {
			return fmt.Errorf("failed to create Test %s: %w", info.name, err)
		}

		ownership := models.TestOwnership{
			UniqueID:     info.uniqueID,
			Name:         info.name,
			TestID:       existingTest.ID,
			Suite:        "synthetic",
			SuiteID:      &suite.ID,
			Component:    info.component,
			Capabilities: info.capabilities,
		}
		var existingOwnership models.TestOwnership
		if err := dbc.DB.Where("name = ? AND suite = ?", info.name, "synthetic").FirstOrCreate(&existingOwnership, ownership).Error; err != nil {
			return fmt.Errorf("failed to create TestOwnership for %s: %w", info.name, err)
		}
	}
	log.Infof("Created %d tests with ownership records", len(seenTests))
	return nil
}

type jobReleaseKey struct {
	jobTemplate string
	release     string
}

func seedJobRunsAndResults(dbc *db.DB) (int, int, error) {
	var suite models.Suite
	if err := dbc.DB.Where("name = ?", "synthetic").First(&suite).Error; err != nil {
		return 0, 0, fmt.Errorf("failed to find suite: %w", err)
	}

	maxRuns := map[jobReleaseKey]int{}
	for _, ts := range syntheticTests {
		for jobTpl, releaseCounts := range ts.jobCounts {
			for release, counts := range releaseCounts {
				key := jobReleaseKey{jobTpl, release}
				if counts.total > maxRuns[key] {
					maxRuns[key] = counts.total
				}
			}
		}
	}

	testIDsByName := map[string]uint{}
	var allTests []models.Test
	if err := dbc.DB.Find(&allTests).Error; err != nil {
		return 0, 0, fmt.Errorf("failed to fetch tests: %w", err)
	}
	for _, t := range allTests {
		testIDsByName[t.Name] = t.ID
	}

	totalRuns := 0
	totalResults := 0
	for jrKey, runCount := range maxRuns {
		if runCount == 0 {
			continue
		}

		jobName := fmt.Sprintf(jrKey.jobTemplate, jrKey.release)
		var prowJob models.ProwJob
		if err := dbc.DB.Where("name = ?", jobName).First(&prowJob).Error; err != nil {
			return 0, 0, fmt.Errorf("failed to find ProwJob %s: %w", jobName, err)
		}

		runs, results, err := seedRunsForJob(dbc, &suite, prowJob, jrKey, runCount, testIDsByName)
		if err != nil {
			return 0, 0, err
		}
		totalRuns += runs
		totalResults += results

		log.Debugf("Created %d runs for %s", runCount, jobName)
	}

	return totalRuns, totalResults, nil
}

func seedRunsForJob(dbc *db.DB, suite *models.Suite, prowJob models.ProwJob, jrKey jobReleaseKey, runCount int, testIDsByName map[string]uint) (int, int, error) {
	start, end := releaseTimeWindow(jrKey.release)
	window := end.Sub(start)
	interval := window / time.Duration(runCount)

	runIDs := make([]uint, runCount)
	for i := range runCount {
		timestamp := start.Add(time.Duration(i) * interval)
		run := models.ProwJobRun{
			ProwJobID: prowJob.ID,
			Cluster:   "build01",
			Timestamp: timestamp,
			Duration:  3 * time.Hour,
		}
		if err := dbc.DB.Create(&run).Error; err != nil {
			return 0, 0, fmt.Errorf("failed to create ProwJobRun: %w", err)
		}
		runIDs[i] = run.ID
	}

	// Runs that get test results (all except the last 2)
	testableRuns := runCount
	if testableRuns > 2 {
		testableRuns = runCount - 2
	}

	runsWithFailure := map[uint]bool{}
	totalResults := 0

	for _, ts := range syntheticTests {
		releaseCounts, hasJob := ts.jobCounts[jrKey.jobTemplate]
		if !hasJob {
			continue
		}
		counts, hasRelease := releaseCounts[jrKey.release]
		if !hasRelease || counts.total == 0 {
			continue
		}

		testID, ok := testIDsByName[ts.testName]
		if !ok {
			return 0, 0, fmt.Errorf("test %q not found in DB", ts.testName)
		}

		for i := 0; i < counts.total && i < testableRuns; i++ {
			var status int
			switch {
			case i < counts.success-counts.flake:
				status = 1 // pass
			case i < counts.success:
				status = 13 // flake (counts as success too)
			default:
				status = 12 // failure
				runsWithFailure[runIDs[i]] = true
			}

			result := models.ProwJobRunTest{
				ProwJobRunID: runIDs[i],
				TestID:       testID,
				SuiteID:      &suite.ID,
				Status:       status,
				Duration:     5.0,
				CreatedAt:    start.Add(time.Duration(i) * interval),
			}
			if err := dbc.DB.Create(&result).Error; err != nil {
				return 0, 0, fmt.Errorf("failed to create ProwJobRunTest: %w", err)
			}
			totalResults++
		}
	}

	// Set OverallResult on all runs
	for i, runID := range runIDs {
		var overallResult v1.JobOverallResult
		var succeeded, failed bool

		if i >= testableRuns {
			overallResult = v1.JobInternalInfrastructureFailure
			failed = true
		} else if runsWithFailure[runID] {
			overallResult = v1.JobTestFailure
			failed = true
		} else {
			overallResult = v1.JobSucceeded
			succeeded = true
		}

		if err := dbc.DB.Model(&models.ProwJobRun{}).Where("id = ?", runID).
			Updates(map[string]any{
				"overall_result": overallResult,
				"succeeded":      succeeded,
				"failed":         failed,
			}).Error; err != nil {
			return 0, 0, fmt.Errorf("failed to update ProwJobRun result: %w", err)
		}
	}

	// Update test_failures count
	if err := dbc.DB.Exec(`
		UPDATE prow_job_runs SET test_failures = COALESCE((
			SELECT COUNT(*) FROM prow_job_run_tests
			WHERE prow_job_run_id = prow_job_runs.id AND status = 12
		), 0) WHERE prow_job_id = ?`, prowJob.ID).Error; err != nil {
		log.WithError(err).Warn("failed to update test_failures count")
	}

	return runCount, totalResults, nil
}

func syncRegressions(dbc *db.DB) error {
	provider := pgprovider.NewPostgresProvider(dbc, nil)
	ctx := context.Background()

	releases, err := provider.QueryReleases(ctx)
	if err != nil {
		return fmt.Errorf("querying releases: %w", err)
	}

	viewsData, err := os.ReadFile(syntheticViewsFile)
	if err != nil {
		return fmt.Errorf("reading views file: %w", err)
	}
	var views apitype.SippyViews
	if err := yaml.Unmarshal(viewsData, &views); err != nil {
		return fmt.Errorf("parsing views file: %w", err)
	}

	tracker := componentreadiness.NewRegressionTracker(
		provider, dbc,
		cache.RequestOptions{},
		releases,
		componentreadiness.NewPostgresRegressionStore(dbc, nil),
		views.ComponentReadiness,
		false,
	)
	tracker.Load()
	if len(tracker.Errors()) > 0 {
		for _, err := range tracker.Errors() {
			log.WithError(err).Warn("regression tracker error")
		}
		return fmt.Errorf("regression tracker encountered %d errors", len(tracker.Errors()))
	}
	return nil
}

const syntheticViewsFile = "config/e2e-views.yaml"

// writeSyntheticViewsFile generates a views file with include_variants matching the seed data.
func writeSyntheticViewsFile() error {
	// Collect all unique variant values from synthetic jobs
	allVariants := map[string]map[string]bool{}
	for _, job := range syntheticJobs {
		for k, v := range job.variants {
			if allVariants[k] == nil {
				allVariants[k] = map[string]bool{}
			}
			allVariants[k][v] = true
		}
	}

	includeVariants := map[string][]string{}
	for k, vals := range allVariants {
		sorted := make([]string, 0, len(vals))
		for v := range vals {
			sorted = append(sorted, v)
		}
		sort.Strings(sorted)
		includeVariants[k] = sorted
	}

	dbGroupBy := sets.NewString("Architecture", "FeatureSet", "Installer", "Network", "Platform",
		"Suite", "Topology", "Upgrade", "LayeredProduct")
	columnGroupBy := sets.NewString("Network", "Platform", "Topology")

	views := apitype.SippyViews{
		ComponentReadiness: []crview.View{
			{
				Name: "4.22-main",
				BaseRelease: reqopts.RelativeRelease{
					Release:       reqopts.Release{Name: "4.21"},
					RelativeStart: "now-60d",
					RelativeEnd:   "now-30d",
				},
				SampleRelease: reqopts.RelativeRelease{
					Release:       reqopts.Release{Name: "4.22"},
					RelativeStart: "now-3d",
					RelativeEnd:   "now",
				},
				VariantOptions: reqopts.Variants{
					ColumnGroupBy:   columnGroupBy,
					DBGroupBy:       dbGroupBy,
					IncludeVariants: includeVariants,
				},
				AdvancedOptions: reqopts.Advanced{
					Confidence:                  95,
					PityFactor:                  5,
					MinimumFailure:              3,
					PassRateRequiredNewTests:    90,
					IncludeMultiReleaseAnalysis: true,
				},
				PrimeCache:         crview.PrimeCache{Enabled: true},
				RegressionTracking: crview.RegressionTracking{Enabled: true},
			},
		},
	}

	data, err := yaml.Marshal(views)
	if err != nil {
		return fmt.Errorf("marshaling views: %w", err)
	}

	if err := os.WriteFile(syntheticViewsFile, data, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", syntheticViewsFile, err)
	}

	log.Infof("Generated views file: %s", syntheticViewsFile)
	return nil
}

// variantMapToArray converts a variant map to a pq.StringArray.
func variantMapToArray(m map[string]string) pq.StringArray {
	result := make([]string, 0, len(m))
	for k, v := range m {
		result = append(result, k+":"+v)
	}
	return result
}

func createTestSuite(dbc *db.DB, name string) error {
	suite := models.Suite{
		Name: name,
	}

	var existingSuite models.Suite
	if err := dbc.DB.Where("name = ?", suite.Name).FirstOrCreate(&existingSuite, suite).Error; err != nil {
		return fmt.Errorf("failed to create or find Suite %s: %v", suite.Name, err)
	}

	return nil
}

func createLabelsAndSymptoms(dbc *db.DB) error {
	metadata := jobrunscan.Metadata{
		CreatedBy: "seed-data",
		UpdatedBy: "seed-data",
	}

	labels := []jobrunscan.Label{
		{
			LabelContent: jobrunscan.LabelContent{
				ID:          "InfraFailure",
				LabelTitle:  "Infrastructure failure: omit job from CR",
				Explanation: "Job failed due to **infrastructure issues** not related to product code.",
			},
			Metadata: metadata,
		},
		{
			LabelContent: jobrunscan.LabelContent{
				ID:          "ClusterDNSFlake",
				LabelTitle:  "Cluster DNS resolution failure(s)",
				Explanation: "Job experienced DNS resolution timeouts in the cluster.",
			},
			Metadata: metadata,
		},
		{
			LabelContent: jobrunscan.LabelContent{
				ID:          "ClusterInstallTimeout",
				LabelTitle:  "Cluster install timeout",
				Explanation: "Cluster installation exceeded timeout threshold.",
			},
			Metadata: metadata,
		},
		{
			LabelContent: jobrunscan.LabelContent{
				ID:          "IntervalFile",
				LabelTitle:  "Has interval file(s)",
				Explanation: "Job produced interval monitoring files.",
			},
			HideDisplayContexts: []string{jobrunscan.MetricsContext, jobrunscan.JAQOptsContext},
			Metadata:            metadata,
		},
		{
			LabelContent: jobrunscan.LabelContent{
				ID:          "APIServerTimeout",
				LabelTitle:  "API server timeout",
				Explanation: "Requests to the API server timed out.",
			},
			Metadata: metadata,
		},
	}

	for _, label := range labels {
		var existing jobrunscan.Label
		if err := dbc.DB.Where("id = ?", label.ID).FirstOrCreate(&existing, label).Error; err != nil {
			return fmt.Errorf("failed to create or find label %s: %v", label.ID, err)
		}
	}

	symptoms := []jobrunscan.Symptom{
		{
			SymptomContent: jobrunscan.SymptomContent{
				ID:          "DNSTimeoutSymptom",
				Summary:     "Cluster DNS resolution failures detected",
				MatcherType: jobrunscan.MatcherTypeString,
				FilePattern: "**/e2e-timelines/**/*.json",
				MatchString: "dial tcp",
				LabelIDs:    []string{"ClusterDNSFlake"},
			},
			Metadata: metadata,
		},
		{
			SymptomContent: jobrunscan.SymptomContent{
				ID:          "InstallTimeoutSymptom",
				Summary:     "Cluster install timeout detected",
				MatcherType: jobrunscan.MatcherTypeRegex,
				FilePattern: "**/build-log.txt",
				MatchString: "timeout waiting for.*install",
				LabelIDs:    []string{"ClusterInstallTimeout"},
			},
			Metadata: metadata,
		},
		{
			SymptomContent: jobrunscan.SymptomContent{
				ID:          "HasIntervalsSymptom",
				Summary:     "Has interval file(s)",
				MatcherType: jobrunscan.MatcherTypeFile,
				FilePattern: "**/intervals*.json",
				MatchString: "",
				LabelIDs:    []string{"IntervalFile"},
			},
			Metadata: metadata,
		},
		{
			SymptomContent: jobrunscan.SymptomContent{
				ID:          "APITimeoutSymptom",
				Summary:     "API server timeouts detected",
				MatcherType: jobrunscan.MatcherTypeString,
				FilePattern: "**/build-log.txt",
				MatchString: "context deadline exceeded",
				LabelIDs:    []string{"APIServerTimeout"},
			},
			Metadata: metadata,
		},
	}

	for _, symptom := range symptoms {
		var existing jobrunscan.Symptom
		if err := dbc.DB.Where("id = ?", symptom.ID).FirstOrCreate(&existing, symptom).Error; err != nil {
			return fmt.Errorf("failed to create or find symptom %s: %v", symptom.ID, err)
		}
	}

	return nil
}

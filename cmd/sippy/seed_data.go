package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/models/jobrunscan"
	"github.com/openshift/sippy/pkg/flags"
	"github.com/openshift/sippy/pkg/sippyserver"
)

type SeedDataFlags struct {
	DBFlags        *flags.PostgresFlags
	InitDatabase   bool
	Releases       []string
	JobsPerRelease int
	TestNames      []string
	RunsPerJob     int
}

func NewSeedDataFlags() *SeedDataFlags {
	return &SeedDataFlags{
		DBFlags:        flags.NewPostgresDatabaseFlags(),
		Releases:       []string{"4.20", "4.19"}, // Default releases
		JobsPerRelease: 3,                        // Default jobs per release
		TestNames: []string{
			"install should succeed: infrastructure",
			"install should succeed: overall",
			"install should succeed: configuration",
			"install should succeed: cluster bootstrap",
			"install should succeed: other",
			"[sig-cluster-lifecycle] Cluster completes upgrade",
			"[sig-sippy] upgrade should work",
			"[sig-sippy] openshift-tests should work",
		},
		RunsPerJob: 20, // Default runs per job
	}
}

func (f *SeedDataFlags) BindFlags(fs *pflag.FlagSet) {
	f.DBFlags.BindFlags(fs)
	fs.BoolVar(&f.InitDatabase, "init-database", false, "Initialize the DB schema before seeding data")
	fs.StringSliceVar(&f.Releases, "release", f.Releases, "Releases to create ProwJobs for (can be specified multiple times)")
	fs.IntVar(&f.JobsPerRelease, "jobs", f.JobsPerRelease, "Number of ProwJobs to create for each release")
	fs.StringSliceVar(&f.TestNames, "test", f.TestNames, "Test names to create (can be specified multiple times)")
	fs.IntVar(&f.RunsPerJob, "runs", f.RunsPerJob, "Number of ProwJobRuns to create for each ProwJob")
}

func NewSeedDataCommand() *cobra.Command {
	f := NewSeedDataFlags()

	cmd := &cobra.Command{
		Use:   "seed-data",
		Short: "Populate test data in the database",
		Long: `Populate test data in the database for development purposes.
This command creates sample ProwJob and Test records with realistic test data
that can be used for local development and testing.

Test results are randomized with 85% pass rate, 10% flake rate, and 5% failure rate.
All counts, releases, and test names are configurable via command-line flags.

The command can be re-run as needed to add more runs, or because your old job runs 
rolled off the 1 week window.
`,
		RunE: func(cmd *cobra.Command, args []string) error {
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

			// Create the test suite
			if err := createTestSuite(dbc); err != nil {
				return errors.WithMessage(err, "failed to create test suite")
			}
			log.Info("Created test suite 'ourtests'")

			// Create ProwJobs for each release
			for _, release := range f.Releases {
				if err := createProwJobsForRelease(dbc, release, f.JobsPerRelease); err != nil {
					return errors.WithMessagef(err, "failed to create ProwJobs for release %s", release)
				}
				log.Infof("Processed %d ProwJobs for release %s", f.JobsPerRelease, release)
			}

			// Create Test models
			if err := createTestModels(dbc, f.TestNames); err != nil {
				return errors.WithMessage(err, "failed to create Test models")
			}
			log.Infof("Processed %d Test models", len(f.TestNames))

			// Create labels and symptoms
			if err := createLabelsAndSymptoms(dbc); err != nil {
				return errors.WithMessage(err, "failed to create labels and symptoms")
			}
			log.Info("Created sample labels and symptoms")

			// Create ProwJobRuns for each ProwJob
			if err := createProwJobRuns(dbc, f.RunsPerJob); err != nil {
				return errors.WithMessage(err, "failed to create ProwJobRuns")
			}
			log.Info("Created ProwJobRuns and test results for all ProwJobs")

			// Apply labels to job runs
			if err := applyLabelsToJobRuns(dbc); err != nil {
				return errors.WithMessage(err, "failed to apply labels to job runs")
			}
			log.Info("Applied labels to ~25% of job runs")

			totalProwJobs := len(f.Releases) * f.JobsPerRelease
			totalRuns := totalProwJobs * f.RunsPerJob
			totalTestResults := totalRuns * len(f.TestNames)

			log.Info("Refreshing materialized views...")
			sippyserver.RefreshData(dbc, nil, false)

			log.Infof("Successfully seeded test data! Created %d ProwJobs, %d Tests, %d ProwJobRuns, and %d test results",
				totalProwJobs, len(f.TestNames), totalRuns, totalTestResults)
			return nil
		},
	}

	f.BindFlags(cmd.Flags())

	return cmd
}

func createProwJobsForRelease(dbc *db.DB, release string, jobsPerRelease int) error {
	for i := 1; i <= jobsPerRelease; i++ {
		// Choose JobTier based on whether i is even or odd
		var jobTier = "JobTier:standard" // even number job index = standard
		if i%2 != 0 {
			jobTier = "JobTier:hidden" // odd = hidden
		}

		prowJob := models.ProwJob{
			Kind:    models.ProwKind("periodic"),
			Name:    fmt.Sprintf("sippy-test-job-%s-test-%d", release, i),
			Release: release,
			// TestGridURL, Bugs, and JobRuns are left empty as requested
			Variants: []string{"Platform:aws", "Upgrade:none", jobTier},
		}

		// Use FirstOrCreate to avoid duplicates - only creates if a ProwJob with this name doesn't exist
		var existingJob models.ProwJob
		if err := dbc.DB.Where("name = ?", prowJob.Name).FirstOrCreate(&existingJob, prowJob).Error; err != nil {
			return fmt.Errorf("failed to create or find ProwJob %s: %v", prowJob.Name, err)
		}

		// Log whether we created a new job or found an existing one
		if existingJob.CreatedAt.IsZero() || existingJob.CreatedAt.Equal(existingJob.UpdatedAt) {
			log.Debugf("Created new ProwJob: %s", prowJob.Name)
		} else {
			log.Debugf("ProwJob already exists: %s", prowJob.Name)
		}
	}

	return nil
}

func createTestModels(dbc *db.DB, testNames []string) error {
	for _, testName := range testNames {
		testModel := models.Test{
			Name: testName,
		}

		// Use FirstOrCreate to avoid duplicates - only creates if a Test with this name doesn't exist
		var existingTest models.Test
		if err := dbc.DB.Where("name = ?", testModel.Name).FirstOrCreate(&existingTest, testModel).Error; err != nil {
			return fmt.Errorf("failed to create or find Test %s: %v", testModel.Name, err)
		}

		if existingTest.CreatedAt.IsZero() || existingTest.CreatedAt.Equal(existingTest.UpdatedAt) {
			log.Debugf("Created new Test: %s", testModel.Name)
		} else {
			log.Debugf("Test already exists: %s", testModel.Name)
		}
	}

	return nil
}

func createTestSuite(dbc *db.DB) error {
	suite := models.Suite{
		Name: "ourtests",
	}

	// Use FirstOrCreate to avoid duplicates
	var existingSuite models.Suite
	if err := dbc.DB.Where("name = ?", suite.Name).FirstOrCreate(&existingSuite, suite).Error; err != nil {
		return fmt.Errorf("failed to create or find Suite %s: %v", suite.Name, err)
	}

	return nil
}

func createProwJobRuns(dbc *db.DB, runsPerJob int) error {
	var prowJobs []models.ProwJob
	if err := dbc.DB.Find(&prowJobs).Error; err != nil {
		return fmt.Errorf("failed to fetch existing ProwJobs: %v", err)
	}

	var tests []models.Test
	if err := dbc.DB.Find(&tests).Error; err != nil {
		return fmt.Errorf("failed to fetch existing Tests: %v", err)
	}

	var suite models.Suite
	if err := dbc.DB.Where("name = ?", "ourtests").First(&suite).Error; err != nil {
		return fmt.Errorf("failed to find Suite 'ourtests': %v", err)
	}

	log.Infof("Found %d ProwJobs, creating %d runs for each", len(prowJobs), runsPerJob)

	// Calculate time range: past 2 weeks from now
	now := time.Now()
	twoWeeksAgo := now.AddDate(0, 0, -14)

	// Duration for each run: 3 hours
	runDuration := 3 * time.Hour

	for _, prowJob := range prowJobs {
		log.Infof("Creating %d ProwJobRuns for ProwJob: %s", runsPerJob, prowJob.Name)

		for i := 0; i < runsPerJob; i++ {
			// Log progress every 10 runs to show activity
			if (i+1)%10 == 0 {
				log.Infof("  Progress: %d/%d runs created for %s", i+1, runsPerJob, prowJob.Name)
			}

			// Calculate timestamp: spread evenly over the past 2 weeks
			totalDuration := 14 * 24 * time.Hour
			// Time between runs = total duration / runs
			timeBetweenRuns := totalDuration / time.Duration(runsPerJob)
			timestamp := twoWeeksAgo.Add(time.Duration(i) * timeBetweenRuns)

			prowJobRun := models.ProwJobRun{
				ProwJobID: prowJob.ID,
				Cluster:   "build01",
				Timestamp: timestamp,
				Duration:  runDuration,
				TestCount: len(tests),
			}

			if err := dbc.DB.Create(&prowJobRun).Error; err != nil {
				return fmt.Errorf("failed to create ProwJobRun for ProwJob %s: %v", prowJob.Name, err)
			}

			var testFailures int
			for _, test := range tests {
				// Determine test status based on random chance
				// 5% chance of failure, 10% chance of flake, 85% chance of pass
				// nolint: gosec
				randNum := rand.Float64()
				var status int
				if randNum < 0.05 {
					status = 12 // failure
					testFailures++
				} else if randNum < 0.15 {
					status = 13 // flake
				} else {
					status = 1 // pass
				}

				prowJobRunTest := models.ProwJobRunTest{
					ProwJobRunID: prowJobRun.ID,
					TestID:       test.ID,
					SuiteID:      &suite.ID,
					Status:       status,
					Duration:     5.0, // 5 seconds
					CreatedAt:    timestamp,
				}

				if err := dbc.DB.Create(&prowJobRunTest).Error; err != nil {
					return fmt.Errorf("failed to create ProwJobRunTest for test %s: %v", test.Name, err)
				}
			}

			// Set overall result based on test failures and random factors
			var overallResult v1.JobOverallResult
			if testFailures > 0 {
				prowJobRun.Failed = true
				prowJobRun.Succeeded = false
				prowJobRun.TestFailures = testFailures

				// Randomly assign different failure types
				// nolint: gosec
				failureType := rand.Float64()
				if failureType < 0.7 {
					overallResult = v1.JobTestFailure // 70% test failures
				} else if failureType < 0.85 {
					overallResult = v1.JobUpgradeFailure // 15% upgrade failures
				} else if failureType < 0.92 {
					overallResult = v1.JobInstallFailure // 7% install failures
				} else {
					overallResult = v1.JobInfrastructureFailure // 8% infrastructure failures
				}
			} else {
				prowJobRun.Failed = false
				prowJobRun.Succeeded = true
				prowJobRun.TestFailures = 0
				overallResult = v1.JobSucceeded
			}
			prowJobRun.OverallResult = overallResult

			if err := dbc.DB.Save(&prowJobRun).Error; err != nil {
				return fmt.Errorf("failed to update ProwJobRun for ProwJob %s: %v", prowJob.Name, err)
			}
		}

		log.Infof("Completed creating %d ProwJobRuns for ProwJob: %s", runsPerJob, prowJob.Name)
	}

	return nil
}

func createLabelsAndSymptoms(dbc *db.DB) error {
	// Create sample labels
	labels := []jobrunscan.Label{
		{
			ID:          "InfraFailure",
			LabelTitle:  "Infrastructure failure: omit job from CR",
			Explanation: "Job failed due to **infrastructure issues** not related to product code. See [TRT documentation](https://docs.ci.openshift.org/docs/architecture/ci-operator/) for more details.",
		},
		{
			ID:          "ClusterDNSFlake",
			LabelTitle:  "Cluster DNS resolution failure(s)",
			Explanation: "Job experienced DNS resolution timeouts in the cluster:\n\n- Check for network issues\n- Review DNS server logs\n- Examine cluster network configuration",
		},
		{
			ID:          "ClusterInstallTimeout",
			LabelTitle:  "Cluster install timeout",
			Explanation: "Cluster installation exceeded timeout threshold. This may indicate:\n\n1. Slow infrastructure provisioning\n2. Network connectivity problems\n3. Image pull failures",
		},
		{
			ID:                  "IntervalFile",
			LabelTitle:          "Has interval file(s)",
			Explanation:         "Job produced interval monitoring files. Use the `intervals` tool to analyze timing data.",
			HideDisplayContexts: []string{jobrunscan.MetricsContext, jobrunscan.JAQOptsContext},
		},
		{
			ID:          "APIServerTimeout",
			LabelTitle:  "API server timeout",
			Explanation: "Requests to the API server timed out. Common causes:\n\n- High API server load\n- Network latency issues\n- Slow etcd responses",
		},
	}

	for _, label := range labels {
		var existing jobrunscan.Label
		if err := dbc.DB.Where("id = ?", label.ID).FirstOrCreate(&existing, label).Error; err != nil {
			return fmt.Errorf("failed to create or find label %s: %v", label.ID, err)
		}
		if existing.CreatedAt.IsZero() || existing.CreatedAt.Equal(existing.UpdatedAt) {
			log.Debugf("Created new Label: %s", label.ID)
		} else {
			log.Debugf("Label already exists: %s", label.ID)
		}
	}

	// Create sample symptoms
	symptoms := []jobrunscan.Symptom{
		{
			ID:          "DNSTimeoutSymptom",
			Summary:     "Cluster DNS resolution failures detected",
			MatcherType: jobrunscan.MatcherTypeString,
			FilePattern: "**/e2e-timelines/**/*.json",
			MatchString: "dial tcp",
			LabelIDs:    []string{"ClusterDNSFlake"},
		},
		{
			ID:          "InstallTimeoutSymptom",
			Summary:     "Cluster install timeout detected",
			MatcherType: jobrunscan.MatcherTypeRegex,
			FilePattern: "**/build-log.txt",
			MatchString: "timeout waiting for.*install",
			LabelIDs:    []string{"ClusterInstallTimeout"},
		},
		{
			ID:          "HasIntervalsSymptom",
			Summary:     "Has interval file(s)",
			MatcherType: jobrunscan.MatcherTypeFile,
			FilePattern: "**/intervals*.json",
			MatchString: "",
			LabelIDs:    []string{"IntervalFile"},
		},
		{
			ID:          "APITimeoutSymptom",
			Summary:     "API server timeouts detected",
			MatcherType: jobrunscan.MatcherTypeString,
			FilePattern: "**/build-log.txt",
			MatchString: "context deadline exceeded",
			LabelIDs:    []string{"APIServerTimeout"},
		},
	}

	for _, symptom := range symptoms {
		var existing jobrunscan.Symptom
		if err := dbc.DB.Where("id = ?", symptom.ID).FirstOrCreate(&existing, symptom).Error; err != nil {
			return fmt.Errorf("failed to create or find symptom %s: %v", symptom.ID, err)
		}
		if existing.CreatedAt.IsZero() || existing.CreatedAt.Equal(existing.UpdatedAt) {
			log.Debugf("Created new Symptom: %s", symptom.ID)
		} else {
			log.Debugf("Symptom already exists: %s", symptom.ID)
		}
	}

	return nil
}

func applyLabelsToJobRuns(dbc *db.DB) error {
	// Fetch all job runs
	var jobRuns []models.ProwJobRun
	if err := dbc.DB.Find(&jobRuns).Error; err != nil {
		return fmt.Errorf("failed to fetch job runs: %v", err)
	}

	// Fetch all labels
	var labels []jobrunscan.Label
	if err := dbc.DB.Find(&labels).Error; err != nil {
		return fmt.Errorf("failed to fetch labels: %v", err)
	}

	if len(labels) == 0 {
		log.Warn("No labels found, skipping label application")
		return nil
	}

	labelIDs := make([]string, len(labels))
	for i, label := range labels {
		labelIDs[i] = label.ID
	}

	// Apply labels to approximately 25% of job runs
	labeledCount := 0
	for i := range jobRuns {
		// nolint: gosec // we do not care that the randomness is weak
		if rand.Float64() > 0.25 {
			continue
		}
		// Randomly select 1-3 labels
		// nolint: gosec
		numLabels := rand.Intn(3) + 1
		selectedLabels := make([]string, 0, numLabels)

		// Randomly pick unique labels
		usedIndices := make(map[int]bool)
		for len(selectedLabels) < numLabels && len(selectedLabels) < len(labelIDs) {
			// nolint: gosec
			idx := rand.Intn(len(labelIDs))
			if !usedIndices[idx] {
				selectedLabels = append(selectedLabels, labelIDs[idx])
				usedIndices[idx] = true
			}
		}

		jobRuns[i].Labels = selectedLabels
		if err := dbc.DB.Save(&jobRuns[i]).Error; err != nil {
			return fmt.Errorf("failed to update job run %d with labels: %v", jobRuns[i].ID, err)
		}
		labeledCount++
	}

	log.Infof("Applied labels to %d of %d job runs (%.1f%%)",
		labeledCount, len(jobRuns), float64(labeledCount)/float64(len(jobRuns))*100)

	return nil
}

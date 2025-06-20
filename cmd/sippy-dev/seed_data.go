package main

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/flags"
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
	fs.BoolVar(&f.InitDatabase, "init-database", false, "Migrate the DB before seeding data")
}

func NewSeedDataCommand() *cobra.Command {
	f := NewSeedDataFlags()

	cmd := &cobra.Command{
		Use:   "seed-data",
		Short: "Populate test data in the database",
		Long: `Populate test data in the database for development purposes.
This command creates sample ProwJob and Test records with realistic test data
that can be used for local development and testing.

Creates:
- 15 ProwJob records (5 each for releases 4.20, 4.19, 4.18)
- 20 Test records (test01 through test20)
- 1500 ProwJobRun records (100 for each ProwJob, spread over past 2 weeks)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dbc, err := f.DBFlags.GetDBClient()
			if err != nil {
				return errors.WithMessage(err, "could not connect to database")
			}

			// Initialize database schema if requested
			if f.InitDatabase {
				log.Info("Initializing database schema...")
				t := f.DBFlags.GetPinnedTime()
				if err := dbc.UpdateSchema(t); err != nil {
					return errors.WithMessage(err, "could not migrate database")
				}
				log.Info("Database schema initialized successfully")
			}

			log.Info("Starting to seed test data...")

			// Create ProwJobs for each release
			releases := []string{"4.20", "4.19", "4.18"}

			for _, release := range releases {
				if err := createProwJobsForRelease(dbc, release); err != nil {
					return errors.WithMessagef(err, "failed to create ProwJobs for release %s", release)
				}
				log.Infof("Processed 5 ProwJobs for release %s", release)
			}

			// Create Test models
			if err := createTestModels(dbc); err != nil {
				return errors.WithMessage(err, "failed to create Test models")
			}
			log.Info("Processed 20 Test models")

			// Create ProwJobRuns for each ProwJob
			if err := createProwJobRuns(dbc); err != nil {
				return errors.WithMessage(err, "failed to create ProwJobRuns")
			}
			log.Info("Created ProwJobRuns for all ProwJobs")

			log.Info("Successfully seeded test data!")
			return nil
		},
	}

	f.BindFlags(cmd.Flags())

	return cmd
}

func createProwJobsForRelease(dbc *db.DB, release string) error {
	// Create 5 ProwJobs for this release
	for i := 1; i <= 5; i++ {
		prowJob := models.ProwJob{
			Kind:    models.ProwKind("periodic"),
			Name:    fmt.Sprintf("sippy-test-job-%s-test-%d", release, i),
			Release: release,
			// TestGridURL, Bugs, and JobRuns are left empty as requested
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

func createTestModels(dbc *db.DB) error {
	// Create 20 Test models with names test01 through test20
	for i := 1; i <= 20; i++ {
		testModel := models.Test{
			Name: fmt.Sprintf("test%02d", i), // Format as test01, test02, ..., test20
			// Bugs and TestOwnerships are left empty as requested
		}

		// Use FirstOrCreate to avoid duplicates - only creates if a Test with this name doesn't exist
		var existingTest models.Test
		if err := dbc.DB.Where("name = ?", testModel.Name).FirstOrCreate(&existingTest, testModel).Error; err != nil {
			return fmt.Errorf("failed to create or find Test %s: %v", testModel.Name, err)
		}

		// Log whether we created a new test or found an existing one
		if existingTest.CreatedAt.IsZero() || existingTest.CreatedAt.Equal(existingTest.UpdatedAt) {
			log.Debugf("Created new Test: %s", testModel.Name)
		} else {
			log.Debugf("Test already exists: %s", testModel.Name)
		}
	}

	return nil
}

func createProwJobRuns(dbc *db.DB) error {
	// First, get all existing ProwJobs from the database
	var prowJobs []models.ProwJob
	if err := dbc.DB.Find(&prowJobs).Error; err != nil {
		return fmt.Errorf("failed to fetch existing ProwJobs: %v", err)
	}

	log.Infof("Found %d ProwJobs, creating 100 runs for each", len(prowJobs))

	// Calculate time range: past 2 weeks from now
	now := time.Now()
	twoWeeksAgo := now.AddDate(0, 0, -14)

	// Duration for each run: 3 hours
	runDuration := 3 * time.Hour

	for _, prowJob := range prowJobs {
		log.Debugf("Creating ProwJobRuns for ProwJob: %s", prowJob.Name)

		// Create 100 ProwJobRuns for this ProwJob
		for i := 0; i < 100; i++ {
			// Calculate timestamp: spread evenly over the past 2 weeks
			// Total duration: 14 days = 14 * 24 * time.Hour
			totalDuration := 14 * 24 * time.Hour
			// Time between runs = total duration / 100 runs
			timeBetweenRuns := totalDuration / 100
			timestamp := twoWeeksAgo.Add(time.Duration(i) * timeBetweenRuns)

			prowJobRun := models.ProwJobRun{
				ProwJobID: prowJob.ID,
				Cluster:   "build01",
				Timestamp: timestamp,
				Duration:  runDuration,
				TestCount: 5000,
				// All other fields are left at their zero values as requested
			}

			// Create the ProwJobRun (no duplicate checking as requested)
			if err := dbc.DB.Create(&prowJobRun).Error; err != nil {
				return fmt.Errorf("failed to create ProwJobRun for ProwJob %s: %v", prowJob.Name, err)
			}
		}
	}

	return nil
}

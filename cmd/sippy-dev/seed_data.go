package main

import (
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/flags"
)

type SeedDataFlags struct {
	DBFlags *flags.PostgresFlags
}

func NewSeedDataFlags() *SeedDataFlags {
	return &SeedDataFlags{
		DBFlags: flags.NewPostgresDatabaseFlags(),
	}
}

func (f *SeedDataFlags) BindFlags(fs *pflag.FlagSet) {
	f.DBFlags.BindFlags(fs)
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
- 20 Test records (test01 through test20)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dbc, err := f.DBFlags.GetDBClient()
			if err != nil {
				return errors.WithMessage(err, "could not connect to database")
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

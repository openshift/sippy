package util

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/models/jobrunscan"
)

// SetupIntegrationSchema creates all tables (non-partitioned) and SQL
// functions needed by API query methods. It intentionally skips
// materialized views, partitioning, triggers, and GIN indexes.
func SetupIntegrationSchema(dbc *db.DB) error {
	if err := dbc.DB.SetupJoinTable(&models.ProwJobRun{}, "PullRequests", &models.ProwJobRunProwPullRequest{}); err != nil {
		return fmt.Errorf("setup join table ProwJobRun.PullRequests: %w", err)
	}

	allModels := []any{
		// Models normally managed by AutoMigrate in UpdateSchema
		&models.ReleaseDefinition{},
		&models.ReleaseTag{},
		&models.ReleasePullRequest{},
		&models.ReleaseRepository{},
		&models.ReleaseJobRun{},
		&models.ProwGARawTestDatum{},
		&models.VariantCombination{},
		&models.ProwJob{},
		&models.ProwJobRun{},
		&models.ProwJobRunIDMap{},
		&models.ProwJobRunAnnotation{},
		&models.Test{},
		&models.Suite{},
		&models.APISnapshot{},
		&models.Bug{},
		&models.ProwPullRequest{},
		&models.ProwJobRunProwPullRequest{},
		&models.SchemaHash{},
		&models.PullRequestComment{},
		&models.JiraIncident{},
		&models.JiraComponent{},
		&models.TestOwnership{},
		&models.FeatureGate{},
		&models.TestRegression{},
		&models.RegressionJobRun{},
		&models.RegressionView{},
		&models.Triage{},
		&models.TriageSymptom{},
		&models.AuditLog{},
		&jobrunscan.Label{},
		&jobrunscan.Symptom{},

		// Models normally managed by migrations (partitioned tables).
		// Created here as regular tables for integration testing.
		&models.ProwJobRunTest{},
		&models.ProwJobRunTestOutput{},
		&models.TestDailyTotal{},
		&models.TestCumulativeSummary{},
	}

	for _, model := range allModels {
		if err := dbc.DB.AutoMigrate(model); err != nil {
			return fmt.Errorf("auto-migrating %T: %w", model, err)
		}
	}

	if err := createSQLFunctions(dbc.DB); err != nil {
		return fmt.Errorf("creating SQL functions: %w", err)
	}

	return nil
}

// createSQLFunctions installs the server-side SQL functions that API
// query methods call (e.g. job_results, test_results).
func createSQLFunctions(gormDB *gorm.DB) error {
	for _, pgFunc := range db.PostgresFunctions {
		dropSQL := fmt.Sprintf("DROP FUNCTION IF EXISTS %s", pgFunc.Name)
		if err := gormDB.Exec(dropSQL).Error; err != nil {
			return fmt.Errorf("dropping function %s: %w", pgFunc.Name, err)
		}
		if err := gormDB.Exec(pgFunc.Definition).Error; err != nil {
			return fmt.Errorf("creating function %s: %w", pgFunc.Name, err)
		}
	}
	return nil
}

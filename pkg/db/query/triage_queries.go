package query

import (
	"context"
	"fmt"
	"time"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	log "github.com/sirupsen/logrus"
)

func ListTriages(dbc *db.DB) ([]models.Triage, error) {
	var triages []models.Triage
	res := dbc.DB.Preload("Bug").Preload("Regressions.JobRuns").Preload("Regressions.Views").Preload("Regressions").Find(&triages)
	if res.Error != nil {
		log.WithError(res.Error).Error("error listing all triages")
	}
	return triages, res.Error
}

func ListOpenRegressions(dbc *db.DB, release string) ([]*models.TestRegression, error) {
	var openRegressions []*models.TestRegression
	res := dbc.DB.
		Model(&models.TestRegression{}).
		Preload("Triages.Bug").
		Where("test_regressions.release = ?", release).
		Where("test_regressions.closed IS NULL").
		Find(&openRegressions)
	if res.Error != nil {
		log.WithError(res.Error).Error("error listing all regressions")
	}
	return openRegressions, res.Error
}

// CountRegressionFailuresAfter counts failed job runs for a regression that
// started after the specified time.
func CountRegressionFailuresAfter(dbc *db.DB, regressionID uint, after time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var count int64
	res := dbc.DB.WithContext(ctx).Model(&models.RegressionJobRun{}).
		Where("regression_id = ? AND start_time > ? AND test_failed", regressionID, after).
		Count(&count)
	if res.Error != nil {
		return 0, fmt.Errorf("error counting post-resolution failures for regression %d: %w", regressionID, res.Error)
	}
	return int(count), nil
}

func ListRegressions(dbc *db.DB, release string) ([]models.TestRegression, error) {
	var regressions []models.TestRegression
	query := dbc.DB.Model(&models.TestRegression{}).Preload("Triages").Preload("JobRuns").Preload("Views")

	if release != "" {
		query = query.Where("test_regressions.release = ?", release)
	}

	res := query.Find(&regressions)
	if res.Error != nil {
		log.WithError(res.Error).Error("error listing regressions")
	}
	return regressions, res.Error
}

func GetRegressionsForTest(dbc *db.DB, release, testName string) ([]models.TestRegression, error) {
	var regressions []models.TestRegression
	query := dbc.DB.Model(&models.TestRegression{}).Preload("Triages").Preload("JobRuns").Preload("Views")

	if release != "" {
		query = query.Where("test_regressions.release = ?", release)
	}

	query = query.Where("test_regressions.test_name = ?", testName)

	res := query.Find(&regressions)
	if res.Error != nil {
		log.WithError(res.Error).Error("error getting regressions for test")
	}
	return regressions, res.Error
}

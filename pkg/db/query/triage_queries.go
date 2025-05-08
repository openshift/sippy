package query

import (
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	log "github.com/sirupsen/logrus"
)

func ListTriages(dbc *db.DB) ([]models.Triage, error) {
	var triages []models.Triage
	res := dbc.DB.Preload("Bug").Preload("Regressions").Find(&triages)
	if res.Error != nil {
		log.WithError(res.Error).Error("error listing all triages")
	}
	return triages, res.Error
}

func TriagesForTestIDRelease(dbc *db.DB, testID, release string) ([]models.Triage, error) {
	var triages []models.Triage
	query := dbc.DB.
		Joins("JOIN triage_regressions trr ON trr.triage_id = triages.id").
		Joins("JOIN test_regressions tr ON tr.id = trr.test_regression_id")

	if testID != "" {
		query.Where("tr.test_id = ?", testID)
	}
	if release != "" {
		query.Where("tr.release = ?", release)
	}

	res := query.
		Preload("Bug").
		Preload("Regressions").
		Find(&triages)
	if res.Error != nil {
		log.WithError(res.Error).Error("error finding triages")
	}
	return triages, res.Error
}

func ListRegressions(dbc *db.DB, release string) ([]*models.TestRegression, error) {
	var openRegressions []*models.TestRegression
	res := dbc.DB.
		Model(&models.TestRegression{}).
		Preload("Triages").
		Where("test_regressions.release = ?", release).
		Where("test_regressions.closed IS NULL").
		Find(&openRegressions)
	if res.Error != nil {
		log.WithError(res.Error).Error("error listing all regressions")
	}
	return openRegressions, res.Error
}

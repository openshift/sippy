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

func ListRegressions(dbc *db.DB, release string) ([]models.TestRegression, error) {
	var regressions []models.TestRegression
	query := dbc.DB.Model(&models.TestRegression{}).Preload("Triages")

	if release != "" {
		query = query.Where("test_regressions.release = ?", release)
	}

	res := query.Find(&regressions)
	if res.Error != nil {
		log.WithError(res.Error).Error("error listing regressions")
	}
	return regressions, res.Error
}

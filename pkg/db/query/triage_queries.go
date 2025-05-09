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

func TriagesForRegressionID(dbc *db.DB, regressionID string) ([]models.Triage, error) {
	var triages []models.Triage
	res := dbc.DB.
		Joins("JOIN triage_regressions trr ON trr.triage_id = triages.id").
		Where("trr.test_regression_id = ?", regressionID).
		Preload("Bug").
		Preload("Regressions").
		Find(&triages)
	if res.Error != nil {
		log.WithError(res.Error).Error("error finding triages")
	}
	return triages, res.Error
}

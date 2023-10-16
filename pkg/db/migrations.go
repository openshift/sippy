package db

import (
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/db/models"
)

func testNameWithoutSuite(dbc *DB) error {
	// Get list of suites
	var knownSuites []models.Suite
	if res := dbc.DB.Model(&models.Suite{}).Find(&knownSuites); res.Error != nil {
		return res.Error
	}
	for _, suite := range knownSuites {
		log.Infof("processing suite %q", suite.Name)
		suiteID := suite.ID

		var testsWithPrefix []models.Test
		if err := dbc.DB.Table("tests").
			Where(fmt.Sprintf("name like '%s.%%'", suite.Name)).
			Scan(&testsWithPrefix).Error; err != nil {
			return err
		}

		for _, oldTest := range testsWithPrefix {
			log.Infof("processing test %q", oldTest.Name)
			var newTest models.Test
			newTestName := strings.TrimPrefix(oldTest.Name, fmt.Sprintf("%s.", suite.Name))

			if err := dbc.DB.Where("name = ?", newTestName).First(&newTest).Error; err != nil {
				log.Infof("no existing test found, renaming and adding suite to prow job run tests...")
				if err == gorm.ErrRecordNotFound {
					dbc.DB.Transaction(func(tx *gorm.DB) error {
						// Update the oldTest's name if there's no existing oldTest with the new name.
						oldTest.Name = newTestName
						if err := tx.Save(&oldTest).Error; err != nil {
							log.WithError(err).Warningf("error updating oldTest name for ID %d", oldTest.ID)
							return err
						}

						// Update rows in the prow_job_run_tests table to include the suite
						if err := tx.Model(&models.ProwJobRunTest{}).Where("test_id = ?", oldTest.ID).Updates(models.ProwJobRunTest{SuiteID: &suiteID}).Error; err != nil {
							log.WithError(err).Warningf("Error updating prow_job_run_tests for oldTest ID %d", oldTest.ID)
							return err
						}

						return nil
					})
				} else {
					log.WithError(err).Warningf("error looking for oldTest with name %q", newTestName)
					return err
				}
			} else {
				log.Infof("existing test found, making it the default and removing the old one...")
				dbc.DB.Transaction(func(tx *gorm.DB) error {
					// Update rows in the prow_job_run_tests table and then delete the old oldTest row.
					if err := tx.Model(&models.ProwJobRunTest{}).Where("test_id = ?", oldTest.ID).Updates(models.ProwJobRunTest{TestID: newTest.ID, SuiteID: &suiteID}).Error; err != nil {
						log.WithError(err).Warningf("Error updating prow_job_run_tests for oldTest ID %d", oldTest.ID)
						return err
					}

					if err := tx.Delete(&oldTest).Error; err != nil {
						log.WithError(err).Warningf("Error deleting oldTest with ID %d: %v", oldTest.ID)
						return err
					}

					return nil
				})
			}
		}
	}

	return nil
}

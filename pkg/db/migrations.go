package db

import (
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/db/models"
)

var migrations = map[string]func(*gorm.DB) error{
	"9999_testNameWithoutSuite": testNameWithoutSuite,
}

// testNameWithoutSuite removes test suite prefixes from tests in the database
// and assigns the suite to all the prow_job_run_tests. When the test can't be
// renamed because the unprefixed version exists in the DB, use that one and remove
// the prefixed version.
func testNameWithoutSuite(dbc *gorm.DB) error {
	// Get list of suites
	var knownSuites []models.Suite
	if res := dbc.Model(&models.Suite{}).Find(&knownSuites); res.Error != nil {
		return res.Error
	}
	for _, suite := range knownSuites {
		log.Infof("processing suite %q", suite.Name)
		suiteID := suite.ID

		var testsWithPrefix []models.Test
		if err := dbc.Table("tests").
			Where(fmt.Sprintf("name like '%s.%%'", suite.Name)).
			Scan(&testsWithPrefix).Error; err != nil {
			return err
		}

		for i, oldTest := range testsWithPrefix {
			log.Infof("processing test %q", oldTest.Name)
			var newTest models.Test
			newTestName := strings.TrimPrefix(oldTest.Name, fmt.Sprintf("%s.", suite.Name))

			if err := dbc.Where("name = ?", newTestName).First(&newTest).Error; err != nil {
				log.Infof("no existing test found, renaming and adding suite to prow job run tests...")
				if err == gorm.ErrRecordNotFound {
					err := dbc.Transaction(func(tx *gorm.DB) error {
						// Update the oldTest's name if there's no existing oldTest with the new name.
						testsWithPrefix[i].Name = newTestName
						if err := tx.Save(&testsWithPrefix[i]).Error; err != nil {
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
					if err != nil {
						log.WithError(err).Warningf("test migration failed")
						return err
					}
				}
				log.WithError(err).Warningf("error looking for oldTest with name %q", newTestName)
				return err
			}

			log.Infof("existing test found, making it the default and removing the old one...")
			err := dbc.Transaction(func(tx *gorm.DB) error {
				// Update rows in the prow_job_run_tests table and then delete the old oldTest row.
				if err := tx.Model(&models.ProwJobRunTest{}).Where("test_id = ?", oldTest.ID).Updates(models.ProwJobRunTest{TestID: newTest.ID, SuiteID: &suiteID}).Error; err != nil {
					log.WithError(err).Warningf("Error updating prow_job_run_tests for oldTest ID %d", oldTest.ID)
					return err
				}

				if err := tx.Delete(&testsWithPrefix[i]).Error; err != nil {
					log.WithError(err).Warningf("Error deleting oldTest with ID %d", oldTest.ID)
					return err
				}

				return nil
			})
			if err != nil {
				log.WithError(err).Warningf("test migration failed")
				return err
			}
		}
	}

	return nil
}

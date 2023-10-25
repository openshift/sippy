package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/sippyserver"
)

func main() {
	dsn := os.Getenv("SIPPY_DATABASE_DSN")
	if dsn == "" {
		fmt.Println("Set SIPPY_DATABSE_DSN")
		os.Exit(1)
	}

	gormLogger := gormlogger.New(
		log2LogrusWriter{entry: log.WithField("source", "gorm")},
		gormlogger.Config{
			SlowThreshold:             60 * time.Second,
			LogLevel:                  gormlogger.Info,
			IgnoreRecordNotFoundError: true,
			Colorful:                  true,
		},
	)

	dbc, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		panic(err)
	}

	if err := testNameWithoutSuite(dbc); err != nil {
		panic(err)
	}
}

// log2LogrusWriter bridges gorm logging to logrus logging.
// All messages will come through at DEBUG level.
type log2LogrusWriter struct {
	entry *log.Entry
}

func (w log2LogrusWriter) Printf(msg string, args ...interface{}) {
	w.entry.Debugf(msg, args...)
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
	rowsUpdated := int64(0)
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
				if err == gorm.ErrRecordNotFound { //nolint
					log.Infof("no existing test found, renaming and adding suite to prow job run tests...")
					err := dbc.Transaction(func(tx *gorm.DB) error {
						// Update the oldTest's name if there's no existing oldTest with the new name.
						testsWithPrefix[i].Name = newTestName
						if err := tx.Save(&testsWithPrefix[i]).Error; err != nil {
							log.WithError(err).Warningf("error updating oldTest name for ID %d", oldTest.ID)
							return err
						}

						// Update rows in the prow_job_run_tests table to include the suite
						res := tx.Model(&models.ProwJobRunTest{}).Where("test_id = ?", oldTest.ID).Updates(models.ProwJobRunTest{SuiteID: &suiteID})
						if res.Error != nil {
							log.WithError(res.Error).Warningf("Error updating prow_job_run_tests for oldTest ID %d", oldTest.ID)
							return res.Error
						}
						log.WithFields(map[string]interface{}{
							"test":         newTest.ID,
							"suite":        suiteID,
							"rows_updated": res.RowsAffected,
						}).Infof("update complete for %q", newTestName)
						rowsUpdated += res.RowsAffected

						return nil
					})
					if err != nil {
						log.WithError(err).Warningf("test migration failed")
						return err
					}
				} else { //nolint
					log.WithError(err).Warningf("error looking for oldTest with name %q", newTestName)
					return err
				}
			} else { //nolint
				log.Infof("existing test found, making it the default and removing the old one...")
				err := dbc.Transaction(func(tx *gorm.DB) error {
					// Update rows in the prow_job_run_tests table and then delete the old oldTest row.
					res := tx.Model(&models.ProwJobRunTest{}).Where("test_id = ?", oldTest.ID).Updates(models.ProwJobRunTest{TestID: newTest.ID, SuiteID: &suiteID})
					if res.Error != nil {
						log.WithError(res.Error).Warningf("error updating prow_job_run_tests for oldTest ID %d", oldTest.ID)
						return res.Error
					}
					log.WithFields(map[string]interface{}{
						"old_test":     oldTest.ID,
						"new_test":     newTest.ID,
						"suite_id":     suiteID,
						"rows_updated": res.RowsAffected,
					}).Infof("update complete for %q", newTestName)
					rowsUpdated += res.RowsAffected

					if err := tx.Model(&models.Test{}).Unscoped().Delete(&testsWithPrefix[i]).Error; err != nil {
						log.WithError(err).Warningf("error deleting oldTest with ID %d", oldTest.ID)
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
	}
	log.Infof("update complete, total rows updated %d", rowsUpdated)

	// Refresh materialized views
	sippyserver.RefreshData(&db.DB{
		DB: dbc,
	}, nil, false)

	return nil
}

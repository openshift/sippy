package db

import (
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// testSuitePrefixes are known test suites we want to detect in testgrid test names (appears as suiteName.testName)
// and parse out so we can view results for the same test across any suite it might be used in. The suite info is
// stored on the ProwJobRunTest row allowing us to query data specific to a suite if needed.
var testSuitePrefixes = []string{
	"openshift-tests",         // a primary origin test suite name
	"openshift-tests-upgrade", // a primary origin test suite name
	"sippy",                   // used for all synthetic tests sippy adds
	// "Symptom detection.",       // TODO: origin unknown, possibly deprecated
	// "OSD e2e suite.",           // TODO: origin unknown, possibly deprecated
	// "Log Metrics.",             // TODO: origin unknown, possibly deprecated
}

func populateTestSuitesInDB(db *gorm.DB) error {
	for _, suiteName := range testSuitePrefixes {
		s := models.Suite{}
		res := db.Where("name = ?", suiteName).First(&s)
		if res.Error != nil {
			if !errors.Is(res.Error, gorm.ErrRecordNotFound) {
				return res.Error
			}
			s = models.Suite{
				Name: suiteName,
			}
			err := db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&s).Error
			if err != nil {
				return errors.Wrapf(err, "error loading suite into db: %s", suiteName)
			}
			log.WithField("suite", suiteName).Info("created new test suite")
		}
	}
	return nil
}

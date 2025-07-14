package db

import (
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/openshift/sippy/pkg/db/models"
)

// testSuites are known test suites we want to import into sippy. tests from other suites will not be
// imported into sippy. Get the list of seen test suites from bigquery with:
//
//	SELECT DISTINCT(testsuite), count(*) count
//		FROM `openshift-gce-devel.ci_analysis_us.junit` \
//		GROUP BY testsuite
//		ORDER BY count desc
var testSuites = []string{
	// Primary origin suite names
	"openshift-tests",
	"openshift-tests-upgrade",

	// Sippy synthetic tests
	"sippy",

	// ROSA
	"OSD e2e suite",

	// Other
	"BackendDisruption",
	"Cluster upgrade",
	"E2E Suite",
	"Kubernetes e2e suite",
	"Log Metrics",
	"Operator results",
	"Symptom Detection",
	"Tests Suite",
	"cluster install",
	"cluster nodes ready",
	"cluster nodes",
	"gather core dump",
	"hypershift-e2e",
	"metal infra",
	"step graph",
	"telco-verification",
	"github.com/openshift/console-operator/test/e2e",
	"prowjob-junit",
	"OLM-Catalog-Validation",
	"insights-operator-tests",
}

func populateTestSuitesInDB(db *gorm.DB) error {
	for _, suiteName := range testSuites {
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

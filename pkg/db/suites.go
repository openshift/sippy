package db

import (
	"regexp"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/util/sets"

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

	// ARO
	"rp-api-compat-all/parallel",
	"integration/parallel",
	"stage/parallel",
	"prod/parallel",
	"aro-hcp-tests",
	"github.com/stolostron/capi-tests/test",

	// ROSA
	"OSD e2e suite",
	"ROSA Regional Platform API E2E Suite",

	// Performance
	"olmv1-GCP nightly compare",

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
	"CNV-lp-interop",
	"ODF-lp-interop",
	"OADP-lp-interop",
	"ACS-lp-interop",
	"ACSLatest-lp-interop",
	"Fusion-access-lp-interop",
	"MTA-lp-interop",
	"Gitops-lp-interop",
	"Quay-lp-interop",
	"Serverless-lp-interop",
	"ServiceMesh-lp-interop",
	"OpenshiftPipelines-lp-interop",
	"tracing-uiplugin",
}

// testSuitePatterns are regular expressions for suite names that should be imported
// without listing every literal name. Invalid patterns panic at process start.
var testSuitePatterns = []*regexp.Regexp{
	// LP interop naming: `lp-interop--<product>--<suffix>`.
	regexp.MustCompile(`^lp-chaos--`),
	regexp.MustCompile(`^lp-interop--`),
	regexp.MustCompile(`^lp-ocp-compat--`),
}

var testSuiteSet = sets.New[string](testSuites...)

// IsSuiteImportable checks if a suite name should be imported based on
// the explicit testSuites list or dynamic patterns.
func IsSuiteImportable(name string) bool {
	if testSuiteSet.Has(name) {
		return true
	}

	for _, re := range testSuitePatterns {
		if re.MatchString(name) {
			return true
		}
	}

	return false
}

// getOrCreateSuite finds or creates a suite by name. Returns the suite ID on success, nil on error.
// Uses FirstOrCreate for thread-safe upsert behavior.
func getOrCreateSuite(db *gorm.DB, name string) *uint {
	suite := models.Suite{Name: name}
	result := db.Where("name = ?", name).FirstOrCreate(&suite)
	if result.Error != nil {
		// Fallback read handles concurrent creator-wins race windows.
		read := db.Where("name = ?", name).First(&suite)
		if read.Error != nil {
			log.WithError(result.Error).Errorf("failed to get or create suite %q", name)
			return nil
		}

		// Fallback read succeeded, continue to return suite ID
	}

	// Validate that we got a valid suite ID
	if suite.ID == 0 {
		log.Errorf("suite %q has invalid ID 0", name)
		return nil
	}

	// Found (RowsAffected > 0) even for existing records
	if result.RowsAffected > 0 {
		log.WithField("suite", name).Info("retrieved test suite")
	}

	id := suite.ID
	return &id
}

// Runs when the DB is set up / migrated.
func populateTestSuitesInDB(db *gorm.DB) error {
	for _, suiteName := range testSuites {
		if getOrCreateSuite(db, suiteName) == nil {
			return errors.Errorf("error loading suite into db: %s", suiteName)
		}
	}
	return nil
}

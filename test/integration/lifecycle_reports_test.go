package integration

// TestLifecycleReportsMatchBase pins /api/install, /api/upgrade and /api/health response bodies
// for non-product releases to base commit 74967418d (the merge-base of feat/quay-lifecycle-tabs
// with origin/main), proving the additive product-lifecycle selector introduced on this branch
// changes nothing for OpenShift-only releases. TestLifecycleReportsQuay and
// TestLifecycleReportsQuayMissingUpgrade cover the additive Quay behaviour itself.
//
// Regenerating the goldens after a legitimate change to base behaviour:
//
//	git worktree add /tmp/sippy-base 74967418d
//	cp test/integration/lifecycle_reports_test.go /tmp/sippy-base/test/integration/
//	(cd /tmp/sippy-base && go test ./test/integration/ -run TestLifecycleReportsMatchBase -count 1 -update)
//	cp -r /tmp/sippy-base/test/integration/testdata/lifecycle_reports test/integration/testdata/
//	git worktree remove /tmp/sippy-base
//	go test ./test/integration/ -run TestLifecycleReportsMatchBase -count 1

import (
	"bytes"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/testidentification"
	intutil "github.com/openshift/sippy/test/integration/util"
)

var updateLifecycleGoldens = flag.Bool("update", false, "rewrite lifecycle_reports golden files")

const lifecycleGoldenDir = "testdata/lifecycle_reports"

// Releases exercised by the lifecycle report tests. releaseOCPModern and releaseOKD both use the
// modern install/infra testcase names (see useNewInstallTest in pkg/api/lifecycle_tests.go);
// releaseOCPLegacy and releaseAROStage use the legacy names. releaseQuay is the only one of the
// five that is a product release (see lifecycleProduct).
const (
	releaseOCPModern = "4.21"
	releaseOCPLegacy = "4.10"
	releaseAROStage  = "aro-stage"
	releaseOKD       = "4.21-okd"
	releaseQuay      = "quay-3.18"
)

// Lifecycle testcase names, built from the same constants pkg/api/lifecycle_tests.go and
// pkg/testidentification select on, so a rename there breaks this test at compile/seed time
// rather than silently seeding data the handlers never look at.
var (
	testSippyInstallLegacy = testidentification.InstallTestName
	testInstallOverall     = testidentification.NewInstallTestName
	testInstallInfra       = testidentification.NewInfrastructureTestName
	testInstallConfig      = testidentification.InstallConfigTestName
	testInstallBootstrap   = testidentification.InstallBootstrapTestName
	testInstallOther       = testidentification.InstallOtherTestName
	testOperatorInstall    = testidentification.OperatorInstallPrefix + "kube-apiserver"
	testSippyUpgrade       = testidentification.UpgradeTestName
	testOperatorUpgrade    = testidentification.OperatorUpgradePrefix + "kube-apiserver"
	testOpenShiftTests     = testidentification.OpenShiftTestsName

	// Not built from testidentification.LifecycleInstallTestName/LifecycleUpgradeTestName:
	// those constants don't exist at base commit 74967418d, and this file is copied verbatim
	// into a worktree at that commit to regenerate the TestLifecycleReportsMatchBase goldens.
	testQuayInstall = "[sig-quay] install should succeed"
	testQuayUpgrade = "[sig-quay] upgrade should succeed"
)

// coreLifecycleTests are seeded, identically, for every release in seedLifecycleFixture. Which
// ones a given release's install/upgrade/health responses actually select depends on that
// release's lifecycleTestSelection (pkg/api/lifecycle_tests.go), so seeding the same superset
// everywhere lets the selection logic itself do the filtering the test is meant to exercise.
var coreLifecycleTests = []string{
	testSippyInstallLegacy,
	testInstallOverall,
	testInstallInfra,
	testInstallConfig,
	testInstallBootstrap,
	testInstallOther,
	testOperatorInstall,
	testSippyUpgrade,
	testOperatorUpgrade,
	testOpenShiftTests,
}

// coreTestCounts returns deterministic, distinct (runs, successes, flakes) for a core
// testcase/job pair. Distinct denominators per testcase and per job mean a bug that swaps one
// test's or one job's counts for another's changes the response body.
func coreTestCounts(testIndex, jobIndex int) (runs, successes, flakes int64) {
	runs = int64(1000 + testIndex*57 + jobIndex*23)
	successes = runs - int64(5+jobIndex)
	flakes = int64(1 + testIndex%3)
	return runs, successes, flakes
}

// Quay product testcase counts. Seeded on a single, non-excluded job only (see
// seedLifecycleFixture), so current_runs for these rows is exactly these constants with no join
// arithmetic to account for.
const (
	quayInstallRuns, quayInstallSuccesses, quayInstallFlakes = 4242, 4200, 2
	quayUpgradeRuns, quayUpgradeSuccesses, quayUpgradeFlakes = 3131, 3100, 1
)

// seedLifecycleFixture creates a release definition, two prow jobs (aws/ovn, and
// gcp/ovn/upgrade-minor) and cumulative summaries for coreLifecycleTests on both jobs, for each
// of releaseOCPModern, releaseOCPLegacy, releaseAROStage, releaseOKD and releaseQuay. The second
// job's "upgrade-minor" variant is what pkg/api/install.go and the health install-related
// indicators exclude (excludedInstallVariants), so seeding both jobs exercises that exclusion
// rather than just assuming it works.
//
// For releaseQuay it additionally seeds the Quay product install testcase, and (when
// includeQuayUpgrade is true) the Quay product upgrade testcase, both on the aws/ovn job only so
// their current_runs are exactly the seeded constants with no exclusion or aggregation to reason
// about. includeQuayUpgrade=false lets TestLifecycleReportsQuayMissingUpgrade cover a product
// whose upgrade job hasn't reported yet.
//
// Every row is seeded at a single date, today (UTC), with no earlier row for that
// test/job/suite. testReportCoreJoin (pkg/db/query/cumulative_query.go) treats a missing
// earlier row as a zero baseline, so current_runs is simply the seeded value and previous_runs
// is zero; this keeps the fixture's arithmetic trivial while still giving every row a distinct,
// non-zero current count.
func seedLifecycleFixture(t *testing.T, dbc *db.DB, includeQuayUpgrade bool) {
	t.Helper()

	today := civil.DateOf(time.Now().UTC())

	suite := intutil.CreateSuite(t, dbc, "openshift-tests")
	quaySuite := intutil.CreateSuite(t, dbc, "quay-lifecycle")

	testIDs := make(map[string]uint, len(coreLifecycleTests)+2)
	for _, name := range coreLifecycleTests {
		testIDs[name] = intutil.CreateTest(t, dbc, name).ID
	}
	testIDs[testQuayInstall] = intutil.CreateTest(t, dbc, testQuayInstall).ID
	testIDs[testQuayUpgrade] = intutil.CreateTest(t, dbc, testQuayUpgrade).ID

	vcAWS := intutil.CreateVariantCombination(t, dbc, []string{"aws", "ovn"})
	vcGCP := intutil.CreateVariantCombination(t, dbc, []string{"gcp", "ovn", "upgrade-minor"})

	releases := []struct {
		release      string
		major, minor int
	}{
		{releaseOCPModern, 4, 21},
		{releaseOCPLegacy, 4, 10},
		{releaseAROStage, 0, 0},
		{releaseOKD, 4, 21},
		{releaseQuay, 3, 18},
	}

	for _, r := range releases {
		intutil.CreateReleaseDefinition(t, dbc, r.release, r.major, r.minor)

		jobAWS := intutil.CreateProwJobWithOptions(t, dbc, "periodic-"+r.release+"-aws-ovn", r.release, nil, intutil.WithVariantCombination(vcAWS))
		jobGCP := intutil.CreateProwJobWithOptions(t, dbc, "periodic-"+r.release+"-gcp-ovn-upgrade-minor", r.release, nil, intutil.WithVariantCombination(vcGCP))

		for i, name := range coreLifecycleTests {
			for j, job := range []models.ProwJob{jobAWS, jobGCP} {
				runs, successes, flakes := coreTestCounts(i, j)
				intutil.CreateCumulativeSummary(t, dbc, today, r.release, testIDs[name], job.ID, suite.ID, runs, successes, flakes)
			}
		}

		if r.release != releaseQuay {
			continue
		}

		intutil.CreateCumulativeSummary(t, dbc, today, r.release, testIDs[testQuayInstall], jobAWS.ID, quaySuite.ID, quayInstallRuns, quayInstallSuccesses, quayInstallFlakes)
		if includeQuayUpgrade {
			intutil.CreateCumulativeSummary(t, dbc, today, r.release, testIDs[testQuayUpgrade], jobAWS.ID, quaySuite.ID, quayUpgradeRuns, quayUpgradeSuccesses, quayUpgradeFlakes)
		}
	}
}

func fetchInstallReport(t *testing.T, dbc *db.DB, release string) []byte {
	t.Helper()
	w := httptest.NewRecorder()
	api.PrintInstallJSONReportFromDB(w, dbc, release)
	return w.Body.Bytes()
}

func fetchUpgradeReport(t *testing.T, dbc *db.DB, release string) []byte {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/upgrade?release="+release, nil)
	api.PrintUpgradeJSONReportFromDB(w, req, dbc, release)
	return w.Body.Bytes()
}

func fetchHealthReport(t *testing.T, dbc *db.DB, release string) []byte {
	t.Helper()
	w := httptest.NewRecorder()
	api.PrintOverallReleaseHealthFromDB(w, dbc, release, time.Now().UTC())
	return w.Body.Bytes()
}

func TestLifecycleReportsMatchBase(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	seedLifecycleFixture(t, dbc, true)

	releases := []string{releaseOCPModern, releaseOCPLegacy, releaseAROStage, releaseOKD}
	fetchers := map[string]func(*testing.T, *db.DB, string) []byte{
		"install": fetchInstallReport,
		"upgrade": fetchUpgradeReport,
		"health":  fetchHealthReport,
	}

	for _, release := range releases {
		for _, kind := range []string{"install", "upgrade", "health"} {
			t.Run(release+"/"+kind, func(t *testing.T) {
				got := fetchers[kind](t, dbc, release)
				goldenPath := filepath.Join(lifecycleGoldenDir, release, kind+".json")

				if *updateLifecycleGoldens {
					require.NoError(t, os.MkdirAll(filepath.Dir(goldenPath), 0o755))
					require.NoError(t, os.WriteFile(goldenPath, got, 0o644)) //nolint:gosec // golden fixture, not sensitive
					return
				}

				want, err := os.ReadFile(goldenPath) //nolint:gosec // fixed test-local path
				require.NoError(t, err, "reading golden %s (run with -update against base %s to regenerate)", goldenPath, "74967418d")
				assert.True(t, bytes.Equal(want, got), "%s body for release %s differs from base golden %s", kind, release, goldenPath)
			})
		}
	}
}

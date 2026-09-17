package integration

import (
	"fmt"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/util/sets"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/query"
	intutil "github.com/openshift/sippy/test/integration/util"
)

// ---------------------------------------------------------------------------
// GetPayloadDiff: "What PRs changed between two payloads?"
// ---------------------------------------------------------------------------

func TestGetPayloadDiff_NewPRsBetweenPayloads(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	t0 := time.Date(2024, 6, 10, 1, 0, 0, 0, time.UTC)
	tagFrom := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-10-010000", "4.16", "nightly", "amd64", t0)
	tagTo := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-11-010000", "4.16", "nightly", "amd64", t0.Add(24*time.Hour))

	shared := intutil.CreateReleasePullRequest(t, dbc, "https://github.com/org/repo/pull/1", "repo", "shared PR")
	onlyFrom := intutil.CreateReleasePullRequest(t, dbc, "https://github.com/org/repo/pull/2", "repo", "only in from")
	onlyTo := intutil.CreateReleasePullRequest(t, dbc, "https://github.com/org/repo/pull/3", "repo", "only in to")

	intutil.LinkReleaseTagPullRequests(t, dbc, &tagFrom, shared, onlyFrom)
	intutil.LinkReleaseTagPullRequests(t, dbc, &tagTo, shared, onlyTo)

	results, err := query.GetPayloadDiff(dbc.DB, tagFrom.ReleaseTag, tagTo.ReleaseTag)
	require.NoError(t, err)

	require.Len(t, results, 1, "should return only PRs in target but not in baseline")
	assert.Equal(t, onlyTo.URL, results[0].URL)
	assert.Equal(t, onlyTo.Description, results[0].Description)
}

func TestGetPayloadDiff_IdenticalPayloads(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	tag := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-10-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 10, 1, 0, 0, 0, time.UTC))
	pr := intutil.CreateReleasePullRequest(t, dbc, "https://github.com/org/repo/pull/1", "repo", "some PR")
	intutil.LinkReleaseTagPullRequests(t, dbc, &tag, pr)

	results, err := query.GetPayloadDiff(dbc.DB, tag.ReleaseTag, tag.ReleaseTag)
	require.NoError(t, err)
	assert.Empty(t, results, "self-diff should produce no results")
}

func TestGetPayloadDiff_AllPRsAreNew(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	t0 := time.Date(2024, 6, 10, 1, 0, 0, 0, time.UTC)
	tagFrom := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-10-010000", "4.16", "nightly", "amd64", t0)
	tagTo := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-11-010000", "4.16", "nightly", "amd64", t0.Add(24*time.Hour))

	pr1 := intutil.CreateReleasePullRequest(t, dbc, "https://github.com/org/repo/pull/1", "repo", "new PR 1")
	pr2 := intutil.CreateReleasePullRequest(t, dbc, "https://github.com/org/repo/pull/2", "repo", "new PR 2")
	intutil.LinkReleaseTagPullRequests(t, dbc, &tagTo, pr1, pr2)
	// tagFrom has no PRs

	results, err := query.GetPayloadDiff(dbc.DB, tagFrom.ReleaseTag, tagTo.ReleaseTag)
	require.NoError(t, err)
	assert.Len(t, results, 2, "all target PRs should appear when baseline has none")
}

func TestGetPayloadDiff_ResultsOrderedByURL(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	t0 := time.Date(2024, 6, 10, 1, 0, 0, 0, time.UTC)
	tagFrom := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-10-010000", "4.16", "nightly", "amd64", t0)
	tagTo := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-11-010000", "4.16", "nightly", "amd64", t0.Add(24*time.Hour))

	prZ := intutil.CreateReleasePullRequest(t, dbc, "https://github.com/z-org/repo/pull/1", "z-repo", "z PR")
	prA := intutil.CreateReleasePullRequest(t, dbc, "https://github.com/a-org/repo/pull/1", "a-repo", "a PR")
	prM := intutil.CreateReleasePullRequest(t, dbc, "https://github.com/m-org/repo/pull/1", "m-repo", "m PR")
	intutil.LinkReleaseTagPullRequests(t, dbc, &tagTo, prZ, prA, prM)

	results, err := query.GetPayloadDiff(dbc.DB, tagFrom.ReleaseTag, tagTo.ReleaseTag)
	require.NoError(t, err)
	require.Len(t, results, 3)

	assert.Equal(t, prA.URL, results[0].URL)
	assert.Equal(t, prM.URL, results[1].URL)
	assert.Equal(t, prZ.URL, results[2].URL)
}

// ---------------------------------------------------------------------------
// GetLastAcceptedByArchitectureAndStream: "What was the last accepted
// payload for each arch/stream?"
// ---------------------------------------------------------------------------

func TestGetLastAccepted_MostRecentPerArchStream(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	reportEnd := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	// amd64/nightly: two accepted, one rejected. Most recent accepted should win.
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC), intutil.WithPhase(apitype.PayloadRejected))
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-12-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 12, 1, 0, 0, 0, time.UTC))
	newerAccepted := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-13-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 13, 1, 0, 0, 0, time.UTC))

	// arm64/nightly: one accepted
	arm64Tag := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-arm64-2024-06-13-010000", "4.16", "nightly", "arm64",
		time.Date(2024, 6, 13, 1, 0, 0, 0, time.UTC))

	// amd64/ci: one accepted
	ciTag := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.ci-2024-06-13-010000", "4.16", "ci", "amd64",
		time.Date(2024, 6, 13, 1, 0, 0, 0, time.UTC))

	results, err := query.GetLastAcceptedByArchitectureAndStream(dbc.DB, "4.16", reportEnd)
	require.NoError(t, err)
	require.Len(t, results, 3, "one result per unique (arch, stream) combo")

	byKey := make(map[string]models.ReleaseTag)
	for _, r := range results {
		byKey[r.Architecture+"/"+r.Stream] = r
	}

	assert.Equal(t, newerAccepted.ReleaseTag, byKey["amd64/nightly"].ReleaseTag,
		"should pick the most recently accepted, not the most recently created")
	assert.Equal(t, arm64Tag.ReleaseTag, byKey["arm64/nightly"].ReleaseTag)
	assert.Equal(t, ciTag.ReleaseTag, byKey["amd64/ci"].ReleaseTag)
}

func TestGetLastAccepted_ExcludesPayloadsAfterReportEnd(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	reportEnd := time.Date(2024, 6, 13, 0, 0, 0, 0, time.UTC)

	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-12-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 12, 1, 0, 0, 0, time.UTC))
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC))

	results, err := query.GetLastAcceptedByArchitectureAndStream(dbc.DB, "4.16", reportEnd)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "4.16.0-0.nightly-2024-06-12-010000", results[0].ReleaseTag)
}

func TestGetLastAccepted_NoAcceptedPayloads(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC), intutil.WithPhase(apitype.PayloadRejected))

	results, err := query.GetLastAcceptedByArchitectureAndStream(dbc.DB, "4.16",
		time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Empty(t, results)
}

// ---------------------------------------------------------------------------
// GetTestFailuresForPayload: "What tests failed in this specific payload?"
// ---------------------------------------------------------------------------

type failureFixture struct {
	dbc       *db.DB
	release   string
	stream    string
	arch      string
	tag       models.ReleaseTag
	job       models.ProwJob
	run       models.ProwJobRun
	testA     models.Test
	testB     models.Test
	runTime   time.Time
	tagTime   time.Time
	reportEnd time.Time
}

func setupFailureFixture(t *testing.T, dbc *db.DB) failureFixture {
	t.Helper()

	f := failureFixture{
		dbc:       dbc,
		release:   "4.16",
		stream:    "nightly",
		arch:      "amd64",
		tagTime:   time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC),
		runTime:   time.Date(2024, 6, 14, 2, 0, 0, 0, time.UTC),
		reportEnd: time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
	}

	f.tag = intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-010000", f.release, f.stream, f.arch,
		f.tagTime, intutil.WithPhase(apitype.PayloadRejected))

	const runURL = "https://prow/run/1"
	f.job = intutil.CreateProwJob(t, dbc, "periodic-ci-e2e-aws-ovn-4.16", f.release, nil)
	f.run = intutil.CreateProwJobRun(t, dbc, f.job.ID, f.release, f.runTime, false, "F", intutil.WithURL(runURL))

	intutil.CreateReleaseJobRun(t, dbc, f.tag.ID, f.run.ID, f.job.Name, "Blocking", "Failed", runURL)

	f.testA = intutil.CreateTest(t, dbc, "test-a-network-check")
	f.testB = intutil.CreateTest(t, dbc, "test-b-install-check")

	intutil.CreateProwJobRunTest(t, dbc, f.run.ID, f.job.ID, f.testA.ID, f.release, f.runTime, int(v1.TestStatusFailure))
	intutil.CreateProwJobRunTest(t, dbc, f.run.ID, f.job.ID, f.testB.ID, f.release, f.runTime, int(v1.TestStatusFailure))

	return f
}

func TestGetTestFailuresForPayload_ReturnsFailedTests(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	f := setupFailureFixture(t, dbc)

	results, err := query.GetTestFailuresForPayload(dbc.DB, f.tag.ReleaseTag, f.release, f.tagTime)
	require.NoError(t, err)

	require.Len(t, results, 2)
	byName := make(map[string]models.PayloadFailedTest)
	for _, r := range results {
		byName[r.Name] = r
	}

	testA := byName[f.testA.Name]
	assert.Equal(t, f.release, testA.Release)
	assert.Equal(t, "amd64", testA.Architecture)
	assert.Equal(t, "nightly", testA.Stream)
	assert.Equal(t, f.tag.ReleaseTag, testA.ReleaseTag)
	assert.Equal(t, f.job.Name, testA.ProwJobName)
	assert.NotEmpty(t, testA.ProwJobRunURL)
}

func TestGetTestFailuresForPayload_IncludesAllJobKinds(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	release := "4.16"
	tagTime := time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC)
	runTime := time.Date(2024, 6, 14, 2, 0, 0, 0, time.UTC)

	tag := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-010000", release, "nightly", "amd64",
		tagTime, intutil.WithPhase(apitype.PayloadRejected))

	blockingJob := intutil.CreateProwJob(t, dbc, "periodic-ci-e2e-aws-blocking-4.16", release, nil)
	informingJob := intutil.CreateProwJob(t, dbc, "periodic-ci-e2e-gcp-informing-4.16", release, nil)

	const blockingURL = "https://prow/blocking/1"
	const informingURL = "https://prow/informing/1"
	blockingRun := intutil.CreateProwJobRun(t, dbc, blockingJob.ID, release, runTime, false, "F", intutil.WithURL(blockingURL))
	informingRun := intutil.CreateProwJobRun(t, dbc, informingJob.ID, release, runTime.Add(time.Hour), false, "F", intutil.WithURL(informingURL))

	intutil.CreateReleaseJobRun(t, dbc, tag.ID, blockingRun.ID, blockingJob.Name, "Blocking", "Failed", blockingURL)
	intutil.CreateReleaseJobRun(t, dbc, tag.ID, informingRun.ID, informingJob.Name, "Informing", "Failed", informingURL)

	testX := intutil.CreateTest(t, dbc, "test-x")
	intutil.CreateProwJobRunTest(t, dbc, blockingRun.ID, blockingJob.ID, testX.ID, release, runTime, int(v1.TestStatusFailure))
	intutil.CreateProwJobRunTest(t, dbc, informingRun.ID, informingJob.ID, testX.ID, release, informingRun.Timestamp, int(v1.TestStatusFailure))

	results, err := query.GetTestFailuresForPayload(dbc.DB, tag.ReleaseTag, release, tagTime)
	require.NoError(t, err)

	assert.Len(t, results, 2, "should include failures from both blocking and informing jobs")
	jobNames := sets.New[string]()
	for _, r := range results {
		jobNames.Insert(r.ProwJobName)
	}
	assert.True(t, jobNames.Has(blockingJob.Name), "should include blocking job failures")
	assert.True(t, jobNames.Has(informingJob.Name), "should include informing job failures")
}

func TestGetTestFailuresForPayload_ExcludesSucceededJobRuns(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	release := "4.16"
	tagTime := time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC)
	runTime := time.Date(2024, 6, 14, 2, 0, 0, 0, time.UTC)

	tag := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-010000", release, "nightly", "amd64",
		tagTime, intutil.WithPhase(apitype.PayloadAccepted))

	const runURL = "https://prow/run/succeeded"
	job := intutil.CreateProwJob(t, dbc, "periodic-ci-e2e-aws-4.16", release, nil)
	run := intutil.CreateProwJobRun(t, dbc, job.ID, release, runTime, true, "S", intutil.WithURL(runURL))
	intutil.CreateReleaseJobRun(t, dbc, tag.ID, run.ID, job.Name, "Blocking", "Succeeded", runURL)

	testX := intutil.CreateTest(t, dbc, "test-x")
	intutil.CreateProwJobRunTest(t, dbc, run.ID, job.ID, testX.ID, release, runTime, int(v1.TestStatusFailure))

	results, err := query.GetTestFailuresForPayload(dbc.DB, tag.ReleaseTag, release, tagTime)
	require.NoError(t, err)
	assert.Empty(t, results, "succeeded job runs should not produce test failure results")
}

func TestGetTestFailuresForPayload_ExcludesPassingTestResults(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	f := setupFailureFixture(t, dbc)

	// Add a passing test result (status=1) for a new test
	passingTest := intutil.CreateTest(t, dbc, "test-passes")
	intutil.CreateProwJobRunTest(t, dbc, f.run.ID, f.job.ID, passingTest.ID, f.release, f.runTime, int(v1.TestStatusSuccess))

	results, err := query.GetTestFailuresForPayload(dbc.DB, f.tag.ReleaseTag, f.release, f.tagTime)
	require.NoError(t, err)

	for _, r := range results {
		assert.NotEqual(t, "test-passes", r.Name, "passing tests should not appear in failure results")
	}
	assert.Len(t, results, 2, "only the two failing tests from the fixture should appear")
}

func TestGetTestFailuresForPayload_NoFailures(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	release := "4.16"
	tagTime := time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC)

	tag := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-010000", release, "nightly", "amd64",
		tagTime, intutil.WithPhase(apitype.PayloadAccepted))

	results, err := query.GetTestFailuresForPayload(dbc.DB, tag.ReleaseTag, release, tagTime)
	require.NoError(t, err)
	assert.Empty(t, results, "payload with no job runs should have no test failures")
}

// ---------------------------------------------------------------------------
// GetLastOSUpgradeByArchitectureAndStream: "When did each arch/stream
// last upgrade the OS?"
// ---------------------------------------------------------------------------

func TestGetLastOSUpgrade_MostRecentPerArchStream(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	// amd64/nightly: two tags with OS upgrades, one without. Most recent upgrade wins.
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-10-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 10, 1, 0, 0, 0, time.UTC),
		intutil.WithPreviousOSVersion("9.2"), intutil.WithCurrentOSVersion("9.3"))
	newerUpgrade := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-13-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 13, 1, 0, 0, 0, time.UTC),
		intutil.WithPreviousOSVersion("9.3"), intutil.WithCurrentOSVersion("9.4"))
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC))

	// arm64/nightly: one upgrade
	arm64Upgrade := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-arm64-2024-06-12-010000", "4.16", "nightly", "arm64",
		time.Date(2024, 6, 12, 1, 0, 0, 0, time.UTC),
		intutil.WithPreviousOSVersion("9.1"), intutil.WithCurrentOSVersion("9.2"))

	results, err := query.GetLastOSUpgradeByArchitectureAndStream(dbc.DB, "4.16")
	require.NoError(t, err)
	require.Len(t, results, 2)

	byKey := make(map[string]models.ReleaseTag)
	for _, r := range results {
		byKey[r.Architecture+"/"+r.Stream] = r
	}

	assert.Equal(t, newerUpgrade.ReleaseTag, byKey["amd64/nightly"].ReleaseTag)
	assert.Equal(t, arm64Upgrade.ReleaseTag, byKey["arm64/nightly"].ReleaseTag)
}

func TestGetLastOSUpgrade_SkipsPayloadsWithoutUpgrade(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	// Tag with OS upgrade
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-10-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 10, 1, 0, 0, 0, time.UTC),
		intutil.WithPreviousOSVersion("9.2"), intutil.WithCurrentOSVersion("9.3"))
	// Tag without OS upgrade (more recent, but no previous_os_version)
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC))

	results, err := query.GetLastOSUpgradeByArchitectureAndStream(dbc.DB, "4.16")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "4.16.0-0.nightly-2024-06-10-010000", results[0].ReleaseTag)
	assert.Equal(t, "9.2", results[0].PreviousOSVersion)
}

func TestGetLastOSUpgrade_NoUpgradesExist(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC))

	results, err := query.GetLastOSUpgradeByArchitectureAndStream(dbc.DB, "4.16")
	require.NoError(t, err)
	assert.Empty(t, results)
}

// ---------------------------------------------------------------------------
// GetLastPayloadTags: "What payloads were produced in the last 14 days?"
// ---------------------------------------------------------------------------

func TestGetLastPayloadTags_RecentPayloads(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	reportEnd := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	// Within window (13 days ago)
	within := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-02-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 2, 1, 0, 0, 0, time.UTC))
	// At boundary (exactly 14 days ago)
	boundary := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-01-120000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC))
	// Recent
	recent := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC))
	// Outside window (15 days ago)
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-05-31-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 5, 31, 1, 0, 0, 0, time.UTC))

	results, err := query.GetLastPayloadTags(dbc.DB, "4.16", "nightly", "amd64", reportEnd)
	require.NoError(t, err)

	require.Len(t, results, 3, "should include recent, within-window, and boundary tags")
	assert.Equal(t, recent.ReleaseTag, results[0].ReleaseTag, "most recent should be first")
	assert.Equal(t, within.ReleaseTag, results[1].ReleaseTag)
	assert.Equal(t, boundary.ReleaseTag, results[2].ReleaseTag, "boundary-inclusive")
}

func TestGetLastPayloadTags_IsolatedByStreamAndArch(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	reportEnd := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	t0 := time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC)

	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-010000", "4.16", "nightly", "amd64", t0)
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.ci-2024-06-14-010000", "4.16", "ci", "amd64", t0)
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-arm64-2024-06-14-010000", "4.16", "nightly", "arm64", t0)

	results, err := query.GetLastPayloadTags(dbc.DB, "4.16", "nightly", "amd64", reportEnd)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "4.16.0-0.nightly-2024-06-14-010000", results[0].ReleaseTag)
}

func TestGetLastPayloadTags_IncludesPayloadsAfterReportEnd(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	reportEnd := time.Date(2024, 6, 13, 0, 0, 0, 0, time.UTC)

	// Within the 14-day lookback window but after reportEnd
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC))
	// Before reportEnd
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-12-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 12, 1, 0, 0, 0, time.UTC))

	results, err := query.GetLastPayloadTags(dbc.DB, "4.16", "nightly", "amd64", reportEnd)
	require.NoError(t, err)
	// GetLastPayloadTags has no upper-bound filter on release_time, so tags after
	// reportEnd within the 14-day window are included in results.
	assert.Len(t, results, 2, "tags after reportEnd are included (no upper-bound filter)")
}

func TestGetLastPayloadTags_MostRecentFirst(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	reportEnd := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-12-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 12, 1, 0, 0, 0, time.UTC))
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC))
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-13-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 13, 1, 0, 0, 0, time.UTC))

	results, err := query.GetLastPayloadTags(dbc.DB, "4.16", "nightly", "amd64", reportEnd)
	require.NoError(t, err)
	require.Len(t, results, 3)

	for i := 0; i < len(results)-1; i++ {
		assert.True(t, results[i].ReleaseTime.After(results[i+1].ReleaseTime) || results[i].ReleaseTime.Equal(results[i+1].ReleaseTime),
			"results should be ordered newest to oldest")
	}
}

// ---------------------------------------------------------------------------
// GetLastPayloadStatus: "What is the current acceptance streak?"
// ---------------------------------------------------------------------------

func createPhaseSequence(t *testing.T, dbc *db.DB, release, stream, arch string, baseTime time.Time, phases []string) []models.ReleaseTag {
	t.Helper()
	tags := make([]models.ReleaseTag, len(phases))
	for i, phase := range phases {
		tagTime := baseTime.Add(time.Duration(-i) * 24 * time.Hour)
		tags[i] = intutil.CreateReleaseTag(t, dbc,
			fmt.Sprintf("%s.0-0.%s-%s", release, stream, tagTime.Format("2006-01-02-150405")),
			release, stream, arch, tagTime,
			intutil.WithPhase(phase))
	}
	return tags
}

func TestGetLastPayloadStatus_PhaseStreaks(t *testing.T) {
	testCases := []struct {
		name          string
		release       string
		stream        string
		arch          string
		phases        []string
		expectedPhase string
		expectedCount int
	}{
		{
			name:    "consecutive rejections",
			release: "4.16", stream: "nightly", arch: "amd64",
			phases: []string{
				apitype.PayloadRejected, apitype.PayloadRejected, apitype.PayloadRejected,
				apitype.PayloadAccepted, apitype.PayloadRejected, apitype.PayloadAccepted,
			},
			expectedPhase: apitype.PayloadRejected,
			expectedCount: 3,
		},
		{
			name:    "single latest accepted",
			release: "4.16", stream: "nightly", arch: "arm64",
			phases:        []string{apitype.PayloadAccepted, apitype.PayloadRejected, apitype.PayloadRejected},
			expectedPhase: apitype.PayloadAccepted,
			expectedCount: 1,
		},
		{
			name:    "entire history same phase",
			release: "4.17", stream: "nightly", arch: "amd64",
			phases: []string{
				apitype.PayloadAccepted, apitype.PayloadAccepted, apitype.PayloadAccepted,
				apitype.PayloadAccepted, apitype.PayloadAccepted,
			},
			expectedPhase: apitype.PayloadAccepted,
			expectedCount: 5,
		},
		{
			name:    "alternating phases",
			release: "4.16", stream: "ci", arch: "amd64",
			phases: []string{
				apitype.PayloadAccepted, apitype.PayloadRejected, apitype.PayloadAccepted,
				apitype.PayloadRejected, apitype.PayloadAccepted,
			},
			expectedPhase: apitype.PayloadAccepted,
			expectedCount: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dbc := intutil.NewTestDB(t, pgContainer)
			baseTime := time.Date(2024, 6, 15, 1, 0, 0, 0, time.UTC)
			reportEnd := time.Date(2024, 6, 16, 0, 0, 0, 0, time.UTC)

			tags := createPhaseSequence(t, dbc, tc.release, tc.stream, tc.arch, baseTime, tc.phases)
			require.Len(t, tags, len(tc.phases))

			phase, count, err := query.GetLastPayloadStatus(dbc.DB, tc.arch, tc.stream, tc.release, reportEnd)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedPhase, phase)
			assert.Equal(t, tc.expectedCount, count)
		})
	}
}

func TestGetLastPayloadStatus_RespectsReportCutoff(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	// Two accepted tags after reportEnd, three rejected before
	reportEnd := time.Date(2024, 6, 13, 0, 0, 0, 0, time.UTC)

	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-15-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 15, 1, 0, 0, 0, time.UTC))
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC))
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-12-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 12, 1, 0, 0, 0, time.UTC), intutil.WithPhase(apitype.PayloadRejected))
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-11-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 11, 1, 0, 0, 0, time.UTC), intutil.WithPhase(apitype.PayloadRejected))
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-10-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 10, 1, 0, 0, 0, time.UTC), intutil.WithPhase(apitype.PayloadRejected))

	phase, count, err := query.GetLastPayloadStatus(dbc.DB, "amd64", "nightly", "4.16", reportEnd)
	require.NoError(t, err)
	assert.Equal(t, apitype.PayloadRejected, phase, "should only see payloads before report end")
	assert.Equal(t, 3, count)
}

// ---------------------------------------------------------------------------
// GetPayloadStreamPhaseCounts: "How many payloads in each acceptance phase?"
// ---------------------------------------------------------------------------

func TestGetPayloadStreamPhaseCounts_CountsByPhase(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	reportEnd := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		intutil.CreateReleaseTag(t, dbc,
			fmt.Sprintf("4.16.0-0.nightly-accepted-%d", i), "4.16", "nightly", "amd64",
			time.Date(2024, 6, 10+i, 1, 0, 0, 0, time.UTC))
	}
	for i := 0; i < 2; i++ {
		intutil.CreateReleaseTag(t, dbc,
			fmt.Sprintf("4.16.0-0.nightly-rejected-%d", i), "4.16", "nightly", "amd64",
			time.Date(2024, 6, 10+i, 2, 0, 0, 0, time.UTC),
			intutil.WithPhase(apitype.PayloadRejected))
	}

	results, err := query.GetPayloadStreamPhaseCounts(dbc.DB, "4.16", "amd64", "nightly", nil, reportEnd)
	require.NoError(t, err)

	byPhase := make(map[string]int)
	for _, r := range results {
		byPhase[r.Phase] = r.Count
	}

	assert.Equal(t, 3, byPhase[apitype.PayloadAccepted])
	assert.Equal(t, 2, byPhase[apitype.PayloadRejected])
}

func TestGetPayloadStreamPhaseCounts_WithStartTime(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	reportEnd := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	since := time.Date(2024, 6, 12, 0, 0, 0, 0, time.UTC)

	// Before since
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-old", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 10, 1, 0, 0, 0, time.UTC))
	// After since
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-new-1", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 13, 1, 0, 0, 0, time.UTC))
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-new-2", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC), intutil.WithPhase(apitype.PayloadRejected))

	results, err := query.GetPayloadStreamPhaseCounts(dbc.DB, "4.16", "amd64", "nightly", &since, reportEnd)
	require.NoError(t, err)

	total := 0
	for _, r := range results {
		total += r.Count
	}
	assert.Equal(t, 2, total, "only payloads after since should be counted")
}

func TestGetPayloadStreamPhaseCounts_ExcludesPayloadsAfterReportEnd(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	reportEnd := time.Date(2024, 6, 13, 0, 0, 0, 0, time.UTC)

	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-before", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 12, 1, 0, 0, 0, time.UTC))
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-after", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC))

	results, err := query.GetPayloadStreamPhaseCounts(dbc.DB, "4.16", "amd64", "nightly", nil, reportEnd)
	require.NoError(t, err)

	total := 0
	for _, r := range results {
		total += r.Count
	}
	assert.Equal(t, 1, total, "only payloads before report end should be counted")
}

// ---------------------------------------------------------------------------
// GetPayloadAcceptanceStatistics: "How long between accepted payloads?"
// ---------------------------------------------------------------------------

func TestGetPayloadAcceptanceStatistics_ComputesTimeBetweenAccepted(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	reportEnd := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	// Four accepted payloads with known gaps:
	// t0 -> t1: 2 hours (7200s)
	// t1 -> t2: 3 hours (10800s)
	// t2 -> t3: 6 hours (21600s)
	// min=7200, mean=13200, max=21600
	t0 := time.Date(2024, 6, 14, 0, 0, 0, 0, time.UTC)
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-000000", "4.16", "nightly", "amd64", t0)
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-020000", "4.16", "nightly", "amd64", t0.Add(2*time.Hour))
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-050000", "4.16", "nightly", "amd64", t0.Add(5*time.Hour))
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-110000", "4.16", "nightly", "amd64", t0.Add(11*time.Hour))

	stats, err := query.GetPayloadAcceptanceStatistics(dbc.DB, "4.16", "amd64", "nightly", nil, reportEnd)
	require.NoError(t, err)

	assert.Equal(t, int64(7200), stats.MinSecondsBetween)
	assert.Equal(t, int64(13200), stats.MeanSecondsBetween)
	assert.Equal(t, int64(21600), stats.MaxSecondsBetween)
}

func TestGetPayloadAcceptanceStatistics_IgnoresRejectedPayloads(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	reportEnd := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	// Two accepted payloads with a 4-hour gap. Rejected payloads between them should not affect stats.
	t0 := time.Date(2024, 6, 14, 0, 0, 0, 0, time.UTC)
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-000000", "4.16", "nightly", "amd64", t0)
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-020000", "4.16", "nightly", "amd64", t0.Add(2*time.Hour),
		intutil.WithPhase(apitype.PayloadRejected))
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-040000", "4.16", "nightly", "amd64", t0.Add(4*time.Hour))

	stats, err := query.GetPayloadAcceptanceStatistics(dbc.DB, "4.16", "amd64", "nightly", nil, reportEnd)
	require.NoError(t, err)

	assert.Equal(t, int64(14400), stats.MinSecondsBetween, "gap should be 4 hours = 14400s")
	assert.Equal(t, int64(14400), stats.MeanSecondsBetween)
	assert.Equal(t, int64(14400), stats.MaxSecondsBetween)
}

func TestGetPayloadAcceptanceStatistics_WithStartTime(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	reportEnd := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	since := time.Date(2024, 6, 13, 0, 0, 0, 0, time.UTC)

	// Before since: gap of 2 hours
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-12-000000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 12, 0, 0, 0, 0, time.UTC))
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-12-020000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 12, 2, 0, 0, 0, time.UTC))

	// After since: gap of 6 hours
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-000000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 14, 0, 0, 0, 0, time.UTC))
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-060000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 14, 6, 0, 0, 0, time.UTC))

	stats, err := query.GetPayloadAcceptanceStatistics(dbc.DB, "4.16", "amd64", "nightly", &since, reportEnd)
	require.NoError(t, err)

	assert.Equal(t, int64(21600), stats.MinSecondsBetween, "only the 6-hour gap after since should count")
	assert.Equal(t, int64(21600), stats.MeanSecondsBetween)
	assert.Equal(t, int64(21600), stats.MaxSecondsBetween)
}

func TestGetPayloadAcceptanceStatistics_SingleAcceptedPayload(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	reportEnd := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-000000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 14, 0, 0, 0, 0, time.UTC))

	stats, err := query.GetPayloadAcceptanceStatistics(dbc.DB, "4.16", "amd64", "nightly", nil, reportEnd)
	require.NoError(t, err)

	assert.Equal(t, int64(0), stats.MinSecondsBetween, "no gap to measure with a single payload")
	assert.Equal(t, int64(0), stats.MeanSecondsBetween)
	assert.Equal(t, int64(0), stats.MaxSecondsBetween)
}

// ---------------------------------------------------------------------------
// GetReleaseTag: "Fetch a specific payload by tag name"
// ---------------------------------------------------------------------------

func TestGetReleaseTag_Found(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	created := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC), intutil.WithPhase(apitype.PayloadRejected))

	result, err := query.GetReleaseTag(dbc.DB, "4.16.0-0.nightly-2024-06-14-010000")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, created.ID, result.ID)
	assert.Equal(t, "4.16", result.Release)
	assert.Equal(t, "nightly", result.Stream)
	assert.Equal(t, "amd64", result.Architecture)
	assert.Equal(t, apitype.PayloadRejected, result.Phase)
}

func TestGetReleaseTag_NotFound(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	result, err := query.GetReleaseTag(dbc.DB, "nonexistent-tag")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	assert.Nil(t, result)
}

// ---------------------------------------------------------------------------
// GetPreviousPayload: "What payload came immediately before this one?"
// ---------------------------------------------------------------------------

func TestGetPreviousPayload_FindsPredecessor(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-10-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 10, 1, 0, 0, 0, time.UTC))
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-11-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 11, 1, 0, 0, 0, time.UTC))
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-12-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 12, 1, 0, 0, 0, time.UTC))

	result, err := query.GetPreviousPayload(dbc.DB, "4.16.0-0.nightly-2024-06-12-010000")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "4.16.0-0.nightly-2024-06-11-010000", result.ReleaseTag,
		"should return the immediately preceding payload")
}

func TestGetPreviousPayload_FirstPayloadHasNoPredecessor(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-10-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 10, 1, 0, 0, 0, time.UTC))

	_, err := query.GetPreviousPayload(dbc.DB, "4.16.0-0.nightly-2024-06-10-010000")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestGetPreviousPayload_OnlySameStreamAndArch(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	// Earlier tag exists in a different stream
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.ci-2024-06-09-010000", "4.16", "ci", "amd64",
		time.Date(2024, 6, 9, 1, 0, 0, 0, time.UTC))
	// The target tag in nightly
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-10-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 10, 1, 0, 0, 0, time.UTC))

	_, err := query.GetPreviousPayload(dbc.DB, "4.16.0-0.nightly-2024-06-10-010000")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound,
		"should not find a predecessor in a different stream")
}

func TestGetPreviousPayload_NonexistentTarget(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	_, err := query.GetPreviousPayload(dbc.DB, "nonexistent-tag")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestGetPreviousPayload_OnlySameArchitecture(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	// Earlier tag in same stream but different architecture
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-09-010000", "4.16", "nightly", "arm64",
		time.Date(2024, 6, 9, 1, 0, 0, 0, time.UTC))
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-10-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 10, 1, 0, 0, 0, time.UTC))

	_, err := query.GetPreviousPayload(dbc.DB, "4.16.0-0.nightly-2024-06-10-010000")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound,
		"should not find a predecessor in a different architecture")
}

// ---------------------------------------------------------------------------
// GetTestFailuresForPayloadStream: "What tests are failing across a stream's
// recent payloads?"
// ---------------------------------------------------------------------------

type streamFailureRow struct {
	TestID       uint
	Name         string
	FailureCount int
	ReleaseTags  pq.StringArray `gorm:"type:text[]"`
	JobNames     pq.StringArray `gorm:"type:text[]"`
	JobRunURLs   pq.StringArray `gorm:"type:text[]"`
}

func scanStreamFailures(t *testing.T, dbc *db.DB, subquery *gorm.DB) []streamFailureRow {
	t.Helper()
	var rows []streamFailureRow
	require.NoError(t, dbc.DB.Table("(?) as test_failures", subquery).Scan(&rows).Error)
	return rows
}

func TestGetTestFailuresForPayloadStream_BasicAggregation(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	f := setupFailureFixture(t, dbc)

	subquery := query.GetTestFailuresForPayloadStream(dbc.DB, f.release, f.stream, f.arch, f.reportEnd, "")
	rows := scanStreamFailures(t, dbc, subquery)

	require.Len(t, rows, 2)
	byName := make(map[string]streamFailureRow)
	for _, r := range rows {
		byName[r.Name] = r
	}

	rowA := byName[f.testA.Name]
	assert.Equal(t, 1, rowA.FailureCount)
	require.Len(t, rowA.ReleaseTags, 1)
	assert.Equal(t, f.tag.ReleaseTag, rowA.ReleaseTags[0])
	require.Len(t, rowA.JobNames, 1)
	assert.Equal(t, f.job.Name, rowA.JobNames[0])
	require.Len(t, rowA.JobRunURLs, 1)
	assert.Equal(t, "https://prow/run/1", rowA.JobRunURLs[0])
}

func TestGetTestFailuresForPayloadStream_OnlyBlockingJobs(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	f := setupFailureFixture(t, dbc)

	const informingURL = "https://prow/run/informing"
	informingJob := intutil.CreateProwJob(t, dbc, "periodic-ci-e2e-gcp-informing-4.16", f.release, nil)
	informingRunTime := f.runTime.Add(time.Hour)
	informingRun := intutil.CreateProwJobRun(t, dbc, informingJob.ID, f.release, informingRunTime, false, "F", intutil.WithURL(informingURL))
	intutil.CreateReleaseJobRun(t, dbc, f.tag.ID, informingRun.ID, informingJob.Name, "Informing", "Failed", informingURL)

	informingOnlyTest := intutil.CreateTest(t, dbc, "test-informing-only")
	intutil.CreateProwJobRunTest(t, dbc, informingRun.ID, informingJob.ID, informingOnlyTest.ID, f.release, informingRunTime, int(v1.TestStatusFailure))

	subquery := query.GetTestFailuresForPayloadStream(dbc.DB, f.release, f.stream, f.arch, f.reportEnd, "")
	rows := scanStreamFailures(t, dbc, subquery)

	names := sets.New[string]()
	for _, r := range rows {
		names.Insert(r.Name)
	}
	assert.True(t, names.Has(f.testA.Name), "blocking job failures should appear")
	assert.False(t, names.Has("test-informing-only"), "informing job failures should be excluded")
}

func TestGetTestFailuresForPayloadStream_ExcludesTestByName(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	f := setupFailureFixture(t, dbc)

	subquery := query.GetTestFailuresForPayloadStream(dbc.DB, f.release, f.stream, f.arch, f.reportEnd, f.testA.Name)
	rows := scanStreamFailures(t, dbc, subquery)

	require.Len(t, rows, 1, "excluded test should not appear")
	assert.Equal(t, f.testB.Name, rows[0].Name)
}

func TestGetTestFailuresForPayloadStream_FourteenDayWindow(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	release := "4.16"
	stream := "nightly"
	arch := "amd64"
	reportEnd := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	job := intutil.CreateProwJob(t, dbc, "periodic-ci-e2e-aws-4.16", release, nil)
	test := intutil.CreateTest(t, dbc, "test-window-check")

	// In-window tag (1 day before reportEnd)
	const inWindowURL = "https://prow/run/in-window"
	inWindowTag := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-010000", release, stream, arch,
		time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC), intutil.WithPhase(apitype.PayloadRejected))
	inWindowRunTime := time.Date(2024, 6, 14, 2, 0, 0, 0, time.UTC)
	inWindowRun := intutil.CreateProwJobRun(t, dbc, job.ID, release, inWindowRunTime, false, "F", intutil.WithURL(inWindowURL))
	intutil.CreateReleaseJobRun(t, dbc, inWindowTag.ID, inWindowRun.ID, job.Name, "Blocking", "Failed", inWindowURL)
	intutil.CreateProwJobRunTest(t, dbc, inWindowRun.ID, job.ID, test.ID, release, inWindowRunTime, int(v1.TestStatusFailure))

	// Out-of-window tag (20 days before reportEnd)
	const outOfWindowURL = "https://prow/run/out-of-window"
	outOfWindowTag := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-05-26-010000", release, stream, arch,
		time.Date(2024, 5, 26, 1, 0, 0, 0, time.UTC), intutil.WithPhase(apitype.PayloadRejected))
	outOfWindowRunTime := time.Date(2024, 5, 26, 2, 0, 0, 0, time.UTC)
	outOfWindowRun := intutil.CreateProwJobRun(t, dbc, job.ID, release, outOfWindowRunTime, false, "F", intutil.WithURL(outOfWindowURL))
	intutil.CreateReleaseJobRun(t, dbc, outOfWindowTag.ID, outOfWindowRun.ID, job.Name, "Blocking", "Failed", outOfWindowURL)
	intutil.CreateProwJobRunTest(t, dbc, outOfWindowRun.ID, job.ID, test.ID, release, outOfWindowRunTime, int(v1.TestStatusFailure))

	subquery := query.GetTestFailuresForPayloadStream(dbc.DB, release, stream, arch, reportEnd, "")
	rows := scanStreamFailures(t, dbc, subquery)

	require.Len(t, rows, 1, "only in-window failures should appear")
	assert.Equal(t, 1, rows[0].FailureCount, "only the in-window failure should be counted")
	require.Len(t, rows[0].ReleaseTags, 1)
	assert.Equal(t, inWindowTag.ReleaseTag, rows[0].ReleaseTags[0])
}

func TestGetTestFailuresForPayloadStream_AggregatesAcrossPayloads(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	release := "4.16"
	stream := "nightly"
	arch := "amd64"
	reportEnd := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	job1 := intutil.CreateProwJob(t, dbc, "periodic-ci-e2e-aws-4.16", release, nil)
	job2 := intutil.CreateProwJob(t, dbc, "periodic-ci-e2e-gcp-4.16", release, nil)
	test := intutil.CreateTest(t, dbc, "test-aggregated")

	// First rejected payload with job1 failing
	const agg1URL = "https://prow/run/agg-1"
	tag1 := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-010000", release, stream, arch,
		time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC), intutil.WithPhase(apitype.PayloadRejected))
	run1Time := time.Date(2024, 6, 14, 2, 0, 0, 0, time.UTC)
	run1 := intutil.CreateProwJobRun(t, dbc, job1.ID, release, run1Time, false, "F", intutil.WithURL(agg1URL))
	intutil.CreateReleaseJobRun(t, dbc, tag1.ID, run1.ID, job1.Name, "Blocking", "Failed", agg1URL)
	intutil.CreateProwJobRunTest(t, dbc, run1.ID, job1.ID, test.ID, release, run1Time, int(v1.TestStatusFailure))

	// Second rejected payload with job2 failing
	const agg2URL = "https://prow/run/agg-2"
	tag2 := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-13-010000", release, stream, arch,
		time.Date(2024, 6, 13, 1, 0, 0, 0, time.UTC), intutil.WithPhase(apitype.PayloadRejected))
	run2Time := time.Date(2024, 6, 13, 2, 0, 0, 0, time.UTC)
	run2 := intutil.CreateProwJobRun(t, dbc, job2.ID, release, run2Time, false, "F", intutil.WithURL(agg2URL))
	intutil.CreateReleaseJobRun(t, dbc, tag2.ID, run2.ID, job2.Name, "Blocking", "Failed", agg2URL)
	intutil.CreateProwJobRunTest(t, dbc, run2.ID, job2.ID, test.ID, release, run2Time, int(v1.TestStatusFailure))

	subquery := query.GetTestFailuresForPayloadStream(dbc.DB, release, stream, arch, reportEnd, "")
	rows := scanStreamFailures(t, dbc, subquery)

	require.Len(t, rows, 1, "same test across payloads should be aggregated into one row")
	assert.Equal(t, 2, rows[0].FailureCount, "failure count should sum across payloads")
	assert.Len(t, rows[0].ReleaseTags, 2, "should have entries for both payload tags")
	assert.Len(t, rows[0].JobNames, 2)
	assert.Len(t, rows[0].JobRunURLs, 2)

	tags := sets.New[string](rows[0].ReleaseTags...)
	assert.True(t, tags.Has(tag1.ReleaseTag))
	assert.True(t, tags.Has(tag2.ReleaseTag))
}

func TestGetTestFailuresForPayloadStream_ExcludesPassingTests(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	f := setupFailureFixture(t, dbc)

	passingTest := intutil.CreateTest(t, dbc, "test-passes")
	intutil.CreateProwJobRunTest(t, dbc, f.run.ID, f.job.ID, passingTest.ID, f.release, f.runTime, int(v1.TestStatusSuccess))

	subquery := query.GetTestFailuresForPayloadStream(dbc.DB, f.release, f.stream, f.arch, f.reportEnd, "")
	rows := scanStreamFailures(t, dbc, subquery)

	names := sets.New[string]()
	for _, r := range rows {
		names.Insert(r.Name)
	}
	assert.False(t, names.Has("test-passes"), "passing tests should not appear")
	assert.Equal(t, 2, names.Len(), "only the two failing tests should appear")
}

func TestGetTestFailuresForPayloadStream_IsolatedByStreamAndArch(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)
	f := setupFailureFixture(t, dbc)

	// Add a ci-stream tag with a different test failure
	const ciURL = "https://prow/run/ci-stream"
	ciTag := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.ci-2024-06-14-010000", f.release, "ci", f.arch,
		f.tagTime, intutil.WithPhase(apitype.PayloadRejected))
	ciRunTime := f.runTime.Add(2 * time.Hour)
	ciJob := intutil.CreateProwJob(t, dbc, "periodic-ci-e2e-ci-stream-4.16", f.release, nil)
	ciRun := intutil.CreateProwJobRun(t, dbc, ciJob.ID, f.release, ciRunTime, false, "F", intutil.WithURL(ciURL))
	intutil.CreateReleaseJobRun(t, dbc, ciTag.ID, ciRun.ID, ciJob.Name, "Blocking", "Failed", ciURL)
	ciTest := intutil.CreateTest(t, dbc, "test-ci-only")
	intutil.CreateProwJobRunTest(t, dbc, ciRun.ID, ciJob.ID, ciTest.ID, f.release, ciRunTime, int(v1.TestStatusFailure))

	subquery := query.GetTestFailuresForPayloadStream(dbc.DB, f.release, f.stream, f.arch, f.reportEnd, "")
	rows := scanStreamFailures(t, dbc, subquery)

	names := sets.New[string]()
	for _, r := range rows {
		names.Insert(r.Name)
	}
	assert.False(t, names.Has("test-ci-only"), "ci-stream failures should not appear in nightly results")
	assert.True(t, names.Has(f.testA.Name), "nightly failures should appear")
}

func TestGetTestFailuresForPayloadStream_ExcludesSucceededJobs(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	release := "4.16"
	stream := "nightly"
	arch := "amd64"
	reportEnd := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	tagTime := time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC)
	runTime := time.Date(2024, 6, 14, 2, 0, 0, 0, time.UTC)

	const runURL = "https://prow/run/succeeded"
	tag := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-010000", release, stream, arch,
		tagTime, intutil.WithPhase(apitype.PayloadAccepted))
	job := intutil.CreateProwJob(t, dbc, "periodic-ci-e2e-aws-4.16", release, nil)
	run := intutil.CreateProwJobRun(t, dbc, job.ID, release, runTime, true, "S", intutil.WithURL(runURL))
	intutil.CreateReleaseJobRun(t, dbc, tag.ID, run.ID, job.Name, "Blocking", "Succeeded", runURL)

	test := intutil.CreateTest(t, dbc, "test-in-succeeded-job")
	intutil.CreateProwJobRunTest(t, dbc, run.ID, job.ID, test.ID, release, runTime, int(v1.TestStatusFailure))

	subquery := query.GetTestFailuresForPayloadStream(dbc.DB, release, stream, arch, reportEnd, "")
	rows := scanStreamFailures(t, dbc, subquery)

	assert.Empty(t, rows, "test failures from succeeded blocking jobs should not appear")
}

// ---------------------------------------------------------------------------
// GetTestFailuresForPayload: payload isolation
// ---------------------------------------------------------------------------

func TestGetTestFailuresForPayload_ExcludesFailuresFromOtherPayloads(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	release := "4.16"
	stream := "nightly"
	arch := "amd64"

	tag1Time := time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC)
	tag1 := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-010000", release, stream, arch,
		tag1Time, intutil.WithPhase(apitype.PayloadRejected))

	tag2Time := time.Date(2024, 6, 13, 1, 0, 0, 0, time.UTC)
	tag2 := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-13-010000", release, stream, arch,
		tag2Time, intutil.WithPhase(apitype.PayloadRejected))

	job := intutil.CreateProwJob(t, dbc, "periodic-ci-e2e-aws-4.16", release, nil)

	const run1URL = "https://prow/run/tag1"
	run1Time := tag1Time.Add(time.Hour)
	run1 := intutil.CreateProwJobRun(t, dbc, job.ID, release, run1Time, false, "F", intutil.WithURL(run1URL))
	intutil.CreateReleaseJobRun(t, dbc, tag1.ID, run1.ID, job.Name, "Blocking", "Failed", run1URL)

	const run2URL = "https://prow/run/tag2"
	run2Time := tag2Time.Add(time.Hour)
	run2 := intutil.CreateProwJobRun(t, dbc, job.ID, release, run2Time, false, "F", intutil.WithURL(run2URL))
	intutil.CreateReleaseJobRun(t, dbc, tag2.ID, run2.ID, job.Name, "Blocking", "Failed", run2URL)

	testTag1Only := intutil.CreateTest(t, dbc, "test-tag1-only")
	testTag2Only := intutil.CreateTest(t, dbc, "test-tag2-only")

	intutil.CreateProwJobRunTest(t, dbc, run1.ID, job.ID, testTag1Only.ID, release, run1Time, int(v1.TestStatusFailure))
	intutil.CreateProwJobRunTest(t, dbc, run2.ID, job.ID, testTag2Only.ID, release, run2Time, int(v1.TestStatusFailure))

	results, err := query.GetTestFailuresForPayload(dbc.DB, tag1.ReleaseTag, release, tag1Time)
	require.NoError(t, err)

	names := sets.New[string]()
	for _, r := range results {
		names.Insert(r.Name)
	}
	assert.True(t, names.Has("test-tag1-only"), "failure from queried payload should appear")
	assert.False(t, names.Has("test-tag2-only"), "failure from other payload should not appear")
}

// ---------------------------------------------------------------------------
// Boundary conditions at exact reportEnd
// ---------------------------------------------------------------------------

func TestGetLastAccepted_ExcludesPayloadAtExactReportEnd(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	reportEnd := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-15-120000", "4.16", "nightly", "amd64",
		reportEnd)

	results, err := query.GetLastAcceptedByArchitectureAndStream(dbc.DB, "4.16", reportEnd)
	require.NoError(t, err)
	assert.Empty(t, results, "tag at exact reportEnd should be excluded (release_time < reportEnd)")
}

func TestGetPayloadStreamPhaseCounts_ExcludesPayloadAtExactReportEnd(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	reportEnd := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC))
	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-15-120000", "4.16", "nightly", "amd64",
		reportEnd)

	counts, err := query.GetPayloadStreamPhaseCounts(dbc.DB, "4.16", "amd64", "nightly", nil, reportEnd)
	require.NoError(t, err)

	total := 0
	for _, c := range counts {
		total += c.Count
	}
	assert.Equal(t, 1, total, "tag at exact reportEnd should be excluded from count")
}

func TestGetPayloadAcceptanceStatistics_ExcludesPayloadAtExactReportEnd(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	reportEnd := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-15-120000", "4.16", "nightly", "amd64",
		reportEnd)

	stats, err := query.GetPayloadAcceptanceStatistics(dbc.DB, "4.16", "amd64", "nightly", nil, reportEnd)
	require.NoError(t, err)
	assert.Equal(t, int64(0), stats.MeanSecondsBetween,
		"tag at exact reportEnd should be excluded, leaving no data for stats")
}

func TestGetLastPayloadStatus_IncludesPayloadAtExactReportEnd(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	reportEnd := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-15-120000", "4.16", "nightly", "amd64",
		reportEnd, intutil.WithPhase(apitype.PayloadRejected))

	phase, count, err := query.GetLastPayloadStatus(dbc.DB, "amd64", "nightly", "4.16", reportEnd)
	require.NoError(t, err)
	assert.Equal(t, apitype.PayloadRejected, phase,
		"tag at exact reportEnd should be included (release_time <= reportEnd)")
	assert.Equal(t, 1, count)
}

// ---------------------------------------------------------------------------
// Release isolation
// ---------------------------------------------------------------------------

func TestGetLastAccepted_IsolatedByRelease(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	reportEnd := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC))
	intutil.CreateReleaseTag(t, dbc, "4.17.0-0.nightly-2024-06-14-010000", "4.17", "nightly", "amd64",
		time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC))

	results, err := query.GetLastAcceptedByArchitectureAndStream(dbc.DB, "4.16", reportEnd)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "4.16", results[0].Release,
		"only tags from the queried release should be returned")
}

// ---------------------------------------------------------------------------
// Empty result edge cases
// ---------------------------------------------------------------------------

func TestGetPayloadStreamPhaseCounts_NoMatchingPayloads(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	reportEnd := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	counts, err := query.GetPayloadStreamPhaseCounts(dbc.DB, "4.99", "amd64", "nightly", nil, reportEnd)
	require.NoError(t, err)
	assert.Empty(t, counts, "no tags should produce empty counts")
}

func TestGetPayloadAcceptanceStatistics_NoAcceptedPayloads(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	reportEnd := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC), intutil.WithPhase(apitype.PayloadRejected))

	stats, err := query.GetPayloadAcceptanceStatistics(dbc.DB, "4.16", "amd64", "nightly", nil, reportEnd)
	require.NoError(t, err)
	assert.Equal(t, int64(0), stats.MeanSecondsBetween)
	assert.Equal(t, int64(0), stats.MinSecondsBetween)
	assert.Equal(t, int64(0), stats.MaxSecondsBetween)
}

func TestGetLastPayloadStatus_NoPayloads(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	reportEnd := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	phase, count, err := query.GetLastPayloadStatus(dbc.DB, "amd64", "nightly", "4.99", reportEnd)
	require.NoError(t, err)
	assert.Equal(t, "", phase, "no tags should produce empty phase")
	assert.Equal(t, 0, count, "no tags should produce zero count")
}

// ---------------------------------------------------------------------------
// GetPayloadDiff: column coverage
// ---------------------------------------------------------------------------

func TestGetPayloadDiff_ReturnsAllPRFields(t *testing.T) {
	dbc := intutil.NewTestDB(t, pgContainer)

	tag1 := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-14-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 14, 1, 0, 0, 0, time.UTC))
	tag2 := intutil.CreateReleaseTag(t, dbc, "4.16.0-0.nightly-2024-06-15-010000", "4.16", "nightly", "amd64",
		time.Date(2024, 6, 15, 1, 0, 0, 0, time.UTC))

	pr := intutil.CreateReleasePullRequest(t, dbc, "https://github.com/openshift/origin/pull/999",
		"openshift/origin", "Add TLS validation for API routes",
		intutil.WithPullRequestID("999"), intutil.WithBugURL("https://bugzilla.redhat.com/12345"))

	intutil.LinkReleaseTagPullRequests(t, dbc, &tag2, pr)

	results, err := query.GetPayloadDiff(dbc.DB, tag1.ReleaseTag, tag2.ReleaseTag)
	require.NoError(t, err)
	require.Len(t, results, 1)

	got := results[0]
	assert.Equal(t, "https://github.com/openshift/origin/pull/999", got.URL)
	assert.Equal(t, "999", got.PullRequestID)
	assert.Equal(t, "openshift/origin", got.Name)
	assert.Equal(t, "Add TLS validation for API routes", got.Description)
	assert.Equal(t, "https://bugzilla.redhat.com/12345", got.BugURL)
}

package componentreadiness

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/crstatus"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
)

func TestMissingMultiTestStatusKeys(t *testing.T) {
	present := crtest.KeyWithVariants{TestID: "present", Variants: map[string]string{"Platform": "aws"}}
	missing := crtest.KeyWithVariants{TestID: "missing", Variants: map[string]string{"Platform": "gcp"}}

	missingKeys := missingMultiTestStatusKeys(crstatus.TestJobRunStatuses{
		SampleStatus: map[string][]crstatus.TestDetailsSummary{
			"job": {{TestKeyStr: present.Encode()}},
		},
	}, []reqopts.TestIdentification{
		{TestID: present.TestID, RequestedVariants: present.Variants},
		{TestID: missing.TestID, RequestedVariants: missing.Variants},
	}, sets.New[string]("Platform"))

	assert.Equal(t, []string{missing.Encode()}, missingKeys)
}

func TestGetCompleteMultiTestJobRunStatusesGroupsRequestedVariantsByDBGroupBy(t *testing.T) {
	groupedKey := crtest.KeyWithVariants{
		TestID:   "shared-test",
		Variants: map[string]string{"Platform": "aws", "Network": "ovn"},
	}
	statuses := crstatus.TestJobRunStatuses{
		SampleStatus: map[string][]crstatus.TestDetailsSummary{
			"job": {{TestKeyStr: groupedKey.Encode()}},
		},
	}
	generator := ComponentReportGenerator{
		ReqOptions: reqopts.RequestOptions{
			TestOptions: reqopts.TestOptions{TestIDOptions: []reqopts.TestIdentification{{
				TestID: "shared-test",
				RequestedVariants: map[string]string{
					"Platform":     "aws",
					"Network":      "ovn",
					"Architecture": "amd64",
				},
			}}},
			AnalysisOptions: reqopts.AnalysisOptions{VariantOption: reqopts.Variants{DBGroupBy: sets.New[string]("Platform", "Network")}},
		},
		jobRunTestStatusFetcher: func(*ComponentReportGenerator, context.Context) (crstatus.TestJobRunStatuses, []error) {
			return statuses, nil
		},
		multiTestStatusBackoff: wait.Backoff{Steps: 1, Duration: time.Millisecond},
	}

	got, errs := generator.getCompleteMultiTestJobRunStatuses(context.Background())
	require.Empty(t, errs)
	assert.Equal(t, statuses, got)
}

func TestSplitTestJobRunStatusesByTestKey(t *testing.T) {
	awsKey := crtest.KeyWithVariants{TestID: "shared-test", Variants: map[string]string{"Platform": "aws"}}
	gcpKey := crtest.KeyWithVariants{TestID: "shared-test", Variants: map[string]string{"Platform": "gcp"}}
	generatedAt := time.Now()
	awsSummary := crstatus.TestDetailsSummary{TestKeyStr: awsKey.Encode(), ProwJob: "aws-job"}
	gcpSummary := crstatus.TestDetailsSummary{TestKeyStr: gcpKey.Encode(), ProwJob: "gcp-job"}

	statusesByKey := splitTestJobRunStatusesByTestKey(crstatus.TestJobRunStatuses{
		BaseStatus:         map[string][]crstatus.TestDetailsSummary{"job": {awsSummary, gcpSummary}},
		BaseOverrideStatus: map[string][]crstatus.TestDetailsSummary{"override-job": {gcpSummary}},
		SampleStatus:       map[string][]crstatus.TestDetailsSummary{"job": {gcpSummary}},
		GeneratedAt:        &generatedAt,
	}, []reqopts.TestIdentification{
		{TestID: awsKey.TestID, RequestedVariants: awsKey.Variants},
		{TestID: gcpKey.TestID, RequestedVariants: gcpKey.Variants},
	}, sets.New[string]("Platform"), false)

	awsStatuses, ok := statusesByKey[awsKey.Encode()]
	require.True(t, ok)
	assert.Equal(t, []crstatus.TestDetailsSummary{awsSummary}, awsStatuses.BaseStatus["job"])
	assert.Empty(t, awsStatuses.BaseOverrideStatus)
	assert.Empty(t, awsStatuses.SampleStatus)
	assert.Same(t, &generatedAt, awsStatuses.GeneratedAt)

	gcpStatuses, ok := statusesByKey[gcpKey.Encode()]
	require.True(t, ok)
	assert.Equal(t, []crstatus.TestDetailsSummary{gcpSummary}, gcpStatuses.BaseStatus["job"])
	assert.Equal(t, []crstatus.TestDetailsSummary{gcpSummary}, gcpStatuses.BaseOverrideStatus["override-job"])
	assert.Equal(t, []crstatus.TestDetailsSummary{gcpSummary}, gcpStatuses.SampleStatus["job"])
}

func TestSplitTestJobRunStatusesByTestKeyProjectsRequestedVariants(t *testing.T) {
	groupedKey := crtest.KeyWithVariants{
		TestID:   "shared-test",
		Variants: map[string]string{"Platform": "aws", "Network": "ovn"},
	}
	requested := reqopts.TestIdentification{
		TestID: groupedKey.TestID,
		RequestedVariants: map[string]string{
			"Platform":     "aws",
			"Network":      "ovn",
			"Architecture": "amd64",
		},
	}
	summary := crstatus.TestDetailsSummary{TestKeyStr: groupedKey.Encode()}

	statusesByKey := splitTestJobRunStatusesByTestKey(
		crstatus.TestJobRunStatuses{SampleStatus: map[string][]crstatus.TestDetailsSummary{"job": {summary}}},
		[]reqopts.TestIdentification{requested},
		sets.New[string]("Platform", "Network"),
		false,
	)

	got, ok := statusesByKey[groupedKey.Encode()]
	require.True(t, ok)
	assert.Equal(t, []crstatus.TestDetailsSummary{summary}, got.SampleStatus["job"])
	assert.Len(t, statusesByKey, 1, "status map should use the provider's grouped key")
}

func TestCollidingTestDetailsStatusKeys(t *testing.T) {
	dbGroupBy := sets.New[string]("Platform", "Network")
	base := reqopts.TestIdentification{
		TestID: "shared-test",
		RequestedVariants: map[string]string{
			"Platform":     "aws",
			"Network":      "ovn",
			"Architecture": "amd64",
		},
	}
	otherArchitecture := reqopts.TestIdentification{
		TestID: "shared-test",
		RequestedVariants: map[string]string{
			"Platform":     "aws",
			"Network":      "ovn",
			"Architecture": "arm64",
		},
	}
	otherPlatform := reqopts.TestIdentification{
		TestID: "shared-test",
		RequestedVariants: map[string]string{
			"Platform":     "gcp",
			"Network":      "ovn",
			"Architecture": "amd64",
		},
	}

	wantKey := crtest.KeyWithVariants{TestID: "shared-test", Variants: map[string]string{"Platform": "aws", "Network": "ovn"}}
	assert.Equal(t, sets.New(wantKey.Encode()), collidingTestDetailsStatusKeys([]reqopts.TestIdentification{base, otherArchitecture, otherPlatform}, dbGroupBy))
	assert.Empty(t, collidingTestDetailsStatusKeys([]reqopts.TestIdentification{base, base}, dbGroupBy))
	assert.Empty(t, collidingTestDetailsStatusKeys([]reqopts.TestIdentification{base, otherPlatform}, dbGroupBy))
}

func TestSplitTestJobRunStatusesByTestKeyCreatesEmptyRequestedEntry(t *testing.T) {
	key := crtest.KeyWithVariants{TestID: "missing", Variants: map[string]string{"Platform": "aws"}}
	statusesByKey := splitTestJobRunStatusesByTestKey(crstatus.TestJobRunStatuses{}, []reqopts.TestIdentification{
		{TestID: key.TestID, RequestedVariants: key.Variants},
	}, sets.New[string]("Platform"), true)

	statuses, ok := statusesByKey[key.Encode()]
	require.True(t, ok)
	assert.Empty(t, statuses.BaseStatus)
	assert.Empty(t, statuses.BaseOverrideStatus)
	assert.Empty(t, statuses.SampleStatus)
}

func TestGenerateTestDetailsReportMultiTestBatchesNonCollidingKeys(t *testing.T) {
	identifications := []reqopts.TestIdentification{
		{TestID: "shared-test", RequestedVariants: map[string]string{"Platform": "aws", "Architecture": "amd64"}},
		{TestID: "other-test", RequestedVariants: map[string]string{"Platform": "aws"}},
		{TestID: "shared-test", RequestedVariants: map[string]string{"Platform": "aws", "Architecture": "arm64"}},
		{TestID: "shared-test", RequestedVariants: map[string]string{"Platform": "gcp"}},
	}
	wantErr := errors.New("status query failed")
	var queries [][]reqopts.TestIdentification
	generator := ComponentReportGenerator{
		ReqOptions: reqopts.RequestOptions{
			TestOptions:     reqopts.TestOptions{TestIDOptions: identifications},
			AnalysisOptions: reqopts.AnalysisOptions{VariantOption: reqopts.Variants{DBGroupBy: sets.New[string]("Platform")}},
		},
		jobRunTestStatusFetcher: func(g *ComponentReportGenerator, _ context.Context) (crstatus.TestJobRunStatuses, []error) {
			queries = append(queries, g.ReqOptions.TestIDOptions)
			return crstatus.TestJobRunStatuses{}, []error{wantErr}
		},
	}

	reports, errs := generator.GenerateTestDetailsReportMultiTest(context.Background())

	assert.Empty(t, reports)
	assert.Len(t, errs, 3, "one error should be returned for each query")
	assert.ElementsMatch(t, [][]reqopts.TestIdentification{
		{identifications[0]},
		{identifications[1], identifications[3]},
		{identifications[2]},
	}, queries, "only requests sharing a projected key should need individual queries")
}

func TestGenerateTestDetailsReportsErrorsForMissingBatchStatusKey(t *testing.T) {
	requested := reqopts.TestIdentification{
		TestID:            "missing",
		RequestedVariants: map[string]string{"Platform": "aws"},
	}
	generator := ComponentReportGenerator{
		ReqOptions: reqopts.RequestOptions{
			TestOptions:     reqopts.TestOptions{TestIDOptions: []reqopts.TestIdentification{requested}},
			AnalysisOptions: reqopts.AnalysisOptions{VariantOption: reqopts.Variants{DBGroupBy: sets.New[string]("Platform")}},
		},
	}

	reports, errs := generator.generateTestDetailsReportsFromStatuses(
		context.Background(), crstatus.TestJobRunStatuses{}, []reqopts.TestIdentification{requested}, false, false,
	)

	assert.Empty(t, reports)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "missing test key in results")
}

func TestGenerateTestDetailsReportMultiTestValidatesGroupedVariantsBeforeFetching(t *testing.T) {
	fetches := 0
	generator := ComponentReportGenerator{
		ReqOptions: reqopts.RequestOptions{
			TestOptions:     reqopts.TestOptions{TestIDOptions: []reqopts.TestIdentification{{TestID: "test-id"}}},
			AnalysisOptions: reqopts.AnalysisOptions{VariantOption: reqopts.Variants{DBGroupBy: sets.New[string]("Platform")}},
		},
		jobRunTestStatusFetcher: func(*ComponentReportGenerator, context.Context) (crstatus.TestJobRunStatuses, []error) {
			fetches++
			return crstatus.TestJobRunStatuses{}, nil
		},
	}

	_, errs := generator.GenerateTestDetailsReportMultiTest(context.Background())

	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "all dbGroupBy variants have to be defined")
	assert.Zero(t, fetches, "invalid wildcard details requests should fail before querying statuses")
}

func TestGenerateTestDetailsReportValidatesGroupedVariantsBeforeFetching(t *testing.T) {
	fetches := 0
	generator := ComponentReportGenerator{
		ReqOptions: reqopts.RequestOptions{
			TestOptions:     reqopts.TestOptions{TestIDOptions: []reqopts.TestIdentification{{TestID: "test-id"}}},
			AnalysisOptions: reqopts.AnalysisOptions{VariantOption: reqopts.Variants{DBGroupBy: sets.New[string]("Platform")}},
		},
		jobRunTestStatusFetcher: func(*ComponentReportGenerator, context.Context) (crstatus.TestJobRunStatuses, []error) {
			fetches++
			return crstatus.TestJobRunStatuses{}, nil
		},
	}

	_, errs := generator.GenerateTestDetailsReport(context.Background())

	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "all dbGroupBy variants have to be defined")
	assert.Zero(t, fetches, "invalid wildcard details requests should fail before querying statuses")
}

func TestMultiTestStatusQueryBackoff(t *testing.T) {
	assert.Equal(t, wait.Backoff{
		Steps:    5,
		Duration: time.Minute,
		Factor:   2,
	}, defaultMultiTestStatusQueryBackoff())
}

func TestSummarizeRecordedTestStatsKeepsStoredJobsSeparateInNormalizedBucket(t *testing.T) {
	baseRunTime := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)
	sampleRunTime := baseRunTime.Add(time.Hour)
	const (
		normalizedJob = "periodic-ci-openshift-release-master-nightly-aws"
		f24Job        = "periodic-ci-openshift-release-master-nightly-f24-aws"
		f48Job        = "periodic-ci-openshift-release-master-nightly-f48-aws"
	)

	generator := ComponentReportGenerator{
		ReqOptions: reqopts.RequestOptions{AnalysisOptions: reqopts.AnalysisOptions{AdvancedOption: reqopts.Advanced{Confidence: 95}}},
	}
	baseTotal, sampleTotal, analysis, result, lastFailure := generator.summarizeRecordedTestStats(
		map[string][]crstatus.TestDetailsSummary{
			normalizedJob: {
				{
					ProwJob:       f24Job,
					Stats:         crtest.NewTestStats(4, 1, 0, false),
					JiraComponent: "Storage",
					TestName:      "openshift-tests:normalized-job-identity",
					Lifecycle:     "legacy",
					JobRuns: []crstatus.JobRunDetail{{
						ProwJobRunID: "f24-base",
						StartTime:    baseRunTime,
						Count:        crtest.Count{TotalCount: 1, SuccessCount: 1},
					}},
				},
				{
					ProwJob:   f48Job,
					Lifecycle: "informing",
					JobRuns: []crstatus.JobRunDetail{{
						ProwJobRunID: "f48-base",
						StartTime:    baseRunTime,
						Count:        crtest.Count{TotalCount: 1, SuccessCount: 1},
					}},
				},
			},
		},
		map[string][]crstatus.TestDetailsSummary{
			normalizedJob: {{
				ProwJob: f24Job,
				Stats:   crtest.NewTestStats(3, 1, 0, false),
				JobRuns: []crstatus.JobRunDetail{{
					ProwJobRunID: "f24-sample",
					StartTime:    sampleRunTime,
					Count:        crtest.Count{TotalCount: 1},
				}},
			}},
		},
		crtest.Identification{},
	)

	assert.Equal(t, crtest.NewTestStats(4, 1, 0, false), baseTotal)
	assert.Equal(t, crtest.NewTestStats(3, 1, 0, false), sampleTotal)
	assert.Equal(t, sampleRunTime, lastFailure)
	assert.Equal(t, "Storage", result.JiraComponent)
	assert.Equal(t, "openshift-tests:normalized-job-identity", result.TestName)
	assert.Equal(t, "informing", result.Lifecycle)
	require.Len(t, analysis.JobStats, 2)

	byBaseJob := make(map[string]int, len(analysis.JobStats))
	for i, jobStats := range analysis.JobStats {
		byBaseJob[jobStats.BaseJobName] = i
	}

	f24Index, ok := byBaseJob[f24Job]
	require.True(t, ok)
	f24Stats := analysis.JobStats[f24Index]
	assert.Equal(t, f24Job, f24Stats.SampleJobName)
	assert.Equal(t, crtest.NewTestStats(4, 1, 0, false), f24Stats.BaseStats)
	assert.Equal(t, crtest.NewTestStats(3, 1, 0, false), f24Stats.SampleStats)
	require.Len(t, f24Stats.BaseJobRunStats, 1)
	assert.Equal(t, "f24-base", f24Stats.BaseJobRunStats[0].JobRunID)
	require.Len(t, f24Stats.SampleJobRunStats, 1)
	assert.Equal(t, "f24-sample", f24Stats.SampleJobRunStats[0].JobRunID)

	f48Index, ok := byBaseJob[f48Job]
	require.True(t, ok)
	f48Stats := analysis.JobStats[f48Index]
	assert.Empty(t, f48Stats.SampleJobName)
	assert.Equal(t, crtest.Stats{}, f48Stats.SampleStats)
	require.Len(t, f48Stats.BaseJobRunStats, 1)
	assert.Equal(t, "f48-base", f48Stats.BaseJobRunStats[0].JobRunID)
}

func TestSummarizeRecordedTestStatsPairsRenamedStoredJobsOnlyWhenUnambiguous(t *testing.T) {
	const normalizedJob = "periodic-ci-openshift-release-master-nightly-aws"
	generator := ComponentReportGenerator{ReqOptions: reqopts.RequestOptions{AnalysisOptions: reqopts.AnalysisOptions{AdvancedOption: reqopts.Advanced{Confidence: 95}}}}
	_, _, analysis, _, _ := generator.summarizeRecordedTestStats(
		map[string][]crstatus.TestDetailsSummary{
			normalizedJob: {
				{ProwJob: "f24", Stats: crtest.NewTestStats(1, 0, 0, false)},
				{ProwJob: "f48", Stats: crtest.NewTestStats(1, 0, 0, false)},
			},
		},
		map[string][]crstatus.TestDetailsSummary{
			normalizedJob: {
				{ProwJob: "f24", Stats: crtest.NewTestStats(1, 0, 0, false)},
				{ProwJob: "f72", Stats: crtest.NewTestStats(1, 0, 0, false)},
			},
		},
		crtest.Identification{},
	)

	require.Len(t, analysis.JobStats, 3)
	byJobNames := make(map[string]struct{}, len(analysis.JobStats))
	for _, jobStats := range analysis.JobStats {
		byJobNames[jobStats.SampleJobName+":"+jobStats.BaseJobName] = struct{}{}
	}
	assert.Contains(t, byJobNames, "f24:f24")
	assert.Contains(t, byJobNames, ":f48")
	assert.Contains(t, byJobNames, "f72:")
}

func TestSummarizeRecordedTestStatsPairsSingleExactStoredJob(t *testing.T) {
	const normalizedJob = "periodic-ci-openshift-release-master-nightly-aws"
	generator := ComponentReportGenerator{ReqOptions: reqopts.RequestOptions{AnalysisOptions: reqopts.AnalysisOptions{AdvancedOption: reqopts.Advanced{Confidence: 95}}}}
	_, _, analysis, _, _ := generator.summarizeRecordedTestStats(
		map[string][]crstatus.TestDetailsSummary{
			normalizedJob: {{ProwJob: "same-job-name", Stats: crtest.NewTestStats(1, 0, 0, false)}},
		},
		map[string][]crstatus.TestDetailsSummary{
			normalizedJob: {{ProwJob: "same-job-name", Stats: crtest.NewTestStats(1, 0, 0, false)}},
		},
		crtest.Identification{},
	)

	require.Len(t, analysis.JobStats, 1)
	assert.Equal(t, "same-job-name", analysis.JobStats[0].BaseJobName)
	assert.Equal(t, "same-job-name", analysis.JobStats[0].SampleJobName)
}

func TestSummarizeRecordedTestStatsPairsSingleRenamedStoredJobs(t *testing.T) {
	const normalizedJob = "periodic-ci-openshift-release-master-nightly-aws"
	generator := ComponentReportGenerator{ReqOptions: reqopts.RequestOptions{AnalysisOptions: reqopts.AnalysisOptions{AdvancedOption: reqopts.Advanced{Confidence: 95}}}}
	_, _, analysis, _, _ := generator.summarizeRecordedTestStats(
		map[string][]crstatus.TestDetailsSummary{
			normalizedJob: {{ProwJob: "previous-job-name", Stats: crtest.NewTestStats(1, 0, 0, false)}},
		},
		map[string][]crstatus.TestDetailsSummary{
			normalizedJob: {{ProwJob: "renamed-job-name", Stats: crtest.NewTestStats(1, 0, 0, false)}},
		},
		crtest.Identification{},
	)

	require.Len(t, analysis.JobStats, 1)
	assert.Equal(t, "previous-job-name", analysis.JobStats[0].BaseJobName)
	assert.Equal(t, "renamed-job-name", analysis.JobStats[0].SampleJobName)
}

func TestSummarizeRecordedTestStatsPairsCountedJobsWhenBaseAlsoHasRawOnlyJob(t *testing.T) {
	const normalizedJob = "periodic-ci-openshift-release-master-nightly-aws"
	generator := ComponentReportGenerator{ReqOptions: reqopts.RequestOptions{AnalysisOptions: reqopts.AnalysisOptions{AdvancedOption: reqopts.Advanced{Confidence: 95}}}}
	_, _, analysis, _, _ := generator.summarizeRecordedTestStats(
		map[string][]crstatus.TestDetailsSummary{
			normalizedJob: {
				{ProwJob: "f24", Stats: crtest.NewTestStats(1, 0, 0, false)},
				{
					ProwJob: "f72",
					JobRuns: []crstatus.JobRunDetail{{
						ProwJobRunID: "f72-base-run",
						Count:        crtest.Count{TotalCount: 1, SuccessCount: 1},
					}},
				},
			},
		},
		map[string][]crstatus.TestDetailsSummary{
			normalizedJob: {{ProwJob: "f48", Stats: crtest.NewTestStats(1, 0, 0, false)}},
		},
		crtest.Identification{},
	)

	require.Len(t, analysis.JobStats, 2)
	assert.Contains(t, analysis.JobStats, testdetails.JobStats{
		BaseJobName:   "f24",
		SampleJobName: "f48",
		BaseStats:     crtest.NewTestStats(1, 0, 0, false),
		SampleStats:   crtest.NewTestStats(1, 0, 0, false),
	})
	assert.Contains(t, analysis.JobStats, testdetails.JobStats{
		BaseJobName:       "f72",
		BaseJobRunStats:   []testdetails.JobRunStats{{JobRunID: "f72-base-run", TestStats: crtest.NewTestStats(1, 0, 0, false)}},
		BaseStats:         crtest.Stats{},
		SampleStats:       crtest.Stats{},
		SampleJobRunStats: nil,
	})
}

func TestSummarizeRecordedTestStatsDoesNotPairRawOnlyBaseWithCountedExactSample(t *testing.T) {
	const normalizedJob = "periodic-ci-openshift-release-master-nightly-aws"
	generator := ComponentReportGenerator{ReqOptions: reqopts.RequestOptions{AnalysisOptions: reqopts.AnalysisOptions{AdvancedOption: reqopts.Advanced{Confidence: 95}}}}
	_, _, analysis, _, _ := generator.summarizeRecordedTestStats(
		map[string][]crstatus.TestDetailsSummary{
			normalizedJob: {
				{
					ProwJob: "f24",
					Stats:   crtest.NewTestStats(1, 0, 0, false),
					JobRuns: []crstatus.JobRunDetail{{
						ProwJobRunID: "f24-base-run",
						Count:        crtest.Count{TotalCount: 1, SuccessCount: 1},
					}},
				},
				{
					ProwJob: "f48",
					JobRuns: []crstatus.JobRunDetail{{
						ProwJobRunID: "f48-base-run",
						Count:        crtest.Count{TotalCount: 1, SuccessCount: 1},
					}},
				},
			},
		},
		map[string][]crstatus.TestDetailsSummary{
			normalizedJob: {{
				ProwJob: "f48",
				Stats:   crtest.NewTestStats(1, 0, 0, false),
				JobRuns: []crstatus.JobRunDetail{{
					ProwJobRunID: "f48-sample-run",
					Count:        crtest.Count{TotalCount: 1, SuccessCount: 1},
				}},
			}},
		},
		crtest.Identification{},
	)

	require.Len(t, analysis.JobStats, 2)
	assert.Contains(t, analysis.JobStats, testdetails.JobStats{
		BaseJobName:   "f24",
		SampleJobName: "f48",
		BaseStats:     crtest.NewTestStats(1, 0, 0, false),
		SampleStats:   crtest.NewTestStats(1, 0, 0, false),
		BaseJobRunStats: []testdetails.JobRunStats{{
			JobRunID:  "f24-base-run",
			TestStats: crtest.NewTestStats(1, 0, 0, false),
		}},
		SampleJobRunStats: []testdetails.JobRunStats{{
			JobRunID:  "f48-sample-run",
			TestStats: crtest.NewTestStats(1, 0, 0, false),
		}},
	})
	assert.Contains(t, analysis.JobStats, testdetails.JobStats{
		BaseJobName: "f48",
		BaseStats:   crtest.Stats{},
		BaseJobRunStats: []testdetails.JobRunStats{{
			JobRunID:  "f48-base-run",
			TestStats: crtest.NewTestStats(1, 0, 0, false),
		}},
	})
}

func TestGetCompleteMultiTestJobRunStatuses(t *testing.T) {
	present := crtest.KeyWithVariants{TestID: "present", Variants: map[string]string{"Platform": "aws"}}
	missing := crtest.KeyWithVariants{TestID: "missing", Variants: map[string]string{"Platform": "gcp"}}
	fullStatuses := crstatus.TestJobRunStatuses{
		SampleStatus: map[string][]crstatus.TestDetailsSummary{
			"job": {{TestKeyStr: present.Encode()}, {TestKeyStr: missing.Encode()}},
		},
	}
	partialStatuses := crstatus.TestJobRunStatuses{
		SampleStatus: map[string][]crstatus.TestDetailsSummary{
			"job": {{TestKeyStr: present.Encode()}},
		},
	}
	baseStatuses := crstatus.TestJobRunStatuses{
		BaseStatus: map[string][]crstatus.TestDetailsSummary{
			"job": {{TestKeyStr: present.Encode()}, {TestKeyStr: missing.Encode()}},
		},
	}
	baseOverrideStatuses := crstatus.TestJobRunStatuses{
		BaseOverrideStatus: map[string][]crstatus.TestDetailsSummary{
			"job": {{TestKeyStr: present.Encode()}, {TestKeyStr: missing.Encode()}},
		},
	}

	for _, testCase := range []struct {
		name       string
		responses  []jobRunTestStatusResponse
		wantCalls  int
		wantErr    error
		wantResult crstatus.TestJobRunStatuses
	}{
		{
			name: "retries incomplete response until complete",
			responses: []jobRunTestStatusResponse{
				{statuses: partialStatuses},
				{statuses: fullStatuses},
			},
			wantCalls:  2,
			wantResult: fullStatuses,
		},
		{
			name: "reports incomplete response after exhausting retries",
			responses: []jobRunTestStatusResponse{
				{statuses: partialStatuses}, {statuses: partialStatuses}, {statuses: partialStatuses},
				{statuses: partialStatuses}, {statuses: partialStatuses},
			},
			wantCalls: 5,
			wantErr:   errors.New("test details status query remained incomplete after 5 attempts"),
		},
		{
			name:      "propagates query errors without retrying",
			responses: []jobRunTestStatusResponse{{errs: []error{errors.New("BigQuery unavailable")}}},
			wantCalls: 1,
			wantErr:   errors.New("BigQuery unavailable"),
		},
		{
			name:       "accepts key from base status",
			responses:  []jobRunTestStatusResponse{{statuses: baseStatuses}},
			wantCalls:  1,
			wantResult: baseStatuses,
		},
		{
			name:       "accepts key from base override status",
			responses:  []jobRunTestStatusResponse{{statuses: baseOverrideStatuses}},
			wantCalls:  1,
			wantResult: baseOverrideStatuses,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			calls := 0
			generator := ComponentReportGenerator{
				ReqOptions: reqopts.RequestOptions{TestOptions: reqopts.TestOptions{TestIDOptions: []reqopts.TestIdentification{
					{TestID: present.TestID, RequestedVariants: present.Variants},
					{TestID: missing.TestID, RequestedVariants: missing.Variants},
				}}, AnalysisOptions: reqopts.AnalysisOptions{VariantOption: reqopts.Variants{DBGroupBy: sets.New[string]("Platform")}}},
				jobRunTestStatusFetcher: func(*ComponentReportGenerator, context.Context) (crstatus.TestJobRunStatuses, []error) {
					response := testCase.responses[calls]
					calls++
					return response.statuses, response.errs
				},
				multiTestStatusBackoff: wait.Backoff{Steps: 5},
			}

			statuses, errs := generator.getCompleteMultiTestJobRunStatuses(context.Background())
			assert.Equal(t, testCase.wantCalls, calls)
			if testCase.wantErr != nil {
				require.Error(t, errors.Join(errs...))
				assert.Contains(t, errors.Join(errs...).Error(), testCase.wantErr.Error())
				return
			}
			require.Empty(t, errs)
			assert.Equal(t, testCase.wantResult, statuses)
		})
	}
}

func TestGetCompleteMultiTestJobRunStatusesWrapsWaitError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	generator := ComponentReportGenerator{
		jobRunTestStatusFetcher: func(*ComponentReportGenerator, context.Context) (crstatus.TestJobRunStatuses, []error) {
			return crstatus.TestJobRunStatuses{}, nil
		},
	}
	_, errs := generator.getCompleteMultiTestJobRunStatuses(ctx)
	err := errors.Join(errs...)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Contains(t, err.Error(), "wait for complete multi-test job run statuses")
}

type jobRunTestStatusResponse struct {
	statuses crstatus.TestJobRunStatuses
	errs     []error
}

func TestSplitTestJobRunStatusesByTestKeyAlwaysCreatesSpotCheckEntry(t *testing.T) {
	spotCheck := reqopts.TestIdentification{TestID: crtest.SpotCheckTestID("spotcheck-30d", "etcd", "scaling")}
	ordinary := reqopts.TestIdentification{TestID: "missing", RequestedVariants: map[string]string{"Platform": "aws"}}
	dbGroupBy := sets.New[string]("Platform")

	statusesByKey := splitTestJobRunStatusesByTestKey(
		crstatus.TestJobRunStatuses{}, []reqopts.TestIdentification{spotCheck, ordinary}, dbGroupBy, false)

	assert.Contains(t, statusesByKey, testDetailsStatusKey(spotCheck, dbGroupBy).Encode(),
		"spot-check results come from middleware, so batched requests need an empty entry to fill")
	assert.NotContains(t, statusesByKey, testDetailsStatusKey(ordinary, dbGroupBy).Encode())
}

func TestValidateTestDetailsIdentificationSkipsGroupedVariantsForSpotCheck(t *testing.T) {
	dbGroupBy := sets.New[string]("Platform")

	assert.NoError(t, validateTestDetailsIdentification(
		reqopts.TestIdentification{TestID: crtest.SpotCheckTestID("spotcheck-30d", "etcd", "scaling")}, dbGroupBy))
	assert.Error(t, validateTestDetailsIdentification(reqopts.TestIdentification{TestID: "test-id"}, dbGroupBy))
}

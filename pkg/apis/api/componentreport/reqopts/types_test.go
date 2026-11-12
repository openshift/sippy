package reqopts

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestRequestOptionsSerializationCompatibility(t *testing.T) {
	start := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	options := RequestOptions{
		ReleaseOptions: ReleaseOptions{
			BaseRelease: Release{
				Name: "4.20",
				PullRequestOptions: &PullRequest{
					Org:      "openshift",
					Repo:     "sippy",
					PRNumber: "123",
				},
				Start: start,
				End:   end,
			},
			SampleRelease: Release{
				Name:           "4.21",
				PayloadOptions: &Payload{Tags: []string{"4.21.0-0.nightly"}},
				Start:          start,
				End:            end,
			},
		},
		AnalysisOptions: AnalysisOptions{
			VariantOption: Variants{
				ColumnGroupBy:       sets.New("Platform"),
				DBGroupBy:           sets.New("Architecture"),
				IncludeVariants:     map[string][]string{"Platform": {"aws"}},
				CompareVariants:     map[string][]string{"Network": {"ovn"}},
				VariantCrossCompare: []string{"Platform"},
			},
			AdvancedOption: Advanced{
				MinimumFailure:              1,
				Confidence:                  95,
				PityFactor:                  5,
				PassRateRequiredNewTests:    90,
				PassRateRequiredAllTests:    95,
				IgnoreMissing:               true,
				IgnoreDisruption:            true,
				FlakeAsFailure:              true,
				IncludeMultiReleaseAnalysis: true,
				KeyTestNames:                []string{"installer"},
			},
		},
		CacheOptions: CacheOptions{CacheOption: cache.RequestOptions{Expiry: time.Hour}},
		TestOptions: TestOptions{
			TestFilters: TestFilters{
				Capabilities: []string{"Networking"},
				Lifecycles:   []string{"blocking"},
			},
			TestIDOptions: []TestIdentification{{
				Component:           "network",
				Capability:          "Connectivity",
				TestID:              "test-id",
				RequestedVariants:   map[string]string{"Platform": "aws"},
				BaseOverrideRelease: "4.19",
			}},
		},
		JobRunOptions: JobRunOptions{SpotCheckJobSamples: []SpotCheckJobSampleOpts{{
			Name:            "30d",
			Release:         Release{Name: "4.21", Start: start, End: end},
			IncludeVariants: map[string][]string{"JobTier": {"spotcheck-30d"}},
		}}},
		ReportContentOptions: ReportContentOptions{IncludeAllTests: true},
		ViewDataSourceOptions: ViewDataSourceOptions{
			ViewName:   "4.21-main",
			DataSource: DataSourcePostgres,
		},
	}

	jsonBytes, err := json.Marshal(options)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"BaseRelease": {
			"release": "4.20",
			"pull_request_options": {"Org": "openshift", "Repo": "sippy", "PRNumber": "123"},
			"start": "2026-01-02T03:04:05Z",
			"end": "2026-01-03T03:04:05Z"
		},
		"SampleRelease": {
			"release": "4.21",
			"payload_options": {"Tags": ["4.21.0-0.nightly"]},
			"start": "2026-01-02T03:04:05Z",
			"end": "2026-01-03T03:04:05Z"
		},
		"VariantOption": {
			"column_group_by": {"Platform": {}}, "db_group_by": {"Architecture": {}},
			"include_variants": {"Platform": ["aws"]}, "compare_variants": {"Network": ["ovn"]},
			"variant_cross_compare": ["Platform"]
		},
		"AdvancedOption": {
			"minimum_failure": 1, "confidence": 95, "pity_factor": 5,
			"pass_rate_required_new_tests": 90, "pass_rate_required_all_tests": 95,
			"ignore_missing": true, "ignore_disruption": true, "flake_as_failure": true,
			"include_multi_release_analysis": true, "key_test_names": ["installer"]
		},
		"CacheOption": {"Expiry": 3600000000000, "StableAge": 0, "StableExpiry": 0, "CRTimeRoundingFactor": 0, "CRTimeRoundingOffset": 0, "ForceRefresh": false, "RefreshRecent": false, "SkipCacheWrites": false},
		"capabilities": ["Networking"], "lifecycles": ["blocking"],
		"TestIDOptions": [{"component": "network", "capability": "Connectivity", "test_id": "test-id", "requested_variants": {"Platform": "aws"}, "base_override_release": "4.19"}],
		"spot_check_job_samples": [{"name": "30d", "release": "4.21", "start": "2026-01-02T03:04:05Z", "end": "2026-01-03T03:04:05Z", "include_variants": {"JobTier": ["spotcheck-30d"]}}],
		"include_all_tests": true, "view_name": "4.21-main", "data_source": "postgres"
	}`, string(jsonBytes))

	var jsonRoundTrip RequestOptions
	require.NoError(t, json.Unmarshal(jsonBytes, &jsonRoundTrip))
	require.Equal(t, options, jsonRoundTrip)

	yamlBytes, err := yaml.Marshal(options)
	require.NoError(t, err)
	require.Equal(t, `baserelease:
    release: "4.20"
    pull_request_options:
        org: openshift
        repo: sippy
        prnumber: "123"
    start: 2026-01-02T03:04:05Z
    end: 2026-01-03T03:04:05Z
samplerelease:
    release: "4.21"
    payload_options:
        tags:
            - 4.21.0-0.nightly
    start: 2026-01-02T03:04:05Z
    end: 2026-01-03T03:04:05Z
variantoption:
    column_group_by:
        Platform: {}
    db_group_by:
        Architecture: {}
    include_variants:
        Platform:
            - aws
    compare_variants:
        Network:
            - ovn
    variant_cross_compare:
        - Platform
advancedoption:
    minimum_failure: 1
    confidence: 95
    pity_factor: 5
    pass_rate_required_new_tests: 90
    pass_rate_required_all_tests: 95
    ignore_missing: true
    ignore_disruption: true
    flake_as_failure: true
    include_multi_release_analysis: true
    key_test_names:
        - installer
cacheoption:
    expiry: 1h0m0s
    crtimeroundingfactor: 0s
    crtimeroundingoffset: 0s
    skipcachewrites: false
    stableexpiry: 0s
    stableage: 0s
    forcerefresh: false
    refreshrecent: false
testfilters:
    capabilities:
        - Networking
    lifecycles:
        - blocking
testidoptions:
    - component: network
      capability: Connectivity
      test_id: test-id
      requested_variants:
        Platform: aws
      base_override_release: "4.19"
spot_check_job_samples:
    - name: 30d
      release: "4.21"
      start: 2026-01-02T03:04:05Z
      end: 2026-01-03T03:04:05Z
      include_variants:
        JobTier:
            - spotcheck-30d
includealltests: true
view_name: 4.21-main
data_source: postgres
`, string(yamlBytes))

	var yamlRoundTrip RequestOptions
	require.NoError(t, yaml.Unmarshal(yamlBytes, &yamlRoundTrip))
	require.Equal(t, options, yamlRoundTrip)
}

package releasefallback

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/bq"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/stretchr/testify/assert"
)

func Test_PreAnalysis(t *testing.T) {
	reqOpts419 := reqopts.RequestOptions{
		BaseRelease: reqopts.Release{
			Name: "4.19",
		},
		AdvancedOption: reqopts.Advanced{IncludeMultiReleaseAnalysis: true},
	}
	test1ID := "test1ID"
	test1Variants := map[string]string{
		"Arch":     "amd64",
		"Platform": "aws",
	}
	test1VariantsFlattened := []string{"Arch:amd64", "Platform:aws"}
	test1MapKey := crtest.KeyWithVariants{
		TestID:   test1ID,
		Variants: test1Variants,
	}
	test1KeyBytes, err := json.Marshal(test1MapKey)
	test1KeyStr := string(test1KeyBytes)
	assert.NoError(t, err)
	test1RTI := crtest.Identification{
		RowIdentification: crtest.RowIdentification{
			Component:  "",
			Capability: "",
			TestName:   "test 1",
			TestSuite:  "",
			TestID:     test1ID,
		},
		ColumnIdentification: crtest.ColumnIdentification{
			Variants: test1Variants,
		},
	}

	// 4.19 will be our assumed requested base release, which may trigger fallback to 4.18 or 4.17 in these tests
	start419 := time.Date(2025, 3, 2, 0, 0, 0, 0, time.UTC)
	end419 := time.Date(2025, 4, 30, 0, 0, 0, 0, time.UTC)
	release419 := crtest.Release{
		Release: "4.19",
		Start:   &start419,
		End:     &end419,
	}

	start418 := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	end418 := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	release418 := crtest.Release{
		Release: "4.18",
		Start:   &start418,
		End:     &end418,
	}
	fallbackMap418 := ReleaseTestMap{
		Release: release418,
		Tests: map[string]bq.TestStatus{
			test1KeyStr: buildTestStatus("test1", test1VariantsFlattened, 100, 95, 0),
		},
	}

	start417 := time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC)
	end417 := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	release417 := crtest.Release{
		Release: "4.17",
		Start:   &start417,
		End:     &end417,
	}
	fallbackMap417 := ReleaseTestMap{
		Release: release417,
		Tests: map[string]bq.TestStatus{
			test1KeyStr: buildTestStatus("test1", test1VariantsFlattened, 100, 98, 0),
		},
	}

	releaseConfigs := []v1.Release{
		{Release: "4.19", PreviousRelease: "4.18"},
		{Release: "4.18", PreviousRelease: "4.17"},
		{Release: "4.17", PreviousRelease: "4.16"},
	}

	tests := []struct {
		name             string
		reqOpts          reqopts.RequestOptions
		testKey          crtest.Identification
		fallbackReleases FallbackReleases
		testStats        *testdetails.TestComparison
		expectedStatus   *testdetails.TestComparison
	}{
		{
			name:    "fallback to prior release",
			reqOpts: reqOpts419,
			testKey: test1RTI,
			fallbackReleases: FallbackReleases{
				Releases: map[string]ReleaseTestMap{
					fallbackMap418.Release.Release: fallbackMap418,
				},
			},
			testStats:      buildTestStats(100, 93, release419, nil),
			expectedStatus: buildTestStats(100, 95, release418, []string{"Overrode base stats (0.9300) using release 4.18 (0.9500)"}),
		},
		{
			name:    "fallback twice to prior release",
			reqOpts: reqOpts419,
			testKey: test1RTI,
			fallbackReleases: FallbackReleases{
				Releases: map[string]ReleaseTestMap{
					fallbackMap418.Release.Release: fallbackMap418,
					fallbackMap417.Release.Release: fallbackMap417, // 4.17 improves even further
				},
			},
			testStats:      buildTestStats(100, 93, release419, nil),
			expectedStatus: buildTestStats(100, 98, release417, []string{"Overrode base stats (0.9500) using release 4.17 (0.9800)"}),
		},
		{
			name:    "fallback once to two releases ago",
			reqOpts: reqOpts419,
			testKey: test1RTI,
			fallbackReleases: FallbackReleases{
				Releases: map[string]ReleaseTestMap{
					fallbackMap418.Release.Release: fallbackMap418,
					fallbackMap417.Release.Release: fallbackMap417, // 4.17 improves even further
				},
			},
			testStats:      buildTestStats(100, 97, release419, nil),
			expectedStatus: buildTestStats(100, 98, release417, []string{"Overrode base stats (0.9700) using release 4.17 (0.9800)"}),
		},
		{
			name:    "don't fallback to prior release",
			reqOpts: reqOpts419,
			testKey: test1RTI,
			fallbackReleases: FallbackReleases{
				Releases: map[string]ReleaseTestMap{
					fallbackMap418.Release.Release: fallbackMap418,
				},
			},
			testStats:      buildTestStats(100, 100, release419, nil),
			expectedStatus: buildTestStats(100, 100, release419, nil),
		},
		{
			name:    "don't fallback to prior release with insufficient runs",
			reqOpts: reqOpts419,
			testKey: test1RTI,
			fallbackReleases: FallbackReleases{
				Releases: map[string]ReleaseTestMap{
					fallbackMap418.Release.Release: fallbackMap418,
					fallbackMap417.Release.Release: fallbackMap417,
				},
			},
			testStats: buildTestStats(10000, 9700, release419, nil),
			// No fallback release had at least 60% of our run count
			expectedStatus: buildTestStats(10000, 9700, release419, nil),
		},
	}
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rfb := NewReleaseFallbackMiddleware(nil, test.reqOpts, releaseConfigs)
			rfb.cachedFallbackTestStatuses = &tests[i].fallbackReleases
			err := rfb.PreAnalysis(test.testKey, test.testStats)
			assert.NoError(t, err)
			assert.Equal(t, *test.expectedStatus, *test.testStats)
		})
	}
}
func TestCalculateFallbackReleases(t *testing.T) {
	start419 := time.Date(2025, 3, 2, 0, 0, 0, 0, time.UTC)
	end419 := time.Date(2025, 4, 30, 0, 0, 0, 0, time.UTC)
	release419 := crtest.Release{
		Release: "4.19",
		Start:   &start419,
		End:     &end419,
	}

	start418 := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	end418 := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	release418 := crtest.Release{
		Release: "4.18",
		Start:   &start418,
		End:     &end418,
	}

	start417 := time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC)
	end417 := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	release417 := crtest.Release{
		Release: "4.17",
		Start:   &start417,
		End:     &end417,
	}

	start416 := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	end416 := time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC)
	release416 := crtest.Release{
		Release: "4.16",
		Start:   &start416,
		End:     &end416,
	}

	allReleases := []crtest.Release{release419, release418, release417, release416}
	expectedReleases := []crtest.Release{release419, release418, release417}
	releaseConfigs := []v1.Release{
		{Release: "4.20", PreviousRelease: "4.19"},
		{Release: "4.19", PreviousRelease: "4.18"},
		{Release: "4.18", PreviousRelease: "4.17"},
		{Release: "4.17", PreviousRelease: "4.16"},
		{Release: "4.16", PreviousRelease: ""},
	}

	fallbackReleases := calculateFallbackReleases("4.20", allReleases, releaseConfigs)
	for i := range expectedReleases {
		assert.Equal(t, expectedReleases[i].Release, fallbackReleases[i].Release)
		assert.Equal(t, expectedReleases[i].Start, fallbackReleases[i].Start)
		assert.Equal(t, expectedReleases[i].End, fallbackReleases[i].End)
	}
}

//nolint:unparam
func buildTestStatus(testName string, variants []string, total, success, flake int) bq.TestStatus {
	return bq.TestStatus{
		TestName:     testName,
		TestSuite:    "conformance",
		Component:    "foo",
		Capabilities: nil,
		Variants:     variants,
		Count: crtest.Count{
			TotalCount:   total,
			SuccessCount: success,
			FlakeCount:   flake,
		},
	}
}

func buildTestStats(total, success int, baseRelease crtest.Release, explanations []string) *testdetails.TestComparison {
	fails := total - success
	ts := &testdetails.TestComparison{
		BaseStats: &testdetails.ReleaseStats{
			Release: baseRelease.Release,
			Start:   baseRelease.Start,
			End:     baseRelease.End,
			Stats: crtest.Stats{
				FailureCount: fails,
				SuccessCount: success,
				FlakeCount:   0,
				SuccessRate:  crtest.CalculatePassRate(success, fails, 0, false),
			},
		},
	}
	if explanations != nil {
		ts.Explanations = explanations
	}
	return ts
}

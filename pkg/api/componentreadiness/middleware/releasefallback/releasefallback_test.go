package releasefallback

import (
	"encoding/json"
	"testing"
	"time"

	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
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
	test1MapKey := crtest.TestWithVariantsKey{
		TestID:   test1ID,
		Variants: test1Variants,
	}
	test1KeyBytes, err := json.Marshal(test1MapKey)
	test1KeyStr := string(test1KeyBytes)
	assert.NoError(t, err)
	test1RTI := crtest.ReportTestIdentification{
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
	release419 := crtype.Release{
		Release: "4.19",
		Start:   &start419,
		End:     &end419,
	}

	start418 := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	end418 := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	release418 := crtype.Release{
		Release: "4.18",
		Start:   &start418,
		End:     &end418,
	}
	fallbackMap418 := crtype.ReleaseTestMap{
		Release: release418,
		Tests: map[string]crtype.TestStatus{
			test1KeyStr: buildTestStatus("test1", test1VariantsFlattened, 100, 95, 0),
		},
	}

	start417 := time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC)
	end417 := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	release417 := crtype.Release{
		Release: "4.17",
		Start:   &start417,
		End:     &end417,
	}
	fallbackMap417 := crtype.ReleaseTestMap{
		Release: release417,
		Tests: map[string]crtype.TestStatus{
			test1KeyStr: buildTestStatus("test1", test1VariantsFlattened, 100, 98, 0),
		},
	}

	tests := []struct {
		name             string
		reqOpts          reqopts.RequestOptions
		testKey          crtest.ReportTestIdentification
		fallbackReleases crtype.FallbackReleases
		testStats        *crtype.ReportTestStats
		expectedStatus   *crtype.ReportTestStats
	}{
		{
			name:    "fallback to prior release",
			reqOpts: reqOpts419,
			testKey: test1RTI,
			fallbackReleases: crtype.FallbackReleases{
				Releases: map[string]crtype.ReleaseTestMap{
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
			fallbackReleases: crtype.FallbackReleases{
				Releases: map[string]crtype.ReleaseTestMap{
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
			fallbackReleases: crtype.FallbackReleases{
				Releases: map[string]crtype.ReleaseTestMap{
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
			fallbackReleases: crtype.FallbackReleases{
				Releases: map[string]crtype.ReleaseTestMap{
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
			fallbackReleases: crtype.FallbackReleases{
				Releases: map[string]crtype.ReleaseTestMap{
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
			rfb := NewReleaseFallbackMiddleware(nil, test.reqOpts)
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
	release419 := crtype.Release{
		Release: "4.19",
		Start:   &start419,
		End:     &end419,
	}

	start418 := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	end418 := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	release418 := crtype.Release{
		Release: "4.18",
		Start:   &start418,
		End:     &end418,
	}

	start417 := time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC)
	end417 := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	release417 := crtype.Release{
		Release: "4.17",
		Start:   &start417,
		End:     &end417,
	}

	start416 := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	end416 := time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC)
	release416 := crtype.Release{
		Release: "4.16",
		Start:   &start416,
		End:     &end416,
	}

	allReleases := []crtype.Release{release419, release418, release417, release416}
	expectedReleases := []crtype.Release{release419, release418, release417}

	fallbackReleases := calculateFallbackReleases("4.20", allReleases)
	for i := range expectedReleases {
		assert.Equal(t, expectedReleases[i].Release, fallbackReleases[i].Release)
		assert.Equal(t, expectedReleases[i].Start, fallbackReleases[i].Start)
		assert.Equal(t, expectedReleases[i].End, fallbackReleases[i].End)
	}
}

//nolint:unparam
func buildTestStatus(testName string, variants []string, total, success, flake int) crtype.TestStatus {
	return crtype.TestStatus{
		TestName:     testName,
		TestSuite:    "conformance",
		Component:    "foo",
		Capabilities: nil,
		Variants:     variants,
		TestCount: crtest.TestCount{
			TotalCount:   total,
			SuccessCount: success,
			FlakeCount:   flake,
		},
	}
}

func buildTestStats(total, success int, baseRelease crtype.Release, explanations []string) *crtype.ReportTestStats {
	fails := total - success
	ts := &crtype.ReportTestStats{
		BaseStats: &crtype.TestDetailsReleaseStats{
			Release: baseRelease.Release,
			Start:   baseRelease.Start,
			End:     baseRelease.End,
			TestDetailsTestStats: crtest.TestDetailsTestStats{
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

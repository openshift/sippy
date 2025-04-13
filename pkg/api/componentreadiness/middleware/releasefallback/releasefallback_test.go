package releasefallback

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/stretchr/testify/assert"
)

func Test_Transform(t *testing.T) {
	reqOpts419 := crtype.RequestOptions{
		BaseRelease: crtype.RequestReleaseOptions{
			Release: "4.19",
		},
		AdvancedOption: crtype.RequestAdvancedOptions{IncludeMultiReleaseAnalysis: true},
	}
	test1ID := "test1ID"
	//test1Variants := []string{"Arch:amd64", "Platform:aws"}
	test1Variants := map[string]string{
		"Arch":     "amd64",
		"Platform": "aws",
	}
	test1VariantsFlattened := []string{"Arch:amd64", "Platform:aws"}
	test1MapKey := crtype.TestWithVariantsKey{
		TestID:   test1ID,
		Variants: test1Variants,
	}
	test1KeyBytes, err := json.Marshal(test1MapKey)
	test1KeyStr := string(test1KeyBytes)
	assert.NoError(t, err)
	test1RTI := crtype.ReportTestIdentification{
		RowIdentification: crtype.RowIdentification{
			Component:  "",
			Capability: "",
			TestName:   "test 1",
			TestSuite:  "",
			TestID:     test1ID,
		},
		ColumnIdentification: crtype.ColumnIdentification{
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
			test1KeyStr: buildTestStatus("test1", test1VariantsFlattened, 100, 95, 0, &release418),
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
			test1KeyStr: buildTestStatus("test1", test1VariantsFlattened, 100, 98, 0, &release417),
		},
	}

	tests := []struct {
		name             string
		reqOpts          crtype.RequestOptions
		testKey          crtype.ReportTestIdentification
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
			testStats:      buildTestStats(100, 93, 0, release419),
			expectedStatus: buildTestStats(100, 95, 0, release418),
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
			testStats:      buildTestStats(100, 93, 0, release419),
			expectedStatus: buildTestStats(100, 98, 0, release417),
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
			testStats:      buildTestStats(100, 97, 0, release419),
			expectedStatus: buildTestStats(100, 98, 0, release417),
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
			testStats:      buildTestStats(100, 100, 0, release419),
			expectedStatus: buildTestStats(100, 100, 0, release419),
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
			testStats: buildTestStats(10000, 9700, 0, release419),
			// No fallback release had at least 60% of our run count
			expectedStatus: buildTestStats(10000, 9700, 0, release419),
		},
	}
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rfb := NewReleaseFallbackMiddleware(nil, test.reqOpts)
			rfb.cachedFallbackTestStatuses = &tests[i].fallbackReleases
			err := rfb.Transform(test.testKey, test.testStats)
			assert.NoError(t, err)
			assert.Equal(t, *test.expectedStatus, *test.testStats)
		})
	}
}

//nolint:unparam
func buildTestStatus(testName string, variants []string, total, success, flake int, release *crtype.Release) crtype.TestStatus {
	return crtype.TestStatus{
		TestName:     testName,
		TestSuite:    "conformance",
		Component:    "foo",
		Capabilities: nil,
		Variants:     variants,
		TotalCount:   total,
		SuccessCount: success,
		FlakeCount:   flake,
		Release:      release,
	}
}
func buildTestStats(total, success, flake int, baseRelease crtype.Release) *crtype.ReportTestStats {
	fails := total - success - flake
	ts := &crtype.ReportTestStats{
		BaseStats: &crtype.TestDetailsReleaseStats{
			Release: baseRelease.Release,
			Start:   baseRelease.Start,
			End:     baseRelease.End,
			TestDetailsTestStats: crtype.TestDetailsTestStats{
				FailureCount: fails,
				SuccessCount: success,
				FlakeCount:   flake,
				SuccessRate:  utils.CalculatePassRate(success, fails, flake, false),
			},
		},
	}
	return ts
}

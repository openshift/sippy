package releasefallback

import (
	"encoding/json"
	"testing"
	"time"

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
	test1Variants := []string{"Arch:amd64", "Platform:aws"}
	test1Key := crtype.TestWithVariantsKey{
		TestID: test1ID,
		Variants: map[string]string{
			"Arch":     "amd64",
			"Platform": "aws",
		},
	}
	test1KeyBytes, err := json.Marshal(test1Key)
	test1KeyStr := string(test1KeyBytes)
	assert.NoError(t, err)

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
			test1KeyStr: buildTestStatus("test1", test1Variants, 100, 95, 0, nil),
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
			test1KeyStr: buildTestStatus("test1", test1Variants, 100, 98, 0, nil),
		},
	}

	tests := []struct {
		name             string
		reqOpts          crtype.RequestOptions
		fallbackReleases crtype.FallbackReleases
		baseStatus       map[string]crtype.TestStatus
		expectedStatus   map[string]crtype.TestStatus
	}{
		{
			name:    "fallback to prior release",
			reqOpts: reqOpts419,
			fallbackReleases: crtype.FallbackReleases{
				Releases: map[string]crtype.ReleaseTestMap{
					fallbackMap418.Release.Release: fallbackMap418,
				},
			},
			baseStatus: map[string]crtype.TestStatus{
				test1KeyStr: buildTestStatus("test1", test1Variants, 100, 93, 0, nil),
			},
			expectedStatus: map[string]crtype.TestStatus{
				test1KeyStr: buildTestStatus("test1", test1Variants, 100, 95, 0, &release418),
			},
		},
		{
			name:    "fallback twice to prior release",
			reqOpts: reqOpts419,
			fallbackReleases: crtype.FallbackReleases{
				Releases: map[string]crtype.ReleaseTestMap{
					fallbackMap418.Release.Release: fallbackMap418,
					fallbackMap417.Release.Release: fallbackMap417, // 4.17 improves even further
				},
			},
			baseStatus: map[string]crtype.TestStatus{
				test1KeyStr: buildTestStatus("test1", test1Variants, 100, 93, 0, nil),
			},
			expectedStatus: map[string]crtype.TestStatus{
				test1KeyStr: buildTestStatus("test1", test1Variants, 100, 98, 0, &release417),
			},
		},
		{
			name:    "fallback once to two releases ago",
			reqOpts: reqOpts419,
			fallbackReleases: crtype.FallbackReleases{
				Releases: map[string]crtype.ReleaseTestMap{
					fallbackMap418.Release.Release: fallbackMap418,
					fallbackMap417.Release.Release: fallbackMap417, // 4.17 improves even further
				},
			},
			baseStatus: map[string]crtype.TestStatus{
				test1KeyStr: buildTestStatus("test1", test1Variants, 100, 97, 0, nil),
			},
			expectedStatus: map[string]crtype.TestStatus{
				test1KeyStr: buildTestStatus("test1", test1Variants, 100, 98, 0, &release417),
			},
		},
		{
			name:    "don't fallback to prior release",
			reqOpts: reqOpts419,
			fallbackReleases: crtype.FallbackReleases{
				Releases: map[string]crtype.ReleaseTestMap{
					fallbackMap418.Release.Release: fallbackMap418,
				},
			},
			baseStatus: map[string]crtype.TestStatus{
				test1KeyStr: buildTestStatus("test1", test1Variants, 100, 100, 0, nil),
			},
			expectedStatus: map[string]crtype.TestStatus{
				test1KeyStr: buildTestStatus("test1", test1Variants, 100, 100, 0, nil),
			},
		},
		{
			name:    "don't fallback to prior release with insufficient runs",
			reqOpts: reqOpts419,
			fallbackReleases: crtype.FallbackReleases{
				Releases: map[string]crtype.ReleaseTestMap{
					fallbackMap418.Release.Release: fallbackMap418,
					fallbackMap417.Release.Release: fallbackMap417,
				},
			},
			baseStatus: map[string]crtype.TestStatus{
				test1KeyStr: buildTestStatus("test1", test1Variants, 10000, 9700, 0, nil),
			},
			expectedStatus: map[string]crtype.TestStatus{
				// No fallback release had at least 60% of our run count
				test1KeyStr: buildTestStatus("test1", test1Variants, 10000, 9700, 0, nil),
			},
		},
	}
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rfb := NewReleaseFallbackMiddleware(nil, test.reqOpts)
			rfb.cachedFallbackTestStatuses = &tests[i].fallbackReleases
			status := &crtype.ReportTestStatus{BaseStatus: test.baseStatus}
			err := rfb.Transform(status)
			assert.NoError(t, err)
			assert.Equal(t, test.expectedStatus, status.BaseStatus)
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

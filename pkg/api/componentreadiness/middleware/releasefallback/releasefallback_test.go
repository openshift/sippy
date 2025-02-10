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

	start418 := time.Date(2025, 02, 1, 0, 0, 0, 0, time.UTC)
	end418 := time.Date(2025, 03, 1, 0, 0, 0, 0, time.UTC)
	release418 := crtype.Release{
		Release: "4.18",
		Start:   &start418,
		End:     &end418,
	}
	fallbackReleases := &crtype.FallbackReleases{
		Releases: map[string]crtype.ReleaseTestMap{
			"4.18": {
				Release: release418,
				Tests: map[string]crtype.TestStatus{
					test1KeyStr: {
						TestName:     "test 1",
						TestSuite:    "conformance",
						Component:    "foo",
						Capabilities: nil,
						Variants:     test1Variants,
						TotalCount:   100,
						SuccessCount: 100,
						FlakeCount:   0,
					},
				},
			},
		},
	}

	tests := []struct {
		name             string
		reqOpts          crtype.RequestOptions
		fallbackReleases *crtype.FallbackReleases
		baseStatus       map[string]crtype.TestStatus
		expectedStatus   map[string]crtype.TestStatus
	}{
		{
			name:             "fallback to prior release",
			reqOpts:          reqOpts419,
			fallbackReleases: fallbackReleases,
			baseStatus: map[string]crtype.TestStatus{
				// Same test but got a little worse in 4.18
				test1KeyStr: {
					TestName:     "test 1",
					TestSuite:    "conformance",
					Component:    "foo",
					Capabilities: nil,
					Variants:     []string{"Arch:amd64", "Platform:aws"},
					TotalCount:   100,
					SuccessCount: 93,
					FlakeCount:   3,
				},
			},
			expectedStatus: map[string]crtype.TestStatus{
				test1KeyStr: {
					TestName:     "test 1",
					TestSuite:    "conformance",
					Component:    "foo",
					Capabilities: nil,
					Variants:     []string{"Arch:amd64", "Platform:aws"},
					TotalCount:   100,
					SuccessCount: 100,
					FlakeCount:   0,
					Release:      &release418,
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rfb := NewReleaseFallbackMiddleware(nil, test.reqOpts)
			rfb.cachedFallbackTestStatuses = test.fallbackReleases
			baseStatus, _, err := rfb.Transform(test.baseStatus, map[string]crtype.TestStatus{})
			assert.NoError(t, err)
			assert.Equal(t, test.expectedStatus, baseStatus)
		})
	}
}

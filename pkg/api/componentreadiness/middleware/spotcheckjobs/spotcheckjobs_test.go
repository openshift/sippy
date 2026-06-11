package spotcheckjobs

import (
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyze(t *testing.T) {
	now := time.Now()
	sampleStart := now.Add(-30 * 24 * time.Hour)
	sampleEnd := now

	spotCheckSample := reqopts.Release{
		Start: sampleStart,
		End:   sampleEnd,
	}

	mw := &SpotCheckJobs{
		reqOptions: reqopts.RequestOptions{
			SpotCheckSample: &spotCheckSample,
		},
	}

	spotCheckTestKey := crtest.Identification{
		RowIdentification: crtest.RowIdentification{
			TestID: "spotcheck:etcd:scaling",
		},
	}

	nonSpotCheckTestKey := crtest.Identification{
		RowIdentification: crtest.RowIdentification{
			TestID: "openshift-tests:some-real-test",
		},
	}

	tests := []struct {
		name                string
		testKey             crtest.Identification
		successCount        int
		failureCount        int
		expectedStatus      crtest.Status
		expectHandled       bool
		explanationContains string
	}{
		{
			name:           "non-spot-check test is not handled",
			testKey:        nonSpotCheckTestKey,
			successCount:   0,
			failureCount:   3,
			expectedStatus: 0, // unchanged
			expectHandled:  false,
		},
		{
			name:                "no runs - MissingSample",
			testKey:             spotCheckTestKey,
			successCount:        0,
			failureCount:        0,
			expectedStatus:      crtest.MissingSample,
			expectHandled:       true,
			explanationContains: "No spot-check job runs found",
		},
		{
			name:                "1 failure 0 successes - MissingSample awaiting retry",
			testKey:             spotCheckTestKey,
			successCount:        0,
			failureCount:        1,
			expectedStatus:      crtest.MissingSample,
			expectHandled:       true,
			explanationContains: "awaiting retry",
		},
		{
			name:                "2 failures 0 successes - SignificantRegression",
			testKey:             spotCheckTestKey,
			successCount:        0,
			failureCount:        2,
			expectedStatus:      crtest.SignificantRegression,
			expectHandled:       true,
			explanationContains: "failed 2 times",
		},
		{
			name:                "3 failures 0 successes - ExtremeRegression",
			testKey:             spotCheckTestKey,
			successCount:        0,
			failureCount:        3,
			expectedStatus:      crtest.ExtremeRegression,
			expectHandled:       true,
			explanationContains: "did not pass",
		},
		{
			name:                "5 failures 0 successes - ExtremeRegression",
			testKey:             spotCheckTestKey,
			successCount:        0,
			failureCount:        5,
			expectedStatus:      crtest.ExtremeRegression,
			expectHandled:       true,
			explanationContains: "5 runs, 0 successes",
		},
		{
			name:                "1 success 0 failures - NotSignificant",
			testKey:             spotCheckTestKey,
			successCount:        1,
			failureCount:        0,
			expectedStatus:      crtest.NotSignificant,
			expectHandled:       true,
			explanationContains: "passed 1 out of 1",
		},
		{
			name:                "1 success 3 failures - NotSignificant",
			testKey:             spotCheckTestKey,
			successCount:        1,
			failureCount:        3,
			expectedStatus:      crtest.NotSignificant,
			expectHandled:       true,
			explanationContains: "passed 1 out of 4",
		},
		{
			name:                "4 successes 0 failures - NotSignificant",
			testKey:             spotCheckTestKey,
			successCount:        4,
			failureCount:        0,
			expectedStatus:      crtest.NotSignificant,
			expectHandled:       true,
			explanationContains: "passed 4 out of 4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testStats := &testdetails.TestComparison{
				SampleStats: testdetails.ReleaseStats{
					Stats: crtest.NewTestStats(tt.successCount, tt.failureCount, 0, false),
				},
			}

			handled, err := mw.Analyze(tt.testKey, testStats)
			require.NoError(t, err)
			assert.Equal(t, tt.expectHandled, handled)

			if !tt.expectHandled {
				return
			}

			assert.Equal(t, tt.expectedStatus, testStats.ReportStatus, "unexpected status")
			assert.Equal(t, crtest.SpotCheck, testStats.Comparison)
			assert.Nil(t, testStats.BaseStats)
			require.Len(t, testStats.Explanations, 1)
			assert.Contains(t, testStats.Explanations[0], tt.explanationContains)
		})
	}
}

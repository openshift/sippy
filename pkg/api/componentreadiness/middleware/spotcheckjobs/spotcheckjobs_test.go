package spotcheckjobs

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/api/componentreadiness/dataprovider"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crstatus"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubProvider returns a fixed set of SpotCheckGroups; all other methods panic.
type stubProvider struct {
	dataprovider.DataProvider // embed to satisfy the interface; only spot-check methods are implemented
	groups                    []dataprovider.SpotCheckGroup
}

func (s *stubProvider) QuerySpotCheckJobRuns(_ context.Context, _ reqopts.RequestOptions,
	_ crtest.JobVariants, _ map[string][]string, _, _ time.Time) ([]dataprovider.SpotCheckGroup, error) {
	return s.groups, nil
}

func (s *stubProvider) QuerySpotCheckJobRunDetails(_ context.Context, _ reqopts.RequestOptions,
	_ crtest.JobVariants, _ map[string][]string, _ map[string]string, _, _ string, _, _ time.Time) ([]dataprovider.JobRunDetail, error) {
	return nil, nil
}

// runQuery is a test helper that runs the middleware Query and collects injected sample statuses.
func runQuery(t *testing.T, mw *SpotCheckJobs) map[string]crstatus.TestStatus {
	t.Helper()
	wg := &sync.WaitGroup{}
	baseCh := make(chan map[string]crstatus.TestStatus, 1)
	sampleCh := make(chan map[string]crstatus.TestStatus, 10)
	errCh := make(chan error, 10)

	mw.Query(context.Background(), wg, crtest.JobVariants{}, baseCh, sampleCh, errCh)
	wg.Wait()
	close(sampleCh)
	close(errCh)

	for err := range errCh {
		t.Fatalf("unexpected error from Query: %v", err)
	}

	merged := map[string]crstatus.TestStatus{}
	for batch := range sampleCh {
		for k, v := range batch {
			merged[k] = v
		}
	}
	return merged
}

func TestVariantsMatch(t *testing.T) {
	tests := []struct {
		name              string
		groupVariants     map[string]string
		requestedVariants map[string]string
		expected          bool
	}{
		{
			name:              "empty requested matches everything",
			groupVariants:     map[string]string{"Network": "ovn", "Platform": "aws"},
			requestedVariants: map[string]string{},
			expected:          true,
		},
		{
			name:              "nil requested matches everything",
			groupVariants:     map[string]string{"Network": "ovn", "Platform": "aws"},
			requestedVariants: nil,
			expected:          true,
		},
		{
			name:              "single match",
			groupVariants:     map[string]string{"Network": "ovn", "Platform": "aws", "Topology": "ha"},
			requestedVariants: map[string]string{"Platform": "aws"},
			expected:          true,
		},
		{
			name:              "all match",
			groupVariants:     map[string]string{"Network": "ovn", "Platform": "aws", "Topology": "ha"},
			requestedVariants: map[string]string{"Network": "ovn", "Platform": "aws", "Topology": "ha"},
			expected:          true,
		},
		{
			name:              "one mismatch",
			groupVariants:     map[string]string{"Network": "ovn", "Platform": "aws", "Topology": "ha"},
			requestedVariants: map[string]string{"Network": "ovn", "Platform": "gcp"},
			expected:          false,
		},
		{
			name:              "key missing from group",
			groupVariants:     map[string]string{"Network": "ovn"},
			requestedVariants: map[string]string{"Platform": "aws"},
			expected:          false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, variantsMatch(tt.groupVariants, tt.requestedVariants))
		})
	}
}

func newSpotCheckSamples() []reqopts.SpotCheckJobSampleOpts {
	return []reqopts.SpotCheckJobSampleOpts{
		{
			Name: "spotcheck-30d",
			Release: reqopts.Release{
				Start: time.Now().Add(-30 * 24 * time.Hour),
				End:   time.Now(),
			},
			IncludeVariants: map[string][]string{"JobTier": {"spotcheck-30d"}},
		},
	}
}

func TestQueryFiltering(t *testing.T) {
	allGroups := []dataprovider.SpotCheckGroup{
		{Component: "Etcd", Capability: "Scaling", Variants: map[string]string{"Network": "ovn", "Platform": "aws", "Topology": "ha"}, TotalRuns: 3, SuccessfulRuns: 2},
		{Component: "Etcd", Capability: "Scaling", Variants: map[string]string{"Network": "ovn", "Platform": "gcp", "Topology": "ha"}, TotalRuns: 3, SuccessfulRuns: 3},
		{Component: "Node / Kubelet", Capability: "CPU Partitioning", Variants: map[string]string{"Network": "ovn", "Platform": "aws", "Topology": "ha"}, TotalRuns: 2, SuccessfulRuns: 1},
	}

	tests := []struct {
		name              string
		requestedVariants map[string]string
		requestedComp     string
		expectedKeys      []string // syntheticTestID substrings to expect
		notExpectedKeys   []string
	}{
		{
			name:              "no filter - all groups injected",
			requestedVariants: map[string]string{},
			expectedKeys:      []string{"spotcheck-30d:etcd:scaling", "spotcheck-30d:node / kubelet:cpu-partitioning"},
		},
		{
			name:              "environment filter Platform=aws - excludes gcp",
			requestedVariants: map[string]string{"Platform": "aws"},
			expectedKeys:      []string{"spotcheck-30d:etcd:scaling", "spotcheck-30d:node / kubelet:cpu-partitioning"},
			notExpectedKeys:   []string{},
		},
		{
			name:              "environment filter Platform=gcp - only gcp etcd",
			requestedVariants: map[string]string{"Platform": "gcp"},
			notExpectedKeys:   []string{"spotcheck-30d:node / kubelet:cpu-partitioning"},
		},
		{
			name:              "environment filter Platform=metal - nothing matches",
			requestedVariants: map[string]string{"Platform": "metal"},
			expectedKeys:      []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := &SpotCheckJobs{
				dataProvider: &stubProvider{groups: allGroups},
				reqOptions: reqopts.RequestOptions{
					SpotCheckJobSamples: newSpotCheckSamples(),
					TestIDOptions: []reqopts.TestIdentification{
						{Component: tt.requestedComp, RequestedVariants: tt.requestedVariants},
					},
				},
				log: log.WithField("test", tt.name),
			}

			result := runQuery(t, mw)

			for _, key := range tt.expectedKeys {
				found := false
				for k := range result {
					if strings.Contains(k, key) {
						found = true
						break
					}
				}
				assert.True(t, found, "expected key containing %q in results", key)
			}
			for _, key := range tt.notExpectedKeys {
				for k := range result {
					assert.False(t, strings.Contains(k, key), "did not expect key containing %q in results", key)
				}
			}
		})
	}
}

func TestAnalyze(t *testing.T) {
	now := time.Now()
	sampleStart := now.Add(-30 * 24 * time.Hour)
	sampleEnd := now

	mw := &SpotCheckJobs{
		reqOptions: reqopts.RequestOptions{
			SpotCheckJobSamples: []reqopts.SpotCheckJobSampleOpts{
				{
					Name:    "spotcheck-30d",
					Release: reqopts.Release{Start: sampleStart, End: sampleEnd},
				},
			},
		},
	}

	spotCheckTestKey := crtest.Identification{
		RowIdentification: crtest.RowIdentification{
			TestID: "spotcheck-30d:etcd:scaling",
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

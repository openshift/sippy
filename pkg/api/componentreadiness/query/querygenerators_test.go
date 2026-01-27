package query

import (
	"strings"
	"testing"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/util/sets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildComponentReportQuery_ExclusiveTestFiltering(t *testing.T) {
	mockClient := &bqcachedclient.Client{
		Dataset: "test_dataset",
	}

	allJobVariants := crtest.JobVariants{
		Variants: map[string][]string{
			"Platform": {"aws", "gcp"},
			"Network":  {"sdn", "ovn"},
		},
	}

	baseReqOptions := reqopts.RequestOptions{
		VariantOption: reqopts.Variants{
			ColumnGroupBy:   sets.NewString("Platform"),
			DBGroupBy:       sets.NewString("Network"),
			IncludeVariants: map[string][]string{},
		},
		AdvancedOption: reqopts.Advanced{
			IgnoreDisruption: true,
		},
	}

	includeVariants := map[string][]string{}

	tests := []struct {
		name               string
		exclusiveTestNames []string
		expectedCTE        bool
		expectedFilter     bool
		expectedParam      bool
		expectedCTEContent string
	}{
		{
			name:               "No exclusive tests - no filtering",
			exclusiveTestNames: nil,
			expectedCTE:        false,
			expectedFilter:     false,
			expectedParam:      false,
		},
		{
			name:               "Empty exclusive tests - no filtering",
			exclusiveTestNames: []string{},
			expectedCTE:        false,
			expectedFilter:     false,
			expectedParam:      false,
		},
		{
			name: "With exclusive tests - filtering applied",
			exclusiveTestNames: []string{
				"[sig-cluster-lifecycle] Cluster completes upgrade",
				"install should succeed: overall",
			},
			expectedCTE:        true,
			expectedFilter:     true,
			expectedParam:      true,
			expectedCTEContent: "jobs_with_failed_exclusive_tests",
		},
		{
			name: "Single exclusive test - filtering applied",
			exclusiveTestNames: []string{
				"install should succeed: overall",
			},
			expectedCTE:        true,
			expectedFilter:     true,
			expectedParam:      true,
			expectedCTEContent: "jobs_with_failed_exclusive_tests",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqOptions := baseReqOptions
			reqOptions.AdvancedOption.ExclusiveTestNames = tt.exclusiveTestNames

			commonQuery, _, queryParams := BuildComponentReportQuery(
				mockClient,
				reqOptions,
				allJobVariants,
				includeVariants,
				DefaultJunitTable,
				false,
				tt.exclusiveTestNames...,
			)

			// Check if CTE is present when expected
			if tt.expectedCTE {
				assert.Contains(t, commonQuery, "jobs_with_failed_exclusive_tests",
					"Query should contain jobs_with_failed_exclusive_tests CTE")
				assert.Contains(t, commonQuery, "AND success_val = 0",
					"CTE should only identify jobs where exclusive tests FAILED (success_val = 0)")
				assert.Contains(t, commonQuery, "test_name IN UNNEST(@ExclusiveTestNames)",
					"CTE should filter by exclusive test names")
			} else {
				assert.NotContains(t, commonQuery, "jobs_with_failed_exclusive_tests",
					"Query should not contain jobs_with_failed_exclusive_tests CTE when no exclusive tests")
			}

			// Check if filtering WHERE clause is present when expected
			if tt.expectedFilter {
				assert.Contains(t, commonQuery, "test_name IN UNNEST(@ExclusiveTestNames)",
					"Query should include exclusive test names filter")
				assert.Contains(t, commonQuery, "prowjob_build_id NOT IN (SELECT prowjob_build_id FROM jobs_with_failed_exclusive_tests)",
					"Query should exclude tests from jobs with failed exclusive tests")
			}

			// Check if query parameter is present when expected
			if tt.expectedParam {
				foundParam := false
				for _, param := range queryParams {
					if param.Name == "ExclusiveTestNames" {
						foundParam = true
						assert.Equal(t, tt.exclusiveTestNames, param.Value,
							"ExclusiveTestNames parameter should match input")
						break
					}
				}
				assert.True(t, foundParam, "ExclusiveTestNames parameter should be present in query parameters")
			} else {
				for _, param := range queryParams {
					assert.NotEqual(t, "ExclusiveTestNames", param.Name,
						"ExclusiveTestNames parameter should not be present when no exclusive tests")
				}
			}

			// Verify CTE content structure if CTE is expected
			if tt.expectedCTEContent != "" {
				// Extract the CTE section
				parts := strings.Split(commonQuery, "latest_component_mapping")
				require.Greater(t, len(parts), 1, "Query should contain latest_component_mapping CTE")

				cteSection := parts[0]
				assert.Contains(t, cteSection, tt.expectedCTEContent,
					"Query should contain expected CTE")

				// Verify the CTE only selects prowjob_build_id for jobs where exclusive tests failed
				// Use a more lenient check that ignores whitespace variations
				normalizedCTE := strings.ReplaceAll(strings.ReplaceAll(cteSection, "\t", " "), "\n", " ")
				assert.Contains(t, normalizedCTE, "DISTINCT prowjob_build_id",
					"CTE should select distinct prowjob_build_id")
			}
		})
	}
}

func TestBuildComponentReportQuery_ExclusiveTestLogic(t *testing.T) {
	// This test verifies the specific logic: we only exclude OTHER tests from jobs
	// where exclusive tests FAILED (not just present)
	mockClient := &bqcachedclient.Client{
		Dataset: "test_dataset",
	}

	allJobVariants := crtest.JobVariants{
		Variants: map[string][]string{
			"Platform": {"aws"},
		},
	}

	reqOptions := reqopts.RequestOptions{
		VariantOption: reqopts.Variants{
			ColumnGroupBy:   sets.NewString("Platform"),
			DBGroupBy:       sets.NewString(),
			IncludeVariants: map[string][]string{},
		},
		AdvancedOption: reqopts.Advanced{
			ExclusiveTestNames: []string{"install should succeed: overall"},
		},
	}

	commonQuery, _, _ := BuildComponentReportQuery(
		mockClient,
		reqOptions,
		allJobVariants,
		map[string][]string{},
		DefaultJunitTable,
		false,
		"install should succeed: overall",
	)

	// The query should:
	// 1. Create a CTE that identifies jobs where exclusive tests FAILED
	assert.Contains(t, commonQuery, "WITH jobs_with_failed_exclusive_tests AS",
		"Should create CTE for failed exclusive tests")

	// 2. The CTE should check success_val = 0 (failure)
	cteEnd := strings.Index(commonQuery, "latest_component_mapping")
	require.Greater(t, cteEnd, 0, "Should contain latest_component_mapping CTE")

	cteSection := commonQuery[:cteEnd]
	assert.Contains(t, cteSection, "success_val = 0",
		"CTE should only match FAILED exclusive tests (success_val = 0), not all instances")

	// 3. Include the exclusive test itself OR tests from jobs without failed exclusive tests
	assert.Contains(t, commonQuery, "test_name IN UNNEST(@ExclusiveTestNames)",
		"Should always include the exclusive tests themselves")
	assert.Contains(t, commonQuery, "prowjob_build_id NOT IN (SELECT prowjob_build_id FROM jobs_with_failed_exclusive_tests)",
		"Should exclude other tests from jobs where exclusive tests failed")

	// 4. Verify the logic is an OR condition (include exclusive tests OR tests from clean jobs)
	// Normalize whitespace for easier parsing
	normalizedQuery := strings.ReplaceAll(strings.ReplaceAll(commonQuery, "\t", " "), "\n", " ")
	normalizedQuery = strings.Join(strings.Fields(normalizedQuery), " ") // Collapse all whitespace

	// The query should contain the OR logic between the two conditions
	assert.Contains(t, normalizedQuery, "test_name IN UNNEST(@ExclusiveTestNames) OR prowjob_build_id NOT IN",
		"Should have OR condition between exclusive test filter and job filter")
}

func TestBuildComponentReportQuery_WithAndWithoutExclusiveTests(t *testing.T) {
	// This test compares queries with and without exclusive tests to ensure
	// the base query structure is the same, only the filtering differs
	mockClient := &bqcachedclient.Client{
		Dataset: "test_dataset",
	}

	allJobVariants := crtest.JobVariants{
		Variants: map[string][]string{
			"Platform": {"aws"},
		},
	}

	baseReqOptions := reqopts.RequestOptions{
		VariantOption: reqopts.Variants{
			ColumnGroupBy:   sets.NewString("Platform"),
			DBGroupBy:       sets.NewString(),
			IncludeVariants: map[string][]string{},
		},
		AdvancedOption: reqopts.Advanced{},
	}

	// Query without exclusive tests
	queryWithout, _, paramsWithout := BuildComponentReportQuery(
		mockClient,
		baseReqOptions,
		allJobVariants,
		map[string][]string{},
		DefaultJunitTable,
		false,
	)

	// Query with exclusive tests
	reqOptionsWithExclusive := baseReqOptions
	reqOptionsWithExclusive.AdvancedOption.ExclusiveTestNames = []string{"install should succeed: overall"}

	queryWith, _, paramsWith := BuildComponentReportQuery(
		mockClient,
		reqOptionsWithExclusive,
		allJobVariants,
		map[string][]string{},
		DefaultJunitTable,
		false,
		"install should succeed: overall",
	)

	// Both should have the component_mapping CTE
	assert.Contains(t, queryWithout, "latest_component_mapping",
		"Query without exclusive tests should have component mapping CTE")
	assert.Contains(t, queryWith, "latest_component_mapping",
		"Query with exclusive tests should have component mapping CTE")

	// Only the query with exclusive tests should have the filtering CTE
	assert.NotContains(t, queryWithout, "jobs_with_failed_exclusive_tests",
		"Query without exclusive tests should not have filtering CTE")
	assert.Contains(t, queryWith, "jobs_with_failed_exclusive_tests",
		"Query with exclusive tests should have filtering CTE")

	// Check parameters
	assert.Len(t, paramsWithout, 0, "Query without exclusive tests should have no extra parameters")
	assert.Len(t, paramsWith, 1, "Query with exclusive tests should have 1 parameter")
	if len(paramsWith) > 0 {
		assert.Equal(t, "ExclusiveTestNames", paramsWith[0].Name,
			"Parameter should be named ExclusiveTestNames")
	}
}

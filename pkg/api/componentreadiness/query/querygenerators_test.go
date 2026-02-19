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
			expectedCTEContent: "jobs_with_highest_priority_test",
		},
		{
			name: "Single exclusive test - filtering applied",
			exclusiveTestNames: []string{
				"install should succeed: overall",
			},
			expectedCTE:        true,
			expectedFilter:     true,
			expectedParam:      true,
			expectedCTEContent: "jobs_with_highest_priority_test",
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
				assert.Contains(t, commonQuery, "jobs_with_highest_priority_test",
					"Query should contain jobs_with_highest_priority_test CTE")
				assert.Contains(t, commonQuery, "exclusive_test_priorities",
					"Query should contain exclusive_test_priorities CTE for priority calculation")
				assert.Contains(t, commonQuery, "AND success_val = 0",
					"CTE should only identify jobs where exclusive tests FAILED (success_val = 0)")
				assert.Contains(t, commonQuery, "test_name IN UNNEST(@ExclusiveTestNames)",
					"CTE should filter by exclusive test names")
				assert.Contains(t, commonQuery, "test_priority",
					"CTE should calculate test priority based on list order")
			} else {
				assert.NotContains(t, commonQuery, "jobs_with_highest_priority_test",
					"Query should not contain jobs_with_highest_priority_test CTE when no exclusive tests")
				assert.NotContains(t, commonQuery, "exclusive_test_priorities",
					"Query should not contain exclusive_test_priorities CTE when no exclusive tests")
			}

			// Check if filtering WHERE clause is present when expected
			if tt.expectedFilter {
				assert.Contains(t, commonQuery, "junit_data.prowjob_build_id NOT IN (SELECT prowjob_build_id FROM jobs_with_highest_priority_test)",
					"Query should include tests from jobs without failed exclusive tests")
				assert.Contains(t, commonQuery, "EXISTS",
					"Query should use EXISTS to match highest priority test from jobs with exclusive tests")
				assert.Contains(t, commonQuery, "j.prowjob_build_id = junit_data.prowjob_build_id",
					"Query should match both prowjob_build_id and test_name in EXISTS clause")
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

				// Verify the CTE selects prowjob_build_id and test_name for priority-based filtering
				// The new structure identifies the highest priority test per job
				normalizedCTE := strings.ReplaceAll(strings.ReplaceAll(cteSection, "\t", " "), "\n", " ")
				assert.Contains(t, normalizedCTE, "prowjob_build_id",
					"CTE should select prowjob_build_id")
				assert.Contains(t, normalizedCTE, "test_name",
					"CTE should select test_name to identify the highest priority test")
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
	// 1. Create CTEs that identify the highest priority test in each job
	assert.Contains(t, commonQuery, "WITH exclusive_test_priorities AS",
		"Should create CTE for calculating test priorities")
	assert.Contains(t, commonQuery, "jobs_with_highest_priority_test AS",
		"Should create CTE for identifying highest priority test per job")

	// 2. The CTE should check success_val = 0 (failure)
	cteEnd := strings.Index(commonQuery, "latest_component_mapping")
	require.Greater(t, cteEnd, 0, "Should contain latest_component_mapping CTE")

	cteSection := commonQuery[:cteEnd]
	assert.Contains(t, cteSection, "success_val = 0",
		"CTE should only match FAILED exclusive tests (success_val = 0), not all instances")

	// 3. Priority-based filtering: only include highest priority test from jobs with exclusive test failures
	assert.Contains(t, commonQuery, "test_priority",
		"Should calculate test priority based on list order")
	assert.Contains(t, commonQuery, "junit_data.prowjob_build_id NOT IN (SELECT prowjob_build_id FROM jobs_with_highest_priority_test)",
		"Should include tests from jobs without failed exclusive tests")
	assert.Contains(t, commonQuery, "EXISTS",
		"Should use EXISTS to match the highest priority test from jobs with exclusive test failures")
	assert.Contains(t, commonQuery, "j.prowjob_build_id = junit_data.prowjob_build_id",
		"Should match prowjob_build_id in EXISTS clause")
	assert.Contains(t, commonQuery, "j.test_name = junit_data.test_name",
		"Should match test_name in EXISTS clause")

	// 4. Verify the logic correctly filters based on priority
	// Normalize whitespace for easier parsing
	normalizedQuery := strings.ReplaceAll(strings.ReplaceAll(commonQuery, "\t", " "), "\n", " ")
	normalizedQuery = strings.Join(strings.Fields(normalizedQuery), " ") // Collapse all whitespace

	// The query should contain the priority-based filtering logic using EXISTS
	assert.Contains(t, normalizedQuery, "jobs_with_highest_priority_test j",
		"Should use jobs_with_highest_priority_test in EXISTS subquery")
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

	// Only the query with exclusive tests should have the filtering CTEs
	assert.NotContains(t, queryWithout, "jobs_with_highest_priority_test",
		"Query without exclusive tests should not have filtering CTE")
	assert.NotContains(t, queryWithout, "exclusive_test_priorities",
		"Query without exclusive tests should not have priority calculation CTE")
	assert.Contains(t, queryWith, "jobs_with_highest_priority_test",
		"Query with exclusive tests should have filtering CTE")
	assert.Contains(t, queryWith, "exclusive_test_priorities",
		"Query with exclusive tests should have priority calculation CTE")

	// Check parameters
	assert.Len(t, paramsWithout, 0, "Query without exclusive tests should have no extra parameters")
	assert.Len(t, paramsWith, 1, "Query with exclusive tests should have 1 parameter")
	if len(paramsWith) > 0 {
		assert.Equal(t, "ExclusiveTestNames", paramsWith[0].Name,
			"Parameter should be named ExclusiveTestNames")
	}
}

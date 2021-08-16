package api

import (
	"testing"

	apitype "github.com/openshift/sippy/pkg/apis/api"
)

func TestLinkOperator(t *testing.T) {
	cases := []struct {
		name     string
		filter   Filter
		job      apitype.Job
		expected bool
	}{
		{
			name: "filter_by_name_AND_current_pass_rate_false",
			job: apitype.Job{
				ID:                    1,
				Name:                  "e2e-aws",
				CurrentPassPercentage: 90.5,
			},
			filter: Filter{
				Items: []FilterItem{
					{
						Field:    "name",
						Operator: "contains",
						Value:    "e2e",
					},
					{
						Field:    "current_pass_percentage",
						Operator: ">",
						Value:    "95",
					},
				},
				LinkOperator: LinkOperatorAnd,
			},
			expected: false,
		},
		{
			name: "filter_by_name_AND_current_pass_rate_true",
			job: apitype.Job{
				ID:                    1,
				Name:                  "e2e-aws",
				CurrentPassPercentage: 90.5,
			},
			filter: Filter{
				Items: []FilterItem{
					{
						Field:    "name",
						Operator: "contains",
						Value:    "e2e",
					},
					{
						Field:    "current_pass_percentage",
						Operator: "<",
						Value:    "95",
					},
				},
				LinkOperator: LinkOperatorAnd,
			},
			expected: true,
		},
		{
			name: "filter_by_name_OR_current_pass_rate_true",
			job: apitype.Job{
				ID:                    1,
				Name:                  "e2e-aws",
				CurrentPassPercentage: 90.5,
			},
			filter: Filter{
				Items: []FilterItem{
					{
						Field:    "name",
						Operator: "contains",
						Value:    "aws",
					},
					{
						Field:    "current_pass_percentage",
						Operator: ">",
						Value:    "95",
					},
				},
				LinkOperator: LinkOperatorOr,
			},
			expected: true,
		},
		{
			name: "filter_by_name_OR_current_pass_rate_false",
			job: apitype.Job{
				ID:                    1,
				Name:                  "e2e-aws",
				CurrentPassPercentage: 90.5,
			},
			filter: Filter{
				Items: []FilterItem{
					{
						Field:    "name",
						Operator: "contains",
						Value:    "gcp",
					},
					{
						Field:    "current_pass_percentage",
						Operator: ">",
						Value:    "95",
					},
				},
				LinkOperator: LinkOperatorOr,
			},
			expected: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.filter.Filter(tc.job)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if result != tc.expected {
				t.Fatalf("unexpected result, got %v, expected %v", result, tc.expected)
			}

		})
	}
}

func TestFilterableStrings(t *testing.T) {
	cases := []struct {
		name     string
		filter   Filter
		job      apitype.Job
		expected bool
	}{
		{
			name:     "no_filter",
			job:      apitype.Job{Name: "e2e-test"},
			filter:   Filter{},
			expected: true,
		},
		{
			name: "filter_by_name_contains_true",
			job:  apitype.Job{Name: "e2e-test"},
			filter: Filter{
				Items: []FilterItem{
					{
						Field:    "name",
						Operator: "contains",
						Value:    "test",
					},
				},
				LinkOperator: LinkOperatorAnd,
			},
			expected: true,
		},
		{
			name: "filter_by_name_contains_false",
			job:  apitype.Job{Name: "e2e-test"},
			filter: Filter{
				Items: []FilterItem{
					{
						Field:    "name",
						Operator: "contains",
						Value:    "gcp",
					},
				},
				LinkOperator: LinkOperatorAnd,
			},
			expected: false,
		},
		{
			name: "filter_by_name_equals_true",
			job:  apitype.Job{Name: "e2e-test"},
			filter: Filter{
				Items: []FilterItem{
					{
						Field:    "name",
						Operator: "equals",
						Value:    "e2e-test",
					},
				},
				LinkOperator: LinkOperatorAnd,
			},
			expected: true,
		},
		{
			name: "filter_by_name_equals_false",
			job:  apitype.Job{Name: "e2e-test"},
			filter: Filter{
				Items: []FilterItem{
					{
						Field:    "name",
						Operator: "equals",
						Value:    "e2e-",
					},
				},
				LinkOperator: LinkOperatorAnd,
			},
			expected: false,
		},
		{
			name: "filter_by_name_starts_with_true",
			job:  apitype.Job{Name: "e2e-test"},
			filter: Filter{
				Items: []FilterItem{
					{
						Field:    "name",
						Operator: "starts with",
						Value:    "e2e-",
					},
				},
				LinkOperator: LinkOperatorAnd,
			},
			expected: true,
		},
		{
			name: "filter_by_name_starts_with_false",
			job:  apitype.Job{Name: "e2e-test"},
			filter: Filter{
				Items: []FilterItem{
					{
						Field:    "name",
						Operator: "starts with",
						Value:    "test",
					},
				},
				LinkOperator: LinkOperatorAnd,
			},
			expected: false,
		},
		{
			name: "filter_by_name_ends_with",
			job:  apitype.Job{Name: "e2e-test"},
			filter: Filter{
				Items: []FilterItem{
					{
						Field:    "name",
						Operator: "ends with",
						Value:    "-test",
					},
				},
				LinkOperator: LinkOperatorAnd,
			},
			expected: true,
		},
		{
			name: "filter_by_name_ends_with",
			job:  apitype.Job{Name: "e2e-test"},
			filter: Filter{
				Items: []FilterItem{
					{
						Field:    "name",
						Operator: "ends with",
						Value:    "e2e",
					},
				},
				LinkOperator: LinkOperatorAnd,
			},
			expected: false,
		},
		{
			name: "filter_by_name_is_empty_true",
			job:  apitype.Job{Name: ""},
			filter: Filter{
				Items: []FilterItem{
					{
						Field:    "name",
						Operator: "is empty",
					},
				},
				LinkOperator: LinkOperatorAnd,
			},
			expected: true,
		},
		{
			name: "filter_by_name_is_empty_false",
			job:  apitype.Job{Name: "e2e-test"},
			filter: Filter{
				Items: []FilterItem{
					{
						Field:    "name",
						Operator: "is empty",
					},
				},
				LinkOperator: LinkOperatorAnd,
			},
			expected: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.filter.Filter(tc.job)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if result != tc.expected {
				t.Fatalf("unexpected result, got %v, expected %v", result, tc.expected)
			}

		})
	}
}

func TestFilterableNumerical(t *testing.T) {
	cases := []struct {
		name     string
		filter   Filter
		job      apitype.Job
		expected bool
	}{
		{
			name:     "no_filter",
			job:      apitype.Job{Name: "e2e-test", CurrentPassPercentage: 92.5},
			filter:   Filter{},
			expected: true,
		},
		{
			name: "filter_by_percentage_equals",
			job:  apitype.Job{Name: "e2e-test", CurrentPassPercentage: 92.5},
			filter: Filter{
				Items: []FilterItem{
					{
						Field:    "current_pass_percentage",
						Operator: "=",
						Value:    "92.5",
					},
				},
				LinkOperator: LinkOperatorAnd,
			},
			expected: true,
		},
		{
			name: "filter_by_percentage_not_equal",
			job:  apitype.Job{Name: "e2e-test", CurrentPassPercentage: 92.5},
			filter: Filter{
				Items: []FilterItem{
					{
						Field:    "current_pass_percentage",
						Operator: "!=",
						Value:    "92.4",
					},
				},
				LinkOperator: LinkOperatorAnd,
			},
			expected: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.filter.Filter(tc.job)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if result != tc.expected {
				t.Fatalf("unexpected result, got %v, expected %v", result, tc.expected)
			}

		})
	}
}

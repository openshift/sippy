package api

import (
	"context"
	"errors"
	"strings"
	"testing"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/filter"
)

func TestComputeOverallTest(t *testing.T) {
	tests := []struct {
		name     string
		input    []apitype.Test
		expected apitype.Test
	}{
		{
			name:  "empty input",
			input: []apitype.Test{},
			expected: apitype.Test{
				Name: "Overall",
			},
		},
		{
			name: "zero runs",
			input: []apitype.Test{
				{CurrentRuns: 0, PreviousRuns: 0},
			},
			expected: apitype.Test{
				Name: "Overall",
			},
		},
		{
			name: "single test",
			input: []apitype.Test{
				{
					CurrentRuns: 100, CurrentSuccesses: 90, CurrentFailures: 8, CurrentFlakes: 2,
					PreviousRuns: 80, PreviousSuccesses: 70, PreviousFailures: 5, PreviousFlakes: 5,
				},
			},
			expected: apitype.Test{
				Name:                      "Overall",
				CurrentRuns:               100,
				CurrentSuccesses:          90,
				CurrentFailures:           8,
				CurrentFlakes:             2,
				CurrentPassPercentage:     90.0,
				CurrentFailurePercentage:  8.0,
				CurrentFlakePercentage:    2.0,
				CurrentWorkingPercentage:  92.0,
				PreviousRuns:              80,
				PreviousSuccesses:         70,
				PreviousFailures:          5,
				PreviousFlakes:            5,
				PreviousPassPercentage:    87.5,
				PreviousFailurePercentage: 6.25,
				PreviousFlakePercentage:   6.25,
				PreviousWorkingPercentage: 93.75,
				NetFailureImprovement:     -1.75,
				NetFlakeImprovement:       4.25,
				NetWorkingImprovement:     -1.75,
				NetImprovement:            2.5,
			},
		},
		{
			name: "multiple tests aggregated",
			input: []apitype.Test{
				{
					CurrentRuns: 100, CurrentSuccesses: 90, CurrentFailures: 8, CurrentFlakes: 2,
					PreviousRuns: 100, PreviousSuccesses: 80, PreviousFailures: 15, PreviousFlakes: 5,
				},
				{
					CurrentRuns: 100, CurrentSuccesses: 95, CurrentFailures: 3, CurrentFlakes: 2,
					PreviousRuns: 100, PreviousSuccesses: 95, PreviousFailures: 3, PreviousFlakes: 2,
				},
			},
			expected: apitype.Test{
				Name:                      "Overall",
				CurrentRuns:               200,
				CurrentSuccesses:          185,
				CurrentFailures:           11,
				CurrentFlakes:             4,
				CurrentPassPercentage:     92.5,
				CurrentFailurePercentage:  5.5,
				CurrentFlakePercentage:    2.0,
				CurrentWorkingPercentage:  94.5,
				PreviousRuns:              200,
				PreviousSuccesses:         175,
				PreviousFailures:          18,
				PreviousFlakes:            7,
				PreviousPassPercentage:    87.5,
				PreviousFailurePercentage: 9.0,
				PreviousFlakePercentage:   3.5,
				PreviousWorkingPercentage: 91.0,
				NetFailureImprovement:     3.5,
				NetFlakeImprovement:       1.5,
				NetWorkingImprovement:     3.5,
				NetImprovement:            5.0,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := computeOverallTest(tc.input)
			if result.Name != tc.expected.Name {
				t.Errorf("Name = %q, want %q", result.Name, tc.expected.Name)
			}
			if result.CurrentRuns != tc.expected.CurrentRuns {
				t.Errorf("CurrentRuns = %d, want %d", result.CurrentRuns, tc.expected.CurrentRuns)
			}
			if result.CurrentSuccesses != tc.expected.CurrentSuccesses {
				t.Errorf("CurrentSuccesses = %d, want %d", result.CurrentSuccesses, tc.expected.CurrentSuccesses)
			}
			if result.CurrentFailures != tc.expected.CurrentFailures {
				t.Errorf("CurrentFailures = %d, want %d", result.CurrentFailures, tc.expected.CurrentFailures)
			}
			if result.CurrentFlakes != tc.expected.CurrentFlakes {
				t.Errorf("CurrentFlakes = %d, want %d", result.CurrentFlakes, tc.expected.CurrentFlakes)
			}
			if result.PreviousRuns != tc.expected.PreviousRuns {
				t.Errorf("PreviousRuns = %d, want %d", result.PreviousRuns, tc.expected.PreviousRuns)
			}
			if result.PreviousSuccesses != tc.expected.PreviousSuccesses {
				t.Errorf("PreviousSuccesses = %d, want %d", result.PreviousSuccesses, tc.expected.PreviousSuccesses)
			}
			if result.PreviousFailures != tc.expected.PreviousFailures {
				t.Errorf("PreviousFailures = %d, want %d", result.PreviousFailures, tc.expected.PreviousFailures)
			}
			if result.PreviousFlakes != tc.expected.PreviousFlakes {
				t.Errorf("PreviousFlakes = %d, want %d", result.PreviousFlakes, tc.expected.PreviousFlakes)
			}
			assertFloat(t, "CurrentPassPercentage", result.CurrentPassPercentage, tc.expected.CurrentPassPercentage)
			assertFloat(t, "CurrentFailurePercentage", result.CurrentFailurePercentage, tc.expected.CurrentFailurePercentage)
			assertFloat(t, "CurrentFlakePercentage", result.CurrentFlakePercentage, tc.expected.CurrentFlakePercentage)
			assertFloat(t, "CurrentWorkingPercentage", result.CurrentWorkingPercentage, tc.expected.CurrentWorkingPercentage)
			assertFloat(t, "PreviousPassPercentage", result.PreviousPassPercentage, tc.expected.PreviousPassPercentage)
			assertFloat(t, "PreviousFailurePercentage", result.PreviousFailurePercentage, tc.expected.PreviousFailurePercentage)
			assertFloat(t, "PreviousFlakePercentage", result.PreviousFlakePercentage, tc.expected.PreviousFlakePercentage)
			assertFloat(t, "PreviousWorkingPercentage", result.PreviousWorkingPercentage, tc.expected.PreviousWorkingPercentage)
			assertFloat(t, "NetFailureImprovement", result.NetFailureImprovement, tc.expected.NetFailureImprovement)
			assertFloat(t, "NetFlakeImprovement", result.NetFlakeImprovement, tc.expected.NetFlakeImprovement)
			assertFloat(t, "NetWorkingImprovement", result.NetWorkingImprovement, tc.expected.NetWorkingImprovement)
			assertFloat(t, "NetImprovement", result.NetImprovement, tc.expected.NetImprovement)
		})
	}
}

func assertFloat(t *testing.T, name string, got, want float64) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %f, want %f", name, got, want)
	}
}

// TestBuildTestsResultsBQGenerator_RejectsLifecycleFilter guards the /api/tests/v2
// (BigQuery-backed) 400 response: the underlying junit_7day_comparison/
// junit_2day_comparison tables have no lifecycle column, so a lifecycle filter must be
// rejected explicitly rather than silently ignored. This never reaches bqc, so passing
// nil is safe.
func TestBuildTestsResultsBQGenerator_RejectsLifecycleFilter(t *testing.T) {
	spec := &TestResultsSpec{
		Release: "4.16",
		Filter: &filter.Filter{Items: []filter.FilterItem{
			{Field: "lifecycle", Operator: filter.OperatorEquals, Value: "blocking"},
		}},
	}

	_, errs := spec.buildTestsResultsBQGenerator(context.Background(), nil)
	if len(errs) != 1 {
		t.Fatalf("errs = %v, want exactly one error", errs)
	}
	var validationErr *ValidationError
	if !errors.As(errs[0], &validationErr) {
		t.Fatalf("errs[0] = %v (%T), want *ValidationError so it surfaces as HTTP 400", errs[0], errs[0])
	}
	if !strings.Contains(validationErr.Message, "lifecycle") {
		t.Errorf("validation message = %q, want it to mention lifecycle", validationErr.Message)
	}
}

func TestBuildTestsResultsBQGenerator_RejectsLifecycleFilterAlongsideOtherFilters(t *testing.T) {
	spec := &TestResultsSpec{
		Release: "4.16",
		Filter: &filter.Filter{Items: []filter.FilterItem{
			{Field: "name", Operator: filter.OperatorContains, Value: "network"},
			{Field: "lifecycle", Operator: filter.OperatorArithmeticNotEquals, Value: "informing"},
		}},
	}

	_, errs := spec.buildTestsResultsBQGenerator(context.Background(), nil)
	if len(errs) != 1 {
		t.Fatalf("errs = %v, want exactly one error", errs)
	}
	var validationErr *ValidationError
	if !errors.As(errs[0], &validationErr) {
		t.Fatalf("errs[0] = %v (%T), want *ValidationError", errs[0], errs[0])
	}
}

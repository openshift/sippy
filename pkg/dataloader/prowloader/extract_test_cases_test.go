package prowloader

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openshift/sippy/pkg/apis/junit"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

func TestExtractTestCases(t *testing.T) {
	failMsg := "something went wrong"

	tests := []struct {
		name     string
		suite    *junit.TestSuite
		expected map[string]*testCaseEntry
	}{
		{
			name: "passing test",
			suite: &junit.TestSuite{
				Name: "openshift-tests",
				TestCases: []*junit.TestCase{
					{Name: "test-a", Duration: 1.5},
				},
			},
			expected: map[string]*testCaseEntry{
				"openshift-tests.test-a": {
					TestName:  "test-a",
					SuiteName: "openshift-tests",
					Status:    int(sippyprocessingv1.TestStatusSuccess),
					Duration:  1.5,
				},
			},
		},
		{
			name: "failing test with output",
			suite: &junit.TestSuite{
				Name: "openshift-tests",
				TestCases: []*junit.TestCase{
					{Name: "test-a", Duration: 2.0, FailureOutput: &junit.FailureOutput{Output: failMsg}},
				},
			},
			expected: map[string]*testCaseEntry{
				"openshift-tests.test-a": {
					TestName:  "test-a",
					SuiteName: "openshift-tests",
					Status:    int(sippyprocessingv1.TestStatusFailure),
					Duration:  2.0,
					Output:    &failMsg,
				},
			},
		},
		{
			name: "skipped test excluded",
			suite: &junit.TestSuite{
				Name: "openshift-tests",
				TestCases: []*junit.TestCase{
					{Name: "test-a", SkipMessage: &junit.SkipMessage{Message: "skipped"}},
				},
			},
			expected: map[string]*testCaseEntry{},
		},
		{
			name: "flake from pass then fail",
			suite: &junit.TestSuite{
				Name: "openshift-tests",
				TestCases: []*junit.TestCase{
					{Name: "test-a", Duration: 1.0},
					{Name: "test-a", Duration: 2.0, FailureOutput: &junit.FailureOutput{Output: failMsg}},
				},
			},
			expected: map[string]*testCaseEntry{
				"openshift-tests.test-a": {
					TestName:  "test-a",
					SuiteName: "openshift-tests",
					Status:    int(sippyprocessingv1.TestStatusFlake),
					Duration:  1.0,
					Output:    &failMsg,
				},
			},
		},
		{
			name: "child suite uses its own name",
			suite: &junit.TestSuite{
				Name: "openshift-tests",
				TestCases: []*junit.TestCase{
					{Name: "parent-test", Duration: 1.0},
				},
				Children: []*junit.TestSuite{
					{
						Name: "k8s.io",
						TestCases: []*junit.TestCase{
							{Name: "child-test", Duration: 3.0},
						},
					},
				},
			},
			expected: map[string]*testCaseEntry{
				"openshift-tests.parent-test": {
					TestName:  "parent-test",
					SuiteName: "openshift-tests",
					Status:    int(sippyprocessingv1.TestStatusSuccess),
					Duration:  1.0,
				},
				"k8s.io.child-test": {
					TestName:  "child-test",
					SuiteName: "k8s.io",
					Status:    int(sippyprocessingv1.TestStatusSuccess),
					Duration:  3.0,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testCases := make(map[string]*testCaseEntry)
			extractTestCases(tt.suite, testCases)
			assert.Equal(t, tt.expected, testCases)
		})
	}
}

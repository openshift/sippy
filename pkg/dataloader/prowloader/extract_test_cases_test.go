package prowloader

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openshift/sippy/pkg/apis/junit"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/types"
)

func TestExtractTestCases(t *testing.T) {
	failMsg := "something went wrong"

	tests := []struct {
		name     string
		suite    *junit.TestSuite
		expected map[testCaseKey]*types.TestCaseEntry
	}{
		{
			name: "passing test",
			suite: &junit.TestSuite{
				Name: "openshift-tests",
				TestCases: []*junit.TestCase{
					{Name: "test-a", Duration: 1.5},
				},
			},
			expected: map[testCaseKey]*types.TestCaseEntry{
				{SuiteName: "openshift-tests", TestName: "test-a"}: {
					TestName:  "test-a",
					SuiteName: "openshift-tests",
					Status:    int(sippyprocessingv1.TestStatusSuccess),
					Duration:  1.5,
					Lifecycle: "blocking",
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
			expected: map[testCaseKey]*types.TestCaseEntry{
				{SuiteName: "openshift-tests", TestName: "test-a"}: {
					TestName:  "test-a",
					SuiteName: "openshift-tests",
					Status:    int(sippyprocessingv1.TestStatusFailure),
					Duration:  2.0,
					Output:    &failMsg,
					Lifecycle: "blocking",
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
			expected: map[testCaseKey]*types.TestCaseEntry{},
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
			expected: map[testCaseKey]*types.TestCaseEntry{
				{SuiteName: "openshift-tests", TestName: "test-a"}: {
					TestName:  "test-a",
					SuiteName: "openshift-tests",
					Status:    int(sippyprocessingv1.TestStatusFlake),
					Duration:  1.0,
					Output:    &failMsg,
					Lifecycle: "blocking",
				},
			},
		},
		{
			name: "dotted names do not collide",
			suite: &junit.TestSuite{
				Name: "a.b",
				TestCases: []*junit.TestCase{
					{Name: "c", Duration: 1.0},
				},
				Children: []*junit.TestSuite{
					{
						Name: "a",
						TestCases: []*junit.TestCase{
							{Name: "b.c", Duration: 2.0},
						},
					},
				},
			},
			expected: map[testCaseKey]*types.TestCaseEntry{
				{SuiteName: "a.b", TestName: "c"}: {
					TestName:  "c",
					SuiteName: "a.b",
					Status:    int(sippyprocessingv1.TestStatusSuccess),
					Duration:  1.0,
					Lifecycle: "blocking",
				},
				{SuiteName: "a", TestName: "b.c"}: {
					TestName:  "b.c",
					SuiteName: "a",
					Status:    int(sippyprocessingv1.TestStatusSuccess),
					Duration:  2.0,
					Lifecycle: "blocking",
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
			expected: map[testCaseKey]*types.TestCaseEntry{
				{SuiteName: "openshift-tests", TestName: "parent-test"}: {
					TestName:  "parent-test",
					SuiteName: "openshift-tests",
					Status:    int(sippyprocessingv1.TestStatusSuccess),
					Duration:  1.0,
					Lifecycle: "blocking",
				},
				{SuiteName: "k8s.io", TestName: "child-test"}: {
					TestName:  "child-test",
					SuiteName: "k8s.io",
					Status:    int(sippyprocessingv1.TestStatusSuccess),
					Duration:  3.0,
					Lifecycle: "blocking",
				},
			},
		},
		{
			name: "informing lifecycle preserved from JUnit XML",
			suite: &junit.TestSuite{
				Name: "openshift-tests",
				TestCases: []*junit.TestCase{
					{Name: "test-informing", Duration: 1.0, Lifecycle: "informing"},
					{Name: "test-blocking", Duration: 2.0, Lifecycle: "blocking"},
					{Name: "test-empty", Duration: 3.0},
				},
			},
			expected: map[testCaseKey]*types.TestCaseEntry{
				{SuiteName: "openshift-tests", TestName: "test-informing"}: {
					TestName:  "test-informing",
					SuiteName: "openshift-tests",
					Status:    int(sippyprocessingv1.TestStatusSuccess),
					Duration:  1.0,
					Lifecycle: "informing",
				},
				{SuiteName: "openshift-tests", TestName: "test-blocking"}: {
					TestName:  "test-blocking",
					SuiteName: "openshift-tests",
					Status:    int(sippyprocessingv1.TestStatusSuccess),
					Duration:  2.0,
					Lifecycle: "blocking",
				},
				{SuiteName: "openshift-tests", TestName: "test-empty"}: {
					TestName:  "test-empty",
					SuiteName: "openshift-tests",
					Status:    int(sippyprocessingv1.TestStatusSuccess),
					Duration:  3.0,
					Lifecycle: "blocking",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testCases := make(map[testCaseKey]*types.TestCaseEntry)
			extractTestCases(tt.suite, testCases)
			assert.Equal(t, tt.expected, testCases)
		})
	}
}

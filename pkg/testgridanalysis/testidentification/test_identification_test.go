package testidentification_test

import (
	"testing"

	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"
)

func TestMultistageJobName(t *testing.T) {
	emptyOutput := ""

	testCases := []struct {
		inputTestName  string
		expectedOutput string
	}{
		{
			inputTestName:  "operator.Run multi-stage test e2e-aws-csi-migration",
			expectedOutput: "e2e-aws-csi-migration",
		},
		{
			// This should not match because it is the full multistage job name
			// with stage name
			inputTestName:  "operator.Run multi-stage test e2e-aws-arm64 - e2e-aws-arm64-openshift-e2e-test container test",
			expectedOutput: emptyOutput,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.inputTestName, func(t *testing.T) {
			actual := testidentification.GetMultistageJobNameFromTest(testCase.inputTestName)

			if actual != testCase.expectedOutput {
				t.Errorf("expected %s, got: %s", testCase.expectedOutput, actual)
			}

			if actual != emptyOutput {
				if !testidentification.IsMultistageJobName(testCase.inputTestName) {
					t.Errorf("expected to be a multistage job name")
				}
			}

			if actual != emptyOutput && !testidentification.IsMultistageJobName(testCase.inputTestName) {
			}
		})
	}
}

func TestStepRegistryItems(t *testing.T) {
	emptyStepRegistryItem := testidentification.StepRegistryItem{}

	testCases := []struct {
		inputTestName            string
		expectedStepRegistryItem testidentification.StepRegistryItem
	}{
		{
			inputTestName: "operator.Run multi-stage test e2e-aws-arm64 - e2e-aws-arm64-openshift-e2e-test container test",
			expectedStepRegistryItem: testidentification.StepRegistryItem{
				Name:     "e2e-aws-arm64",
				StepName: "openshift-e2e-test",
			},
		},
		{
			inputTestName: "operator.Run multi-stage test e2e-aws-csi-migration - e2e-aws-csi-migration-openshift-e2e-test container test",
			expectedStepRegistryItem: testidentification.StepRegistryItem{
				Name:     "e2e-aws-csi-migration",
				StepName: "openshift-e2e-test",
			},
		},
		{
			inputTestName: "operator.Run multi-stage test e2e-aws-csi-migration - e2e-aws-csi-migration-storage-pv-check container test",
			expectedStepRegistryItem: testidentification.StepRegistryItem{
				Name:     "e2e-aws-csi-migration",
				StepName: "storage-pv-check",
			},
		},
		{
			// This should not match because it is the top-level multistage job
			// name
			inputTestName:            "operator.Run multi-stage test e2e-aws-csi-migration",
			expectedStepRegistryItem: emptyStepRegistryItem,
		},
		{
			inputTestName:            "this should not match",
			expectedStepRegistryItem: emptyStepRegistryItem,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.inputTestName, func(t *testing.T) {
			isStepRegistryItem := testidentification.IsStepRegistryItem(testCase.inputTestName)

			stepRegistryItem := testidentification.GetStepRegistryItemFromTest(testCase.inputTestName)

			if stepRegistryItem != testCase.expectedStepRegistryItem {
				t.Errorf("expected %v: got %v", testCase.expectedStepRegistryItem, stepRegistryItem)
			}

			if stepRegistryItem == emptyStepRegistryItem {
				if isStepRegistryItem {
					t.Errorf("expected %s not to match", testCase.inputTestName)
				}
			}
		})
	}
}

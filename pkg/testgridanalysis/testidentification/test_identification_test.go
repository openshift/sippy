package testidentification_test

import (
	"testing"

	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"
)

func TestStepRegistryItems(t *testing.T) {
	emptyStepRegistryItem := testidentification.StepRegistryItem{}

	testCases := []struct {
		inputTestName            string
		expectedStepRegistryItem testidentification.StepRegistryItem
		expectedError            error
		expectedURL              string
	}{
		{
			inputTestName: "operator.Run multi-stage test e2e-aws-arm64 - e2e-aws-arm64-openshift-e2e-test container test",
			expectedStepRegistryItem: testidentification.StepRegistryItem{
				Name:     "e2e-aws-arm64",
				StepName: "openshift-e2e-test",
			},
			expectedURL: "https://steps.ci.openshift.org/reference/openshift-e2e-test",
		},
		{
			inputTestName: "operator.Run multi-stage test e2e-aws-csi-migration - e2e-aws-csi-migration-openshift-e2e-test container test",
			expectedStepRegistryItem: testidentification.StepRegistryItem{
				Name:     "e2e-aws-csi-migration",
				StepName: "openshift-e2e-test",
			},
			expectedURL: "https://steps.ci.openshift.org/reference/openshift-e2e-test",
		},
		{
			inputTestName: "operator.Run multi-stage test e2e-aws-csi-migration - e2e-aws-csi-migration-storage-pv-check container test",
			expectedStepRegistryItem: testidentification.StepRegistryItem{
				Name:     "e2e-aws-csi-migration",
				StepName: "storage-pv-check",
			},
			expectedURL: "https://steps.ci.openshift.org/reference/storage-pv-check",
		},
		{
			inputTestName:            "this should not match",
			expectedStepRegistryItem: emptyStepRegistryItem,
			expectedURL:              "",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.inputTestName, func(t *testing.T) {
			isStepRegistryItem := testidentification.IsStepRegistryItem(testCase.inputTestName)
			stepRegistryItem := testidentification.GetStepRegistryItemFromTest(testCase.inputTestName)
			registryURL := stepRegistryItem.RegistryURL()

			if stepRegistryItem != testCase.expectedStepRegistryItem {
				t.Errorf("expected %v: got %v", testCase.expectedStepRegistryItem, stepRegistryItem)
			}

			if stepRegistryItem == emptyStepRegistryItem {
				if isStepRegistryItem {
					t.Errorf("expected %s not to match", testCase.inputTestName)
				}

				if registryURL != nil {
					t.Errorf("expected registry URL to be nil, got: %s", registryURL.String())
				}
			}

			if registryURL != nil {
				registryURLString := registryURL.String()

				if registryURLString != testCase.expectedURL {
					t.Errorf("expected %s: got %s", testCase.expectedURL, registryURLString)
				}
			} else {
				if testCase.expectedURL != "" {
					t.Errorf("expected registry URL to not be nil")
				}
			}
		})
	}
}

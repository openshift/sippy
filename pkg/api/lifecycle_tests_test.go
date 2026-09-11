package api

import (
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/sippy/pkg/testidentification"
)

func TestLifecycleProduct(t *testing.T) {
	tests := []struct {
		release string
		want    string
	}{
		{release: "", want: ""},
		{release: "4.21", want: ""},
		{release: "quay-3.18", want: "quay"},
		{release: "acm-2.14", want: "acm"},
		{release: "-3.18", want: ""},
		{release: "quay-", want: "quay"},
		{release: "4.21-okd", want: ""},
		{release: "5.0-okd", want: ""},
		{release: "aro-stage", want: "aro"},
	}

	for _, tt := range tests {
		t.Run(tt.release, func(t *testing.T) {
			if got := lifecycleProduct(tt.release); got != tt.want {
				t.Errorf("lifecycleProduct(%q) = %q, want %q", tt.release, got, tt.want)
			}
		})
	}
}

func TestLifecycleTestsForRelease(t *testing.T) {
	ocpNames := []string{
		testidentification.InstallTestName,
		testidentification.NewInstallTestName,
		testidentification.InstallTestNamePrefix,
		testidentification.OperatorInstallPrefix,
		testidentification.UpgradeTestName,
		testidentification.OperatorUpgradePrefix,
		testidentification.CVOAcknowledgesUpgradeTest,
	}

	tests := []struct {
		name    string
		release string
		want    lifecycleTestSelection
	}{
		{
			name:    "4.21 uses new install names",
			release: "4.21",
			want: lifecycleTestSelection{
				InstallExactNames: sets.New[string](),
				InstallPrefixes: sets.New(
					testidentification.OperatorInstallPrefix,
					testidentification.InstallTestNamePrefix,
				),
				UpgradeExactNames: sets.New(testidentification.UpgradeTestName),
				UpgradePrefixes:   sets.New(testidentification.OperatorUpgradePrefix),
				UpgradeSubstrings: sets.New(
					testidentification.OperatorsUpgradedTest,
					testidentification.APIsRemainAvailTest,
					testidentification.MachineConfigsUpgradedTest,
					testidentification.CVOAcknowledgesUpgradeTest,
				),
				HealthInstallTestName: testidentification.NewInstallTestName,
				HealthUpgradeTestName: testidentification.UpgradeTestName,
				HealthInfraTestName:   testidentification.NewInfrastructureTestName,
			},
		},
		{
			name:    "4.10 uses legacy install name",
			release: "4.10",
			want: lifecycleTestSelection{
				InstallExactNames: sets.New(testidentification.InstallTestName),
				InstallPrefixes:   sets.New(testidentification.OperatorInstallPrefix),
				UpgradeExactNames: sets.New(testidentification.UpgradeTestName),
				UpgradePrefixes:   sets.New(testidentification.OperatorUpgradePrefix),
				UpgradeSubstrings: sets.New(
					testidentification.OperatorsUpgradedTest,
					testidentification.APIsRemainAvailTest,
					testidentification.MachineConfigsUpgradedTest,
					testidentification.CVOAcknowledgesUpgradeTest,
				),
				HealthInstallTestName: testidentification.InstallTestName,
				HealthUpgradeTestName: testidentification.UpgradeTestName,
				HealthInfraTestName:   testidentification.InfrastructureTestName,
			},
		},
		{
			name:    "quay-3.18 uses generic sig-quay lifecycle names",
			release: "quay-3.18",
			want: lifecycleTestSelection{
				InstallExactNames:     sets.New("[sig-quay] install should succeed"),
				InstallPrefixes:       sets.New[string](),
				UpgradeExactNames:     sets.New("[sig-quay] upgrade should succeed"),
				UpgradePrefixes:       sets.New[string](),
				UpgradeSubstrings:     sets.New[string](),
				HealthInstallTestName: "[sig-quay] install should succeed",
				HealthUpgradeTestName: "[sig-quay] upgrade should succeed",
				HealthInfraTestName:   testidentification.InfrastructureTestName,
			},
		},
		{
			name:    "quay-3.19 uses generic sig-quay lifecycle names",
			release: "quay-3.19",
			want: lifecycleTestSelection{
				InstallExactNames:     sets.New("[sig-quay] install should succeed"),
				InstallPrefixes:       sets.New[string](),
				UpgradeExactNames:     sets.New("[sig-quay] upgrade should succeed"),
				UpgradePrefixes:       sets.New[string](),
				UpgradeSubstrings:     sets.New[string](),
				HealthInstallTestName: "[sig-quay] install should succeed",
				HealthUpgradeTestName: "[sig-quay] upgrade should succeed",
				HealthInfraTestName:   testidentification.InfrastructureTestName,
			},
		},
		{
			name:    "acm-2.14 uses generic sig-acm lifecycle names",
			release: "acm-2.14",
			want: lifecycleTestSelection{
				InstallExactNames:     sets.New("[sig-acm] install should succeed"),
				InstallPrefixes:       sets.New[string](),
				UpgradeExactNames:     sets.New("[sig-acm] upgrade should succeed"),
				UpgradePrefixes:       sets.New[string](),
				UpgradeSubstrings:     sets.New[string](),
				HealthInstallTestName: "[sig-acm] install should succeed",
				HealthUpgradeTestName: "[sig-acm] upgrade should succeed",
				HealthInfraTestName:   testidentification.InfrastructureTestName,
			},
		},
		{
			name:    "4.21-okd is an OpenShift release, not a product, and uses legacy install name",
			release: "4.21-okd",
			want: lifecycleTestSelection{
				InstallExactNames: sets.New(testidentification.InstallTestName),
				InstallPrefixes:   sets.New(testidentification.OperatorInstallPrefix),
				UpgradeExactNames: sets.New(testidentification.UpgradeTestName),
				UpgradePrefixes:   sets.New(testidentification.OperatorUpgradePrefix),
				UpgradeSubstrings: sets.New(
					testidentification.OperatorsUpgradedTest,
					testidentification.APIsRemainAvailTest,
					testidentification.MachineConfigsUpgradedTest,
					testidentification.CVOAcknowledgesUpgradeTest,
				),
				HealthInstallTestName: testidentification.InstallTestName,
				HealthUpgradeTestName: testidentification.UpgradeTestName,
				HealthInfraTestName:   testidentification.InfrastructureTestName,
			},
		},
		{
			name:    "5.0-okd is an OpenShift release, not a product, and uses legacy install name",
			release: "5.0-okd",
			want: lifecycleTestSelection{
				InstallExactNames: sets.New(testidentification.InstallTestName),
				InstallPrefixes:   sets.New(testidentification.OperatorInstallPrefix),
				UpgradeExactNames: sets.New(testidentification.UpgradeTestName),
				UpgradePrefixes:   sets.New(testidentification.OperatorUpgradePrefix),
				UpgradeSubstrings: sets.New(
					testidentification.OperatorsUpgradedTest,
					testidentification.APIsRemainAvailTest,
					testidentification.MachineConfigsUpgradedTest,
					testidentification.CVOAcknowledgesUpgradeTest,
				),
				HealthInstallTestName: testidentification.InstallTestName,
				HealthUpgradeTestName: testidentification.UpgradeTestName,
				HealthInfraTestName:   testidentification.InfrastructureTestName,
			},
		},
		{
			name:    "aro-stage uses generic sig-aro lifecycle names",
			release: "aro-stage",
			want: lifecycleTestSelection{
				InstallExactNames:     sets.New("[sig-aro] install should succeed"),
				InstallPrefixes:       sets.New[string](),
				UpgradeExactNames:     sets.New("[sig-aro] upgrade should succeed"),
				UpgradePrefixes:       sets.New[string](),
				UpgradeSubstrings:     sets.New[string](),
				HealthInstallTestName: "[sig-aro] install should succeed",
				HealthUpgradeTestName: "[sig-aro] upgrade should succeed",
				HealthInfraTestName:   testidentification.InfrastructureTestName,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lifecycleTestsForRelease(tt.release)

			if !got.InstallExactNames.Equal(tt.want.InstallExactNames) {
				t.Errorf("InstallExactNames = %v, want %v", sets.List(got.InstallExactNames), sets.List(tt.want.InstallExactNames))
			}
			if !got.InstallPrefixes.Equal(tt.want.InstallPrefixes) {
				t.Errorf("InstallPrefixes = %v, want %v", sets.List(got.InstallPrefixes), sets.List(tt.want.InstallPrefixes))
			}
			if !got.UpgradeExactNames.Equal(tt.want.UpgradeExactNames) {
				t.Errorf("UpgradeExactNames = %v, want %v", sets.List(got.UpgradeExactNames), sets.List(tt.want.UpgradeExactNames))
			}
			if !got.UpgradePrefixes.Equal(tt.want.UpgradePrefixes) {
				t.Errorf("UpgradePrefixes = %v, want %v", sets.List(got.UpgradePrefixes), sets.List(tt.want.UpgradePrefixes))
			}
			if !got.UpgradeSubstrings.Equal(tt.want.UpgradeSubstrings) {
				t.Errorf("UpgradeSubstrings = %v, want %v", sets.List(got.UpgradeSubstrings), sets.List(tt.want.UpgradeSubstrings))
			}
			if got.HealthInstallTestName != tt.want.HealthInstallTestName {
				t.Errorf("HealthInstallTestName = %q, want %q", got.HealthInstallTestName, tt.want.HealthInstallTestName)
			}
			if got.HealthUpgradeTestName != tt.want.HealthUpgradeTestName {
				t.Errorf("HealthUpgradeTestName = %q, want %q", got.HealthUpgradeTestName, tt.want.HealthUpgradeTestName)
			}
			if got.HealthInfraTestName != tt.want.HealthInfraTestName {
				t.Errorf("HealthInfraTestName = %q, want %q", got.HealthInfraTestName, tt.want.HealthInfraTestName)
			}

			if lifecycleProduct(tt.release) != "" {
				for _, name := range ocpNames {
					if got.InstallExactNames.Has(name) || got.InstallPrefixes.Has(name) ||
						got.UpgradeExactNames.Has(name) || got.UpgradePrefixes.Has(name) || got.UpgradeSubstrings.Has(name) {
						t.Errorf("product selection unexpectedly contains OCP name %q", name)
					}
				}
			}
		})
	}
}

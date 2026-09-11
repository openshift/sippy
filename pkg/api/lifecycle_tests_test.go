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
		{release: "quay-3.19", want: "quay"},
		{release: "quay-", want: "quay"},
		{release: "acm-2.14", want: ""},
		{release: "-3.18", want: ""},
		{release: "4.21-okd", want: ""},
		{release: "5.0-okd", want: ""},
		{release: "aro-stage", want: ""},
		{release: "rosa-integration", want: ""},
		{release: "ocp-hypershift", want: ""},
		{release: "mcp-0.5", want: ""},
		{release: "rrp-integration", want: ""},
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

	ocpLegacySelection := lifecycleTestSelection{
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
			want:    ocpLegacySelection,
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
			name:    "acm-2.14 is not on the lifecycle allowlist and uses legacy install name",
			release: "acm-2.14",
			want:    ocpLegacySelection,
		},
		{
			name:    "4.21-okd is an OpenShift release, not a product, and uses legacy install name",
			release: "4.21-okd",
			want:    ocpLegacySelection,
		},
		{
			name:    "5.0-okd is an OpenShift release, not a product, and uses legacy install name",
			release: "5.0-okd",
			want:    ocpLegacySelection,
		},
		{
			name:    "aro-stage is not on the lifecycle allowlist and uses legacy install name",
			release: "aro-stage",
			want:    ocpLegacySelection,
		},
		{
			name:    "rosa-integration is not on the lifecycle allowlist and uses legacy install name",
			release: "rosa-integration",
			want:    ocpLegacySelection,
		},
		{
			name:    "ocp-hypershift is not on the lifecycle allowlist and uses legacy install name",
			release: "ocp-hypershift",
			want:    ocpLegacySelection,
		},
		{
			name:    "mcp-0.5 is not on the lifecycle allowlist and uses legacy install name",
			release: "mcp-0.5",
			want:    ocpLegacySelection,
		},
		{
			name:    "rrp-integration is not on the lifecycle allowlist and uses legacy install name",
			release: "rrp-integration",
			want:    ocpLegacySelection,
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

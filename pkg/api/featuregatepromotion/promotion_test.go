package featuregatepromotion

import (
	"fmt"
	"reflect"
	"sort"
	"testing"
)

func TestValidateJobTiers_Candidate(t *testing.T) {
	tests := []struct {
		name    string
		variant JobVariant
		wantErr bool
	}{
		{
			name:    "candidate is valid",
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha", JobTiers: "candidate"},
			wantErr: false,
		},
		{
			name:    "candidate with standard is valid",
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha", JobTiers: "standard,candidate"},
			wantErr: false,
		},
		{
			name:    "invalid tier still rejected",
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha", JobTiers: "bogus"},
			wantErr: true,
		},
		{
			name:    "candidate with invalid tier rejected",
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha", JobTiers: "candidate,bogus"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateJobTiers(tt.variant)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateJobTiers() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateJobTiers_Comprehensive(t *testing.T) {
	tests := []struct {
		name    string
		variant JobVariant
		wantErr bool
	}{
		{
			name:    "empty job tiers is valid",
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
			wantErr: false,
		},
		{
			name:    "standard is valid",
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha", JobTiers: "standard"},
			wantErr: false,
		},
		{
			name:    "informing is valid",
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha", JobTiers: "informing"},
			wantErr: false,
		},
		{
			name:    "blocking is valid",
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha", JobTiers: "blocking"},
			wantErr: false,
		},
		{
			name:    "all valid tiers combined",
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha", JobTiers: "standard,informing,blocking,candidate"},
			wantErr: false,
		},
		{
			name:    "invalid tier rejected",
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha", JobTiers: "invalid"},
			wantErr: true,
		},
		{
			name:    "valid tier with invalid tier rejected",
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha", JobTiers: "standard,invalid"},
			wantErr: true,
		},
		{
			name:    "only commas rejected",
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha", JobTiers: ",,,"},
			wantErr: true,
		},
		{
			name:    "whitespace only tiers rejected",
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha", JobTiers: " , , "},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateJobTiers(tt.variant)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateJobTiers(%+v) error = %v, wantErr %v", tt.variant, err, tt.wantErr)
			}
		})
	}
}

func TestBuildVariantResult_CandidateVariants(t *testing.T) {
	sufficientRows := func(hasCandidateTier bool) []testQueryRow {
		tier := "standard"
		if hasCandidateTier {
			tier = "candidate"
		}
		var rows []testQueryRow
		for i := 1; i <= 5; i++ {
			rows = append(rows, testQueryRow{
				TestName:         fmt.Sprintf("test%d", i),
				Platform:         "aws",
				Architecture:     "amd64",
				Topology:         "ha",
				JobTier:          tier,
				CurrentRuns:      15,
				CurrentSuccesses: 15,
			})
		}
		return rows
	}

	insufficientRows := func(hasCandidateTier bool) []testQueryRow {
		tier := "standard"
		if hasCandidateTier {
			tier = "candidate"
		}
		var rows []testQueryRow
		for i := 1; i <= 2; i++ {
			rows = append(rows, testQueryRow{
				TestName:         fmt.Sprintf("test%d", i),
				Platform:         "aws",
				Architecture:     "amd64",
				Topology:         "ha",
				JobTier:          tier,
				CurrentRuns:      15,
				CurrentSuccesses: 15,
			})
		}
		return rows
	}

	tests := []struct {
		name       string
		variant    JobVariant
		rows       []testQueryRow
		wantErrors int
		wantWarns  int
	}{
		{
			name:       "candidate tier with sufficient tests - warning about component readiness",
			variant:    JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
			rows:       sufficientRows(true),
			wantErrors: 0,
			wantWarns:  1,
		},
		{
			name:       "candidate tier with insufficient tests - blocking error plus warning",
			variant:    JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
			rows:       insufficientRows(true),
			wantErrors: 1,
			wantWarns:  1,
		},
		{
			name:       "no candidate tier results - no warning",
			variant:    JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
			rows:       sufficientRows(false),
			wantErrors: 0,
			wantWarns:  0,
		},
		{
			name:    "candidate tier with low pass rate - blocking error plus warning",
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
			rows: func() []testQueryRow {
				rows := sufficientRows(true)
				rows[0].CurrentSuccesses = 13 // 86% pass rate
				rows[0].CurrentFailures = 2
				return rows
			}(),
			wantErrors: 1,
			wantWarns:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vr := buildVariantResult(tt.variant, tt.rows)
			if len(vr.Errors) != tt.wantErrors {
				t.Errorf("got %d errors, want %d: %v", len(vr.Errors), tt.wantErrors, vr.Errors)
			}
			if len(vr.Warnings) != tt.wantWarns {
				t.Errorf("got %d warnings, want %d: %v", len(vr.Warnings), tt.wantWarns, vr.Warnings)
			}
		})
	}
}

func TestBuildVariantResult_OptionalVariants(t *testing.T) {
	makeRows := func(platform, os string, numTests int, runs, successes int) []testQueryRow {
		var rows []testQueryRow
		for i := 1; i <= numTests; i++ {
			rows = append(rows, testQueryRow{
				TestName:         fmt.Sprintf("test%d", i),
				Platform:         platform,
				Architecture:     "amd64",
				Topology:         "ha",
				OS:               os,
				JobTier:          "standard",
				CurrentRuns:      runs,
				CurrentSuccesses: successes,
			})
		}
		return rows
	}

	tests := []struct {
		name       string
		variant    JobVariant
		rows       []testQueryRow
		wantErrors int
		wantSuff   bool
	}{
		{
			name:       "required variant with insufficient tests - error",
			variant:    JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
			rows:       makeRows("aws", "", 2, 15, 15),
			wantErrors: 1,
			wantSuff:   false,
		},
		{
			name:       "optional variant with insufficient tests - still errors on variant level",
			variant:    JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha", OS: "rhel10", Optional: true},
			rows:       makeRows("aws", "rhel10", 2, 15, 15),
			wantErrors: 1,
			wantSuff:   false,
		},
		{
			name:       "required variant with insufficient runs - error",
			variant:    JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
			rows:       makeRows("aws", "", 5, 10, 10),
			wantErrors: 5,
			wantSuff:   false,
		},
		{
			name:       "required variant with low pass rate - error",
			variant:    JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
			rows:       makeRows("aws", "", 5, 20, 18),
			wantErrors: 5,
			wantSuff:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vr := buildVariantResult(tt.variant, tt.rows)
			if len(vr.Errors) != tt.wantErrors {
				t.Errorf("got %d errors, want %d: %v", len(vr.Errors), tt.wantErrors, vr.Errors)
			}
			if vr.Sufficient != tt.wantSuff {
				t.Errorf("Sufficient = %v, want %v", vr.Sufficient, tt.wantSuff)
			}
		})
	}
}

func TestBuildPromotionStatus_OptionalDoNotBlock(t *testing.T) {
	rows := []testQueryRow{
		// Required variant with sufficient data
		{TestName: "test1", Platform: "aws", Architecture: "amd64", Topology: "ha", JobTier: "standard", CurrentRuns: 15, CurrentSuccesses: 15},
		{TestName: "test2", Platform: "aws", Architecture: "amd64", Topology: "ha", JobTier: "standard", CurrentRuns: 15, CurrentSuccesses: 15},
		{TestName: "test3", Platform: "aws", Architecture: "amd64", Topology: "ha", JobTier: "standard", CurrentRuns: 15, CurrentSuccesses: 15},
		{TestName: "test4", Platform: "aws", Architecture: "amd64", Topology: "ha", JobTier: "standard", CurrentRuns: 15, CurrentSuccesses: 15},
		{TestName: "test5", Platform: "aws", Architecture: "amd64", Topology: "ha", JobTier: "standard", CurrentRuns: 15, CurrentSuccesses: 15},
		// Optional variant with insufficient data
		{TestName: "test1", Platform: "aws", Architecture: "amd64", Topology: "ha", OS: "rhel10", JobTier: "standard", CurrentRuns: 2, CurrentSuccesses: 2},
	}

	variants := []JobVariant{
		{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
		{Cloud: "aws", Architecture: "amd64", Topology: "ha", OS: "rhel10", Optional: true},
	}

	status := buildPromotionStatus("TestFeature", "5.0", variants, rows)

	if !status.Sufficient {
		t.Error("expected Sufficient=true when only optional variants fail")
	}
	if len(status.Warnings) == 0 {
		t.Error("expected warnings for optional variant failures")
	}
	if len(status.Errors) != 0 {
		t.Errorf("expected no blocking errors, got: %v", status.Errors)
	}
}

func TestDefaultJobTiersIncludeCandidate(t *testing.T) {
	tiers := JobTiersForVariant(JobVariant{Cloud: "vsphere", Architecture: "amd64", Topology: "ha"})
	tierSet := make(map[string]bool)
	for _, tier := range tiers {
		tierSet[tier] = true
	}
	expected := []string{"standard", "informing", "blocking", "candidate"}
	for _, tier := range expected {
		if !tierSet[tier] {
			t.Errorf("default tiers missing %q, got: %v", tier, tiers)
		}
	}
}

func TestAllRequiredVariantsQueryCandidateTier(t *testing.T) {
	allVariants := append(append([]JobVariant{}, RequiredSelfManagedJobVariants...), RequiredHypershiftJobVariants...)
	for _, variant := range allVariants {
		tiers := JobTiersForVariant(variant)
		hasCandidateTier := false
		for _, tier := range tiers {
			if tier == "candidate" {
				hasCandidateTier = true
				break
			}
		}
		if !hasCandidateTier {
			t.Errorf("variant %+v does not query candidate tier", variant)
		}
	}
}

func TestRequiredHypershiftJobVariants_TopologyIsExternal(t *testing.T) {
	for i, variant := range RequiredHypershiftJobVariants {
		if variant.Topology != "external" {
			t.Errorf("RequiredHypershiftJobVariants[%d] has Topology=%q, want %q",
				i, variant.Topology, "external")
		}
	}
}

func TestTopologyDisplayName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"external", "hypershift"},
		{"ha", "ha"},
		{"single", "single"},
		{"two-node-fencing", "two-node-fencing"},
		{"two-node-arbiter", "two-node-arbiter"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := TopologyDisplayName(tt.input)
			if got != tt.want {
				t.Errorf("TopologyDisplayName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNonHypershiftPlatforms(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"NutanixFeature", true},
		{"nutanixfeature", true},
		{"MetalFeature", true},
		{"VSphereFeature", true},
		{"OpenStackFeature", true},
		{"AzureFeature", true},
		{"GCPFeature", true},
		{"AWSFeature", false},
		{"GenericFeature", false},
		{"HypershiftFeature", false},
		{"ExternalFeature", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NonHypershiftPlatforms.MatchString(tt.input)
			if got != tt.want {
				t.Errorf("NonHypershiftPlatforms.MatchString(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFilterVariants(t *testing.T) {
	tests := []struct {
		name        string
		featureGate string
		variants    [][]JobVariant
		want        []JobVariant
	}{
		{
			name:        "AWS feature gate matches aws hypershift variant with external topology",
			featureGate: "AWSServiceLBNetworkSecurityGroup",
			variants:    [][]JobVariant{RequiredHypershiftJobVariants},
			want:        []JobVariant{{Cloud: "aws", Architecture: "amd64", Topology: "external"}},
		},
		{
			name:        "generic feature gate matches no hypershift variants",
			featureGate: "GenericFeature",
			variants:    [][]JobVariant{RequiredHypershiftJobVariants},
			want:        nil,
		},
		{
			name:        "VSphere feature gate matches vsphere self-managed variant",
			featureGate: "VSphereControlPlaneMachineSet",
			variants:    [][]JobVariant{RequiredSelfManagedJobVariants},
			want:        []JobVariant{{Cloud: "vsphere", Architecture: "amd64", Topology: "ha"}},
		},
		{
			name:        "Nutanix feature gate matches optional nutanix variant",
			featureGate: "NutanixGate",
			variants:    [][]JobVariant{OptionalSelfManagedPlatformVariants},
			want:        []JobVariant{{Cloud: "nutanix", Architecture: "amd64", Topology: "ha"}},
		},
		{
			name:        "Metal feature gate matches metal variants with network stacks",
			featureGate: "MetalFeature",
			variants:    [][]JobVariant{RequiredSelfManagedJobVariants},
			want: []JobVariant{
				{Cloud: "metal", Architecture: "amd64", Topology: "ha", NetworkStack: "ipv4"},
				{Cloud: "metal", Architecture: "amd64", Topology: "ha", NetworkStack: "ipv6"},
				{Cloud: "metal", Architecture: "amd64", Topology: "ha", NetworkStack: "dual"},
			},
		},
		{
			name:        "feature gate with multiple variant lists",
			featureGate: "MetalFeature",
			variants:    [][]JobVariant{OptionalSelfManagedPlatformVariants, RequiredSelfManagedJobVariants},
			want: []JobVariant{
				{Cloud: "metal", Architecture: "amd64", Topology: "two-node-arbiter", NetworkStack: "ipv4"},
				{Cloud: "metal", Architecture: "amd64", Topology: "two-node-arbiter", NetworkStack: "ipv6"},
				{Cloud: "metal", Architecture: "amd64", Topology: "two-node-arbiter", NetworkStack: "dual"},
				{Cloud: "metal", Architecture: "amd64", Topology: "two-node-fencing", NetworkStack: "ipv4", JobTiers: "candidate,standard,informing,blocking"},
				{Cloud: "metal", Architecture: "amd64", Topology: "two-node-fencing", NetworkStack: "ipv6", JobTiers: "candidate,standard,informing,blocking"},
				{Cloud: "metal", Architecture: "amd64", Topology: "two-node-fencing", NetworkStack: "dual", JobTiers: "candidate,standard,informing,blocking"},
				{Cloud: "metal", Architecture: "amd64", Topology: "ha", NetworkStack: "ipv4"},
				{Cloud: "metal", Architecture: "amd64", Topology: "ha", NetworkStack: "ipv6"},
				{Cloud: "metal", Architecture: "amd64", Topology: "ha", NetworkStack: "dual"},
			},
		},
		{
			name:        "DualReplica feature gate matches fencing topology variants",
			featureGate: "DualReplicaFeature",
			variants:    [][]JobVariant{OptionalSelfManagedPlatformVariants},
			want: []JobVariant{
				{Cloud: "metal", Architecture: "amd64", Topology: "two-node-fencing", NetworkStack: "ipv4", JobTiers: "candidate,standard,informing,blocking"},
				{Cloud: "metal", Architecture: "amd64", Topology: "two-node-fencing", NetworkStack: "ipv6", JobTiers: "candidate,standard,informing,blocking"},
				{Cloud: "metal", Architecture: "amd64", Topology: "two-node-fencing", NetworkStack: "dual", JobTiers: "candidate,standard,informing,blocking"},
			},
		},
		{
			name:        "Fencing feature gate matches fencing topology variants",
			featureGate: "FencingFeature",
			variants:    [][]JobVariant{OptionalSelfManagedPlatformVariants},
			want: []JobVariant{
				{Cloud: "metal", Architecture: "amd64", Topology: "two-node-fencing", NetworkStack: "ipv4", JobTiers: "candidate,standard,informing,blocking"},
				{Cloud: "metal", Architecture: "amd64", Topology: "two-node-fencing", NetworkStack: "ipv6", JobTiers: "candidate,standard,informing,blocking"},
				{Cloud: "metal", Architecture: "amd64", Topology: "two-node-fencing", NetworkStack: "dual", JobTiers: "candidate,standard,informing,blocking"},
			},
		},
		{
			name:        "amd64 in feature gate name matches amd64 variants only",
			featureGate: "Amd64SpecificFeature",
			variants: [][]JobVariant{{
				{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
				{Cloud: "aws", Architecture: "arm64", Topology: "ha"},
			}},
			want: []JobVariant{{Cloud: "aws", Architecture: "amd64", Topology: "ha"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterVariants(tt.featureGate, tt.variants...)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FilterVariants(%q) =\n  %+v\nwant:\n  %+v", tt.featureGate, got, tt.want)
			}
		})
	}
}

func TestMatchTwoNodeFeatureGates(t *testing.T) {
	tests := []struct {
		featureGate string
		topology    string
		want        bool
	}{
		{"dualreplicafeature", "two-node-fencing", true},
		{"fencingfeature", "two-node-fencing", true},
		{"dualreplicafeature", "ha", false},
		{"fencingfeature", "ha", false},
		{"genericfeature", "two-node-fencing", false},
		{"genericfeature", "ha", false},
		{"dualreplicafeature", "two-node-arbiter", false},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("%s/%s", tt.featureGate, tt.topology)
		t.Run(name, func(t *testing.T) {
			got := MatchTwoNodeFeatureGates(tt.featureGate, tt.topology)
			if got != tt.want {
				t.Errorf("MatchTwoNodeFeatureGates(%q, %q) = %v, want %v",
					tt.featureGate, tt.topology, got, tt.want)
			}
		})
	}
}

func TestOrderedJobVariants(t *testing.T) {
	input := []JobVariant{
		{Cloud: "gcp", Architecture: "amd64", Topology: "ha"},
		{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
		{Cloud: "aws", Architecture: "amd64", Topology: "external"},
		{Cloud: "metal", Architecture: "amd64", Topology: "ha", NetworkStack: "dual"},
		{Cloud: "metal", Architecture: "amd64", Topology: "ha", NetworkStack: "ipv4"},
		{Cloud: "metal", Architecture: "amd64", Topology: "ha", NetworkStack: "ipv6"},
		{Cloud: "aws", Architecture: "amd64", Topology: "single"},
	}

	want := []JobVariant{
		{Cloud: "aws", Architecture: "amd64", Topology: "external"},
		{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
		{Cloud: "gcp", Architecture: "amd64", Topology: "ha"},
		{Cloud: "metal", Architecture: "amd64", Topology: "ha", NetworkStack: "ipv4"},
		{Cloud: "metal", Architecture: "amd64", Topology: "ha", NetworkStack: "ipv6"},
		{Cloud: "metal", Architecture: "amd64", Topology: "ha", NetworkStack: "dual"},
		{Cloud: "aws", Architecture: "amd64", Topology: "single"},
	}

	sorted := make([]JobVariant, len(input))
	copy(sorted, input)
	sort.Sort(OrderedJobVariants(sorted))

	if !reflect.DeepEqual(sorted, want) {
		t.Errorf("OrderedJobVariants sort:\ngot:  %+v\nwant: %+v", sorted, want)
	}
}

func TestAllDefinedVariantsHaveValidJobTiers(t *testing.T) {
	allVariants := []struct {
		name     string
		variants []JobVariant
	}{
		{"RequiredSelfManagedJobVariants", RequiredSelfManagedJobVariants},
		{"OptionalSelfManagedPlatformVariants", OptionalSelfManagedPlatformVariants},
		{"RequiredHypershiftJobVariants", RequiredHypershiftJobVariants},
	}

	for _, group := range allVariants {
		for i, variant := range group.variants {
			name := fmt.Sprintf("%s[%d]-%s-%s-%s", group.name, i, variant.Cloud, variant.Architecture, variant.Topology)
			t.Run(name, func(t *testing.T) {
				if err := ValidateJobTiers(variant); err != nil {
					t.Errorf("variant %+v has invalid JobTiers: %v", variant, err)
				}
			})
		}
	}
}

func TestLookbackLogic(t *testing.T) {
	jv := JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha"}

	tests := []struct {
		name          string
		rows          []testQueryRow
		wantTotalRuns int
	}{
		{
			name: "sufficient current runs uses only current window",
			rows: []testQueryRow{
				{
					TestName:          "test1",
					Platform:          "aws",
					Architecture:      "amd64",
					Topology:          "ha",
					JobTier:           "standard",
					CurrentRuns:       20,
					CurrentSuccesses:  20,
					PreviousRuns:      10,
					PreviousSuccesses: 10,
				},
			},
			wantTotalRuns: 20,
		},
		{
			name: "insufficient current runs extends to previous window",
			rows: []testQueryRow{
				{
					TestName:          "test1",
					Platform:          "aws",
					Architecture:      "amd64",
					Topology:          "ha",
					JobTier:           "standard",
					CurrentRuns:       10,
					CurrentSuccesses:  10,
					PreviousRuns:      10,
					PreviousSuccesses: 10,
				},
			},
			wantTotalRuns: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vr := buildVariantResult(jv, tt.rows)
			if len(vr.TestResults) != 1 {
				t.Fatalf("expected 1 test result, got %d", len(vr.TestResults))
			}
			if vr.TestResults[0].TotalRuns != tt.wantTotalRuns {
				t.Errorf("TotalRuns = %d, want %d", vr.TestResults[0].TotalRuns, tt.wantTotalRuns)
			}
		})
	}
}

func TestMatchesVariant(t *testing.T) {
	tests := []struct {
		name    string
		row     testQueryRow
		variant JobVariant
		want    bool
	}{
		{
			name:    "exact match",
			row:     testQueryRow{Platform: "aws", Architecture: "amd64", Topology: "ha"},
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
			want:    true,
		},
		{
			name:    "case insensitive match",
			row:     testQueryRow{Platform: "AWS", Architecture: "AMD64", Topology: "HA"},
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
			want:    true,
		},
		{
			name:    "platform mismatch",
			row:     testQueryRow{Platform: "gcp", Architecture: "amd64", Topology: "ha"},
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
			want:    false,
		},
		{
			name:    "network stack required but missing",
			row:     testQueryRow{Platform: "metal", Architecture: "amd64", Topology: "ha", NetworkStack: "ipv6"},
			variant: JobVariant{Cloud: "metal", Architecture: "amd64", Topology: "ha", NetworkStack: "ipv4"},
			want:    false,
		},
		{
			name:    "network stack not required, any matches",
			row:     testQueryRow{Platform: "aws", Architecture: "amd64", Topology: "ha", NetworkStack: "ipv4"},
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
			want:    true,
		},
		{
			name:    "OS required and matches",
			row:     testQueryRow{Platform: "aws", Architecture: "amd64", Topology: "ha", OS: "rhel10"},
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha", OS: "rhel10"},
			want:    true,
		},
		{
			name:    "OS required but missing",
			row:     testQueryRow{Platform: "aws", Architecture: "amd64", Topology: "ha"},
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha", OS: "rhel10"},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesVariant(tt.row, tt.variant)
			if got != tt.want {
				t.Errorf("matchesVariant() = %v, want %v", got, tt.want)
			}
		})
	}
}

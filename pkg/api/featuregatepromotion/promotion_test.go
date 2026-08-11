package featuregatepromotion

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/lib/pq"
	"k8s.io/apimachinery/pkg/util/sets"

	apitype "github.com/openshift/sippy/pkg/apis/api"
)

// makeTest creates an apitype.Test with the given variant dimensions and run counts.
func makeTest(name, platform, os, jobTier string, currentRuns, currentSuccesses, currentFailures, previousRuns, previousSuccesses int) apitype.Test {
	variants := pq.StringArray{
		"Platform:" + platform,
		"Architecture:amd64",
		"Topology:ha",
	}
	if os != "" {
		variants = append(variants, "OS:"+os)
	}
	if jobTier != "" {
		variants = append(variants, "JobTier:"+jobTier)
	}
	return apitype.Test{
		Name:              name,
		Variants:          variants,
		CurrentRuns:       currentRuns,
		CurrentSuccesses:  currentSuccesses,
		CurrentFailures:   currentFailures,
		PreviousRuns:      previousRuns,
		PreviousSuccesses: previousSuccesses,
	}
}

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
	sufficientTests := func(hasCandidateTier bool) []apitype.Test {
		tier := "standard"
		if hasCandidateTier {
			tier = "candidate"
		}
		var tests []apitype.Test
		for i := 1; i <= 5; i++ {
			tests = append(tests, makeTest(fmt.Sprintf("test%d", i), "aws", "", tier, 15, 15, 0, 0, 0))
		}
		return tests
	}

	insufficientTests := func(hasCandidateTier bool) []apitype.Test {
		tier := "standard"
		if hasCandidateTier {
			tier = "candidate"
		}
		var tests []apitype.Test
		for i := 1; i <= 2; i++ {
			tests = append(tests, makeTest(fmt.Sprintf("test%d", i), "aws", "", tier, 15, 15, 0, 0, 0))
		}
		return tests
	}

	tests := []struct {
		name       string
		variant    JobVariant
		testData   []apitype.Test
		wantErrors int
		wantWarns  int
	}{
		{
			name:       "candidate tier with sufficient tests - warning about component readiness",
			variant:    JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
			testData:   sufficientTests(true),
			wantErrors: 0,
			wantWarns:  1,
		},
		{
			name:       "candidate tier with insufficient tests - blocking error plus warning",
			variant:    JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
			testData:   insufficientTests(true),
			wantErrors: 1,
			wantWarns:  1,
		},
		{
			name:       "no candidate tier results - no warning",
			variant:    JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
			testData:   sufficientTests(false),
			wantErrors: 0,
			wantWarns:  0,
		},
		{
			name:    "candidate tier with low pass rate - blocking error plus warning",
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
			testData: func() []apitype.Test {
				tests := sufficientTests(true)
				tests[0].CurrentSuccesses = 13
				tests[0].CurrentFailures = 2
				return tests
			}(),
			wantErrors: 1,
			wantWarns:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vr := buildVariantResult(tt.variant, tt.testData)
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
	makeTests := func(platform, os string, numTests int, runs, successes int) []apitype.Test {
		var tests []apitype.Test
		for i := 1; i <= numTests; i++ {
			tests = append(tests, makeTest(fmt.Sprintf("test%d", i), platform, os, "standard", runs, successes, runs-successes, 0, 0))
		}
		return tests
	}

	tests := []struct {
		name       string
		variant    JobVariant
		testData   []apitype.Test
		wantErrors int
		wantSuff   bool
	}{
		{
			name:       "required variant with insufficient tests - error",
			variant:    JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
			testData:   makeTests("aws", "", 2, 15, 15),
			wantErrors: 1,
			wantSuff:   false,
		},
		{
			name:       "optional variant with insufficient tests - still errors on variant level",
			variant:    JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha", OS: "rhel10", Optional: true},
			testData:   makeTests("aws", "rhel10", 2, 15, 15),
			wantErrors: 1,
			wantSuff:   false,
		},
		{
			name:       "required variant with insufficient runs - error",
			variant:    JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
			testData:   makeTests("aws", "", 5, 10, 10),
			wantErrors: 5,
			wantSuff:   false,
		},
		{
			name:       "required variant with low pass rate - error",
			variant:    JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
			testData:   makeTests("aws", "", 5, 20, 18),
			wantErrors: 5,
			wantSuff:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vr := buildVariantResult(tt.variant, tt.testData)
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
	tests := []apitype.Test{
		// Required variant with sufficient data
		makeTest("test1", "aws", "", "standard", 15, 15, 0, 0, 0),
		makeTest("test2", "aws", "", "standard", 15, 15, 0, 0, 0),
		makeTest("test3", "aws", "", "standard", 15, 15, 0, 0, 0),
		makeTest("test4", "aws", "", "standard", 15, 15, 0, 0, 0),
		makeTest("test5", "aws", "", "standard", 15, 15, 0, 0, 0),
		// Optional variant with insufficient data
		makeTest("test1", "aws", "rhel10", "standard", 2, 2, 0, 0, 0),
	}

	variants := []JobVariant{
		{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
		{Cloud: "aws", Architecture: "amd64", Topology: "ha", OS: "rhel10", Optional: true},
	}

	status := buildPromotionStatus("TestFeature", "5.0", variants, tests)

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
			want:        []JobVariant{{Cloud: "nutanix", Architecture: "amd64", Topology: "ha", Optional: true}},
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
				{Cloud: "metal", Architecture: "amd64", Topology: "two-node-arbiter", NetworkStack: "ipv4", Optional: true},
				{Cloud: "metal", Architecture: "amd64", Topology: "two-node-arbiter", NetworkStack: "ipv6", Optional: true},
				{Cloud: "metal", Architecture: "amd64", Topology: "two-node-arbiter", NetworkStack: "dual", Optional: true},
				{Cloud: "metal", Architecture: "amd64", Topology: "two-node-fencing", NetworkStack: "ipv4", JobTiers: "candidate,standard,informing,blocking", Optional: true},
				{Cloud: "metal", Architecture: "amd64", Topology: "two-node-fencing", NetworkStack: "ipv6", JobTiers: "candidate,standard,informing,blocking", Optional: true},
				{Cloud: "metal", Architecture: "amd64", Topology: "two-node-fencing", NetworkStack: "dual", JobTiers: "candidate,standard,informing,blocking", Optional: true},
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
				{Cloud: "metal", Architecture: "amd64", Topology: "two-node-fencing", NetworkStack: "ipv4", JobTiers: "candidate,standard,informing,blocking", Optional: true},
				{Cloud: "metal", Architecture: "amd64", Topology: "two-node-fencing", NetworkStack: "ipv6", JobTiers: "candidate,standard,informing,blocking", Optional: true},
				{Cloud: "metal", Architecture: "amd64", Topology: "two-node-fencing", NetworkStack: "dual", JobTiers: "candidate,standard,informing,blocking", Optional: true},
			},
		},
		{
			name:        "Fencing feature gate matches fencing topology variants",
			featureGate: "FencingFeature",
			variants:    [][]JobVariant{OptionalSelfManagedPlatformVariants},
			want: []JobVariant{
				{Cloud: "metal", Architecture: "amd64", Topology: "two-node-fencing", NetworkStack: "ipv4", JobTiers: "candidate,standard,informing,blocking", Optional: true},
				{Cloud: "metal", Architecture: "amd64", Topology: "two-node-fencing", NetworkStack: "ipv6", JobTiers: "candidate,standard,informing,blocking", Optional: true},
				{Cloud: "metal", Architecture: "amd64", Topology: "two-node-fencing", NetworkStack: "dual", JobTiers: "candidate,standard,informing,blocking", Optional: true},
			},
		},
		{
			name:        "AWS feature gate matches only aws self-managed variants",
			featureGate: "AWSDualStackInstall",
			variants:    [][]JobVariant{RequiredSelfManagedJobVariants},
			want: []JobVariant{
				{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
				{Cloud: "aws", Architecture: "amd64", Topology: "single"},
			},
		},
		{
			name:        "GCP feature gate matches only gcp self-managed variant",
			featureGate: "GCPLabelsTags",
			variants:    [][]JobVariant{RequiredSelfManagedJobVariants},
			want: []JobVariant{
				{Cloud: "gcp", Architecture: "amd64", Topology: "ha"},
			},
		},
		{
			name:        "Azure feature gate matches only azure self-managed variant",
			featureGate: "AzureWorkloadIdentity",
			variants:    [][]JobVariant{RequiredSelfManagedJobVariants},
			want: []JobVariant{
				{Cloud: "azure", Architecture: "amd64", Topology: "ha"},
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
		testData      []apitype.Test
		wantTotalRuns int
	}{
		{
			name: "sufficient current runs uses only current window",
			testData: []apitype.Test{
				makeTest("test1", "aws", "", "standard", 20, 20, 0, 10, 10),
			},
			wantTotalRuns: 20,
		},
		{
			name: "insufficient current runs extends to previous window",
			testData: []apitype.Test{
				makeTest("test1", "aws", "", "standard", 10, 10, 0, 10, 10),
			},
			wantTotalRuns: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vr := buildVariantResult(jv, tt.testData)
			if len(vr.TestResults) != 1 {
				t.Fatalf("expected 1 test result, got %d", len(vr.TestResults))
			}
			if vr.TestResults[0].TotalRuns != tt.wantTotalRuns {
				t.Errorf("TotalRuns = %d, want %d", vr.TestResults[0].TotalRuns, tt.wantTotalRuns)
			}
		})
	}
}

func TestMatchesParsedVariant(t *testing.T) {
	tests := []struct {
		name    string
		parsed  map[string]string
		variant JobVariant
		want    bool
	}{
		{
			name:    "exact match",
			parsed:  map[string]string{"Platform": "aws", "Architecture": "amd64", "Topology": "ha"},
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
			want:    true,
		},
		{
			name:    "case insensitive match",
			parsed:  map[string]string{"Platform": "AWS", "Architecture": "AMD64", "Topology": "HA"},
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
			want:    true,
		},
		{
			name:    "platform mismatch",
			parsed:  map[string]string{"Platform": "gcp", "Architecture": "amd64", "Topology": "ha"},
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
			want:    false,
		},
		{
			name:    "network stack required but different",
			parsed:  map[string]string{"Platform": "metal", "Architecture": "amd64", "Topology": "ha", "NetworkStack": "ipv6"},
			variant: JobVariant{Cloud: "metal", Architecture: "amd64", Topology: "ha", NetworkStack: "ipv4"},
			want:    false,
		},
		{
			name:    "network stack not required, any matches",
			parsed:  map[string]string{"Platform": "aws", "Architecture": "amd64", "Topology": "ha", "NetworkStack": "ipv4"},
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha"},
			want:    true,
		},
		{
			name:    "OS required and matches",
			parsed:  map[string]string{"Platform": "aws", "Architecture": "amd64", "Topology": "ha", "OS": "rhel10"},
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha", OS: "rhel10"},
			want:    true,
		},
		{
			name:    "OS required but missing",
			parsed:  map[string]string{"Platform": "aws", "Architecture": "amd64", "Topology": "ha"},
			variant: JobVariant{Cloud: "aws", Architecture: "amd64", Topology: "ha", OS: "rhel10"},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesParsedVariant(tt.parsed, tt.variant)
			if got != tt.want {
				t.Errorf("matchesParsedVariant() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsFailureDueToUnpromotedGate(t *testing.T) {
	promotedGates := sets.New("PromotedFeature", "AnotherPromoted")

	tests := []struct {
		name     string
		testName string
		want     bool
	}{
		{
			name:     "test with unpromoted gate annotation is excluded",
			testName: "[sig-auth] some test [OCPFeatureGate:UnpromotedFeature] should work",
			want:     true,
		},
		{
			name:     "test with promoted gate annotation is not excluded",
			testName: "[sig-auth] some test [OCPFeatureGate:PromotedFeature] should work",
			want:     false,
		},
		{
			name:     "test with no gate annotation is not excluded",
			testName: "[sig-network] some regular test should work",
			want:     false,
		},
		{
			name:     "test with multiple annotations where one is unpromoted",
			testName: "[sig-auth] test [OCPFeatureGate:PromotedFeature] [OCPFeatureGate:UnpromotedFeature] should work",
			want:     true,
		},
		{
			name:     "test with multiple promoted annotations is not excluded",
			testName: "[sig-auth] test [OCPFeatureGate:PromotedFeature] [OCPFeatureGate:AnotherPromoted] should work",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isFailureDueToUnpromotedGate(tt.testName, promotedGates)
			if got != tt.want {
				t.Errorf("isFailureDueToUnpromotedGate(%q) = %v, want %v", tt.testName, got, tt.want)
			}
		})
	}
}

func TestParseVariants(t *testing.T) {
	tests := []struct {
		name     string
		variants []string
		want     map[string]string
	}{
		{
			name:     "standard variants",
			variants: []string{"Platform:aws", "Architecture:amd64", "Topology:ha"},
			want:     map[string]string{"Platform": "aws", "Architecture": "amd64", "Topology": "ha"},
		},
		{
			name:     "with network stack and job tier",
			variants: []string{"Platform:metal", "Architecture:amd64", "Topology:ha", "NetworkStack:ipv4", "JobTier:standard"},
			want:     map[string]string{"Platform": "metal", "Architecture": "amd64", "Topology": "ha", "NetworkStack": "ipv4", "JobTier": "standard"},
		},
		{
			name:     "empty input",
			variants: nil,
			want:     map[string]string{},
		},
		{
			name:     "entries without colon are skipped",
			variants: []string{"Platform:aws", "never-stable", "Architecture:amd64"},
			want:     map[string]string{"Platform": "aws", "Architecture": "amd64"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseVariants(tt.variants)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseVariants() = %v, want %v", got, tt.want)
			}
		})
	}
}

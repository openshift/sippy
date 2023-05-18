// nolint
package api

import (
	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/stretchr/testify/assert"
	"testing"
)

func fakeComponentAndCapabilityGetter(test apitype.ComponentTestIdentification, stats apitype.ComponentTestStatus) (string, []string) {
	name := test.TestName
	known := map[string]struct {
		component    string
		capabilities []string
	}{
		"test 1": {
			component:    "component 1",
			capabilities: []string{"cap1"},
		},
		"test 2": {
			component:    "component 2",
			capabilities: []string{"cap21", "cap22"},
		},
	}
	if comCap, ok := known[name]; ok {
		return comCap.component, comCap.capabilities
	}
	return "other", []string{"other"}
}

func TestGenerateComponentReport(t *testing.T) {
	defaultAdvancedOption := apitype.ComponentReportRequestAdvancedOptions{
		Confidence:     95,
		PityFactor:     5,
		MinimumFailure: 3,
	}
	defaultComponentReportGenerator := componentReportGenerator{
		ComponentReportRequestVariantOptions:  apitype.ComponentReportRequestVariantOptions{GroupBy: "cloud,arch,network"},
		ComponentReportRequestAdvancedOptions: defaultAdvancedOption,
	}
	groupByVariantComponentReportGenerator := componentReportGenerator{
		ComponentReportRequestVariantOptions:  apitype.ComponentReportRequestVariantOptions{GroupBy: "cloud,arch,network,variant"},
		ComponentReportRequestAdvancedOptions: defaultAdvancedOption,
	}
	componentPageGenerator := componentReportGenerator{
		ComponentReportRequestTestIdentificationOptions: apitype.ComponentReportRequestTestIdentificationOptions{
			Component: "component 2",
		},
		ComponentReportRequestVariantOptions: apitype.ComponentReportRequestVariantOptions{
			GroupBy: "cloud,arch,network",
		},
		ComponentReportRequestAdvancedOptions: defaultAdvancedOption,
	}
	capabilityPageGenerator := componentReportGenerator{
		ComponentReportRequestTestIdentificationOptions: apitype.ComponentReportRequestTestIdentificationOptions{
			Component:  "component 2",
			Capability: "cap22",
		},
		ComponentReportRequestVariantOptions: apitype.ComponentReportRequestVariantOptions{
			GroupBy: "cloud,arch,network",
		},
		ComponentReportRequestAdvancedOptions: defaultAdvancedOption,
	}
	testPageGenerator := componentReportGenerator{
		ComponentReportRequestTestIdentificationOptions: apitype.ComponentReportRequestTestIdentificationOptions{
			Component:  "component 2",
			Capability: "cap22",
			TestID:     "2",
		},
		ComponentReportRequestVariantOptions: apitype.ComponentReportRequestVariantOptions{
			GroupBy: "cloud,arch,network",
		},
		ComponentReportRequestAdvancedOptions: defaultAdvancedOption,
	}
	awsAMD64OVNTest := apitype.ComponentTestIdentification{
		TestName: "test 1",
		TestID:   "1",
		Platform: "aws",
		Arch:     "amd64",
		Network:  "ovn",
		Upgrade:  "upgrade-micro",
	}
	awsAMD64SDNTest := apitype.ComponentTestIdentification{
		TestName: "test 2",
		TestID:   "2",
		Platform: "aws",
		Arch:     "amd64",
		Network:  "sdn",
		Upgrade:  "upgrade-micro",
	}
	awsAMD64OVNBaseTestStats90Percent := apitype.ComponentTestStatus{
		Variants:     []string{"standard"},
		TotalCount:   1000,
		FlakeCount:   10,
		SuccessCount: 900,
	}
	awsAMD64OVNBaseTestStats50Percent := apitype.ComponentTestStatus{
		Variants:     []string{"standard"},
		TotalCount:   1000,
		FlakeCount:   10,
		SuccessCount: 500,
	}
	awsAMD64OVNBaseTestStatsVariants90Percent := apitype.ComponentTestStatus{
		Variants:     []string{"standard", "fips"},
		TotalCount:   1000,
		FlakeCount:   10,
		SuccessCount: 900,
	}
	awsAMD64OVNSampleTestStats90Percent := apitype.ComponentTestStatus{
		Variants:     []string{"standard"},
		TotalCount:   100,
		FlakeCount:   1,
		SuccessCount: 90,
	}
	awsAMD64OVNSampleTestStats85Percent := apitype.ComponentTestStatus{
		Variants:     []string{"standard"},
		TotalCount:   100,
		FlakeCount:   1,
		SuccessCount: 85,
	}
	awsAMD64OVNSampleTestStats50Percent := apitype.ComponentTestStatus{
		Variants:     []string{"standard"},
		TotalCount:   100,
		FlakeCount:   1,
		SuccessCount: 50,
	}
	awsAMD64OVNSampleTestStatsTiny := apitype.ComponentTestStatus{
		Variants:     []string{"standard"},
		TotalCount:   3,
		FlakeCount:   0,
		SuccessCount: 1,
	}
	awsAMD64OVNSampleTestStatsVariants90Percent := apitype.ComponentTestStatus{
		Variants:     []string{"standard", "fips"},
		TotalCount:   100,
		FlakeCount:   1,
		SuccessCount: 90,
	}
	awsAMD64SDNBaseTestStats90Percent := apitype.ComponentTestStatus{
		Variants:     []string{"standard"},
		TotalCount:   1000,
		FlakeCount:   10,
		SuccessCount: 900,
	}
	awsAMD64SDNBaseTestStats50Percent := apitype.ComponentTestStatus{
		Variants:     []string{"standard"},
		TotalCount:   1000,
		FlakeCount:   10,
		SuccessCount: 500,
	}
	awsAMD64SDNSampleTestStats90Percent := apitype.ComponentTestStatus{
		Variants:     []string{"standard"},
		TotalCount:   100,
		FlakeCount:   1,
		SuccessCount: 90,
	}
	columnAWSAMD64OVN := apitype.ComponentReportColumnIdentification{
		Platform: "aws",
		Arch:     "amd64",
		Network:  "ovn",
	}
	columnAWSAMD64OVNVariantStandard := apitype.ComponentReportColumnIdentification{
		Platform: "aws",
		Arch:     "amd64",
		Network:  "ovn",
		Variant:  "standard",
	}
	columnAWSAMD64OVNVariantFips := apitype.ComponentReportColumnIdentification{
		Platform: "aws",
		Arch:     "amd64",
		Network:  "ovn",
		Variant:  "fips",
	}
	columnAWSAMD64SDN := apitype.ComponentReportColumnIdentification{
		Platform: "aws",
		Arch:     "amd64",
		Network:  "sdn",
	}
	columnAWSAMD64SDNVariantStandard := apitype.ComponentReportColumnIdentification{
		Platform: "aws",
		Arch:     "amd64",
		Network:  "sdn",
		Variant:  "standard",
	}
	columnAWSAMD64OVNFull := apitype.ComponentReportColumnIdentification{
		Platform: "aws",
		Arch:     "amd64",
		Network:  "ovn",
		Upgrade:  "upgrade-micro",
		Variant:  "standard",
	}
	columnAWSAMD64SDNFull := apitype.ComponentReportColumnIdentification{
		Platform: "aws",
		Arch:     "amd64",
		Network:  "sdn",
		Upgrade:  "upgrade-micro",
		Variant:  "standard",
	}
	rowComponent1 := apitype.ComponentReportRowIdentification{
		Component: "component 1",
	}
	rowComponent2 := apitype.ComponentReportRowIdentification{
		Component: "component 2",
	}
	rowComponent2Cap21 := apitype.ComponentReportRowIdentification{
		Component:  "component 2",
		Capability: "cap21",
	}
	rowComponent2Cap22 := apitype.ComponentReportRowIdentification{
		Component:  "component 2",
		Capability: "cap22",
	}
	rowComponent2Cap22Test2 := apitype.ComponentReportRowIdentification{
		Component:  "component 2",
		Capability: "cap22",
		TestName:   "test 2",
		TestID:     "2",
	}

	tests := []struct {
		name           string
		generator      componentReportGenerator
		baseStatus     map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus
		sampleStatus   map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus
		expectedReport apitype.ComponentReport
	}{
		{
			name:      "top page test no significant and missing data",
			generator: defaultComponentReportGenerator,
			baseStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus{
				awsAMD64OVNTest: awsAMD64OVNBaseTestStats90Percent,
				awsAMD64SDNTest: awsAMD64SDNBaseTestStats90Percent,
			},
			sampleStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus{
				awsAMD64OVNTest: awsAMD64OVNSampleTestStats85Percent,
				awsAMD64SDNTest: awsAMD64SDNSampleTestStats90Percent,
			},
			expectedReport: apitype.ComponentReport{
				Rows: []apitype.ComponentReportRow{
					{
						ComponentReportRowIdentification: apitype.ComponentReportRowIdentification{
							Component: "component 1",
						},
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: columnAWSAMD64OVN,
								Status:                              apitype.NotSignificant,
							},
							{
								ComponentReportColumnIdentification: columnAWSAMD64SDN,
								Status:                              apitype.MissingBasisAndSample,
							},
						},
					},
					{
						ComponentReportRowIdentification: apitype.ComponentReportRowIdentification{
							Component: "component 2",
						},
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: columnAWSAMD64OVN,
								Status:                              apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: columnAWSAMD64SDN,
								Status:                              apitype.NotSignificant,
							},
						},
					},
				},
			},
		},
		{
			name:      "top page test with both improvement and regression",
			generator: defaultComponentReportGenerator,
			baseStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus{
				awsAMD64OVNTest: awsAMD64OVNBaseTestStats90Percent,
				awsAMD64SDNTest: awsAMD64SDNBaseTestStats50Percent,
			},
			sampleStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus{
				awsAMD64OVNTest: awsAMD64OVNSampleTestStats50Percent,
				awsAMD64SDNTest: awsAMD64SDNSampleTestStats90Percent,
			},
			expectedReport: apitype.ComponentReport{
				Rows: []apitype.ComponentReportRow{
					{
						ComponentReportRowIdentification: rowComponent1,
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: columnAWSAMD64OVN,
								Status:                              apitype.ExtremeRegression,
							},
							{
								ComponentReportColumnIdentification: columnAWSAMD64SDN,
								Status:                              apitype.MissingBasisAndSample,
							},
						},
					},
					{
						ComponentReportRowIdentification: rowComponent2,
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: columnAWSAMD64OVN,
								Status:                              apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: columnAWSAMD64SDN,
								Status:                              apitype.SignificantImprovement,
							},
						},
					},
				},
			},
		},
		{
			name:      "component page test no significant and missing data",
			generator: componentPageGenerator,
			baseStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus{
				awsAMD64OVNTest: awsAMD64OVNBaseTestStats90Percent,
				awsAMD64SDNTest: awsAMD64SDNBaseTestStats90Percent,
			},
			sampleStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus{
				awsAMD64OVNTest: awsAMD64OVNSampleTestStats90Percent,
				awsAMD64SDNTest: awsAMD64SDNSampleTestStats90Percent,
			},
			expectedReport: apitype.ComponentReport{
				Rows: []apitype.ComponentReportRow{
					{
						ComponentReportRowIdentification: rowComponent2Cap21,
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: columnAWSAMD64OVN,
								Status:                              apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: columnAWSAMD64SDN,
								Status:                              apitype.NotSignificant,
							},
						},
					},
					{
						ComponentReportRowIdentification: rowComponent2Cap22,
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: columnAWSAMD64OVN,
								Status:                              apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: columnAWSAMD64SDN,
								Status:                              apitype.NotSignificant,
							},
						},
					},
				},
			},
		},
		{
			name:      "component page test with both improvement and regression",
			generator: componentPageGenerator,
			baseStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus{
				awsAMD64OVNTest: awsAMD64OVNBaseTestStats90Percent,
				awsAMD64SDNTest: awsAMD64SDNBaseTestStats50Percent,
			},
			sampleStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus{
				awsAMD64OVNTest: awsAMD64OVNBaseTestStats50Percent,
				awsAMD64SDNTest: awsAMD64SDNBaseTestStats90Percent,
			},
			expectedReport: apitype.ComponentReport{
				Rows: []apitype.ComponentReportRow{
					{
						ComponentReportRowIdentification: rowComponent2Cap21,
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: columnAWSAMD64OVN,
								Status:                              apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: columnAWSAMD64SDN,
								Status:                              apitype.SignificantImprovement,
							},
						},
					},
					{
						ComponentReportRowIdentification: rowComponent2Cap22,
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: columnAWSAMD64OVN,
								Status:                              apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: columnAWSAMD64SDN,
								Status:                              apitype.SignificantImprovement,
							},
						},
					},
				},
			},
		},
		{
			name:      "capability page test no significant and missing data",
			generator: capabilityPageGenerator,
			baseStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus{
				awsAMD64OVNTest: awsAMD64OVNBaseTestStats90Percent,
				awsAMD64SDNTest: awsAMD64SDNBaseTestStats90Percent,
			},
			sampleStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus{
				awsAMD64OVNTest: awsAMD64OVNSampleTestStats90Percent,
				awsAMD64SDNTest: awsAMD64SDNSampleTestStats90Percent,
			},
			expectedReport: apitype.ComponentReport{
				Rows: []apitype.ComponentReportRow{
					{
						ComponentReportRowIdentification: rowComponent2Cap22Test2,
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: columnAWSAMD64OVN,
								Status:                              apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: columnAWSAMD64SDN,
								Status:                              apitype.NotSignificant,
							},
						},
					},
				},
			},
		},
		{
			name:      "capability page test with both improvement and regression",
			generator: capabilityPageGenerator,
			baseStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus{
				awsAMD64OVNTest: awsAMD64OVNBaseTestStats90Percent,
				awsAMD64SDNTest: awsAMD64SDNBaseTestStats50Percent,
			},
			sampleStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus{
				awsAMD64OVNTest: awsAMD64OVNSampleTestStats50Percent,
				awsAMD64SDNTest: awsAMD64SDNSampleTestStats90Percent,
			},
			expectedReport: apitype.ComponentReport{
				Rows: []apitype.ComponentReportRow{
					{
						ComponentReportRowIdentification: rowComponent2Cap22Test2,
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: columnAWSAMD64OVN,
								Status:                              apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: columnAWSAMD64SDN,
								Status:                              apitype.SignificantImprovement,
							},
						},
					},
				},
			},
		},
		{
			name:      "test page test no significant and missing data",
			generator: testPageGenerator,
			baseStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus{
				awsAMD64OVNTest: awsAMD64OVNBaseTestStats90Percent,
				awsAMD64SDNTest: awsAMD64SDNBaseTestStats90Percent,
			},
			sampleStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus{
				awsAMD64OVNTest: awsAMD64OVNSampleTestStats90Percent,
				awsAMD64SDNTest: awsAMD64SDNSampleTestStats90Percent,
			},
			expectedReport: apitype.ComponentReport{
				Rows: []apitype.ComponentReportRow{
					{
						ComponentReportRowIdentification: rowComponent2Cap22Test2,
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: columnAWSAMD64OVNFull,
								Status:                              apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: columnAWSAMD64SDNFull,
								Status:                              apitype.NotSignificant,
							},
						},
					},
				},
			},
		},
		{
			name:      "test page test with both improvement and regression",
			generator: testPageGenerator,
			baseStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus{
				awsAMD64OVNTest: awsAMD64OVNBaseTestStats90Percent,
				awsAMD64SDNTest: awsAMD64SDNBaseTestStats50Percent,
			},
			sampleStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus{
				awsAMD64OVNTest: awsAMD64OVNSampleTestStats50Percent,
				awsAMD64SDNTest: awsAMD64SDNSampleTestStats90Percent,
			},
			expectedReport: apitype.ComponentReport{
				Rows: []apitype.ComponentReportRow{
					{
						ComponentReportRowIdentification: rowComponent2Cap22Test2,
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: columnAWSAMD64OVNFull,
								Status:                              apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: columnAWSAMD64SDNFull,
								Status:                              apitype.SignificantImprovement,
							},
						},
					},
				},
			},
		},
		{
			name: "top page test confidence 90 result in regression",
			generator: componentReportGenerator{
				ComponentReportRequestVariantOptions: apitype.ComponentReportRequestVariantOptions{GroupBy: "cloud,arch,network"},
				ComponentReportRequestAdvancedOptions: apitype.ComponentReportRequestAdvancedOptions{
					Confidence:     90,
					PityFactor:     5,
					MinimumFailure: 3,
				},
			},
			baseStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus{
				awsAMD64OVNTest: awsAMD64OVNBaseTestStats90Percent,
				awsAMD64SDNTest: awsAMD64SDNBaseTestStats90Percent,
			},
			sampleStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus{
				awsAMD64OVNTest: awsAMD64OVNSampleTestStats85Percent,
				awsAMD64SDNTest: awsAMD64SDNSampleTestStats90Percent,
			},
			expectedReport: apitype.ComponentReport{
				Rows: []apitype.ComponentReportRow{
					{
						ComponentReportRowIdentification: rowComponent1,
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: columnAWSAMD64OVN,
								Status:                              apitype.SignificantRegression,
							},
							{
								ComponentReportColumnIdentification: columnAWSAMD64SDN,
								Status:                              apitype.MissingBasisAndSample,
							},
						},
					},
					{
						ComponentReportRowIdentification: rowComponent2,
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: columnAWSAMD64OVN,
								Status:                              apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: columnAWSAMD64SDN,
								Status:                              apitype.NotSignificant,
							},
						},
					},
				},
			},
		},
		{
			name: "top page test confidence 90 pity 10 result in no regression",
			generator: componentReportGenerator{
				ComponentReportRequestVariantOptions: apitype.ComponentReportRequestVariantOptions{GroupBy: "cloud,arch,network"},
				ComponentReportRequestAdvancedOptions: apitype.ComponentReportRequestAdvancedOptions{
					Confidence:     90,
					PityFactor:     10,
					MinimumFailure: 3,
				},
			},
			baseStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus{
				awsAMD64OVNTest: awsAMD64OVNBaseTestStats90Percent,
				awsAMD64SDNTest: awsAMD64SDNBaseTestStats90Percent,
			},
			sampleStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus{
				awsAMD64OVNTest: awsAMD64OVNSampleTestStats85Percent,
				awsAMD64SDNTest: awsAMD64SDNSampleTestStats90Percent,
			},
			expectedReport: apitype.ComponentReport{
				Rows: []apitype.ComponentReportRow{
					{
						ComponentReportRowIdentification: rowComponent1,
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: columnAWSAMD64OVN,
								Status:                              apitype.NotSignificant,
							},
							{
								ComponentReportColumnIdentification: columnAWSAMD64SDN,
								Status:                              apitype.MissingBasisAndSample,
							},
						},
					},
					{
						ComponentReportRowIdentification: rowComponent2,
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: columnAWSAMD64OVN,
								Status:                              apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: columnAWSAMD64SDN,
								Status:                              apitype.NotSignificant,
							},
						},
					},
				},
			},
		},
		{
			name:      "top page test minimum failure no regression",
			generator: defaultComponentReportGenerator,
			baseStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus{
				awsAMD64OVNTest: awsAMD64OVNBaseTestStats90Percent,
				awsAMD64SDNTest: awsAMD64SDNBaseTestStats90Percent,
			},
			sampleStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus{
				awsAMD64OVNTest: awsAMD64OVNSampleTestStatsTiny,
				awsAMD64SDNTest: awsAMD64SDNSampleTestStats90Percent,
			},
			expectedReport: apitype.ComponentReport{
				Rows: []apitype.ComponentReportRow{
					{
						ComponentReportRowIdentification: rowComponent1,
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: columnAWSAMD64OVN,
								Status:                              apitype.NotSignificant,
							},
							{
								ComponentReportColumnIdentification: columnAWSAMD64SDN,
								Status:                              apitype.MissingBasisAndSample,
							},
						},
					},
					{
						ComponentReportRowIdentification: rowComponent2,
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: columnAWSAMD64OVN,
								Status:                              apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: columnAWSAMD64SDN,
								Status:                              apitype.NotSignificant,
							},
						},
					},
				},
			},
		},
		{
			name:      "top page test group by variant",
			generator: groupByVariantComponentReportGenerator,
			baseStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus{
				awsAMD64OVNTest: awsAMD64OVNBaseTestStatsVariants90Percent,
				awsAMD64SDNTest: awsAMD64SDNBaseTestStats90Percent,
			},
			sampleStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStatus{
				awsAMD64OVNTest: awsAMD64OVNSampleTestStatsVariants90Percent,
				awsAMD64SDNTest: awsAMD64SDNSampleTestStats90Percent,
			},
			expectedReport: apitype.ComponentReport{
				Rows: []apitype.ComponentReportRow{
					{
						ComponentReportRowIdentification: apitype.ComponentReportRowIdentification{
							Component: "component 1",
						},
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: columnAWSAMD64OVNVariantFips,
								Status:                              apitype.NotSignificant,
							},
							{
								ComponentReportColumnIdentification: columnAWSAMD64OVNVariantStandard,
								Status:                              apitype.NotSignificant,
							},
							{
								ComponentReportColumnIdentification: columnAWSAMD64SDNVariantStandard,
								Status:                              apitype.MissingBasisAndSample,
							},
						},
					},
					{
						ComponentReportRowIdentification: apitype.ComponentReportRowIdentification{
							Component: "component 2",
						},
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: columnAWSAMD64OVNVariantFips,
								Status:                              apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: columnAWSAMD64OVNVariantStandard,
								Status:                              apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: columnAWSAMD64SDNVariantStandard,
								Status:                              apitype.NotSignificant,
							},
						},
					},
				},
			},
		},
	}
	componentAndCapabilityGetter = fakeComponentAndCapabilityGetter
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			report := tc.generator.generateComponentTestReport(tc.baseStatus, tc.sampleStatus)
			assert.Equal(t, tc.expectedReport, report, "expected report %+v, got %+v", tc.expectedReport, report)
		})
	}
}

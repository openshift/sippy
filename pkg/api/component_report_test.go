package api

import (
	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/stretchr/testify/assert"
	"testing"
)

func fakeComponentAndCapabilityGetter(name string) (string, []string) {
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
	tests := []struct {
		name           string
		generator      componentReportGenerator
		baseStatus     map[apitype.ComponentTestIdentification]apitype.ComponentTestStats
		sampleStatus   map[apitype.ComponentTestIdentification]apitype.ComponentTestStats
		expectedReport apitype.ComponentReport
	}{
		{
			name: "top page test no significant and missing data",
			generator: componentReportGenerator{
				groupBy:        "cloud,arch,network",
				confidence:     95,
				pityFactor:     5,
				minimumFailure: 3,
			},
			baseStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStats{
				apitype.ComponentTestIdentification{
					TestName: "test 1",
					TestID:   "1",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "ovn",
				}: {
					TotalCount:   1000,
					FlakeCount:   10,
					SuccessCount: 900,
				},
				apitype.ComponentTestIdentification{
					TestName: "test 2",
					TestID:   "2",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "sdn",
				}: {
					TotalCount:   1000,
					FlakeCount:   10,
					SuccessCount: 950,
				},
			},
			sampleStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStats{
				apitype.ComponentTestIdentification{
					TestName: "test 1",
					TestID:   "1",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "ovn",
				}: {
					TotalCount:   100,
					FlakeCount:   1,
					SuccessCount: 85,
				},
				apitype.ComponentTestIdentification{
					TestName: "test 2",
					TestID:   "2",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "sdn",
				}: {
					TotalCount:   100,
					FlakeCount:   1,
					SuccessCount: 95,
				},
			},
			expectedReport: apitype.ComponentReport{
				Rows: []apitype.ComponentReportRow{
					{
						ComponentReportRowIdentification: apitype.ComponentReportRowIdentification{
							Component: "component 1",
						},
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "ovn",
								},
								Status: apitype.NotSignificant,
							},
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "sdn",
								},
								Status: apitype.MissingBasisAndSample,
							},
						},
					},
					{
						ComponentReportRowIdentification: apitype.ComponentReportRowIdentification{
							Component: "component 2",
						},
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "ovn",
								},
								Status: apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "sdn",
								},
								Status: apitype.NotSignificant,
							},
						},
					},
				},
			},
		},
		{
			name: "top page test with both improvement and regression",
			generator: componentReportGenerator{
				groupBy:        "cloud,arch,network",
				confidence:     95,
				pityFactor:     5,
				minimumFailure: 3,
			},
			baseStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStats{
				apitype.ComponentTestIdentification{
					TestName: "test 1",
					TestID:   "1",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "ovn",
				}: {
					TotalCount:   1000,
					FlakeCount:   10,
					SuccessCount: 900,
				},
				apitype.ComponentTestIdentification{
					TestName: "test 2",
					TestID:   "2",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "sdn",
				}: {
					TotalCount:   1000,
					FlakeCount:   10,
					SuccessCount: 500,
				},
			},
			sampleStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStats{
				apitype.ComponentTestIdentification{
					TestName: "test 1",
					TestID:   "1",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "ovn",
				}: {
					TotalCount:   100,
					FlakeCount:   10,
					SuccessCount: 50,
				},
				apitype.ComponentTestIdentification{
					TestName: "test 2",
					TestID:   "2",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "sdn",
				}: {
					TotalCount:   100,
					FlakeCount:   1,
					SuccessCount: 95,
				},
			},
			expectedReport: apitype.ComponentReport{
				Rows: []apitype.ComponentReportRow{
					{
						ComponentReportRowIdentification: apitype.ComponentReportRowIdentification{
							Component: "component 1",
						},
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "ovn",
								},
								Status: apitype.ExtremeRegression,
							},
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "sdn",
								},
								Status: apitype.MissingBasisAndSample,
							},
						},
					},
					{
						ComponentReportRowIdentification: apitype.ComponentReportRowIdentification{
							Component: "component 2",
						},
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "ovn",
								},
								Status: apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "sdn",
								},
								Status: apitype.SignificantImprovement,
							},
						},
					},
				},
			},
		},
		{
			name: "component page test no significant and missing data",
			generator: componentReportGenerator{
				groupBy:        "cloud,arch,network",
				component:      "component 2",
				confidence:     95,
				pityFactor:     5,
				minimumFailure: 3,
			},
			baseStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStats{
				apitype.ComponentTestIdentification{
					TestName: "test 1",
					TestID:   "1",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "ovn",
				}: {
					TotalCount:   1000,
					FlakeCount:   10,
					SuccessCount: 900,
				},
				apitype.ComponentTestIdentification{
					TestName: "test 2",
					TestID:   "2",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "sdn",
				}: {
					TotalCount:   1000,
					FlakeCount:   10,
					SuccessCount: 950,
				},
			},
			sampleStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStats{
				apitype.ComponentTestIdentification{
					TestName: "test 1",
					TestID:   "1",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "ovn",
				}: {
					TotalCount:   100,
					FlakeCount:   1,
					SuccessCount: 90,
				},
				apitype.ComponentTestIdentification{
					TestName: "test 2",
					TestID:   "2",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "sdn",
				}: {
					TotalCount:   100,
					FlakeCount:   1,
					SuccessCount: 95,
				},
			},
			expectedReport: apitype.ComponentReport{
				Rows: []apitype.ComponentReportRow{
					{
						ComponentReportRowIdentification: apitype.ComponentReportRowIdentification{
							Component:  "component 2",
							Capability: "cap21",
						},
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "ovn",
								},
								Status: apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "sdn",
								},
								Status: apitype.NotSignificant,
							},
						},
					},
					{
						ComponentReportRowIdentification: apitype.ComponentReportRowIdentification{
							Component:  "component 2",
							Capability: "cap22",
						},
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "ovn",
								},
								Status: apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "sdn",
								},
								Status: apitype.NotSignificant,
							},
						},
					},
				},
			},
		},
		{
			name: "component page test with both improvement and regression",
			generator: componentReportGenerator{
				groupBy:        "cloud,arch,network",
				component:      "component 2",
				confidence:     95,
				pityFactor:     5,
				minimumFailure: 3,
			},
			baseStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStats{
				apitype.ComponentTestIdentification{
					TestName: "test 1",
					TestID:   "1",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "ovn",
				}: {
					TotalCount:   1000,
					FlakeCount:   10,
					SuccessCount: 900,
				},
				apitype.ComponentTestIdentification{
					TestName: "test 2",
					TestID:   "2",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "sdn",
				}: {
					TotalCount:   1000,
					FlakeCount:   10,
					SuccessCount: 500,
				},
			},
			sampleStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStats{
				apitype.ComponentTestIdentification{
					TestName: "test 1",
					TestID:   "1",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "ovn",
				}: {
					TotalCount:   100,
					FlakeCount:   10,
					SuccessCount: 50,
				},
				apitype.ComponentTestIdentification{
					TestName: "test 2",
					TestID:   "2",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "sdn",
				}: {
					TotalCount:   100,
					FlakeCount:   1,
					SuccessCount: 95,
				},
			},
			expectedReport: apitype.ComponentReport{
				Rows: []apitype.ComponentReportRow{
					{
						ComponentReportRowIdentification: apitype.ComponentReportRowIdentification{
							Component:  "component 2",
							Capability: "cap21",
						},
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "ovn",
								},
								Status: apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "sdn",
								},
								Status: apitype.SignificantImprovement,
							},
						},
					},
					{
						ComponentReportRowIdentification: apitype.ComponentReportRowIdentification{
							Component:  "component 2",
							Capability: "cap22",
						},
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "ovn",
								},
								Status: apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "sdn",
								},
								Status: apitype.SignificantImprovement,
							},
						},
					},
				},
			},
		},
		{
			name: "capability page test no significant and missing data",
			generator: componentReportGenerator{
				groupBy:        "cloud,arch,network",
				component:      "component 2",
				capability:     "cap22",
				confidence:     95,
				pityFactor:     5,
				minimumFailure: 3,
			},
			baseStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStats{
				apitype.ComponentTestIdentification{
					TestName: "test 1",
					TestID:   "1",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "ovn",
				}: {
					TotalCount:   1000,
					FlakeCount:   10,
					SuccessCount: 900,
				},
				apitype.ComponentTestIdentification{
					TestName: "test 2",
					TestID:   "2",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "sdn",
				}: {
					TotalCount:   1000,
					FlakeCount:   10,
					SuccessCount: 950,
				},
			},
			sampleStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStats{
				apitype.ComponentTestIdentification{
					TestName: "test 1",
					TestID:   "1",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "ovn",
				}: {
					TotalCount:   100,
					FlakeCount:   1,
					SuccessCount: 90,
				},
				apitype.ComponentTestIdentification{
					TestName: "test 2",
					TestID:   "2",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "sdn",
				}: {
					TotalCount:   100,
					FlakeCount:   1,
					SuccessCount: 95,
				},
			},
			expectedReport: apitype.ComponentReport{
				Rows: []apitype.ComponentReportRow{
					{
						ComponentReportRowIdentification: apitype.ComponentReportRowIdentification{
							Component:  "component 2",
							Capability: "cap22",
							TestName:   "test 2",
							TestID:     "2",
						},
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "ovn",
								},
								Status: apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "sdn",
								},
								Status: apitype.NotSignificant,
							},
						},
					},
				},
			},
		},
		{
			name: "capability page test with both improvement and regression",
			generator: componentReportGenerator{
				groupBy:        "cloud,arch,network",
				component:      "component 2",
				capability:     "cap22",
				confidence:     95,
				pityFactor:     5,
				minimumFailure: 3,
			},
			baseStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStats{
				apitype.ComponentTestIdentification{
					TestName: "test 1",
					TestID:   "1",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "ovn",
				}: {
					TotalCount:   1000,
					FlakeCount:   10,
					SuccessCount: 900,
				},
				apitype.ComponentTestIdentification{
					TestName: "test 2",
					TestID:   "2",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "sdn",
				}: {
					TotalCount:   1000,
					FlakeCount:   10,
					SuccessCount: 500,
				},
			},
			sampleStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStats{
				apitype.ComponentTestIdentification{
					TestName: "test 1",
					TestID:   "1",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "ovn",
				}: {
					TotalCount:   100,
					FlakeCount:   10,
					SuccessCount: 50,
				},
				apitype.ComponentTestIdentification{
					TestName: "test 2",
					TestID:   "2",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "sdn",
				}: {
					TotalCount:   100,
					FlakeCount:   1,
					SuccessCount: 95,
				},
			},
			expectedReport: apitype.ComponentReport{
				Rows: []apitype.ComponentReportRow{
					{
						ComponentReportRowIdentification: apitype.ComponentReportRowIdentification{
							Component:  "component 2",
							Capability: "cap22",
							TestName:   "test 2",
							TestID:     "2",
						},
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "ovn",
								},
								Status: apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "sdn",
								},
								Status: apitype.SignificantImprovement,
							},
						},
					},
				},
			},
		},
		{
			name: "test page test no significant and missing data",
			generator: componentReportGenerator{
				groupBy:        "cloud,arch,network",
				component:      "component 2",
				capability:     "cap22",
				testId:         "2",
				confidence:     95,
				pityFactor:     5,
				minimumFailure: 3,
			},
			baseStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStats{
				apitype.ComponentTestIdentification{
					TestName: "test 1",
					TestID:   "1",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "ovn",
					Upgrade:  "upgrade-micro",
					Variant:  "standard",
				}: {
					TotalCount:   1000,
					FlakeCount:   10,
					SuccessCount: 900,
				},
				apitype.ComponentTestIdentification{
					TestName: "test 2",
					TestID:   "2",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "sdn",
					Upgrade:  "upgrade-micro",
					Variant:  "standard",
				}: {
					TotalCount:   1000,
					FlakeCount:   10,
					SuccessCount: 950,
				},
			},
			sampleStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStats{
				apitype.ComponentTestIdentification{
					TestName: "test 1",
					TestID:   "1",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "ovn",
					Upgrade:  "upgrade-micro",
					Variant:  "standard",
				}: {
					TotalCount:   100,
					FlakeCount:   1,
					SuccessCount: 90,
				},
				apitype.ComponentTestIdentification{
					TestName: "test 2",
					TestID:   "2",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "sdn",
					Upgrade:  "upgrade-micro",
					Variant:  "standard",
				}: {
					TotalCount:   100,
					FlakeCount:   1,
					SuccessCount: 95,
				},
			},
			expectedReport: apitype.ComponentReport{
				Rows: []apitype.ComponentReportRow{
					{
						ComponentReportRowIdentification: apitype.ComponentReportRowIdentification{
							Component:  "component 2",
							Capability: "cap22",
							TestName:   "test 2",
							TestID:     "2",
						},
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "ovn",
									Upgrade:  "upgrade-micro",
									Variant:  "standard",
								},
								Status: apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "sdn",
									Upgrade:  "upgrade-micro",
									Variant:  "standard",
								},
								Status: apitype.NotSignificant,
							},
						},
					},
				},
			},
		},
		{
			name: "test page test with both improvement and regression",
			generator: componentReportGenerator{
				groupBy:        "cloud,arch,network",
				component:      "component 2",
				capability:     "cap22",
				testId:         "2",
				confidence:     95,
				pityFactor:     5,
				minimumFailure: 3,
			},
			baseStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStats{
				apitype.ComponentTestIdentification{
					TestName: "test 1",
					TestID:   "1",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "ovn",
					Upgrade:  "upgrade-micro",
					Variant:  "standard",
				}: {
					TotalCount:   1000,
					FlakeCount:   10,
					SuccessCount: 900,
				},
				apitype.ComponentTestIdentification{
					TestName: "test 2",
					TestID:   "2",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "sdn",
					Upgrade:  "upgrade-micro",
					Variant:  "standard",
				}: {
					TotalCount:   1000,
					FlakeCount:   10,
					SuccessCount: 500,
				},
			},
			sampleStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStats{
				apitype.ComponentTestIdentification{
					TestName: "test 1",
					TestID:   "1",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "ovn",
					Upgrade:  "upgrade-micro",
					Variant:  "standard",
				}: {
					TotalCount:   100,
					FlakeCount:   10,
					SuccessCount: 50,
				},
				apitype.ComponentTestIdentification{
					TestName: "test 2",
					TestID:   "2",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "sdn",
					Upgrade:  "upgrade-micro",
					Variant:  "standard",
				}: {
					TotalCount:   100,
					FlakeCount:   1,
					SuccessCount: 95,
				},
			},
			expectedReport: apitype.ComponentReport{
				Rows: []apitype.ComponentReportRow{
					{
						ComponentReportRowIdentification: apitype.ComponentReportRowIdentification{
							Component:  "component 2",
							Capability: "cap22",
							TestName:   "test 2",
							TestID:     "2",
						},
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "ovn",
									Upgrade:  "upgrade-micro",
									Variant:  "standard",
								},
								Status: apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "sdn",
									Upgrade:  "upgrade-micro",
									Variant:  "standard",
								},
								Status: apitype.SignificantImprovement,
							},
						},
					},
				},
			},
		},

		{
			name: "top page test confidence 90 result in regression",
			generator: componentReportGenerator{
				groupBy:        "cloud,arch,network",
				confidence:     90,
				pityFactor:     5,
				minimumFailure: 3,
			},
			baseStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStats{
				apitype.ComponentTestIdentification{
					TestName: "test 1",
					TestID:   "1",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "ovn",
				}: {
					TotalCount:   1000,
					FlakeCount:   10,
					SuccessCount: 900,
				},
				apitype.ComponentTestIdentification{
					TestName: "test 2",
					TestID:   "2",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "sdn",
				}: {
					TotalCount:   1000,
					FlakeCount:   10,
					SuccessCount: 950,
				},
			},
			sampleStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStats{
				apitype.ComponentTestIdentification{
					TestName: "test 1",
					TestID:   "1",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "ovn",
				}: {
					TotalCount:   100,
					FlakeCount:   1,
					SuccessCount: 85,
				},
				apitype.ComponentTestIdentification{
					TestName: "test 2",
					TestID:   "2",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "sdn",
				}: {
					TotalCount:   100,
					FlakeCount:   1,
					SuccessCount: 95,
				},
			},
			expectedReport: apitype.ComponentReport{
				Rows: []apitype.ComponentReportRow{
					{
						ComponentReportRowIdentification: apitype.ComponentReportRowIdentification{
							Component: "component 1",
						},
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "ovn",
								},
								Status: apitype.SignificantRegression,
							},
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "sdn",
								},
								Status: apitype.MissingBasisAndSample,
							},
						},
					},
					{
						ComponentReportRowIdentification: apitype.ComponentReportRowIdentification{
							Component: "component 2",
						},
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "ovn",
								},
								Status: apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "sdn",
								},
								Status: apitype.NotSignificant,
							},
						},
					},
				},
			},
		},
		{
			name: "top page test confidence 90 pity 10 result in no regression",
			generator: componentReportGenerator{
				groupBy:        "cloud,arch,network",
				confidence:     90,
				pityFactor:     10,
				minimumFailure: 3,
			},
			baseStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStats{
				apitype.ComponentTestIdentification{
					TestName: "test 1",
					TestID:   "1",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "ovn",
				}: {
					TotalCount:   1000,
					FlakeCount:   10,
					SuccessCount: 900,
				},
				apitype.ComponentTestIdentification{
					TestName: "test 2",
					TestID:   "2",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "sdn",
				}: {
					TotalCount:   1000,
					FlakeCount:   10,
					SuccessCount: 950,
				},
			},
			sampleStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStats{
				apitype.ComponentTestIdentification{
					TestName: "test 1",
					TestID:   "1",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "ovn",
				}: {
					TotalCount:   100,
					FlakeCount:   1,
					SuccessCount: 85,
				},
				apitype.ComponentTestIdentification{
					TestName: "test 2",
					TestID:   "2",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "sdn",
				}: {
					TotalCount:   100,
					FlakeCount:   1,
					SuccessCount: 95,
				},
			},
			expectedReport: apitype.ComponentReport{
				Rows: []apitype.ComponentReportRow{
					{
						ComponentReportRowIdentification: apitype.ComponentReportRowIdentification{
							Component: "component 1",
						},
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "ovn",
								},
								Status: apitype.NotSignificant,
							},
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "sdn",
								},
								Status: apitype.MissingBasisAndSample,
							},
						},
					},
					{
						ComponentReportRowIdentification: apitype.ComponentReportRowIdentification{
							Component: "component 2",
						},
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "ovn",
								},
								Status: apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "sdn",
								},
								Status: apitype.NotSignificant,
							},
						},
					},
				},
			},
		},
		{
			name: "top page test minimum failure no regression",
			generator: componentReportGenerator{
				groupBy:        "cloud,arch,network",
				confidence:     90,
				pityFactor:     10,
				minimumFailure: 3,
			},
			baseStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStats{
				apitype.ComponentTestIdentification{
					TestName: "test 1",
					TestID:   "1",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "ovn",
				}: {
					TotalCount:   1000,
					FlakeCount:   10,
					SuccessCount: 900,
				},
				apitype.ComponentTestIdentification{
					TestName: "test 2",
					TestID:   "2",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "sdn",
				}: {
					TotalCount:   1000,
					FlakeCount:   10,
					SuccessCount: 950,
				},
			},
			sampleStatus: map[apitype.ComponentTestIdentification]apitype.ComponentTestStats{
				apitype.ComponentTestIdentification{
					TestName: "test 1",
					TestID:   "1",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "ovn",
				}: {
					TotalCount:   3,
					FlakeCount:   0,
					SuccessCount: 1,
				},
				apitype.ComponentTestIdentification{
					TestName: "test 2",
					TestID:   "2",
					Platform: "aws",
					Arch:     "amd64",
					Network:  "sdn",
				}: {
					TotalCount:   100,
					FlakeCount:   1,
					SuccessCount: 95,
				},
			},
			expectedReport: apitype.ComponentReport{
				Rows: []apitype.ComponentReportRow{
					{
						ComponentReportRowIdentification: apitype.ComponentReportRowIdentification{
							Component: "component 1",
						},
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "ovn",
								},
								Status: apitype.NotSignificant,
							},
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "sdn",
								},
								Status: apitype.MissingBasisAndSample,
							},
						},
					},
					{
						ComponentReportRowIdentification: apitype.ComponentReportRowIdentification{
							Component: "component 2",
						},
						Columns: []apitype.ComponentReportColumn{
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "ovn",
								},
								Status: apitype.MissingBasisAndSample,
							},
							{
								ComponentReportColumnIdentification: apitype.ComponentReportColumnIdentification{
									Platform: "aws",
									Arch:     "amd64",
									Network:  "sdn",
								},
								Status: apitype.NotSignificant,
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

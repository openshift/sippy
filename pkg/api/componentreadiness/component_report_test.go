// nolint
package componentreadiness

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/stretchr/testify/assert"

	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/util/sets"
)

func fakeComponentAndCapabilityGetter(test crtype.TestIdentification, stats crtype.TestStatus) (string, []string) {
	name := stats.TestName
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
		"test 3": {
			component:    "component 1",
			capabilities: []string{"cap1"},
		},
	}
	if comCap, ok := known[name]; ok {
		return comCap.component, comCap.capabilities
	}
	return "other", []string{"other"}
}

var (
	defaultAdvancedOption = crtype.RequestAdvancedOptions{
		Confidence:     95,
		PityFactor:     5,
		MinimumFailure: 3,
	}
	defaultColumnGroupByVariants    = sets.NewString(strings.Split(DefaultColumnGroupBy, ",")...)
	defaultDBGroupByVariants        = sets.NewString(strings.Split(DefaultDBGroupBy, ",")...)
	defaultComponentReportGenerator = componentReportGenerator{
		gcsBucket: "test-platform-results",
		RequestVariantOptions: crtype.RequestVariantOptions{
			ColumnGroupBy: defaultColumnGroupByVariants,
			DBGroupBy:     defaultDBGroupByVariants,
		},
		RequestAdvancedOptions: defaultAdvancedOption,
	}
	installerColumnGroupByVariants           = sets.NewString("Platform", "Architecture", "Network", "Installer")
	groupByInstallerComponentReportGenerator = componentReportGenerator{
		gcsBucket: "test-platform-results",
		RequestVariantOptions: crtype.RequestVariantOptions{
			ColumnGroupBy: installerColumnGroupByVariants,
			DBGroupBy:     defaultDBGroupByVariants,
		},
		RequestAdvancedOptions: defaultAdvancedOption,
	}
	componentPageGenerator = componentReportGenerator{
		gcsBucket: "test-platform-results",
		RequestTestIdentificationOptions: crtype.RequestTestIdentificationOptions{
			Component: "component 2",
		},
		RequestVariantOptions: crtype.RequestVariantOptions{
			ColumnGroupBy: defaultColumnGroupByVariants,
			DBGroupBy:     defaultDBGroupByVariants,
		},
		RequestAdvancedOptions: defaultAdvancedOption,
	}
	capabilityPageGenerator = componentReportGenerator{
		gcsBucket: "test-platform-results",
		RequestTestIdentificationOptions: crtype.RequestTestIdentificationOptions{
			Component:  "component 2",
			Capability: "cap22",
		},
		RequestVariantOptions: crtype.RequestVariantOptions{
			ColumnGroupBy: defaultColumnGroupByVariants,
			DBGroupBy:     defaultDBGroupByVariants,
		},
		RequestAdvancedOptions: defaultAdvancedOption,
	}
	testPageGenerator = componentReportGenerator{
		gcsBucket: "test-platform-results",
		RequestTestIdentificationOptions: crtype.RequestTestIdentificationOptions{
			Component:  "component 2",
			Capability: "cap22",
			TestID:     "2",
		},
		RequestVariantOptions: crtype.RequestVariantOptions{
			ColumnGroupBy: defaultColumnGroupByVariants,
			DBGroupBy:     defaultDBGroupByVariants,
		},
		RequestAdvancedOptions: defaultAdvancedOption,
	}
	testDetailsGenerator = componentReportGenerator{
		gcsBucket: "test-platform-results",
		RequestTestIdentificationOptions: crtype.RequestTestIdentificationOptions{
			Component:  "component 1",
			Capability: "cap11",
			TestID:     "1",
		},
		RequestVariantOptions: crtype.RequestVariantOptions{
			ColumnGroupBy: defaultColumnGroupByVariants,
			DBGroupBy:     defaultDBGroupByVariants,
			RequestedVariants: map[string]string{
				"Platform":     "aws",
				"Architecture": "amd64",
				"Network":      "ovn",
			},
		},
		RequestAdvancedOptions: defaultAdvancedOption,
	}
)

func filterColumnIDByDefault(id crtype.ColumnIdentification) crtype.ColumnIdentification {
	ret := crtype.ColumnIdentification{Variants: map[string]string{}}
	for _, variant := range strings.Split(DefaultDBGroupBy, ",") {
		if value, ok := id.Variants[variant]; ok {
			ret.Variants[variant] = value
		}
	}
	return ret
}

func TestGenerateComponentReport(t *testing.T) {
	awsAMD64OVNTest := crtype.TestIdentification{
		TestID: "1",
		Variants: map[string]string{
			"Platform":     "aws",
			"Architecture": "amd64",
			"Network":      "ovn",
			"Upgrade":      "upgrade-micro",
			"Topology":     "ha",
			"FeatureSet":   "techpreview",
			"Suite":        "serial",
			"Installer":    "ipi",
		},
	}
	awsAMD64OVNTestBytes, err := json.Marshal(awsAMD64OVNTest)
	if err != nil {
		assert.NoError(t, err, "error marshalling awsAMD64OVNTest")
	}
	awsAMD64SDNTest := crtype.TestIdentification{
		TestID: "2",
		Variants: map[string]string{
			"Platform":     "aws",
			"Architecture": "amd64",
			"Network":      "sdn",
			"Upgrade":      "upgrade-micro",
			"Topology":     "ha",
			"FeatureSet":   "techpreview",
			"Suite":        "serial",
			"Installer":    "ipi",
		},
	}
	awsAMD64SDNTestBytes, err := json.Marshal(awsAMD64SDNTest)
	if err != nil {
		assert.NoError(t, err, "error marshalling awsAMD64SDNTest")
	}
	awsAMD64SDNInstallerUPITest := crtype.TestIdentification{
		TestID: "2",
		Variants: map[string]string{
			"Platform":     "aws",
			"Architecture": "amd64",
			"Network":      "sdn",
			"Upgrade":      "upgrade-micro",
			"Topology":     "ha",
			"FeatureSet":   "techpreview",
			"Suite":        "serial",
			"Installer":    "upi",
		},
	}
	awsAMD64SDNInstallerUPITestBytes, err := json.Marshal(awsAMD64SDNInstallerUPITest)
	if err != nil {
		assert.NoError(t, err, "error marshalling awsAMD64SDNInstallerUPITest")
	}
	awsAMD64OVN2Test := crtype.TestIdentification{
		TestID: "3",
		Variants: map[string]string{
			"Platform":     "aws",
			"Architecture": "amd64",
			"Network":      "ovn",
			"Upgrade":      "upgrade-micro",
		},
	}
	awsAMD64OVN2TestBytes, err := json.Marshal(awsAMD64OVN2Test)
	if err != nil {
		assert.NoError(t, err, "error marshalling awsAMD64OVN2Test")
	}
	awsAMD64OVNInstallerIPITest := crtype.TestIdentification{
		TestID: "1",
		Variants: map[string]string{
			"Platform":     "aws",
			"Architecture": "amd64",
			"Network":      "ovn",
			"Upgrade":      "upgrade-micro",
			"Topology":     "ha",
			"FeatureSet":   "techpreview",
			"Suite":        "serial",
			"Installer":    "ipi",
		},
	}
	awsAMD64OVNVariantsTestBytes, err := json.Marshal(awsAMD64OVNInstallerIPITest)
	if err != nil {
		assert.NoError(t, err, "error marshalling awsAMD64OVNInstallerIPITest")
	}
	awsAMD64OVNBaseTestStats90Percent := crtype.TestStatus{
		TestName:     "test 1",
		Variants:     []string{"standard"},
		TotalCount:   1000,
		FlakeCount:   10,
		SuccessCount: 900,
	}
	awsAMD64OVNBaseTestStats50Percent := crtype.TestStatus{
		TestName:     "test 1",
		Variants:     []string{"standard"},
		TotalCount:   1000,
		FlakeCount:   10,
		SuccessCount: 500,
	}
	awsAMD64OVNBaseTestStatsVariants90Percent := crtype.TestStatus{
		TestName:     "test 1",
		Variants:     []string{"standard", "fips"},
		TotalCount:   1000,
		FlakeCount:   10,
		SuccessCount: 900,
	}
	awsAMD64OVNSampleTestStats90Percent := crtype.TestStatus{
		TestName:     "test 1",
		Variants:     []string{"standard"},
		TotalCount:   100,
		FlakeCount:   1,
		SuccessCount: 90,
	}
	awsAMD64OVNSampleTestStats85Percent := crtype.TestStatus{
		TestName:     "test 1",
		Variants:     []string{"standard"},
		TotalCount:   100,
		FlakeCount:   1,
		SuccessCount: 85,
	}
	awsAMD64OVNSampleTestStats50Percent := crtype.TestStatus{
		TestName:     "test 1",
		Variants:     []string{"standard"},
		TotalCount:   100,
		FlakeCount:   1,
		SuccessCount: 50,
	}
	awsAMD64OVNSampleTestStatsTiny := crtype.TestStatus{
		TestName:     "test 1",
		Variants:     []string{"standard"},
		TotalCount:   3,
		FlakeCount:   0,
		SuccessCount: 1,
	}
	awsAMD64OVNSampleTestStatsVariants90Percent := crtype.TestStatus{
		TestName:     "test 1",
		Variants:     []string{"standard", "fips"},
		TotalCount:   100,
		FlakeCount:   1,
		SuccessCount: 90,
	}
	awsAMD64SDNBaseTestStats90Percent := crtype.TestStatus{
		TestName:     "test 2",
		Variants:     []string{"standard"},
		TotalCount:   1000,
		FlakeCount:   10,
		SuccessCount: 900,
	}
	awsAMD64SDNBaseTestStats50Percent := crtype.TestStatus{
		TestName:     "test 2",
		Variants:     []string{"standard"},
		TotalCount:   1000,
		FlakeCount:   10,
		SuccessCount: 500,
	}
	awsAMD64SDNSampleTestStats90Percent := crtype.TestStatus{
		TestName:     "test 2",
		Variants:     []string{"standard"},
		TotalCount:   100,
		FlakeCount:   1,
		SuccessCount: 90,
	}
	awsAMD64OVN2BaseTestStats90Percent := crtype.TestStatus{
		TestName:     "test 3",
		Variants:     []string{"standard"},
		TotalCount:   1000,
		FlakeCount:   10,
		SuccessCount: 900,
	}
	awsAMD64OVN2SampleTestStats80Percent := crtype.TestStatus{
		TestName:     "test 3",
		Variants:     []string{"standard"},
		TotalCount:   100,
		FlakeCount:   1,
		SuccessCount: 80,
	}
	columnAWSAMD64OVN := crtype.ColumnIdentification{
		Variants: map[string]string{
			"Platform":     "aws",
			"Architecture": "amd64",
			"Network":      "ovn",
		},
	}
	columnAWSAMD64OVNInstallerIPI := crtype.ColumnIdentification{
		Variants: map[string]string{
			"Platform":     "aws",
			"Architecture": "amd64",
			"Network":      "ovn",
			"Installer":    "ipi",
		},
	}
	columnAWSAMD64SDN := crtype.ColumnIdentification{
		Variants: map[string]string{
			"Platform":     "aws",
			"Architecture": "amd64",
			"Network":      "sdn",
		},
	}
	columnAWSAMD64SDNInstallerUPI := crtype.ColumnIdentification{
		Variants: map[string]string{
			"Platform":     "aws",
			"Architecture": "amd64",
			"Network":      "sdn",
			"Installer":    "upi",
		},
	}
	columnAWSAMD64OVNFull := crtype.ColumnIdentification{
		Variants: map[string]string{
			"Platform":     "aws",
			"Architecture": "amd64",
			"Network":      "ovn",
			"Upgrade":      "upgrade-micro",
			"Topology":     "ha",
			"FeatureSet":   "techpreview",
			"Suite":        "serial",
			"Installer":    "ipi",
		},
	}
	columnAWSAMD64SDNFull := crtype.ColumnIdentification{
		Variants: map[string]string{
			"Platform":     "aws",
			"Architecture": "amd64",
			"Network":      "sdn",
			"Upgrade":      "upgrade-micro",
			"Topology":     "ha",
			"FeatureSet":   "techpreview",
			"Suite":        "serial",
			"Installer":    "ipi",
		},
	}
	rowComponent1 := crtype.RowIdentification{
		Component: "component 1",
	}
	rowComponent2 := crtype.RowIdentification{
		Component: "component 2",
	}
	rowComponent2Cap21 := crtype.RowIdentification{
		Component:  "component 2",
		Capability: "cap21",
	}
	rowComponent2Cap22 := crtype.RowIdentification{
		Component:  "component 2",
		Capability: "cap22",
	}
	rowComponent2Cap22Test2 := crtype.RowIdentification{
		Component:  "component 2",
		Capability: "cap22",
		TestName:   "test 2",
		TestID:     "2",
	}

	tests := []struct {
		name           string
		generator      componentReportGenerator
		baseStatus     map[string]crtype.TestStatus
		sampleStatus   map[string]crtype.TestStatus
		expectedReport crtype.ComponentReport
	}{
		{
			name:      "top page test no significant and missing data",
			generator: defaultComponentReportGenerator,
			baseStatus: map[string]crtype.TestStatus{
				string(awsAMD64OVNTestBytes): awsAMD64OVNBaseTestStats90Percent,
				string(awsAMD64SDNTestBytes): awsAMD64SDNBaseTestStats90Percent,
			},
			sampleStatus: map[string]crtype.TestStatus{
				string(awsAMD64OVNTestBytes): awsAMD64OVNSampleTestStats85Percent,
				string(awsAMD64SDNTestBytes): awsAMD64SDNSampleTestStats90Percent,
			},
			expectedReport: crtype.ComponentReport{
				Rows: []crtype.ReportRow{
					{
						RowIdentification: crtype.RowIdentification{
							Component: "component 1",
						},
						Columns: []crtype.ReportColumn{
							{
								ColumnIdentification: columnAWSAMD64OVN,
								Status:               crtype.NotSignificant,
							},
							{
								ColumnIdentification: columnAWSAMD64SDN,
								Status:               crtype.MissingBasisAndSample,
							},
						},
					},
					{
						RowIdentification: crtype.RowIdentification{
							Component: "component 2",
						},
						Columns: []crtype.ReportColumn{
							{
								ColumnIdentification: columnAWSAMD64OVN,
								Status:               crtype.MissingBasisAndSample,
							},
							{
								ColumnIdentification: columnAWSAMD64SDN,
								Status:               crtype.NotSignificant,
							},
						},
					},
				},
			},
		},
		{
			name:      "top page test with both improvement and regression",
			generator: defaultComponentReportGenerator,
			baseStatus: map[string]crtype.TestStatus{
				string(awsAMD64OVNTestBytes):  awsAMD64OVNBaseTestStats90Percent,
				string(awsAMD64OVN2TestBytes): awsAMD64OVN2BaseTestStats90Percent,
				string(awsAMD64SDNTestBytes):  awsAMD64SDNBaseTestStats50Percent,
			},
			sampleStatus: map[string]crtype.TestStatus{
				string(awsAMD64OVNTestBytes):  awsAMD64OVNSampleTestStats50Percent,
				string(awsAMD64OVN2TestBytes): awsAMD64OVN2SampleTestStats80Percent,
				string(awsAMD64SDNTestBytes):  awsAMD64SDNSampleTestStats90Percent,
			},
			expectedReport: crtype.ComponentReport{
				Rows: []crtype.ReportRow{
					{
						RowIdentification: rowComponent1,
						Columns: []crtype.ReportColumn{
							{
								ColumnIdentification: columnAWSAMD64OVN,
								Status:               crtype.ExtremeRegression,
								RegressedTests: []crtype.ReportTestSummary{
									{
										ReportTestIdentification: crtype.ReportTestIdentification{
											RowIdentification: crtype.RowIdentification{
												TestName: awsAMD64OVNBaseTestStats90Percent.TestName,
												TestID:   awsAMD64OVNTest.TestID,
											},
											ColumnIdentification: crtype.ColumnIdentification{
												Variants: awsAMD64OVNTest.Variants,
											},
										},
										ReportTestStats: crtype.ReportTestStats{
											Comparison: crtype.FisherExact,
											Explanations: []string{
												"Extreme regression detected.",
												"Fishers Exact probability of a regression: 100.00%.",
												"Test pass rate dropped from 91.00% to 51.00%.",
											},
											ReportStatus: crtype.ExtremeRegression,
											FisherExact:  thrift.Float64Ptr(1.8251046156331867e-21),
											SampleStats: crtype.TestDetailsReleaseStats{
												TestDetailsTestStats: crtype.TestDetailsTestStats{
													SuccessRate:  0.51,
													SuccessCount: 50,
													FailureCount: 49,
													FlakeCount:   1,
												},
											},
											BaseStats: &crtype.TestDetailsReleaseStats{
												TestDetailsTestStats: crtype.TestDetailsTestStats{
													SuccessRate:  0.91,
													SuccessCount: 900,
													FailureCount: 90,
													FlakeCount:   10,
												},
											},
										},
									},
									{
										ReportTestIdentification: crtype.ReportTestIdentification{
											RowIdentification: crtype.RowIdentification{
												TestName: awsAMD64OVN2BaseTestStats90Percent.TestName,
												TestID:   awsAMD64OVN2Test.TestID,
											},
											ColumnIdentification: crtype.ColumnIdentification{
												Variants: awsAMD64OVN2Test.Variants,
											},
										},
										ReportTestStats: crtype.ReportTestStats{
											Comparison: crtype.FisherExact,
											Explanations: []string{
												"Significant regression detected.",
												"Fishers Exact probability of a regression: 100.00%.",
												"Test pass rate dropped from 91.00% to 81.00%.",
											},
											ReportStatus: crtype.SignificantRegression,
											FisherExact:  thrift.Float64Ptr(0.002621948654892275),
											SampleStats: crtype.TestDetailsReleaseStats{
												TestDetailsTestStats: crtype.TestDetailsTestStats{
													SuccessRate:  0.81,
													SuccessCount: 80,
													FailureCount: 19,
													FlakeCount:   1,
												},
											},
											BaseStats: &crtype.TestDetailsReleaseStats{
												TestDetailsTestStats: crtype.TestDetailsTestStats{
													SuccessRate:  0.91,
													SuccessCount: 900,
													FailureCount: 90,
													FlakeCount:   10,
												},
											},
										},
									},
								},
							},
							{
								ColumnIdentification: columnAWSAMD64SDN,
								Status:               crtype.MissingBasisAndSample,
							},
						},
					},
					{
						RowIdentification: rowComponent2,
						Columns: []crtype.ReportColumn{
							{
								ColumnIdentification: columnAWSAMD64OVN,
								Status:               crtype.MissingBasisAndSample,
							},
							{
								ColumnIdentification: columnAWSAMD64SDN,
								Status:               crtype.SignificantImprovement,
							},
						},
					},
				},
			},
		},
		{
			name:      "component page test no significant and missing data",
			generator: componentPageGenerator,
			baseStatus: map[string]crtype.TestStatus{
				string(awsAMD64OVNTestBytes): awsAMD64OVNBaseTestStats90Percent,
				string(awsAMD64SDNTestBytes): awsAMD64SDNBaseTestStats90Percent,
			},
			sampleStatus: map[string]crtype.TestStatus{
				string(awsAMD64OVNTestBytes): awsAMD64OVNSampleTestStats90Percent,
				string(awsAMD64SDNTestBytes): awsAMD64SDNSampleTestStats90Percent,
			},
			expectedReport: crtype.ComponentReport{
				Rows: []crtype.ReportRow{
					{
						RowIdentification: rowComponent2Cap21,
						Columns: []crtype.ReportColumn{
							{
								ColumnIdentification: columnAWSAMD64OVN,
								Status:               crtype.MissingBasisAndSample,
							},
							{
								ColumnIdentification: columnAWSAMD64SDN,
								Status:               crtype.NotSignificant,
							},
						},
					},
					{
						RowIdentification: rowComponent2Cap22,
						Columns: []crtype.ReportColumn{
							{
								ColumnIdentification: columnAWSAMD64OVN,
								Status:               crtype.MissingBasisAndSample,
							},
							{
								ColumnIdentification: columnAWSAMD64SDN,
								Status:               crtype.NotSignificant,
							},
						},
					},
				},
			},
		},
		{
			name:      "component page test with both improvement and regression",
			generator: componentPageGenerator,
			baseStatus: map[string]crtype.TestStatus{
				string(awsAMD64OVNTestBytes): awsAMD64OVNBaseTestStats90Percent,
				string(awsAMD64SDNTestBytes): awsAMD64SDNBaseTestStats50Percent,
			},
			sampleStatus: map[string]crtype.TestStatus{
				string(awsAMD64OVNTestBytes): awsAMD64OVNBaseTestStats50Percent,
				string(awsAMD64SDNTestBytes): awsAMD64SDNBaseTestStats90Percent,
			},
			expectedReport: crtype.ComponentReport{
				Rows: []crtype.ReportRow{
					{
						RowIdentification: rowComponent2Cap21,
						Columns: []crtype.ReportColumn{
							{
								ColumnIdentification: columnAWSAMD64OVN,
								Status:               crtype.MissingBasisAndSample,
							},
							{
								ColumnIdentification: columnAWSAMD64SDN,
								Status:               crtype.SignificantImprovement,
							},
						},
					},
					{
						RowIdentification: rowComponent2Cap22,
						Columns: []crtype.ReportColumn{
							{
								ColumnIdentification: columnAWSAMD64OVN,
								Status:               crtype.MissingBasisAndSample,
							},
							{
								ColumnIdentification: columnAWSAMD64SDN,
								Status:               crtype.SignificantImprovement,
							},
						},
					},
				},
			},
		},
		{
			name:      "capability page test no significant and missing data",
			generator: capabilityPageGenerator,
			baseStatus: map[string]crtype.TestStatus{
				string(awsAMD64OVNTestBytes): awsAMD64OVNBaseTestStats90Percent,
				string(awsAMD64SDNTestBytes): awsAMD64SDNBaseTestStats90Percent,
			},
			sampleStatus: map[string]crtype.TestStatus{
				string(awsAMD64OVNTestBytes): awsAMD64OVNSampleTestStats90Percent,
				string(awsAMD64SDNTestBytes): awsAMD64SDNSampleTestStats90Percent,
			},
			expectedReport: crtype.ComponentReport{
				Rows: []crtype.ReportRow{
					{
						RowIdentification: rowComponent2Cap22Test2,
						Columns: []crtype.ReportColumn{
							{
								ColumnIdentification: columnAWSAMD64OVN,
								Status:               crtype.MissingBasisAndSample,
							},
							{
								ColumnIdentification: columnAWSAMD64SDN,
								Status:               crtype.NotSignificant,
							},
						},
					},
				},
			},
		},
		{
			name:      "capability page test with both improvement and regression",
			generator: capabilityPageGenerator,
			baseStatus: map[string]crtype.TestStatus{
				string(awsAMD64OVNTestBytes): awsAMD64OVNBaseTestStats90Percent,
				string(awsAMD64SDNTestBytes): awsAMD64SDNBaseTestStats50Percent,
			},
			sampleStatus: map[string]crtype.TestStatus{
				string(awsAMD64OVNTestBytes): awsAMD64OVNSampleTestStats50Percent,
				string(awsAMD64SDNTestBytes): awsAMD64SDNSampleTestStats90Percent,
			},
			expectedReport: crtype.ComponentReport{
				Rows: []crtype.ReportRow{
					{
						RowIdentification: rowComponent2Cap22Test2,
						Columns: []crtype.ReportColumn{
							{
								ColumnIdentification: columnAWSAMD64OVN,
								Status:               crtype.MissingBasisAndSample,
							},
							{
								ColumnIdentification: columnAWSAMD64SDN,
								Status:               crtype.SignificantImprovement,
							},
						},
					},
				},
			},
		},
		{
			name:      "test page test no significant and missing data",
			generator: testPageGenerator,
			baseStatus: map[string]crtype.TestStatus{
				string(awsAMD64OVNTestBytes): awsAMD64OVNBaseTestStats90Percent,
				string(awsAMD64SDNTestBytes): awsAMD64SDNBaseTestStats90Percent,
			},
			sampleStatus: map[string]crtype.TestStatus{
				string(awsAMD64OVNTestBytes): awsAMD64OVNSampleTestStats90Percent,
				string(awsAMD64SDNTestBytes): awsAMD64SDNSampleTestStats90Percent,
			},
			expectedReport: crtype.ComponentReport{
				Rows: []crtype.ReportRow{
					{
						RowIdentification: rowComponent2Cap22Test2,
						Columns: []crtype.ReportColumn{
							{
								ColumnIdentification: filterColumnIDByDefault(columnAWSAMD64OVNFull),
								Status:               crtype.MissingBasisAndSample,
							},
							{
								ColumnIdentification: filterColumnIDByDefault(columnAWSAMD64SDNFull),
								Status:               crtype.NotSignificant,
							},
						},
					},
				},
			},
		},
		{
			name:      "test page test with both improvement and regression",
			generator: testPageGenerator,
			baseStatus: map[string]crtype.TestStatus{
				string(awsAMD64OVNTestBytes): awsAMD64OVNBaseTestStats90Percent,
				string(awsAMD64SDNTestBytes): awsAMD64SDNBaseTestStats50Percent,
			},
			sampleStatus: map[string]crtype.TestStatus{
				string(awsAMD64OVNTestBytes): awsAMD64OVNSampleTestStats50Percent,
				string(awsAMD64SDNTestBytes): awsAMD64SDNSampleTestStats90Percent,
			},
			expectedReport: crtype.ComponentReport{
				Rows: []crtype.ReportRow{
					{
						RowIdentification: rowComponent2Cap22Test2,
						Columns: []crtype.ReportColumn{
							{
								ColumnIdentification: filterColumnIDByDefault(columnAWSAMD64OVNFull),
								Status:               crtype.MissingBasisAndSample,
							},
							{
								ColumnIdentification: filterColumnIDByDefault(columnAWSAMD64SDNFull),
								Status:               crtype.SignificantImprovement,
							},
						},
					},
				},
			},
		},
		{
			name: "top page test confidence 90 result in regression",
			generator: componentReportGenerator{
				RequestVariantOptions: crtype.RequestVariantOptions{
					ColumnGroupBy: defaultColumnGroupByVariants,
				},
				RequestAdvancedOptions: crtype.RequestAdvancedOptions{
					Confidence:     90,
					PityFactor:     5,
					MinimumFailure: 3,
				},
			},
			baseStatus: map[string]crtype.TestStatus{
				string(awsAMD64OVNTestBytes): awsAMD64OVNBaseTestStats90Percent,
				string(awsAMD64SDNTestBytes): awsAMD64SDNBaseTestStats90Percent,
			},
			sampleStatus: map[string]crtype.TestStatus{
				string(awsAMD64OVNTestBytes): awsAMD64OVNSampleTestStats85Percent,
				string(awsAMD64SDNTestBytes): awsAMD64SDNSampleTestStats90Percent,
			},
			expectedReport: crtype.ComponentReport{
				Rows: []crtype.ReportRow{
					{
						RowIdentification: rowComponent1,
						Columns: []crtype.ReportColumn{
							{
								ColumnIdentification: columnAWSAMD64OVN,
								Status:               crtype.SignificantRegression,
								RegressedTests: []crtype.ReportTestSummary{
									{
										ReportTestIdentification: crtype.ReportTestIdentification{
											RowIdentification: crtype.RowIdentification{
												TestName: awsAMD64OVNBaseTestStats90Percent.TestName,
												TestID:   awsAMD64OVNTest.TestID,
											},
											ColumnIdentification: crtype.ColumnIdentification{
												Variants: awsAMD64OVNTest.Variants,
											},
										},
										ReportTestStats: crtype.ReportTestStats{
											Comparison: crtype.FisherExact,
											Explanations: []string{
												"Significant regression detected.",
												"Fishers Exact probability of a regression: 99.92%.",
												"Test pass rate dropped from 91.00% to 86.00%.",
											},
											ReportStatus: crtype.SignificantRegression,
											FisherExact:  thrift.Float64Ptr(0.07837082801914011),
											SampleStats: crtype.TestDetailsReleaseStats{
												TestDetailsTestStats: crtype.TestDetailsTestStats{
													SuccessRate:  0.86,
													SuccessCount: 85,
													FailureCount: 14,
													FlakeCount:   1,
												},
											},
											BaseStats: &crtype.TestDetailsReleaseStats{
												TestDetailsTestStats: crtype.TestDetailsTestStats{
													SuccessRate:  0.91,
													SuccessCount: 900,
													FailureCount: 90,
													FlakeCount:   10,
												},
											},
										},
									},
								},
							},
							{
								ColumnIdentification: columnAWSAMD64SDN,
								Status:               crtype.MissingBasisAndSample,
							},
						},
					},
					{
						RowIdentification: rowComponent2,
						Columns: []crtype.ReportColumn{
							{
								ColumnIdentification: columnAWSAMD64OVN,
								Status:               crtype.MissingBasisAndSample,
							},
							{
								ColumnIdentification: columnAWSAMD64SDN,
								Status:               crtype.NotSignificant,
							},
						},
					},
				},
			},
		},
		{
			name: "top page test confidence 90 pity 10 result in no regression",
			generator: componentReportGenerator{
				RequestVariantOptions: crtype.RequestVariantOptions{
					ColumnGroupBy: defaultColumnGroupByVariants,
				},
				RequestAdvancedOptions: crtype.RequestAdvancedOptions{
					Confidence:     90,
					PityFactor:     10,
					MinimumFailure: 3,
				},
			},
			baseStatus: map[string]crtype.TestStatus{
				string(awsAMD64OVNTestBytes): awsAMD64OVNBaseTestStats90Percent,
				string(awsAMD64SDNTestBytes): awsAMD64SDNBaseTestStats90Percent,
			},
			sampleStatus: map[string]crtype.TestStatus{
				string(awsAMD64OVNTestBytes): awsAMD64OVNSampleTestStats85Percent,
				string(awsAMD64SDNTestBytes): awsAMD64SDNSampleTestStats90Percent,
			},
			expectedReport: crtype.ComponentReport{
				Rows: []crtype.ReportRow{
					{
						RowIdentification: rowComponent1,
						Columns: []crtype.ReportColumn{
							{
								ColumnIdentification: columnAWSAMD64OVN,
								Status:               crtype.NotSignificant,
							},
							{
								ColumnIdentification: columnAWSAMD64SDN,
								Status:               crtype.MissingBasisAndSample,
							},
						},
					},
					{
						RowIdentification: rowComponent2,
						Columns: []crtype.ReportColumn{
							{
								ColumnIdentification: columnAWSAMD64OVN,
								Status:               crtype.MissingBasisAndSample,
							},
							{
								ColumnIdentification: columnAWSAMD64SDN,
								Status:               crtype.NotSignificant,
							},
						},
					},
				},
			},
		},
		{
			name:      "top page test minimum failure no regression",
			generator: defaultComponentReportGenerator,
			baseStatus: map[string]crtype.TestStatus{
				string(awsAMD64OVNTestBytes): awsAMD64OVNBaseTestStats90Percent,
				string(awsAMD64SDNTestBytes): awsAMD64SDNBaseTestStats90Percent,
			},
			sampleStatus: map[string]crtype.TestStatus{
				string(awsAMD64OVNTestBytes): awsAMD64OVNSampleTestStatsTiny,
				string(awsAMD64SDNTestBytes): awsAMD64SDNSampleTestStats90Percent,
			},
			expectedReport: crtype.ComponentReport{
				Rows: []crtype.ReportRow{
					{
						RowIdentification: rowComponent1,
						Columns: []crtype.ReportColumn{
							{
								ColumnIdentification: columnAWSAMD64OVN,
								Status:               crtype.NotSignificant,
							},
							{
								ColumnIdentification: columnAWSAMD64SDN,
								Status:               crtype.MissingBasisAndSample,
							},
						},
					},
					{
						RowIdentification: rowComponent2,
						Columns: []crtype.ReportColumn{
							{
								ColumnIdentification: columnAWSAMD64OVN,
								Status:               crtype.MissingBasisAndSample,
							},
							{
								ColumnIdentification: columnAWSAMD64SDN,
								Status:               crtype.NotSignificant,
							},
						},
					},
				},
			},
		},
		{
			name:      "top page test group by installer",
			generator: groupByInstallerComponentReportGenerator,
			baseStatus: map[string]crtype.TestStatus{
				string(awsAMD64OVNVariantsTestBytes):     awsAMD64OVNBaseTestStatsVariants90Percent,
				string(awsAMD64SDNInstallerUPITestBytes): awsAMD64SDNBaseTestStats90Percent,
			},
			sampleStatus: map[string]crtype.TestStatus{
				string(awsAMD64OVNVariantsTestBytes):     awsAMD64OVNSampleTestStatsVariants90Percent,
				string(awsAMD64SDNInstallerUPITestBytes): awsAMD64SDNSampleTestStats90Percent,
			},
			expectedReport: crtype.ComponentReport{
				Rows: []crtype.ReportRow{
					{
						RowIdentification: crtype.RowIdentification{
							Component: "component 1",
						},
						Columns: []crtype.ReportColumn{
							{
								ColumnIdentification: columnAWSAMD64OVNInstallerIPI,
								Status:               crtype.NotSignificant,
							},
							{
								ColumnIdentification: columnAWSAMD64SDNInstallerUPI,
								Status:               crtype.MissingBasisAndSample,
							},
						},
					},
					{
						RowIdentification: crtype.RowIdentification{
							Component: "component 2",
						},
						Columns: []crtype.ReportColumn{
							{
								ColumnIdentification: columnAWSAMD64OVNInstallerIPI,
								Status:               crtype.MissingBasisAndSample,
							},
							{
								ColumnIdentification: columnAWSAMD64SDNInstallerUPI,
								Status:               crtype.NotSignificant,
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
			report, err := tc.generator.generateComponentTestReport(context.TODO(), tc.baseStatus, tc.sampleStatus)
			assert.NoError(t, err, "error generating component report")
			assert.Equal(t, tc.expectedReport, report, "expected report %+v, got %+v", tc.expectedReport, report)
		})
	}
}

func TestGenerateComponentTestDetailsReport(t *testing.T) {
	prowJob1 := "ProwJob1"
	prowJob2 := "ProwJob2"
	type testStats struct {
		Success int
		Failure int
		Flake   int
	}
	type requiredJobStats struct {
		job string
		testStats
	}
	baseHighSuccessStats := testStats{
		Success: 1000,
		Failure: 100,
		Flake:   50,
	}
	baseLowSuccessStats := testStats{
		Success: 500,
		Failure: 600,
		Flake:   50,
	}
	sampleHighSuccessStats := testStats{
		Success: 100,
		Failure: 9,
		Flake:   4,
	}
	sampleLowSuccessStats := testStats{
		Success: 50,
		Failure: 59,
		Flake:   4,
	}
	testDetailsRowIdentification := crtype.RowIdentification{
		TestID:     testDetailsGenerator.TestID,
		Component:  testDetailsGenerator.Component,
		Capability: testDetailsGenerator.Capability,
	}
	testDetailsColumnIdentification := crtype.ColumnIdentification{
		Variants: testDetailsGenerator.RequestedVariants,
	}
	sampleReleaseStatsTwoHigh := crtype.TestDetailsReleaseStats{
		Release: testDetailsGenerator.SampleRelease.Release,
		TestDetailsTestStats: crtype.TestDetailsTestStats{
			SuccessRate:  0.9203539823008849,
			SuccessCount: 200,
			FailureCount: 18,
			FlakeCount:   8,
		},
	}
	baseReleaseStatsTwoHigh := crtype.TestDetailsReleaseStats{
		Release: testDetailsGenerator.BaseRelease.Release,
		TestDetailsTestStats: crtype.TestDetailsTestStats{
			SuccessRate:  0.9130434782608695,
			SuccessCount: 2000,
			FailureCount: 200,
			FlakeCount:   100,
		},
	}
	sampleTestStatsHigh := crtype.TestDetailsTestStats{
		SuccessRate:  0.9203539823008849,
		SuccessCount: 100,
		FailureCount: 9,
		FlakeCount:   4,
	}
	baseTestStatsHigh := crtype.TestDetailsTestStats{
		SuccessRate:  0.9130434782608695,
		SuccessCount: 1000,
		FailureCount: 100,
		FlakeCount:   50,
	}
	sampleTestStatsLow := crtype.TestDetailsTestStats{
		SuccessRate:  0.4778761061946903,
		SuccessCount: 50,
		FailureCount: 59,
		FlakeCount:   4,
	}
	baseTestStatsLow := crtype.TestDetailsTestStats{
		SuccessRate:  0.4782608695652174,
		SuccessCount: 500,
		FailureCount: 600,
		FlakeCount:   50,
	}
	sampleReleaseStatsOneHigh := crtype.TestDetailsReleaseStats{
		Release: testDetailsGenerator.SampleRelease.Release,
		TestDetailsTestStats: crtype.TestDetailsTestStats{
			SuccessRate:  0.9203539823008849,
			SuccessCount: 100,
			FailureCount: 9,
			FlakeCount:   4,
		},
	}
	baseReleaseStatsOneHigh := crtype.TestDetailsReleaseStats{
		Release: testDetailsGenerator.BaseRelease.Release,
		TestDetailsTestStats: crtype.TestDetailsTestStats{
			SuccessRate:  0.9130434782608695,
			SuccessCount: 1000,
			FailureCount: 100,
			FlakeCount:   50,
		},
	}
	sampleReleaseStatsOneLow := crtype.TestDetailsReleaseStats{
		Release: testDetailsGenerator.SampleRelease.Release,
		TestDetailsTestStats: crtype.TestDetailsTestStats{
			SuccessRate:  0.4778761061946903,
			SuccessCount: 50,
			FailureCount: 59,
			FlakeCount:   4,
		},
	}
	baseReleaseStatsOneLow := crtype.TestDetailsReleaseStats{
		Release: testDetailsGenerator.BaseRelease.Release,
		TestDetailsTestStats: crtype.TestDetailsTestStats{
			SuccessRate:  0.4782608695652174,
			SuccessCount: 500,
			FailureCount: 600,
			FlakeCount:   50,
		},
	}
	tests := []struct {
		name                    string
		generator               componentReportGenerator
		baseRequiredJobStats    []requiredJobStats
		sampleRequiredJobStats  []requiredJobStats
		expectedReport          crtype.ReportTestDetails
		expectedSampleJobRunLen map[string]int
		expectedBaseJobRunLen   map[string]int
	}{
		{
			name:      "one job with high pass base and sample",
			generator: testDetailsGenerator,
			baseRequiredJobStats: []requiredJobStats{
				{
					job:       prowJob1,
					testStats: baseHighSuccessStats,
				},
			},
			sampleRequiredJobStats: []requiredJobStats{
				{
					job:       prowJob1,
					testStats: sampleHighSuccessStats,
				},
			},
			expectedReport: crtype.ReportTestDetails{
				ReportTestIdentification: crtype.ReportTestIdentification{
					RowIdentification:    testDetailsRowIdentification,
					ColumnIdentification: testDetailsColumnIdentification,
				},
				ReportTestStats: crtype.ReportTestStats{
					Comparison:   crtype.FisherExact,
					SampleStats:  sampleReleaseStatsOneHigh,
					BaseStats:    &baseReleaseStatsOneHigh,
					FisherExact:  thrift.Float64Ptr(.4807457902463764),
					ReportStatus: crtype.NotSignificant,
				},
				JobStats: []crtype.TestDetailsJobStats{
					{
						JobName:     prowJob1,
						SampleStats: sampleTestStatsHigh,
						BaseStats:   baseTestStatsHigh,
						Significant: false,
					},
				},
			},
			expectedSampleJobRunLen: map[string]int{
				prowJob1: 113,
			},
			expectedBaseJobRunLen: map[string]int{
				prowJob1: 1150,
			},
		},
		{
			name:      "one job with high base and low sample pass rate",
			generator: testDetailsGenerator,
			baseRequiredJobStats: []requiredJobStats{
				{
					job:       prowJob1,
					testStats: baseHighSuccessStats,
				},
			},
			sampleRequiredJobStats: []requiredJobStats{
				{
					job:       prowJob1,
					testStats: sampleLowSuccessStats,
				},
			},
			expectedReport: crtype.ReportTestDetails{
				ReportTestIdentification: crtype.ReportTestIdentification{
					RowIdentification:    testDetailsRowIdentification,
					ColumnIdentification: testDetailsColumnIdentification,
				},
				ReportTestStats: crtype.ReportTestStats{
					Comparison:   crtype.FisherExact,
					SampleStats:  sampleReleaseStatsOneLow,
					BaseStats:    &baseReleaseStatsOneHigh,
					FisherExact:  thrift.Float64Ptr(8.209711662216515e-28),
					ReportStatus: crtype.ExtremeRegression,
				},
				JobStats: []crtype.TestDetailsJobStats{
					{
						JobName:     prowJob1,
						SampleStats: sampleTestStatsLow,
						BaseStats:   baseTestStatsHigh,
						Significant: false,
					},
				},
			},
			expectedSampleJobRunLen: map[string]int{
				prowJob1: 113,
			},
			expectedBaseJobRunLen: map[string]int{
				prowJob1: 1150,
			},
		},
		{
			name:      "one job with low base and high sample pass rate",
			generator: testDetailsGenerator,
			baseRequiredJobStats: []requiredJobStats{
				{
					job:       prowJob1,
					testStats: baseLowSuccessStats,
				},
			},
			sampleRequiredJobStats: []requiredJobStats{
				{
					job:       prowJob1,
					testStats: sampleHighSuccessStats,
				},
			},
			expectedReport: crtype.ReportTestDetails{
				ReportTestIdentification: crtype.ReportTestIdentification{
					RowIdentification:    testDetailsRowIdentification,
					ColumnIdentification: testDetailsColumnIdentification,
				},
				ReportTestStats: crtype.ReportTestStats{
					Comparison:   crtype.FisherExact,
					SampleStats:  sampleReleaseStatsOneHigh,
					BaseStats:    &baseReleaseStatsOneLow,
					FisherExact:  thrift.Float64Ptr(4.911246201592593e-22),
					ReportStatus: crtype.SignificantImprovement,
				},
				JobStats: []crtype.TestDetailsJobStats{
					{
						JobName:     prowJob1,
						SampleStats: sampleTestStatsHigh,
						BaseStats:   baseTestStatsLow,
						Significant: false,
					},
				},
			},
			expectedSampleJobRunLen: map[string]int{
				prowJob1: 113,
			},
			expectedBaseJobRunLen: map[string]int{
				prowJob1: 1150,
			},
		},
		{
			name:      "two jobs with high pass rate",
			generator: testDetailsGenerator,
			baseRequiredJobStats: []requiredJobStats{
				{
					job:       prowJob1,
					testStats: baseHighSuccessStats,
				},
				{
					job:       prowJob2,
					testStats: baseHighSuccessStats,
				},
			},
			sampleRequiredJobStats: []requiredJobStats{
				{
					job:       prowJob1,
					testStats: sampleHighSuccessStats,
				},
				{
					job:       prowJob2,
					testStats: sampleHighSuccessStats,
				},
			},
			expectedReport: crtype.ReportTestDetails{
				ReportTestIdentification: crtype.ReportTestIdentification{
					RowIdentification:    testDetailsRowIdentification,
					ColumnIdentification: testDetailsColumnIdentification,
				},
				ReportTestStats: crtype.ReportTestStats{
					Comparison:   crtype.FisherExact,
					SampleStats:  sampleReleaseStatsTwoHigh,
					BaseStats:    &baseReleaseStatsTwoHigh,
					FisherExact:  thrift.Float64Ptr(0.4119831376606586),
					ReportStatus: crtype.NotSignificant,
				},
				JobStats: []crtype.TestDetailsJobStats{
					{
						JobName:     prowJob1,
						SampleStats: sampleTestStatsHigh,
						BaseStats:   baseTestStatsHigh,
						Significant: false,
					},
					{
						JobName:     prowJob2,
						SampleStats: sampleTestStatsHigh,
						BaseStats:   baseTestStatsHigh,
						Significant: false,
					},
				},
			},
			expectedSampleJobRunLen: map[string]int{
				prowJob1: 113,
				prowJob2: 113,
			},
			expectedBaseJobRunLen: map[string]int{
				prowJob1: 1150,
				prowJob2: 1150,
			},
		},
	}
	componentAndCapabilityGetter = fakeComponentAndCapabilityGetter
	for _, tc := range tests {
		baseStats := map[string][]crtype.JobRunTestStatusRow{}
		sampleStats := map[string][]crtype.JobRunTestStatusRow{}
		for _, testStats := range tc.baseRequiredJobStats {
			for i := 0; i < testStats.Success; i++ {
				baseStats[testStats.job] = append(baseStats[testStats.job], crtype.JobRunTestStatusRow{
					ProwJob:      testStats.job,
					TotalCount:   1,
					SuccessCount: 1,
				})
			}
			for i := 0; i < testStats.Failure; i++ {
				baseStats[testStats.job] = append(baseStats[testStats.job], crtype.JobRunTestStatusRow{
					ProwJob:    testStats.job,
					TotalCount: 1,
				})
			}
			for i := 0; i < testStats.Flake; i++ {
				baseStats[testStats.job] = append(baseStats[testStats.job], crtype.JobRunTestStatusRow{
					ProwJob:    testStats.job,
					TotalCount: 1,
					FlakeCount: 1,
				})
			}
		}
		for _, testStats := range tc.sampleRequiredJobStats {
			for i := 0; i < testStats.Success; i++ {
				sampleStats[testStats.job] = append(sampleStats[testStats.job], crtype.JobRunTestStatusRow{
					ProwJob:      testStats.job,
					TotalCount:   1,
					SuccessCount: 1,
				})
			}
			for i := 0; i < testStats.Failure; i++ {
				sampleStats[testStats.job] = append(sampleStats[testStats.job], crtype.JobRunTestStatusRow{
					ProwJob:    testStats.job,
					TotalCount: 1,
				})
			}
			for i := 0; i < testStats.Flake; i++ {
				sampleStats[testStats.job] = append(sampleStats[testStats.job], crtype.JobRunTestStatusRow{
					ProwJob:    testStats.job,
					TotalCount: 1,
					FlakeCount: 1,
				})
			}
		}

		t.Run(tc.name, func(t *testing.T) {
			report := tc.generator.internalGenerateTestDetailsReport(context.TODO(), baseStats, sampleStats)
			assert.Equal(t, tc.expectedReport.RowIdentification, report.RowIdentification, "expected report row identification %+v, got %+v", tc.expectedReport.RowIdentification, report.RowIdentification)
			assert.Equal(t, tc.expectedReport.ColumnIdentification, report.ColumnIdentification, "expected report column identification %+v, got %+v", tc.expectedReport.ColumnIdentification, report.ColumnIdentification)
			assert.Equal(t, tc.expectedReport.BaseStats, report.BaseStats, "expected report base stats %+v, got %+v", tc.expectedReport.BaseStats, report.BaseStats)
			assert.Equal(t, tc.expectedReport.SampleStats, report.SampleStats, "expected report sample stats %+v, got %+v", tc.expectedReport.SampleStats, report.SampleStats)
			assert.Equal(t, *tc.expectedReport.FisherExact, *report.FisherExact, "expected fisher exact number %+v, got %+v", tc.expectedReport.FisherExact, report.FisherExact)
			assert.Equal(t, tc.expectedReport.ReportStatus, report.ReportStatus, "expected report status %+v, got %+v", tc.expectedReport.ReportStatus, report.ReportStatus)
			assert.Equal(t, len(tc.expectedReport.JobStats), len(report.JobStats), "expected len of job stats %+v, got %+v", len(tc.expectedReport.JobStats), report.JobStats)
			for i := range tc.expectedReport.JobStats {
				jobName := report.JobStats[i].JobName
				assert.Equal(t, tc.expectedReport.JobStats[i].JobName, jobName, "expected job name %+v, got %+v", tc.expectedReport.JobStats[i].JobName, jobName)
				assert.Equal(t, tc.expectedReport.JobStats[i].Significant, report.JobStats[i].Significant, "expected per job significant %+v, got %+v", tc.expectedReport.JobStats[i].Significant, report.JobStats[i].Significant)
				assert.Equal(t, tc.expectedReport.JobStats[i].BaseStats, report.JobStats[i].BaseStats, "expected per job base stats for %s to be %+v, got %+v", tc.expectedReport.JobStats[i].JobName, tc.expectedReport.JobStats[i].BaseStats, report.JobStats[i].BaseStats)
				assert.Equal(t, tc.expectedReport.JobStats[i].SampleStats, report.JobStats[i].SampleStats, "expected per job sample stats for %s to be %+v, got %+v", tc.expectedReport.JobStats[i].JobName, tc.expectedReport.JobStats[i].SampleStats, report.JobStats[i].SampleStats)
				assert.Equal(t, tc.expectedSampleJobRunLen[jobName], len(report.JobStats[i].SampleJobRunStats), "expected sample job run counts %+v, got %+v", tc.expectedSampleJobRunLen[jobName], len(report.JobStats[i].SampleJobRunStats))
				assert.Equal(t, tc.expectedBaseJobRunLen[jobName], len(report.JobStats[i].BaseJobRunStats), "expected base job run counts %+v, got %+v", tc.expectedBaseJobRunLen[jobName], len(report.JobStats[i].BaseJobRunStats))
			}
			// assert.Equal(t, tc.expectedReport.ReportStatus, report.ReportStatus, "expected report %+v, got %+v", tc.expectedReport, report)
			// output, _ := json.MarshalIndent(report, "", "    ")
			// fmt.Printf("-----report \n%+v\n", string(output))
		})
	}
}

func Test_componentReportGenerator_normalizeProwJobName(t *testing.T) {
	tests := []struct {
		name          string
		sampleRelease string
		baseRelease   string
		jobName       string
		want          string
	}{
		{
			name:        "base release is removed",
			baseRelease: "4.16",
			jobName:     "periodic-ci-openshift-release-master-ci-4.16-e2e-azure-ovn-upgrade",
			want:        "periodic-ci-openshift-release-master-ci-X.X-e2e-azure-ovn-upgrade",
		},
		{
			name:          "sample release is removed",
			sampleRelease: "4.16",
			jobName:       "periodic-ci-openshift-release-master-ci-4.16-e2e-azure-ovn-upgrade",
			want:          "periodic-ci-openshift-release-master-ci-X.X-e2e-azure-ovn-upgrade",
		},
		{
			name:    "frequency is removed",
			jobName: "periodic-ci-openshift-release-master-ci-test-job-f27",
			want:    "periodic-ci-openshift-release-master-ci-test-job-fXX",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &componentReportGenerator{}
			if tt.baseRelease != "" {
				c.BaseRelease = crtype.RequestReleaseOptions{Release: tt.baseRelease}
			}
			if tt.sampleRelease != "" {
				c.SampleRelease = crtype.RequestReleaseOptions{Release: tt.sampleRelease}
			}

			assert.Equalf(t, tt.want, c.normalizeProwJobName(tt.jobName), "normalizeProwJobName(%v)", tt.jobName)
		})
	}
}

func Test_componentReportGenerator_assessComponentStatus(t *testing.T) {
	tests := []struct {
		name                   string
		sampleTotal            int
		sampleSuccess          int
		sampleFlake            int
		baseTotal              int
		baseSuccess            int
		baseFlake              int
		numberOfIgnoredSamples int

		requiredPassRateForNewTests int
		requiredPassRateForAllTests int
		minFail                     int

		expectedStatus   crtype.Status
		expectedFischers *float64
	}{
		{
			name:                   "triaged still regular regression",
			sampleTotal:            16,
			sampleSuccess:          13,
			sampleFlake:            0,
			baseTotal:              15,
			baseSuccess:            14,
			baseFlake:              1,
			numberOfIgnoredSamples: 2,
			expectedStatus:         -4,
			expectedFischers:       thrift.Float64Ptr(0.4827586206896551),
		},
		{
			name:                   "triaged regular regression",
			sampleTotal:            15,
			sampleSuccess:          13,
			sampleFlake:            0,
			baseTotal:              15,
			baseSuccess:            14,
			baseFlake:              1,
			numberOfIgnoredSamples: 2,
			expectedStatus:         -2,
			expectedFischers:       thrift.Float64Ptr(1),
		},
		{
			name:                   "regular regression",
			sampleTotal:            15,
			sampleSuccess:          13,
			sampleFlake:            0,
			baseTotal:              15,
			baseSuccess:            14,
			baseFlake:              1,
			numberOfIgnoredSamples: 0,
			expectedStatus:         -4,
			expectedFischers:       thrift.Float64Ptr(0.2413793103448262),
		},
		{
			name:                   "zero success",
			sampleTotal:            15,
			sampleSuccess:          0,
			sampleFlake:            0,
			baseTotal:              15,
			baseSuccess:            14,
			baseFlake:              1,
			numberOfIgnoredSamples: 0,
			expectedStatus:         -5,
			expectedFischers:       thrift.Float64Ptr(6.446725037893782e-09),
		},
		{
			name:                   "triaged, zero success",
			sampleTotal:            15,
			sampleSuccess:          0,
			sampleFlake:            0,
			baseTotal:              15,
			baseSuccess:            14,
			baseFlake:              1,
			numberOfIgnoredSamples: 15,
			expectedStatus:         -3,
			expectedFischers:       thrift.Float64Ptr(0),
		},

		{
			name:                   "triaged extreme, fixed",
			sampleTotal:            15,
			sampleSuccess:          5,
			sampleFlake:            0,
			baseTotal:              15,
			baseSuccess:            14,
			baseFlake:              1,
			numberOfIgnoredSamples: 10,
			expectedStatus:         -3,
			expectedFischers:       thrift.Float64Ptr(1),
		},

		{
			name:                   "triaged, still extreme",
			sampleTotal:            15,
			sampleSuccess:          5,
			sampleFlake:            0,
			baseTotal:              15,
			baseSuccess:            14,
			baseFlake:              1,
			numberOfIgnoredSamples: 9,
			expectedStatus:         -5,
			expectedFischers:       thrift.Float64Ptr(0.285714285714284),
		},
		{
			name:                   "triaged, still extreme",
			sampleTotal:            15,
			sampleSuccess:          5,
			sampleFlake:            0,
			baseTotal:              15,
			baseSuccess:            14,
			baseFlake:              1,
			numberOfIgnoredSamples: 9,
			expectedStatus:         -5,
			expectedFischers:       thrift.Float64Ptr(0.285714285714284),
		},
		{
			name:                        "new test no regression",
			sampleTotal:                 1000,
			sampleSuccess:               999,
			requiredPassRateForNewTests: 99,
			expectedStatus:              crtype.MissingBasis,
			expectedFischers:            nil,
		},
		{
			name:                        "new test extreme regression",
			sampleTotal:                 15,
			sampleSuccess:               13,
			requiredPassRateForNewTests: 99,
			expectedStatus:              crtype.ExtremeRegression,
			expectedFischers:            nil,
		},
		{
			name:                        "new test significant regression",
			sampleTotal:                 1000,
			sampleSuccess:               985,
			requiredPassRateForNewTests: 99,
			expectedStatus:              crtype.SignificantRegression,
		},
		{
			name:                        "new test significant regression with triaged runs",
			sampleTotal:                 1000,
			sampleSuccess:               985,
			numberOfIgnoredSamples:      12,
			requiredPassRateForNewTests: 99,
			expectedStatus:              crtype.MissingBasis,
		},
		{
			name:                        "new test all failures triaged",
			sampleTotal:                 1000,
			sampleSuccess:               988,
			numberOfIgnoredSamples:      12,
			requiredPassRateForNewTests: 99,
			expectedStatus:              crtype.MissingBasis,
		},
		{
			name:                        "pass rate mode significant regression",
			sampleTotal:                 100,
			sampleSuccess:               94,
			sampleFlake:                 0,
			baseTotal:                   100,
			baseSuccess:                 94,
			baseFlake:                   0,
			numberOfIgnoredSamples:      0,
			requiredPassRateForAllTests: 95,
			expectedStatus:              crtype.SignificantRegression,
		},
		{
			name:                        "pass rate mode extreme regression",
			sampleTotal:                 100,
			sampleSuccess:               89,
			sampleFlake:                 0,
			baseTotal:                   100,
			baseSuccess:                 89,
			baseFlake:                   0,
			numberOfIgnoredSamples:      0,
			requiredPassRateForAllTests: 95,
			expectedStatus:              crtype.ExtremeRegression,
		},
		{
			name:                        "pass rate mode no regression",
			sampleTotal:                 100,
			sampleSuccess:               97,
			sampleFlake:                 0,
			baseTotal:                   100,
			baseSuccess:                 97,
			baseFlake:                   0,
			numberOfIgnoredSamples:      0,
			requiredPassRateForAllTests: 95,
			expectedStatus:              crtype.NotSignificant,
		},
		{
			name:                        "pass rate mode significant regression ignores minimum failures",
			sampleTotal:                 20,
			sampleSuccess:               18,
			sampleFlake:                 0,
			baseTotal:                   20,
			baseSuccess:                 18,
			baseFlake:                   0,
			numberOfIgnoredSamples:      0,
			requiredPassRateForAllTests: 95,
			minFail:                     5,
			expectedStatus:              crtype.SignificantRegression,
		},
		{
			name:                        "pass rate mode insufficient runs to trigger",
			sampleTotal:                 6,
			sampleSuccess:               0,
			sampleFlake:                 0,
			numberOfIgnoredSamples:      0,
			requiredPassRateForAllTests: 95,
			expectedStatus:              crtype.NotSignificant,
		},
		{
			name:                        "pass rate mode barely sufficient runs to trigger",
			sampleTotal:                 7,
			sampleSuccess:               6,
			sampleFlake:                 0,
			numberOfIgnoredSamples:      0,
			requiredPassRateForAllTests: 95,
			expectedStatus:              crtype.ExtremeRegression,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &componentReportGenerator{}
			c.PassRateRequiredNewTests = tt.requiredPassRateForNewTests
			c.PassRateRequiredAllTests = tt.requiredPassRateForAllTests
			c.MinimumFailure = tt.minFail

			testStats := c.assessComponentStatus(0, tt.sampleTotal, tt.sampleSuccess, tt.sampleFlake, tt.baseTotal, tt.baseSuccess, tt.baseFlake, nil, nil, tt.numberOfIgnoredSamples)
			assert.Equalf(t, tt.expectedStatus, testStats.ReportStatus, "assessComponentStatus expected status not equal")
			if tt.expectedFischers != nil {
				assert.Equalf(t, *tt.expectedFischers, *testStats.FisherExact, "assessComponentStatus expected fischers value not equal")
			} else {
				assert.Nil(t, testStats.FisherExact)
			}

		})
	}
}

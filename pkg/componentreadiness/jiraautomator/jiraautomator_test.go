package jiraautomator

import (
	"fmt"
	"testing"

	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/stretchr/testify/assert"
)

func TestGetComponentRegressedTestsFromReport(t *testing.T) {
	columnAWSAMD64OVN := crtype.ColumnIdentification{
		Variants: map[string]string{
			"Platform":     "aws",
			"Architecture": "amd64",
			"Network":      "ovn",
		},
	}
	columnAzureAMD64OVN := crtype.ColumnIdentification{
		Variants: map[string]string{
			"Platform":     "aws",
			"Architecture": "amd64",
			"Network":      "ovn",
		},
	}
	columnMetalAMD64OVN := crtype.ColumnIdentification{
		Variants: map[string]string{
			"Platform":     "metal",
			"Architecture": "amd64",
			"Network":      "ovn",
		},
	}
	awsAMD64OVNTest := crtype.TestIdentification{
		TestID: "1",
		Variants: map[string]string{
			"Platform":     "aws",
			"Architecture": "amd64",
			"Network":      "ovn",
		},
	}
	testName1 := "Test 1"
	testName2 := "Test 2"

	tests := []struct {
		name           string
		report         crtype.ComponentReport
		expectedResult map[string][]crtype.ReportTestSummary
	}{
		{
			name: "component to regressed tests by component only",
			report: crtype.ComponentReport{
				Rows: []crtype.ReportRow{
					{
						RowIdentification: crtype.RowIdentification{
							Component: "component 1",
						},
						Columns: []crtype.ReportColumn{
							{
								ColumnIdentification: columnAWSAMD64OVN,
								Status:               crtype.ExtremeRegression,
								RegressedTests: []crtype.ReportTestSummary{
									{
										ReportTestIdentification: crtype.ReportTestIdentification{
											RowIdentification: crtype.RowIdentification{
												TestName: testName1,
											},
											ColumnIdentification: crtype.ColumnIdentification{
												Variants: awsAMD64OVNTest.Variants,
											},
										},
										ReportTestStats: crtype.ReportTestStats{
											ReportStatus: crtype.ExtremeRegression,
										},
									},
								},
							},
							{
								ColumnIdentification: columnAzureAMD64OVN,
								Status:               crtype.ExtremeRegression,
								RegressedTests: []crtype.ReportTestSummary{
									{
										ReportTestIdentification: crtype.ReportTestIdentification{
											RowIdentification: crtype.RowIdentification{
												TestName: testName1,
											},
											ColumnIdentification: crtype.ColumnIdentification{
												Variants: columnAzureAMD64OVN.Variants,
											},
										},
										ReportTestStats: crtype.ReportTestStats{
											ReportStatus: crtype.ExtremeRegression,
										},
									},
								},
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
								Status:               crtype.NotSignificant,
								RegressedTests:       []crtype.ReportTestSummary{},
							},
							{
								ColumnIdentification: columnAzureAMD64OVN,
								Status:               crtype.ExtremeRegression,
								RegressedTests: []crtype.ReportTestSummary{
									{
										ReportTestIdentification: crtype.ReportTestIdentification{
											RowIdentification: crtype.RowIdentification{
												TestName: testName2,
											},
											ColumnIdentification: crtype.ColumnIdentification{
												Variants: columnAzureAMD64OVN.Variants,
											},
										},
										ReportTestStats: crtype.ReportTestStats{
											ReportStatus: crtype.ExtremeRegression,
										},
									},
								},
							},
						},
					},
				},
			},
			expectedResult: map[string][]crtype.ReportTestSummary{
				"component 1": {
					{
						ReportTestIdentification: crtype.ReportTestIdentification{
							RowIdentification: crtype.RowIdentification{
								TestName: testName1,
							},
							ColumnIdentification: crtype.ColumnIdentification{
								Variants: awsAMD64OVNTest.Variants,
							},
						},
						ReportTestStats: crtype.ReportTestStats{
							ReportStatus: crtype.ExtremeRegression,
						},
					},
					{
						ReportTestIdentification: crtype.ReportTestIdentification{
							RowIdentification: crtype.RowIdentification{
								TestName: testName1,
							},
							ColumnIdentification: crtype.ColumnIdentification{
								Variants: columnAzureAMD64OVN.Variants,
							},
						},
						ReportTestStats: crtype.ReportTestStats{
							ReportStatus: crtype.ExtremeRegression,
						},
					},
				},
				"component 2": {
					{
						ReportTestIdentification: crtype.ReportTestIdentification{
							RowIdentification: crtype.RowIdentification{
								TestName: testName2,
							},
							ColumnIdentification: crtype.ColumnIdentification{
								Variants: columnAzureAMD64OVN.Variants,
							},
						},
						ReportTestStats: crtype.ReportTestStats{
							ReportStatus: crtype.ExtremeRegression,
						},
					},
				},
			},
		},
		{
			name: "component to regressed tests by component and column grouping",
			report: crtype.ComponentReport{
				Rows: []crtype.ReportRow{
					{
						RowIdentification: crtype.RowIdentification{
							Component: "component 1",
						},
						Columns: []crtype.ReportColumn{
							{
								ColumnIdentification: columnAWSAMD64OVN,
								Status:               crtype.ExtremeRegression,
								RegressedTests: []crtype.ReportTestSummary{
									{
										ReportTestIdentification: crtype.ReportTestIdentification{
											RowIdentification: crtype.RowIdentification{
												TestName: testName1,
											},
											ColumnIdentification: crtype.ColumnIdentification{
												Variants: awsAMD64OVNTest.Variants,
											},
										},
										ReportTestStats: crtype.ReportTestStats{
											ReportStatus: crtype.ExtremeRegression,
										},
									},
								},
							},
							{
								ColumnIdentification: columnMetalAMD64OVN,
								Status:               crtype.ExtremeRegression,
								RegressedTests: []crtype.ReportTestSummary{
									{
										ReportTestIdentification: crtype.ReportTestIdentification{
											RowIdentification: crtype.RowIdentification{
												TestName: testName1,
											},
											ColumnIdentification: crtype.ColumnIdentification{
												Variants: columnMetalAMD64OVN.Variants,
											},
										},
										ReportTestStats: crtype.ReportTestStats{
											ReportStatus: crtype.ExtremeRegression,
										},
									},
								},
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
								Status:               crtype.NotSignificant,
								RegressedTests:       []crtype.ReportTestSummary{},
							},
							{
								ColumnIdentification: columnMetalAMD64OVN,
								Status:               crtype.ExtremeRegression,
								RegressedTests: []crtype.ReportTestSummary{
									{
										ReportTestIdentification: crtype.ReportTestIdentification{
											RowIdentification: crtype.RowIdentification{
												TestName: testName2,
											},
											ColumnIdentification: crtype.ColumnIdentification{
												Variants: columnMetalAMD64OVN.Variants,
											},
										},
										ReportTestStats: crtype.ReportTestStats{
											ReportStatus: crtype.ExtremeRegression,
										},
									},
								},
							},
						},
					},
				},
			},
			expectedResult: map[string][]crtype.ReportTestSummary{
				"component 1": {
					{
						ReportTestIdentification: crtype.ReportTestIdentification{
							RowIdentification: crtype.RowIdentification{
								TestName: testName1,
							},
							ColumnIdentification: crtype.ColumnIdentification{
								Variants: awsAMD64OVNTest.Variants,
							},
						},
						ReportTestStats: crtype.ReportTestStats{
							ReportStatus: crtype.ExtremeRegression,
						},
					},
				},
				"component 2": nil,
				"Bare Metal Hardware Provisioning": {
					{
						ReportTestIdentification: crtype.ReportTestIdentification{
							RowIdentification: crtype.RowIdentification{
								TestName: testName1,
							},
							ColumnIdentification: crtype.ColumnIdentification{
								Variants: columnMetalAMD64OVN.Variants,
							},
						},
						ReportTestStats: crtype.ReportTestStats{
							ReportStatus: crtype.ExtremeRegression,
						},
					},
					{
						ReportTestIdentification: crtype.ReportTestIdentification{
							RowIdentification: crtype.RowIdentification{
								TestName: testName2,
							},
							ColumnIdentification: crtype.ColumnIdentification{
								Variants: columnMetalAMD64OVN.Variants,
							},
						},
						ReportTestStats: crtype.ReportTestStats{
							ReportStatus: crtype.ExtremeRegression,
						},
					},
				},
			},
		},
	}
	j := JiraAutomator{
		columnThresholds: map[Variant]int{
			{
				Name:  "Platform",
				Value: "metal",
			}: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := j.groupRegressedTestsByComponents(tc.report)
			assert.NoError(t, err, "error getting component regressed tests from report")
			fmt.Printf("---- result %+v\n", result)
			assert.Equal(t, tc.expectedResult, result, "expected report %+v, got %+v", tc.expectedResult, result)
		})
	}
}

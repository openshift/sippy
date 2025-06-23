package jiraautomator

import (
	"fmt"
	"testing"

	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/testdetails"
	jiratype "github.com/openshift/sippy/pkg/apis/jira/v1"
	"github.com/stretchr/testify/assert"
)

func TestGetComponentRegressedTestsFromReport(t *testing.T) {
	columnAWSAMD64OVN := crtest.ColumnIdentification{
		Variants: map[string]string{
			"Platform":     "aws",
			"Architecture": "amd64",
			"Network":      "ovn",
		},
	}
	columnAzureAMD64OVN := crtest.ColumnIdentification{
		Variants: map[string]string{
			"Platform":     "aws",
			"Architecture": "amd64",
			"Network":      "ovn",
		},
	}
	columnMetalAMD64OVN := crtest.ColumnIdentification{
		Variants: map[string]string{
			"Platform":     "metal",
			"Architecture": "amd64",
			"Network":      "ovn",
		},
	}
	awsAMD64OVNTest := crtest.KeyWithVariants{
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
		expectedResult map[JiraComponent][]crtype.ReportTestSummary
	}{
		{
			name: "component to regressed tests by component only",
			report: crtype.ComponentReport{
				Rows: []crtype.ReportRow{
					{
						RowIdentification: crtest.RowIdentification{
							Component: "component 1",
						},
						Columns: []crtype.ReportColumn{
							{
								ColumnIdentification: columnAWSAMD64OVN,
								Status:               crtest.ExtremeRegression,
								RegressedTests: []crtype.ReportTestSummary{
									{
										Identification: crtest.Identification{
											RowIdentification: crtest.RowIdentification{
												TestName: testName1,
											},
											ColumnIdentification: crtest.ColumnIdentification{
												Variants: awsAMD64OVNTest.Variants,
											},
										},
										TestComparison: testdetails.TestComparison{
											ReportStatus: crtest.ExtremeRegression,
										},
									},
								},
							},
							{
								ColumnIdentification: columnAzureAMD64OVN,
								Status:               crtest.ExtremeRegression,
								RegressedTests: []crtype.ReportTestSummary{
									{
										Identification: crtest.Identification{
											RowIdentification: crtest.RowIdentification{
												TestName: testName1,
											},
											ColumnIdentification: crtest.ColumnIdentification{
												Variants: columnAzureAMD64OVN.Variants,
											},
										},
										TestComparison: testdetails.TestComparison{
											ReportStatus: crtest.ExtremeRegression,
										},
									},
								},
							},
						},
					},
					{
						RowIdentification: crtest.RowIdentification{
							Component: "component 2",
						},
						Columns: []crtype.ReportColumn{
							{
								ColumnIdentification: columnAWSAMD64OVN,
								Status:               crtest.NotSignificant,
								RegressedTests:       []crtype.ReportTestSummary{},
							},
							{
								ColumnIdentification: columnAzureAMD64OVN,
								Status:               crtest.ExtremeRegression,
								RegressedTests: []crtype.ReportTestSummary{
									{
										Identification: crtest.Identification{
											RowIdentification: crtest.RowIdentification{
												TestName: testName2,
											},
											ColumnIdentification: crtest.ColumnIdentification{
												Variants: columnAzureAMD64OVN.Variants,
											},
										},
										TestComparison: testdetails.TestComparison{
											ReportStatus: crtest.ExtremeRegression,
										},
									},
								},
							},
						},
					},
				},
			},
			expectedResult: map[JiraComponent][]crtype.ReportTestSummary{
				{Project: "OCPBUGS", Component: "component 1"}: {
					{
						Identification: crtest.Identification{
							RowIdentification: crtest.RowIdentification{
								TestName: testName1,
							},
							ColumnIdentification: crtest.ColumnIdentification{
								Variants: awsAMD64OVNTest.Variants,
							},
						},
						TestComparison: testdetails.TestComparison{
							ReportStatus: crtest.ExtremeRegression,
						},
					},
					{
						Identification: crtest.Identification{
							RowIdentification: crtest.RowIdentification{
								TestName: testName1,
							},
							ColumnIdentification: crtest.ColumnIdentification{
								Variants: columnAzureAMD64OVN.Variants,
							},
						},
						TestComparison: testdetails.TestComparison{
							ReportStatus: crtest.ExtremeRegression,
						},
					},
				},
				{Project: "OCPBUGS", Component: "component 2"}: {
					{
						Identification: crtest.Identification{
							RowIdentification: crtest.RowIdentification{
								TestName: testName2,
							},
							ColumnIdentification: crtest.ColumnIdentification{
								Variants: columnAzureAMD64OVN.Variants,
							},
						},
						TestComparison: testdetails.TestComparison{
							ReportStatus: crtest.ExtremeRegression,
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
						RowIdentification: crtest.RowIdentification{
							Component: "component 1",
						},
						Columns: []crtype.ReportColumn{
							{
								ColumnIdentification: columnAWSAMD64OVN,
								Status:               crtest.ExtremeRegression,
								RegressedTests: []crtype.ReportTestSummary{
									{
										Identification: crtest.Identification{
											RowIdentification: crtest.RowIdentification{
												TestName: testName1,
											},
											ColumnIdentification: crtest.ColumnIdentification{
												Variants: awsAMD64OVNTest.Variants,
											},
										},
										TestComparison: testdetails.TestComparison{
											ReportStatus: crtest.ExtremeRegression,
										},
									},
								},
							},
							{
								ColumnIdentification: columnMetalAMD64OVN,
								Status:               crtest.ExtremeRegression,
								RegressedTests: []crtype.ReportTestSummary{
									{
										Identification: crtest.Identification{
											RowIdentification: crtest.RowIdentification{
												TestName: testName1,
											},
											ColumnIdentification: crtest.ColumnIdentification{
												Variants: columnMetalAMD64OVN.Variants,
											},
										},
										TestComparison: testdetails.TestComparison{
											ReportStatus: crtest.ExtremeRegression,
										},
									},
								},
							},
						},
					},
					{
						RowIdentification: crtest.RowIdentification{
							Component: "component 2",
						},
						Columns: []crtype.ReportColumn{
							{
								ColumnIdentification: columnAWSAMD64OVN,
								Status:               crtest.NotSignificant,
								RegressedTests:       []crtype.ReportTestSummary{},
							},
							{
								ColumnIdentification: columnMetalAMD64OVN,
								Status:               crtest.ExtremeRegression,
								RegressedTests: []crtype.ReportTestSummary{
									{
										Identification: crtest.Identification{
											RowIdentification: crtest.RowIdentification{
												TestName: testName2,
											},
											ColumnIdentification: crtest.ColumnIdentification{
												Variants: columnMetalAMD64OVN.Variants,
											},
										},
										TestComparison: testdetails.TestComparison{
											ReportStatus: crtest.ExtremeRegression,
										},
									},
								},
							},
						},
					},
				},
			},
			expectedResult: map[JiraComponent][]crtype.ReportTestSummary{
				{Project: "OCPBUGS", Component: "component 1"}: {
					{
						Identification: crtest.Identification{
							RowIdentification: crtest.RowIdentification{
								TestName: testName1,
							},
							ColumnIdentification: crtest.ColumnIdentification{
								Variants: awsAMD64OVNTest.Variants,
							},
						},
						TestComparison: testdetails.TestComparison{
							ReportStatus: crtest.ExtremeRegression,
						},
					},
				},
				{Project: "OCPBUGS", Component: "Bare Metal Hardware Provisioning"}: {
					{
						Identification: crtest.Identification{
							RowIdentification: crtest.RowIdentification{
								TestName: testName1,
							},
							ColumnIdentification: crtest.ColumnIdentification{
								Variants: columnMetalAMD64OVN.Variants,
							},
						},
						TestComparison: testdetails.TestComparison{
							ReportStatus: crtest.ExtremeRegression,
						},
					},
					{
						Identification: crtest.Identification{
							RowIdentification: crtest.RowIdentification{
								TestName: testName2,
							},
							ColumnIdentification: crtest.ColumnIdentification{
								Variants: columnMetalAMD64OVN.Variants,
							},
						},
						TestComparison: testdetails.TestComparison{
							ReportStatus: crtest.ExtremeRegression,
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
		variantToJiraComponents: map[Variant]JiraComponent{
			{
				Name:  "Platform",
				Value: "metal",
			}: {
				Project:   jiratype.ProjectKeyOCPBugs,
				Component: "Bare Metal Hardware Provisioning",
			},
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

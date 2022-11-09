package sippyserver

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	jira "github.com/andygrunwald/go-jira"
	"github.com/openshift/sippy/pkg/db/models"
)

func findIssuesForVariants() (map[string][]jira.Issue, error) {
	result := map[string][]jira.Issue{
		"sippy-link=[variants=azure,ovn]": []jira.Issue{
			{
				ID: "1",
				Fields: &jira.IssueFields{
					AffectsVersions: []*jira.AffectsVersion{
						{Name: "4.12.z"},
					},
				},
			},
		},
		"sippy-link=[variants=ovn]": []jira.Issue{
			{
				ID: "2",
				Fields: &jira.IssueFields{
					AffectsVersions: []*jira.AffectsVersion{
						{Name: "4.11"},
						{Name: "4.12"},
					},
				},
			},
		},
	}
	return result, nil
}

func errorFindIssuesForVariants() (map[string][]jira.Issue, error) {
	return map[string][]jira.Issue{}, fmt.Errorf("error finding variant issues")
}

func TestAppendJobIssuesFromVariants(t *testing.T) {
	tests := []struct {
		name           string
		jobCache       map[string]*models.ProwJob
		jobIssues      map[string][]jira.Issue
		expectedResult map[string][]jira.Issue
		expectErr      bool
		findIssuesFunc func() (map[string][]jira.Issue, error)
	}{
		{
			name: "find issues by variants with no errors",
			jobCache: map[string]*models.ProwJob{
				"1": &models.ProwJob{Name: "periodic-ci-openshift-release-master-nightly-4.11-e2e-azure-csi",
					Release: "4.11", Variants: []string{"azure", "amd64", "ovn", "ha"},
				},
				"2": &models.ProwJob{Name: "periodic-ci-openshift-release-master-nightly-4.12-e2e-azure-csi",
					Release: "4.12", Variants: []string{"azure", "amd64", "ovn", "ha"},
				},
				"3": &models.ProwJob{Name: "periodic-ci-openshift-release-master-nightly-4.12-e2e-vsphere-sdn-upi-serial",
					Release: "4.12", Variants: []string{"vsphere-ipi", "amd64", "sdn", "ha", "serial"},
				},
			},
			jobIssues:      map[string][]jira.Issue{},
			expectErr:      false,
			findIssuesFunc: findIssuesForVariants,
			expectedResult: map[string][]jira.Issue{
				"job=periodic-ci-openshift-release-master-nightly-4.11-e2e-azure-csi=all": []jira.Issue{
					{
						ID: "2",
						Fields: &jira.IssueFields{
							AffectsVersions: []*jira.AffectsVersion{
								{Name: "4.11"},
								{Name: "4.12"},
							},
						},
					},
				},
				"job=periodic-ci-openshift-release-master-nightly-4.12-e2e-azure-csi=all": []jira.Issue{
					{
						ID: "1",
						Fields: &jira.IssueFields{
							AffectsVersions: []*jira.AffectsVersion{
								{Name: "4.12.z"},
							},
						},
					},
					{
						ID: "2",
						Fields: &jira.IssueFields{
							AffectsVersions: []*jira.AffectsVersion{
								{Name: "4.11"},
								{Name: "4.12"},
							},
						},
					},
				},
			},
		},
		{
			name: "append issues by variants with no errors",
			jobCache: map[string]*models.ProwJob{
				"1": &models.ProwJob{Name: "periodic-ci-openshift-release-master-nightly-4.11-e2e-azure-csi",
					Release: "4.11", Variants: []string{"azure", "amd64", "ovn", "ha"},
				},
				"2": &models.ProwJob{Name: "periodic-ci-openshift-release-master-nightly-4.12-e2e-azure-csi",
					Release: "4.12", Variants: []string{"azure", "amd64", "ovn", "ha"},
				},
				"3": &models.ProwJob{Name: "periodic-ci-openshift-release-master-nightly-4.12-e2e-vsphere-sdn-upi-serial",
					Release: "4.12", Variants: []string{"vsphere-ipi", "amd64", "sdn", "ha", "serial"},
				},
			},
			jobIssues: map[string][]jira.Issue{
				"job=periodic-ci-openshift-release-master-nightly-4.12-e2e-azure-csi=all": []jira.Issue{
					{
						ID: "3",
						Fields: &jira.IssueFields{
							AffectsVersions: []*jira.AffectsVersion{
								{Name: "4.12"},
							},
						},
					},
				},
			},
			expectErr:      false,
			findIssuesFunc: findIssuesForVariants,
			expectedResult: map[string][]jira.Issue{
				"job=periodic-ci-openshift-release-master-nightly-4.11-e2e-azure-csi=all": []jira.Issue{
					{
						ID: "2",
						Fields: &jira.IssueFields{
							AffectsVersions: []*jira.AffectsVersion{
								{Name: "4.11"},
								{Name: "4.12"},
							},
						},
					},
				},
				"job=periodic-ci-openshift-release-master-nightly-4.12-e2e-azure-csi=all": []jira.Issue{
					{
						ID: "1",
						Fields: &jira.IssueFields{
							AffectsVersions: []*jira.AffectsVersion{
								{Name: "4.12.z"},
							},
						},
					},
					{
						ID: "2",
						Fields: &jira.IssueFields{
							AffectsVersions: []*jira.AffectsVersion{
								{Name: "4.11"},
								{Name: "4.12"},
							},
						},
					},
					{
						ID: "3",
						Fields: &jira.IssueFields{
							AffectsVersions: []*jira.AffectsVersion{
								{Name: "4.12"},
							},
						},
					},
				},
			},
		},
		{
			name: "find issues by variants with errors",
			jobCache: map[string]*models.ProwJob{
				"1": &models.ProwJob{Name: "periodic-ci-openshift-release-master-nightly-4.11-e2e-azure-csi",
					Release: "4.11", Variants: []string{"azure", "amd64", "ovn", "ha"},
				},
				"2": &models.ProwJob{Name: "periodic-ci-openshift-release-master-nightly-4.12-e2e-azure-csi",
					Release: "4.12", Variants: []string{"azure", "amd64", "ovn", "ha"},
				},
				"3": &models.ProwJob{Name: "periodic-ci-openshift-release-master-nightly-4.12-e2e-vsphere-sdn-upi-serial",
					Release: "4.12", Variants: []string{"vsphere-ipi", "amd64", "sdn", "ha", "serial"},
				},
			},
			jobIssues:      map[string][]jira.Issue{},
			expectErr:      true,
			findIssuesFunc: errorFindIssuesForVariants,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			origFindIssuesFunc := FindIssuesForVariants
			FindIssuesForVariants = tc.findIssuesFunc
			err := appendJobIssuesFromVariants(tc.jobCache, tc.jobIssues)
			if tc.expectErr && err == nil {
				t.Errorf("Expect test error but get nil")
			} else if !tc.expectErr && err != nil {
				t.Errorf("Expect no test error but get %v", err)
			} else if !tc.expectErr {
				for _, jobIssue := range tc.jobIssues {
					sort.Slice(jobIssue, func(i, j int) bool {
						return jobIssue[i].ID < jobIssue[j].ID
					})
				}
				eq := reflect.DeepEqual(tc.jobIssues, tc.expectedResult)
				if !eq {
					t.Errorf("Final job issues:\n %+v do not match expected:\n %+v", tc.jobIssues, tc.expectedResult)
				}
			}
			FindIssuesForVariants = origFindIssuesFunc
		})
	}
}

package resolvedissues

import (
	apitype "github.com/openshift/sippy/pkg/apis/api"
)

func init() {
	mustAddResolvedIssue(release415, ResolvedIssue{
		TestID:   "cluster install:0cb1bb27e418491b1ffdacab58c5c8c0",
		TestName: "install should succeed: overall",
		Variant: apitype.ComponentReportColumnIdentification{
			Network:  "ovn",
			Upgrade:  "upgrade-micro",
			Arch:     "amd64",
			Platform: "azure",
		},
		Issue: Issue{
			IssueType: "Infrastructure",
			Infrastructure: &InfrastructureIssue{
				Description:    "azure cloud problems during install",
				JiraURL:        "",
				ResolutionDate: mustTime("2024-01-21T06:09:09Z"), // date is after all those jobruns were over
			},
			PayloadBug: nil,
		},
		ImpactedJobRuns: []JobRun{
			{
				// Error: retrieving Network Interface "ci-op-bfvvfycn-aa265-gbx9d-bootstrap-nic"
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1748875344510717952",
				StartTime: mustTime("2024-01-21T01:09:09Z"),
			},
			{
				// Error: creating/updating Virtual Network Link (Subscription: "72e3a972-58b0-4afc-bd4f-da89b39ccebd"
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1748875347295735808",
				StartTime: mustTime("2024-01-21T01:09:09Z"),
			},
			{
				// failed to get vm: compute.VirtualMachinesClient#Get
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1748875348252037120",
				StartTime: mustTime("2024-01-21T01:09:10Z"),
			},
			{
				// failed to get vm: compute.VirtualMachinesClient
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1748875344930148352",
				StartTime: mustTime("2024-01-21T01:09:09Z"),
			},
			{
				// Error: deleting OS Disk "ci-op-g101k5sx-aa265-chh85-bootstrap_OSDisk"
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1748875345819340800",
				StartTime: mustTime("2024-01-21T01:09:09Z"),
			},
			{
				// unable to list provider registration status
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1748875343294369792",
				StartTime: mustTime("2024-01-21T01:09:08Z"),
			},
		},
	})
	mustAddResolvedIssue(release415, ResolvedIssue{
		TestID:   "Operator results:4b5f6af893ad5577904fbaec3254506d",
		TestName: "operator conditions network",
		Variant: apitype.ComponentReportColumnIdentification{
			Network:  "ovn",
			Upgrade:  "upgrade-micro",
			Arch:     "amd64",
			Platform: "azure",
		},
		Issue: Issue{
			IssueType: "Infrastructure",
			Infrastructure: &InfrastructureIssue{
				Description:    "azure cloud problems during install",
				JiraURL:        "https://issues.redhat.com/browse/OCPBUGS-27495",
				ResolutionDate: mustTime("2024-01-21T06:09:09Z"), // date is after all those jobruns were over
			},
			PayloadBug: nil,
		},
		ImpactedJobRuns: []JobRun{
			{
				// failed to get vm: compute.VirtualMachinesClient#Get
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1748875348252037120",
				StartTime: mustTime("2024-01-21T01:09:10Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1748875341679562752",
				StartTime: mustTime("2024-01-21T01:09:12Z"),
			},
			{
				// failed to get vm: compute.VirtualMachinesClient
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1748875344930148352",
				StartTime: mustTime("2024-01-21T01:09:09Z"),
			},
			{
				// failed to get vm: compute.VirtualMachinesClient
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-e2e-azure-upgrade-cnv/1748875321513349120",
				StartTime: mustTime("2024-01-21T01:09:08Z"),
			},
		},
	})
	mustAddResolvedIssue(release415, ResolvedIssue{
		TestID:   "openshift-tests-upgrade:567152bb097fa9ce13dd2fb6885e094a",
		TestName: "[sig-arch] events should not repeat pathologically for ns/openshift-monitoring",
		Variant: apitype.ComponentReportColumnIdentification{
			Network:  "ovn",
			Upgrade:  "upgrade-minor",
			Arch:     "amd64",
			Platform: "metal-ipi",
		},
		Issue: Issue{
			IssueType: "PayloadBug",
			PayloadBug: &PayloadIssue{
				PullRequestURL: "https://github.com/openshift/origin/pull/28549",
				ResolutionDate: mustTime("2024-01-24T23:54:33Z"), // date is after all those jobruns were over
			},
		},
		ImpactedJobRuns: []JobRun{
			{
				// RecreatingTerminatedPod and SuccessfulDelete
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-upgrade-from-stable-4.14-e2e-metal-ipi-upgrade-ovn-ipv6/1750230625601720320",
				StartTime: mustTime("2024-01-24T18:54:33Z"),
			},
			{
				// RecreatingTerminatedPod
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-upgrade-from-stable-4.14-e2e-metal-ipi-upgrade-ovn-ipv6/1749463575023325184",
				StartTime: mustTime("2024-01-22T16:06:32Z"),
			},
			{
				// RecreatingTerminatedPod
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-upgrade-from-stable-4.14-e2e-metal-ipi-upgrade-ovn-ipv6/1748875328249401344",
				StartTime: mustTime("2024-01-21T01:09:05Z"),
			},
		},
	})
}

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

	mustAddResolvedIssue(release415, ResolvedIssue{
		TestID:   "cluster install:0cb1bb27e418491b1ffdacab58c5c8c0",
		TestName: "install should succeed: overall",
		Variant: apitype.ComponentReportColumnIdentification{
			Network:  "ovn",
			Upgrade:  "no-upgrade",
			Arch:     "amd64",
			Platform: "gcp",
		},
		Issue: Issue{
			IssueType: "PayloadBug",
			PayloadBug: &PayloadIssue{
				PullRequestURL: "https://github.com/openshift/release/pull/48714",
				ResolutionDate: mustTime("2024-02-13T11:58:33Z"), // date is after all those jobruns were over
			},
		},
		ImpactedJobRuns: []JobRun{
			{
				// glibc
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-operator-framework-olm-release-4.15-periodics-e2e-gcp-olm/1757192985587486720",
				StartTime: mustTime("2024-02-13T00:00:24Z"),
			},
			{
				// glibc
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-operator-framework-olm-release-4.15-periodics-e2e-gcp-olm/1756468298477735936",
				StartTime: mustTime("2024-02-11T00:00:46Z"),
			},
			{
				// glibc
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-operator-framework-olm-release-4.15-periodics-e2e-gcp-olm/1756105880006299648",
				StartTime: mustTime("2024-02-10T00:00:40Z"),
			},
			{
				// glibc
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-operator-framework-olm-release-4.15-periodics-e2e-gcp-olm/1755743516648017920",
				StartTime: mustTime("2024-02-09T00:00:45Z"),
			},
		},
	})
	mustAddResolvedIssue(release415, ResolvedIssue{
		TestID:   "cluster install:2bc0fe9de9a98831c20e569a21d7ded9",
		TestName: "install should succeed: cluster creation",
		Variant: apitype.ComponentReportColumnIdentification{
			Network:  "ovn",
			Upgrade:  "no-upgrade",
			Arch:     "amd64",
			Platform: "gcp",
		},
		Issue: Issue{
			IssueType: "PayloadBug",
			PayloadBug: &PayloadIssue{
				PullRequestURL: "https://github.com/openshift/release/pull/48714",
				ResolutionDate: mustTime("2024-02-13T11:58:33Z"), // date is after all those jobruns were over
			},
		},
		ImpactedJobRuns: []JobRun{
			{
				// glibc
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-operator-framework-olm-release-4.15-periodics-e2e-gcp-olm/1757192985587486720",
				StartTime: mustTime("2024-02-13T00:00:24Z"),
			},
			{
				// glibc
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-operator-framework-olm-release-4.15-periodics-e2e-gcp-olm/1756468298477735936",
				StartTime: mustTime("2024-02-11T00:00:46Z"),
			},
			{
				// glibc
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-operator-framework-olm-release-4.15-periodics-e2e-gcp-olm/1756105880006299648",
				StartTime: mustTime("2024-02-10T00:00:40Z"),
			},
			{
				// glibc
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-operator-framework-olm-release-4.15-periodics-e2e-gcp-olm/1755743516648017920",
				StartTime: mustTime("2024-02-09T00:00:45Z"),
			},
		},
	})
	mustAddResolvedIssue(release415, ResolvedIssue{
		TestID:   "Operator results:a2bfee761baf19bc7be479d649c54603",
		TestName: "operator conditions operator-lifecycle-manager-catalog",
		Variant: apitype.ComponentReportColumnIdentification{
			Network:  "ovn",
			Upgrade:  "no-upgrade",
			Arch:     "amd64",
			Platform: "gcp",
		},
		Issue: Issue{
			IssueType: "PayloadBug",
			PayloadBug: &PayloadIssue{
				PullRequestURL: "https://github.com/openshift/release/pull/48714",
				ResolutionDate: mustTime("2024-02-13T11:58:33Z"), // date is after all those jobruns were over
			},
		},
		ImpactedJobRuns: []JobRun{
			{
				// glibc
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-operator-framework-olm-release-4.15-periodics-e2e-gcp-olm/1757192985587486720",
				StartTime: mustTime("2024-02-13T00:00:24Z"),
			},
			{
				// glibc
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-operator-framework-olm-release-4.15-periodics-e2e-gcp-olm/1756468298477735936",
				StartTime: mustTime("2024-02-11T00:00:46Z"),
			},
			{
				// glibc
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-operator-framework-olm-release-4.15-periodics-e2e-gcp-olm/1756105880006299648",
				StartTime: mustTime("2024-02-10T00:00:40Z"),
			},
			{
				// glibc
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-operator-framework-olm-release-4.15-periodics-e2e-gcp-olm/1755743516648017920",
				StartTime: mustTime("2024-02-09T00:00:45Z"),
			},
		},
	})
	mustAddResolvedIssue(release415, ResolvedIssue{
		TestID:   "Operator results:8ff97a6ad27e7d31f1898878dfb086cf",
		TestName: "operator conditions operator-lifecycle-manager",
		Variant: apitype.ComponentReportColumnIdentification{
			Network:  "ovn",
			Upgrade:  "no-upgrade",
			Arch:     "amd64",
			Platform: "gcp",
		},
		Issue: Issue{
			IssueType: "PayloadBug",
			PayloadBug: &PayloadIssue{
				PullRequestURL: "https://github.com/openshift/release/pull/48714",
				ResolutionDate: mustTime("2024-02-13T11:58:33Z"), // date is after all those jobruns were over
			},
		},
		ImpactedJobRuns: []JobRun{
			{
				// glibc
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-operator-framework-olm-release-4.15-periodics-e2e-gcp-olm/1757192985587486720",
				StartTime: mustTime("2024-02-13T00:00:24Z"),
			},
			{
				// glibc
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-operator-framework-olm-release-4.15-periodics-e2e-gcp-olm/1756468298477735936",
				StartTime: mustTime("2024-02-11T00:00:46Z"),
			},
			{
				// glibc
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-operator-framework-olm-release-4.15-periodics-e2e-gcp-olm/1756105880006299648",
				StartTime: mustTime("2024-02-10T00:00:40Z"),
			},
			{
				// glibc
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-operator-framework-olm-release-4.15-periodics-e2e-gcp-olm/1755743516648017920",
				StartTime: mustTime("2024-02-09T00:00:45Z"),
			},
		},
	})
	mustAddResolvedIssue(release415, ResolvedIssue{
		TestID:   "Operator results:55a75a8aa11231d0ca36a4d65644e1dd",
		TestName: "operator conditions operator-lifecycle-manager-packageserver",
		Variant: apitype.ComponentReportColumnIdentification{
			Network:  "ovn",
			Upgrade:  "no-upgrade",
			Arch:     "amd64",
			Platform: "gcp",
		},
		Issue: Issue{
			IssueType: "PayloadBug",
			PayloadBug: &PayloadIssue{
				PullRequestURL: "https://github.com/openshift/release/pull/48714",
				ResolutionDate: mustTime("2024-02-13T11:58:33Z"), // date is after all those jobruns were over
			},
		},
		ImpactedJobRuns: []JobRun{
			{
				// glibc
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-operator-framework-olm-release-4.15-periodics-e2e-gcp-olm/1757192985587486720",
				StartTime: mustTime("2024-02-13T00:00:24Z"),
			},
			{
				// glibc
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-operator-framework-olm-release-4.15-periodics-e2e-gcp-olm/1756468298477735936",
				StartTime: mustTime("2024-02-11T00:00:46Z"),
			},
			{
				// glibc
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-operator-framework-olm-release-4.15-periodics-e2e-gcp-olm/1756105880006299648",
				StartTime: mustTime("2024-02-10T00:00:40Z"),
			},
			{
				// glibc
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-operator-framework-olm-release-4.15-periodics-e2e-gcp-olm/1755743516648017920",
				StartTime: mustTime("2024-02-09T00:00:45Z"),
			},
		},
	})

	mustAddResolvedIssue(release415, ResolvedIssue{
		TestID:   "openshift-tests:c1f54790201ec8f4241eca902f854b79",
		TestName: "[sig-instrumentation] Prometheus [apigroup:image.openshift.io] when installed on the cluster shouldn't report any alerts in firing state apart from Watchdog and AlertmanagerReceiversNotConfigured [Early][apigroup:config.openshift.io] [Skipped:Disconnected] [Suite:openshift/conformance/parallel]",
		Variant: apitype.ComponentReportColumnIdentification{
			Network:  "ovn",
			Upgrade:  "upgrade-micro",
			Arch:     "amd64",
			Platform: "aws",
			Variant:  "standard",
		},
		Issue: Issue{
			IssueType: "Infrastructure",
			Infrastructure: &InfrastructureIssue{
				Description:    "Loki outage caused ci logging pods to never go ready and eventually a DaemonSetRolloutStuck alert to fire",
				JiraURL:        "https://issues.redhat.com/browse/TRT-1537",
				ResolutionDate: mustTime("2024-02-28T13:00:00Z"),
			},
			PayloadBug: nil,
		},
		ImpactedJobRuns: []JobRun{
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-aws-ovn-upgrade/1762193523458707456",
				StartTime: mustTime("2024-02-26T19:10:48Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-aws-ovn-upgrade/1762232155330580480",
				StartTime: mustTime("2024-02-26T21:44:20Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-aws-ovn-upgrade/1762273129729626112",
				StartTime: mustTime("2024-02-27T00:27:10Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-aws-ovn-upgrade/1762311461440327680",
				StartTime: mustTime("2024-02-27T02:59:29Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-aws-ovn-upgrade/1762653931088908288",
				StartTime: mustTime("2024-02-28T01:40:20Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-aws-ovn-upgrade/1762686134980186112",
				StartTime: mustTime("2024-02-28T03:48:17Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-aws-ovn-upgrade/1762746439181537280",
				StartTime: mustTime("2024-02-28T07:47:55Z"),
			},
		},
	})

	mustAddResolvedIssue(release415, ResolvedIssue{
		TestID:   "openshift-tests:c1f54790201ec8f4241eca902f854b79",
		TestName: "[sig-instrumentation] Prometheus [apigroup:image.openshift.io] when installed on the cluster shouldn't report any alerts in firing state apart from Watchdog and AlertmanagerReceiversNotConfigured [Early][apigroup:config.openshift.io] [Skipped:Disconnected] [Suite:openshift/conformance/parallel]",
		Variant: apitype.ComponentReportColumnIdentification{
			Network:  "ovn",
			Upgrade:  "upgrade-minor",
			Arch:     "amd64",
			Platform: "aws",
			Variant:  "standard",
		},
		Issue: Issue{
			IssueType: "Infrastructure",
			Infrastructure: &InfrastructureIssue{
				Description:    "Loki outage caused ci logging pods to never go ready and eventually a DaemonSetRolloutStuck alert to fire",
				JiraURL:        "https://issues.redhat.com/browse/TRT-1537",
				ResolutionDate: mustTime("2024-02-28T13:00:00Z"),
			},
			PayloadBug: nil,
		},
		ImpactedJobRuns: []JobRun{
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762167347059101696",
				StartTime: mustTime("2024-02-26T17:26:49Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762167347243651072",
				StartTime: mustTime("2024-02-26T17:26:49Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762167347126210561",
				StartTime: mustTime("2024-02-26T17:26:49Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762167347180736512",
				StartTime: mustTime("2024-02-26T17:26:49Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762167347126210560",
				StartTime: mustTime("2024-02-26T17:26:49Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762167347310759936",
				StartTime: mustTime("2024-02-26T17:26:49Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762167347281399808",
				StartTime: mustTime("2024-02-26T17:26:49Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762167347210096640",
				StartTime: mustTime("2024-02-26T17:26:49Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762290665967849472",
				StartTime: mustTime("2024-02-27T01:36:50Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762290659240185856",
				StartTime: mustTime("2024-02-27T01:36:49Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762290666798321664",
				StartTime: mustTime("2024-02-27T01:36:50Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762290661752573952",
				StartTime: mustTime("2024-02-27T01:36:49Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762290665103822848",
				StartTime: mustTime("2024-02-27T01:36:50Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762290663438684160",
				StartTime: mustTime("2024-02-27T01:36:50Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762290660917907456",
				StartTime: mustTime("2024-02-27T01:36:49Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762290657558269952",
				StartTime: mustTime("2024-02-27T01:36:48Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762290660070658048",
				StartTime: mustTime("2024-02-27T01:36:49Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762290658401325056",
				StartTime: mustTime("2024-02-27T01:36:49Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762414009379721216",
				StartTime: mustTime("2024-02-27T09:46:57Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762414005999112192",
				StartTime: mustTime("2024-02-27T09:46:57Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762414003587387392",
				StartTime: mustTime("2024-02-27T09:46:56Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762414002836606976",
				StartTime: mustTime("2024-02-27T09:46:56Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762414008503111680",
				StartTime: mustTime("2024-02-27T09:46:57Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762414010239553536",
				StartTime: mustTime("2024-02-27T09:46:58Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762414002530422784",
				StartTime: mustTime("2024-02-27T09:46:56Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762414001284714496",
				StartTime: mustTime("2024-02-27T09:46:56Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762414005160251392",
				StartTime: mustTime("2024-02-27T09:46:57Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762414007697805312",
				StartTime: mustTime("2024-02-27T09:46:57Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762631056541880320",
				StartTime: mustTime("2024-02-28T00:09:26Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762631054864158720",
				StartTime: mustTime("2024-02-28T00:09:25Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762631049080213504",
				StartTime: mustTime("2024-02-28T00:09:24Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762631054029492224",
				StartTime: mustTime("2024-02-28T00:09:25Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762631057384935424",
				StartTime: mustTime("2024-02-28T00:09:26Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762631055711408128",
				StartTime: mustTime("2024-02-28T00:09:26Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762631051600990208",
				StartTime: mustTime("2024-02-28T00:09:25Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762631049919074304",
				StartTime: mustTime("2024-02-28T00:09:24Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762754477615484928",
				StartTime: mustTime("2024-02-28T08:19:52Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762754477695176704",
				StartTime: mustTime("2024-02-28T08:19:52Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762754477925863424",
				StartTime: mustTime("2024-02-28T08:19:52Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762754477737119744",
				StartTime: mustTime("2024-02-28T08:19:52Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762754477846171648",
				StartTime: mustTime("2024-02-28T08:19:52Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762754477649039360",
				StartTime: mustTime("2024-02-28T08:19:52Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-aws-ovn-upgrade/1762754477883920384",
				StartTime: mustTime("2024-02-28T08:19:52Z"),
			},
		},
	})

	mustAddResolvedIssue(release415, ResolvedIssue{
		TestID:   "openshift-tests:c1f54790201ec8f4241eca902f854b79",
		TestName: "[sig-instrumentation] Prometheus [apigroup:image.openshift.io] when installed on the cluster shouldn't report any alerts in firing state apart from Watchdog and AlertmanagerReceiversNotConfigured [Early][apigroup:config.openshift.io] [Skipped:Disconnected] [Suite:openshift/conformance/parallel]",
		Variant: apitype.ComponentReportColumnIdentification{
			Network:  "sdn",
			Upgrade:  "upgrade-micro",
			Arch:     "amd64",
			Platform: "aws",
			Variant:  "standard",
		},
		Issue: Issue{
			IssueType: "Infrastructure",
			Infrastructure: &InfrastructureIssue{
				Description:    "Loki outage caused ci logging pods to never go ready and eventually a DaemonSetRolloutStuck alert to fire",
				JiraURL:        "https://issues.redhat.com/browse/TRT-1537",
				ResolutionDate: mustTime("2024-02-28T13:00:00Z"),
			},
			PayloadBug: nil,
		},
		ImpactedJobRuns: []JobRun{
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-sdn-upgrade/1762193517494407168",
				StartTime: mustTime("2024-02-26T19:10:47Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-sdn-upgrade/1762193515002990592",
				StartTime: mustTime("2024-02-26T19:10:46Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-sdn-upgrade/1762193512503185408",
				StartTime: mustTime("2024-02-26T19:10:45Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-sdn-upgrade/1762193515808296960",
				StartTime: mustTime("2024-02-26T19:10:46Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-sdn-upgrade/1762193518316490752",
				StartTime: mustTime("2024-02-26T19:10:47Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-sdn-upgrade/1762193516651352064",
				StartTime: mustTime("2024-02-26T19:10:46Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-sdn-upgrade/1762193513279131648",
				StartTime: mustTime("2024-02-26T19:10:46Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-sdn-upgrade/1762193511190368256",
				StartTime: mustTime("2024-02-26T19:10:45Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-sdn-upgrade/1762193511618187264",
				StartTime: mustTime("2024-02-26T19:10:45Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-sdn-upgrade/1762193514126381056",
				StartTime: mustTime("2024-02-26T19:10:46Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-sdn-upgrade/1762355563506700288",
				StartTime: mustTime("2024-02-27T05:54:43Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-sdn-upgrade/1762355558477729792",
				StartTime: mustTime("2024-02-27T05:54:42Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-sdn-upgrade/1762355562705588224",
				StartTime: mustTime("2024-02-27T05:54:43Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-sdn-upgrade/1762355560168034304",
				StartTime: mustTime("2024-02-27T05:54:42Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-sdn-upgrade/1762355559312396288",
				StartTime: mustTime("2024-02-27T05:54:42Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-sdn-upgrade/1762355566044254208",
				StartTime: mustTime("2024-02-27T05:54:43Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-sdn-upgrade/1762355561858338816",
				StartTime: mustTime("2024-02-27T05:54:42Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-sdn-upgrade/1762653959853445120",
				StartTime: mustTime("2024-02-28T01:40:26Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-sdn-upgrade/1762653961531166720",
				StartTime: mustTime("2024-02-28T01:40:27Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-sdn-upgrade/1762653954820280320",
				StartTime: mustTime("2024-02-28T01:40:25Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-sdn-upgrade/1762653955654946816",
				StartTime: mustTime("2024-02-28T01:40:25Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-sdn-upgrade/1762653958179917824",
				StartTime: mustTime("2024-02-28T01:40:26Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-sdn-upgrade/1762653956921626624",
				StartTime: mustTime("2024-02-28T01:40:26Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-sdn-upgrade/1762653953142558720",
				StartTime: mustTime("2024-02-28T01:40:25Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-e2e-aws-sdn-upgrade/1762653960688111616",
				StartTime: mustTime("2024-02-28T01:40:26Z"),
			},
		},
	})

	mustAddResolvedIssue(release415, ResolvedIssue{
		TestID:   "openshift-tests:c1f54790201ec8f4241eca902f854b79",
		TestName: "[sig-instrumentation] Prometheus [apigroup:image.openshift.io] when installed on the cluster shouldn't report any alerts in firing state apart from Watchdog and AlertmanagerReceiversNotConfigured [Early][apigroup:config.openshift.io] [Skipped:Disconnected] [Suite:openshift/conformance/parallel]",
		Variant: apitype.ComponentReportColumnIdentification{
			Network:  "ovn",
			Upgrade:  "upgrade-micro",
			Arch:     "amd64",
			Platform: "azure",
			Variant:  "standard",
		},
		Issue: Issue{
			IssueType: "Infrastructure",
			Infrastructure: &InfrastructureIssue{
				Description:    "Loki outage caused ci logging pods to never go ready and eventually a DaemonSetRolloutStuck alert to fire",
				JiraURL:        "https://issues.redhat.com/browse/TRT-1537",
				ResolutionDate: mustTime("2024-02-28T13:00:00Z"),
			},
			PayloadBug: nil,
		},
		ImpactedJobRuns: []JobRun{
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1762193545168424960",
				StartTime: mustTime("2024-02-26T19:10:52Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1762193546057617408",
				StartTime: mustTime("2024-02-26T19:10:52Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1762193543490703360",
				StartTime: mustTime("2024-02-26T19:10:52Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1762193542643453952",
				StartTime: mustTime("2024-02-26T19:10:52Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1762193535953539072",
				StartTime: mustTime("2024-02-26T19:10:51Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1762193544409255936",
				StartTime: mustTime("2024-02-26T19:10:52Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1762193538168131584",
				StartTime: mustTime("2024-02-26T19:10:51Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1762193536767234048",
				StartTime: mustTime("2024-02-26T19:10:51Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1762193535114678272",
				StartTime: mustTime("2024-02-26T19:10:51Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1762355587842052096",
				StartTime: mustTime("2024-02-27T05:54:48Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1762355589528162304",
				StartTime: mustTime("2024-02-27T05:54:48Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1762355590459297792",
				StartTime: mustTime("2024-02-27T05:54:49Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1762355586998996992",
				StartTime: mustTime("2024-02-27T05:54:48Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1762355588710273024",
				StartTime: mustTime("2024-02-27T05:54:48Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1762355592095076352",
				StartTime: mustTime("2024-02-27T05:54:49Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1762653983341547520",
				StartTime: mustTime("2024-02-28T01:40:31Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1762653990652219392",
				StartTime: mustTime("2024-02-28T01:40:34Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1762653986705379328",
				StartTime: mustTime("2024-02-28T01:40:32Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1762653986281754624",
				StartTime: mustTime("2024-02-28T01:40:32Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1762653984188796928",
				StartTime: mustTime("2024-02-28T01:40:32Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1762653988982886400",
				StartTime: mustTime("2024-02-28T01:40:33Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1762653987607154688",
				StartTime: mustTime("2024-02-28T01:40:33Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1762653982083256320",
				StartTime: mustTime("2024-02-28T01:40:31Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1762653982498492416",
				StartTime: mustTime("2024-02-28T01:40:31Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-azure-ovn-upgrade/1762653980829159424",
				StartTime: mustTime("2024-02-28T01:40:31Z"),
			},
		},
	})

	mustAddResolvedIssue(release415, ResolvedIssue{
		TestID:   "openshift-tests:c1f54790201ec8f4241eca902f854b79",
		TestName: "[sig-instrumentation] Prometheus [apigroup:image.openshift.io] when installed on the cluster shouldn't report any alerts in firing state apart from Watchdog and AlertmanagerReceiversNotConfigured [Early][apigroup:config.openshift.io] [Skipped:Disconnected] [Suite:openshift/conformance/parallel]",
		Variant: apitype.ComponentReportColumnIdentification{
			Network:  "sdn",
			Upgrade:  "upgrade-minor",
			Arch:     "amd64",
			Platform: "azure",
			Variant:  "standard",
		},
		Issue: Issue{
			IssueType: "Infrastructure",
			Infrastructure: &InfrastructureIssue{
				Description:    "Loki outage caused ci logging pods to never go ready and eventually a DaemonSetRolloutStuck alert to fire",
				JiraURL:        "https://issues.redhat.com/browse/TRT-1537",
				ResolutionDate: mustTime("2024-02-28T13:00:00Z"),
			},
			PayloadBug: nil,
		},
		ImpactedJobRuns: []JobRun{
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762167357150597120",
				StartTime: mustTime("2024-02-26T17:26:51Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762167357972680704",
				StartTime: mustTime("2024-02-26T17:26:52Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762167359696539648",
				StartTime: mustTime("2024-02-26T17:26:52Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762167352939515904",
				StartTime: mustTime("2024-02-26T17:26:50Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762167354613043200",
				StartTime: mustTime("2024-02-26T17:26:51Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762167355464486912",
				StartTime: mustTime("2024-02-26T17:26:51Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762167353774182400",
				StartTime: mustTime("2024-02-26T17:26:51Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762290643847090176",
				StartTime: mustTime("2024-02-27T01:36:45Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762290643486380032",
				StartTime: mustTime("2024-02-27T01:36:45Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762290643813535744",
				StartTime: mustTime("2024-02-27T01:36:45Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762290644123914240",
				StartTime: mustTime("2024-02-27T01:36:45Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762290643763204096",
				StartTime: mustTime("2024-02-27T01:36:45Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762290643633180672",
				StartTime: mustTime("2024-02-27T01:36:45Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762290643347968000",
				StartTime: mustTime("2024-02-27T01:36:45Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762413987028275200",
				StartTime: mustTime("2024-02-27T09:46:53Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762413986944389120",
				StartTime: mustTime("2024-02-27T09:46:53Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762413987636449280",
				StartTime: mustTime("2024-02-27T09:46:53Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762413987049246720",
				StartTime: mustTime("2024-02-27T09:46:53Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762413986835337217",
				StartTime: mustTime("2024-02-27T09:46:53Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762413986982137856",
				StartTime: mustTime("2024-02-27T09:46:53Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762413986936000512",
				StartTime: mustTime("2024-02-27T09:46:53Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762413986889863168",
				StartTime: mustTime("2024-02-27T09:46:53Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762631059897323520",
				StartTime: mustTime("2024-02-28T00:09:27Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762631059058462720",
				StartTime: mustTime("2024-02-28T00:09:26Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762631063261155328",
				StartTime: mustTime("2024-02-28T00:09:27Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762631064959848448",
				StartTime: mustTime("2024-02-28T00:09:28Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762631062984331264",
				StartTime: mustTime("2024-02-28T00:09:27Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762631061575045120",
				StartTime: mustTime("2024-02-28T00:09:27Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762631066608209920",
				StartTime: mustTime("2024-02-28T00:09:28Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762754482556375040",
				StartTime: mustTime("2024-02-28T08:19:53Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762754484250873856",
				StartTime: mustTime("2024-02-28T08:19:53Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762754483416207360",
				StartTime: mustTime("2024-02-28T08:19:53Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762754485085540352",
				StartTime: mustTime("2024-02-28T08:19:54Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762754480882847744",
				StartTime: mustTime("2024-02-28T08:19:53Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-azure-sdn-upgrade/1762754485941178368",
				StartTime: mustTime("2024-02-28T08:19:54Z"),
			},
		},
	})

	mustAddResolvedIssue(release415, ResolvedIssue{
		TestID:   "openshift-tests:c1f54790201ec8f4241eca902f854b79",
		TestName: "[sig-instrumentation] Prometheus [apigroup:image.openshift.io] when installed on the cluster shouldn't report any alerts in firing state apart from Watchdog and AlertmanagerReceiversNotConfigured [Early][apigroup:config.openshift.io] [Skipped:Disconnected] [Suite:openshift/conformance/parallel]",
		Variant: apitype.ComponentReportColumnIdentification{
			Network:  "ovn",
			Upgrade:  "upgrade-minor",
			Arch:     "amd64",
			Platform: "gcp",
			Variant:  "rt",
		},
		Issue: Issue{
			IssueType: "Infrastructure",
			Infrastructure: &InfrastructureIssue{
				Description:    "Loki outage caused ci logging pods to never go ready and eventually a DaemonSetRolloutStuck alert to fire",
				JiraURL:        "https://issues.redhat.com/browse/TRT-1537",
				ResolutionDate: mustTime("2024-02-28T13:00:00Z"),
			},
			PayloadBug: nil,
		},
		ImpactedJobRuns: []JobRun{
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-gcp-ovn-rt-upgrade/1762193453610962944",
				StartTime: mustTime("2024-02-26T19:10:33Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-gcp-ovn-rt-upgrade/1762193451962601472",
				StartTime: mustTime("2024-02-26T19:10:33Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-gcp-ovn-rt-upgrade/1762193455389347840",
				StartTime: mustTime("2024-02-26T19:10:34Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-gcp-ovn-rt-upgrade/1762193452759519232",
				StartTime: mustTime("2024-02-26T19:10:33Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-gcp-ovn-rt-upgrade/1762193451085991936",
				StartTime: mustTime("2024-02-26T19:10:33Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-gcp-ovn-rt-upgrade/1762193450276491264",
				StartTime: mustTime("2024-02-26T19:10:32Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-gcp-ovn-rt-upgrade/1762193457088040960",
				StartTime: mustTime("2024-02-26T19:10:34Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-gcp-ovn-rt-upgrade/1762355612194181120",
				StartTime: mustTime("2024-02-27T05:54:53Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-gcp-ovn-rt-upgrade/1762355607173599232",
				StartTime: mustTime("2024-02-27T05:54:52Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-gcp-ovn-rt-upgrade/1762355611346931712",
				StartTime: mustTime("2024-02-27T05:54:53Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-gcp-ovn-rt-upgrade/1762355613880291328",
				StartTime: mustTime("2024-02-27T05:54:53Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-gcp-ovn-rt-upgrade/1762355606326349824",
				StartTime: mustTime("2024-02-27T05:54:52Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-gcp-ovn-rt-upgrade/1762355607999877120",
				StartTime: mustTime("2024-02-27T05:54:52Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-gcp-ovn-rt-upgrade/1762355609677598720",
				StartTime: mustTime("2024-02-27T05:54:53Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-gcp-ovn-rt-upgrade/1762543262649815040",
				StartTime: mustTime("2024-02-27T18:20:34Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-gcp-ovn-rt-upgrade/1762653936965128192",
				StartTime: mustTime("2024-02-28T01:40:21Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-gcp-ovn-rt-upgrade/1762653940320571392",
				StartTime: mustTime("2024-02-28T01:40:22Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-gcp-ovn-rt-upgrade/1762653941155237888",
				StartTime: mustTime("2024-02-28T01:40:22Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-gcp-ovn-rt-upgrade/1762653939477516288",
				StartTime: mustTime("2024-02-28T01:40:22Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-gcp-ovn-rt-upgrade/1762653936122073088",
				StartTime: mustTime("2024-02-28T01:40:21Z"),
			},
		},
	})

	mustAddResolvedIssue(release415, ResolvedIssue{
		TestID:   "openshift-tests:c1f54790201ec8f4241eca902f854b79",
		TestName: "[sig-instrumentation] Prometheus [apigroup:image.openshift.io] when installed on the cluster shouldn't report any alerts in firing state apart from Watchdog and AlertmanagerReceiversNotConfigured [Early][apigroup:config.openshift.io] [Skipped:Disconnected] [Suite:openshift/conformance/parallel]",
		Variant: apitype.ComponentReportColumnIdentification{
			Network:  "ovn",
			Upgrade:  "upgrade-micro",
			Arch:     "amd64",
			Platform: "gcp",
			Variant:  "standard",
		},
		Issue: Issue{
			IssueType: "Infrastructure",
			Infrastructure: &InfrastructureIssue{
				Description:    "Loki outage caused ci logging pods to never go ready and eventually a DaemonSetRolloutStuck alert to fire",
				JiraURL:        "https://issues.redhat.com/browse/TRT-1537",
				ResolutionDate: mustTime("2024-02-28T13:00:00Z"),
			},
			PayloadBug: nil,
		},
		ImpactedJobRuns: []JobRun{
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762167365430153216",
				StartTime: mustTime("2024-02-26T17:26:53Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762167362166984704",
				StartTime: mustTime("2024-02-26T17:26:53Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762167369620262912",
				StartTime: mustTime("2024-02-26T17:26:54Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762167364591292416",
				StartTime: mustTime("2024-02-26T17:26:53Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762167366260625408",
				StartTime: mustTime("2024-02-26T17:26:53Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762167363739848704",
				StartTime: mustTime("2024-02-26T17:26:53Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762167368773013504",
				StartTime: mustTime("2024-02-26T17:26:54Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762167367942541312",
				StartTime: mustTime("2024-02-26T17:26:54Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762167367099486208",
				StartTime: mustTime("2024-02-26T17:26:54Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762290651686244352",
				StartTime: mustTime("2024-02-27T01:36:47Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762290650746720256",
				StartTime: mustTime("2024-02-27T01:36:47Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762290648238526464",
				StartTime: mustTime("2024-02-27T01:36:46Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762290652512522240",
				StartTime: mustTime("2024-02-27T01:36:47Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762290649077387264",
				StartTime: mustTime("2024-02-27T01:36:46Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762290645730332672",
				StartTime: mustTime("2024-02-27T01:36:46Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762290653363965952",
				StartTime: mustTime("2024-02-27T01:36:47Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762290647395471360",
				StartTime: mustTime("2024-02-27T01:36:46Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762290646564999168",
				StartTime: mustTime("2024-02-27T01:36:46Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762290649920442368",
				StartTime: mustTime("2024-02-27T01:36:47Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762413989280616448",
				StartTime: mustTime("2024-02-27T09:46:53Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762413990941560832",
				StartTime: mustTime("2024-02-27T09:46:53Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762413996029251584",
				StartTime: mustTime("2024-02-27T09:46:55Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762413995140059136",
				StartTime: mustTime("2024-02-27T09:46:54Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762413994313781248",
				StartTime: mustTime("2024-02-27T09:46:54Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762413990106894336",
				StartTime: mustTime("2024-02-27T09:46:53Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762413993458143232",
				StartTime: mustTime("2024-02-27T09:46:54Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762413991784615936",
				StartTime: mustTime("2024-02-27T09:46:54Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762413996826169344",
				StartTime: mustTime("2024-02-27T09:46:55Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762631044315484160",
				StartTime: mustTime("2024-02-28T00:09:23Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762631044235792384",
				StartTime: mustTime("2024-02-28T00:09:23Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762631044198043648",
				StartTime: mustTime("2024-02-28T00:09:23Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762631044361621504",
				StartTime: mustTime("2024-02-28T00:09:23Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762631044273541120",
				StartTime: mustTime("2024-02-28T00:09:23Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762631045720576000",
				StartTime: mustTime("2024-02-28T00:09:23Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762631046559436800",
				StartTime: mustTime("2024-02-28T00:09:24Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762754495143481344",
				StartTime: mustTime("2024-02-28T08:19:55Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762754497249021952",
				StartTime: mustTime("2024-02-28T08:19:56Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762754489258872832",
				StartTime: mustTime("2024-02-28T08:19:54Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762754490932400128",
				StartTime: mustTime("2024-02-28T08:19:55Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762754490089345024",
				StartTime: mustTime("2024-02-28T08:19:55Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-e2e-gcp-ovn-upgrade/1762754492618510336",
				StartTime: mustTime("2024-02-28T08:19:55Z"),
			},
		},
	})

	mustAddResolvedIssue(release415, ResolvedIssue{
		TestID:   "openshift-tests:c1f54790201ec8f4241eca902f854b79",
		TestName: "[sig-instrumentation] Prometheus [apigroup:image.openshift.io] when installed on the cluster shouldn't report any alerts in firing state apart from Watchdog and AlertmanagerReceiversNotConfigured [Early][apigroup:config.openshift.io] [Skipped:Disconnected] [Suite:openshift/conformance/parallel]",
		Variant: apitype.ComponentReportColumnIdentification{
			Network:  "sdn",
			Upgrade:  "upgrade-minor",
			Arch:     "amd64",
			Platform: "gcp",
			Variant:  "standard",
		},
		Issue: Issue{
			IssueType: "Infrastructure",
			Infrastructure: &InfrastructureIssue{
				Description:    "Loki outage caused ci logging pods to never go ready and eventually a DaemonSetRolloutStuck alert to fire",
				JiraURL:        "https://issues.redhat.com/browse/TRT-1537",
				ResolutionDate: mustTime("2024-02-28T13:00:00Z"),
			},
			PayloadBug: nil,
		},
		ImpactedJobRuns: []JobRun{
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-gcp-sdn-upgrade/1762193526713487360",
				StartTime: mustTime("2024-02-26T19:10:49Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-gcp-sdn-upgrade/1762355576106389504",
				StartTime: mustTime("2024-02-27T05:54:45Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.15-upgrade-from-stable-4.14-e2e-gcp-sdn-upgrade/1762654029273370624",
				StartTime: mustTime("2024-02-28T01:40:43Z"),
			},
		},
	})

	mustAddResolvedIssue(release415, ResolvedIssue{
		TestID:   "openshift-tests:c1f54790201ec8f4241eca902f854b79",
		TestName: "[sig-instrumentation] Prometheus [apigroup:image.openshift.io] when installed on the cluster shouldn't report any alerts in firing state apart from Watchdog and AlertmanagerReceiversNotConfigured [Early][apigroup:config.openshift.io] [Skipped:Disconnected] [Suite:openshift/conformance/parallel]",
		Variant: apitype.ComponentReportColumnIdentification{
			Network:  "ovn",
			Upgrade:  "upgrade-minor",
			Arch:     "amd64",
			Platform: "metal-ipi",
		},
		Issue: Issue{
			IssueType: "Infrastructure",
			Infrastructure: &InfrastructureIssue{
				Description:    "Loki outage caused ci logging pods to never go ready and eventually a DaemonSetRolloutStuck alert to fire",
				JiraURL:        "https://issues.redhat.com/browse/TRT-1537",
				ResolutionDate: mustTime("2024-02-27T16:00:00Z"), // issue on-going but fix about to merge, adding 2 hours to now
			},
			PayloadBug: nil,
		},
		ImpactedJobRuns: []JobRun{
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-upgrade-from-stable-4.14-e2e-metal-ipi-upgrade-ovn-ipv6/1760329390320783360",
				StartTime: mustTime("2024-02-21T15:43:26Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-upgrade-from-stable-4.14-e2e-metal-ipi-upgrade-ovn-ipv6/1762193475517812736",
				StartTime: mustTime("2024-02-26T19:10:38Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.15-upgrade-from-stable-4.14-e2e-metal-ipi-upgrade-ovn-ipv6/1762355633987784704",
				StartTime: mustTime("2024-02-27T05:54:58Z"),
			},
		},
	})


}

package resolvedissues

import apitype "github.com/openshift/sippy/pkg/apis/api"

func init() {

	mustAddResolvedIssue(release416, ResolvedIssue{
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
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762202348307877888",
				StartTime: mustTime("2024-02-26T19:45:54Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762202325646053376",
				StartTime: mustTime("2024-02-26T19:45:48Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762202338228965376",
				StartTime: mustTime("2024-02-26T19:45:51Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762202340770713600",
				StartTime: mustTime("2024-02-26T19:45:52Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762202335716577280",
				StartTime: mustTime("2024-02-26T19:45:51Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762202328175218688",
				StartTime: mustTime("2024-02-26T19:45:49Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762202345782906880",
				StartTime: mustTime("2024-02-26T19:45:53Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762202333195800576",
				StartTime: mustTime("2024-02-26T19:45:50Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762202330679218176",
				StartTime: mustTime("2024-02-26T19:45:49Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762325844090425344",
				StartTime: mustTime("2024-02-27T03:56:37Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762325847462645760",
				StartTime: mustTime("2024-02-27T03:56:38Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762325848293117952",
				StartTime: mustTime("2024-02-27T03:56:38Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762325850805506048",
				StartTime: mustTime("2024-02-27T03:56:39Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762325844946063360",
				StartTime: mustTime("2024-02-27T03:56:38Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762325843264147456",
				StartTime: mustTime("2024-02-27T03:56:37Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762325849144561664",
				StartTime: mustTime("2024-02-27T03:56:39Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762325849975033856",
				StartTime: mustTime("2024-02-27T03:56:39Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762325846611202048",
				StartTime: mustTime("2024-02-27T03:56:38Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762450942831104000",
				StartTime: mustTime("2024-02-27T12:13:44Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762450943007264768",
				StartTime: mustTime("2024-02-27T12:13:44Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762450942910795776",
				StartTime: mustTime("2024-02-27T12:13:44Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762450943070179328",
				StartTime: mustTime("2024-02-27T12:13:44Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762450943040819200",
				StartTime: mustTime("2024-02-27T12:13:44Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762450942998876160",
				StartTime: mustTime("2024-02-27T12:13:44Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762450942625583104",
				StartTime: mustTime("2024-02-27T12:13:44Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762573177050894336",
				StartTime: mustTime("2024-02-27T20:19:27Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762573177248026624",
				StartTime: mustTime("2024-02-27T20:19:27Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762573177302552576",
				StartTime: mustTime("2024-02-27T20:19:27Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762573177273192448",
				StartTime: mustTime("2024-02-27T20:19:27Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762573177810063360",
				StartTime: mustTime("2024-02-27T20:19:27Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762573177134780416",
				StartTime: mustTime("2024-02-27T20:19:27Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762573177352884224",
				StartTime: mustTime("2024-02-27T20:19:27Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762573177080254464",
				StartTime: mustTime("2024-02-27T20:19:27Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762573178648924160",
				StartTime: mustTime("2024-02-27T20:19:27Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762699093185925120",
				StartTime: mustTime("2024-02-28T04:39:46Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762699088144371712",
				StartTime: mustTime("2024-02-28T04:39:45Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762699090660954112",
				StartTime: mustTime("2024-02-28T04:39:45Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762699091508203520",
				StartTime: mustTime("2024-02-28T04:39:46Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762699092342870016",
				StartTime: mustTime("2024-02-28T04:39:46Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762699094016397312",
				StartTime: mustTime("2024-02-28T04:39:46Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762699094859452416",
				StartTime: mustTime("2024-02-28T04:39:46Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762699089402662912",
				StartTime: mustTime("2024-02-28T04:39:45Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762699089822093312",
				StartTime: mustTime("2024-02-28T04:39:45Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-ovn-upgrade/1762699095694118912",
				StartTime: mustTime("2024-02-28T04:39:47Z"),
			},
		},
	})

	mustAddResolvedIssue(release416, ResolvedIssue{
		TestID:   "openshift-tests:c1f54790201ec8f4241eca902f854b79",
		TestName: "[sig-instrumentation] Prometheus [apigroup:image.openshift.io] when installed on the cluster shouldn't report any alerts in firing state apart from Watchdog and AlertmanagerReceiversNotConfigured [Early][apigroup:config.openshift.io] [Skipped:Disconnected] [Suite:openshift/conformance/parallel]",
		Variant: apitype.ComponentReportColumnIdentification{
			Network:  "sdn",
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
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-sdn-upgrade/1762202381015060480",
				StartTime: mustTime("2024-02-26T19:46:01Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-sdn-upgrade/1762325841666117632",
				StartTime: mustTime("2024-02-27T03:56:37Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-sdn-upgrade/1762450945024724992",
				StartTime: mustTime("2024-02-27T12:13:44Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-sdn-upgrade/1762573177029922816",
				StartTime: mustTime("2024-02-27T20:19:27Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-aws-sdn-upgrade/1762699087309705216",
				StartTime: mustTime("2024-02-28T04:39:45Z"),
			},
		},
	})

	mustAddResolvedIssue(release416, ResolvedIssue{
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
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-azure-ovn-upgrade/1762143935964123136",
				StartTime: mustTime("2024-02-26T15:53:46Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-azure-ovn-upgrade/1762143941857120256",
				StartTime: mustTime("2024-02-26T15:53:47Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-azure-ovn-upgrade/1762143944394674176",
				StartTime: mustTime("2024-02-26T15:53:47Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-azure-ovn-upgrade/1762143943509676032",
				StartTime: mustTime("2024-02-26T15:53:47Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-azure-ovn-upgrade/1762143940154232832",
				StartTime: mustTime("2024-02-26T15:53:47Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-azure-ovn-upgrade/1762143937633456128",
				StartTime: mustTime("2024-02-26T15:53:46Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-azure-ovn-upgrade/1762143945195786240",
				StartTime: mustTime("2024-02-26T15:53:48Z"),
			},
		},
	})

	mustAddResolvedIssue(release416, ResolvedIssue{
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
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762202355849236480",
				StartTime: mustTime("2024-02-26T19:45:55Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762202358365818880",
				StartTime: mustTime("2024-02-26T19:45:56Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762202370944536576",
				StartTime: mustTime("2024-02-26T19:45:59Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762202363394789376",
				StartTime: mustTime("2024-02-26T19:45:57Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762202365928148992",
				StartTime: mustTime("2024-02-26T19:45:58Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762202360907567104",
				StartTime: mustTime("2024-02-26T19:45:57Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762202373465313280",
				StartTime: mustTime("2024-02-26T19:46:00Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762202353336848384",
				StartTime: mustTime("2024-02-26T19:45:55Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762202375981895680",
				StartTime: mustTime("2024-02-26T19:46:00Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762325854165143552",
				StartTime: mustTime("2024-02-27T03:56:40Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762325857524781056",
				StartTime: mustTime("2024-02-27T03:56:41Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762325856702697472",
				StartTime: mustTime("2024-02-27T03:56:40Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762325858367836160",
				StartTime: mustTime("2024-02-27T03:56:41Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762325855930945536",
				StartTime: mustTime("2024-02-27T03:56:40Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762325855024975872",
				StartTime: mustTime("2024-02-27T03:56:40Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762325853347254272",
				StartTime: mustTime("2024-02-27T03:56:40Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762325859210891264",
				StartTime: mustTime("2024-02-27T03:56:41Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762325860045557760",
				StartTime: mustTime("2024-02-27T03:56:41Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762325852495810560",
				StartTime: mustTime("2024-02-27T03:56:39Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762450948665380864",
				StartTime: mustTime("2024-02-27T12:13:45Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762450949466492928",
				StartTime: mustTime("2024-02-27T12:13:45Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762450951160991744",
				StartTime: mustTime("2024-02-27T12:13:45Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762450946102661120",
				StartTime: mustTime("2024-02-27T12:13:44Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762450953669185536",
				StartTime: mustTime("2024-02-27T12:13:46Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762450947784577024",
				StartTime: mustTime("2024-02-27T12:13:45Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762450950313742336",
				StartTime: mustTime("2024-02-27T12:13:45Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762450951983075328",
				StartTime: mustTime("2024-02-27T12:13:46Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762573192020365312",
				StartTime: mustTime("2024-02-27T20:19:30Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762573195388391424",
				StartTime: mustTime("2024-02-27T20:19:30Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762573198727057408",
				StartTime: mustTime("2024-02-27T20:19:31Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762573192846643200",
				StartTime: mustTime("2024-02-27T20:19:30Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762573194545336320",
				StartTime: mustTime("2024-02-27T20:19:30Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762699069223866368",
				StartTime: mustTime("2024-02-28T04:39:42Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762699069794291712",
				StartTime: mustTime("2024-02-28T04:39:42Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762699069261615104",
				StartTime: mustTime("2024-02-28T04:39:42Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762699069135785984",
				StartTime: mustTime("2024-02-28T04:39:42Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762699069081260032",
				StartTime: mustTime("2024-02-28T04:39:42Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762699069290975232",
				StartTime: mustTime("2024-02-28T04:39:42Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762699069156757504",
				StartTime: mustTime("2024-02-28T04:39:42Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-azure-sdn-upgrade/1762699069194506240",
				StartTime: mustTime("2024-02-28T04:39:42Z"),
			},
		},
	})

	mustAddResolvedIssue(release416, ResolvedIssue{
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
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762202403664302080",
				StartTime: mustTime("2024-02-26T19:46:07Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762202398635331584",
				StartTime: mustTime("2024-02-26T19:46:06Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762202383531642880",
				StartTime: mustTime("2024-02-26T19:46:02Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762202386056613888",
				StartTime: mustTime("2024-02-26T19:46:03Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762202406189273088",
				StartTime: mustTime("2024-02-26T19:46:07Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762202388577390592",
				StartTime: mustTime("2024-02-26T19:46:03Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762325837815746560",
				StartTime: mustTime("2024-02-27T03:56:36Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762325837782192128",
				StartTime: mustTime("2024-02-27T03:56:36Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762325837664751616",
				StartTime: mustTime("2024-02-27T03:56:36Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762325839166312448",
				StartTime: mustTime("2024-02-27T03:56:36Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762325837731860480",
				StartTime: mustTime("2024-02-27T03:56:36Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762325838314868736",
				StartTime: mustTime("2024-02-27T03:56:36Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762325837631197184",
				StartTime: mustTime("2024-02-27T03:56:36Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762450957884461056",
				StartTime: mustTime("2024-02-27T12:13:47Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762450955334324224",
				StartTime: mustTime("2024-02-27T12:13:46Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762450957095931904",
				StartTime: mustTime("2024-02-27T12:13:47Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762450958756876288",
				StartTime: mustTime("2024-02-27T12:13:47Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762450962892460032",
				StartTime: mustTime("2024-02-27T12:13:48Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762450961206349824",
				StartTime: mustTime("2024-02-27T12:13:48Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762450959545405440",
				StartTime: mustTime("2024-02-27T12:13:47Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762573186152534016",
				StartTime: mustTime("2024-02-27T20:19:28Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762699079755763712",
				StartTime: mustTime("2024-02-28T04:39:44Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762699072310874112",
				StartTime: mustTime("2024-02-28T04:39:42Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762699078921097216",
				StartTime: mustTime("2024-02-28T04:39:44Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762699073988595712",
				StartTime: mustTime("2024-02-28T04:39:43Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762699075670511616",
				StartTime: mustTime("2024-02-28T04:39:43Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762699083954262016",
				StartTime: mustTime("2024-02-28T04:39:44Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762699074827456512",
				StartTime: mustTime("2024-02-28T04:39:43Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762699077339844608",
				StartTime: mustTime("2024-02-28T04:39:43Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-e2e-gcp-ovn-upgrade/1762699078090625024",
				StartTime: mustTime("2024-02-28T04:39:43Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-nightly-4.16-e2e-gcp-ovn-upgrade/1762207605465288704",
				StartTime: mustTime("2024-02-26T20:06:47Z"),
			},
		},
	})

	mustAddResolvedIssue(release416, ResolvedIssue{
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
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-gcp-ovn-rt-upgrade/1762143919207878656",
				StartTime: mustTime("2024-02-26T15:53:43Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-gcp-ovn-rt-upgrade/1762143927575515136",
				StartTime: mustTime("2024-02-26T15:53:44Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-gcp-ovn-rt-upgrade/1762143924211683328",
				StartTime: mustTime("2024-02-26T15:53:44Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-gcp-ovn-rt-upgrade/1762143917492408320",
				StartTime: mustTime("2024-02-26T15:53:43Z"),
			},
			{
				URL:       "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-ci-openshift-release-master-ci-4.16-upgrade-from-stable-4.15-e2e-gcp-ovn-rt-upgrade/1762143920008990720",
				StartTime: mustTime("2024-02-26T15:53:43Z"),
			},
		},
	})

}

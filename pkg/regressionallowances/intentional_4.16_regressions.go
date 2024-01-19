package regressionallowances

import "github.com/openshift/sippy/pkg/apis/api"

func init() {
	mustAddIntentionalRegression(
		release416,
		IntentionalRegression{
			JiraComponent: "Unknown",                                                                                                   // Jira Component,  not team name
			TestID:        "openshift-tests-upgrade:37f1600d4f8d75c47fc5f575025068d2",                                                  // ask TRT for the ID for your TestName
			TestName:      "[sig-cluster-lifecycle] pathological event should not see excessive Back-off restarting failed containers", // this helps approvers recognize at a glance
			Variant: api.ComponentReportColumnIdentification{ // this indicates the selectivity of the choice
				Network:  "ovn",
				Upgrade:  "upgrade-micro",
				Arch:     "amd64",
				Platform: "azure",
			},
			PreviousPassPercentage:    100,
			PreviousSampleSize:        735, // number of runs used for the percentage
			RegressedPassPercentage:   84,
			RegressedSampleSize:       58, // number of runs used for the percentage
			ReasonToAllowInsteadOfFix: "just for testing",
		})
}

package regressionallowances

import "github.com/openshift/sippy/pkg/apis/api"

func init() {
	mustAddIntentionalRegression(
		release415,
		IntentionalRegression{
			JiraComponent: "Networking / openshift-sdn",                       // Jira Component,  not team name
			TestID:        "cluster install:0cb1bb27e418491b1ffdacab58c5c8c0", // ask TRT for the ID for your TestName
			TestName:      "install should succeed: overall",                  // this helps approvers recognize at a glance
			Variant: api.ComponentReportColumnIdentification{ // this indicates the selectivity of the choice
				Network:  "sdn",
				Upgrade:  "upgrade-micro",
				Arch:     "amd64",
				Platform: "metal-ipi",
			},
			PreviousPassPercentage:    100,
			PreviousSampleSize:        61, // number of runs used for the percentage
			RegressedPassPercentage:   50,
			RegressedSampleSize:       12, // number of runs used for the percentage
			ReasonToAllowInsteadOfFix: "I can't explain this, but Ben Bennett maybe?",
		})
}

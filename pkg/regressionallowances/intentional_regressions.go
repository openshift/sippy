package regressionallowances

import (
	_ "embed"
	"encoding/json"
)

// embed regressions415.json
// var regressions4_15 []byte

// example regressions json
// [
// {
//  "JiraComponent": "OLM",
//  "TestID": "Operator results:55a75a8aa11231d0ca36a4d65644e1dd",
//  "TestName": "operator conditions operator-lifecycle-manager-packageserver",
//  "variant": {
//   "variants": {
//    "Architecture": "amd64",
//    "FeatureSet": "default",
//    "Installer": "ipi",
//    "Network": "ovn",
//    "Platform": "metal",
//    "Suite": "unknown",
//    "Topology": "ha",
//    "Upgrade": "micro"
//   }
//  },
//  "PreviousSuccesses": 166,
//  "PreviousFailures": 0,
//  "PreviousFlakes": 0,
//  "RegressedSuccesses": 57,
//  "RegressedFailures": 5,
//  "RegressedFlakes": 0,
//  "JiraBug": "https://issues.redhat.com/browse/OCPBUGS-33255",
//  "ReasonToAllowInsteadOfFix": "Waiting for upstream fix to propagate"
// },
// {
//  "JiraComponent": "openshift-apiserver",
//  "TestID": "Operator results:a4dfe6caa55e94230b4ee0ff127b6d64",
//  "TestName": "operator conditions openshift-apiserver",
//  "variant": {
//   "variants": {
//    "Architecture": "amd64",
//    "FeatureSet": "default",
//    "Installer": "ipi",
//    "Network": "ovn",
//    "Platform": "metal",
//    "Suite": "unknown",
//    "Topology": "ha",
//    "Upgrade": "micro"
//   }
//  },
//  "PreviousSuccesses": 166,
//  "PreviousFailures": 0,
//  "PreviousFlakes": 0,
//  "RegressedSuccesses": 57,
//  "RegressedFailures": 5,
//  "RegressedFlakes": 0,
//  "JiraBug": "https://issues.redhat.com/browse/OCPBUGS-33255",
//  "ReasonToAllowInsteadOfFix": "Waiting for upstream fix to propagate"
// }
// ]

//go:embed regressions_4.17.json
var regressions4_17 []byte

//go:embed regressions_4.18.json
var regressions4_18 []byte

var (
	release417 release = "4.17"
	release418 release = "4.18"
)

//nolint:all
func init() {
	importIntentionalRegressions(release417, regressions4_17)
	importIntentionalRegressions(release418, regressions4_18)
}

func importIntentionalRegressions(releaseTarget release, jsonRegressions []byte) {
	regressions := []IntentionalRegression{}

	err := json.Unmarshal(jsonRegressions, &regressions)

	if err != nil {
		panic(err)
	}

	for _, regression := range regressions {
		mustAddIntentionalRegression(
			releaseTarget,
			regression,
		)
	}
}

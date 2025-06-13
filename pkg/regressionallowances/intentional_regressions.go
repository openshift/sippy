package regressionallowances

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
)

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

/* example regressions directory
- regressions
  - OWNERS (ignored)
  - 4.17:
	- regressions_4.17.json
  - 4.18:
	- OCPBUGS-23456-some-problem.json
	- TRT-1234-some-other-problem.json
	- TRT-5678-several-for-same-problem.json
*/
//go:embed regressions
var regressionsDir embed.FS

func init() {
	// get a list of directories in the regressions directory
	// for each directory, get the json files
	// for each json file, unmarshal the json into a slice of IntentionalRegression
	// for each IntentionalRegression, add it to the map

	fs, err := regressionsDir.ReadDir("regressions")
	if err != nil {
		panic(err)
	}
	for _, entry := range fs {
		if !entry.IsDir() {
			continue
		}

		releaseStr := entry.Name()
		dirPath := "regressions/" + releaseStr
		releaseDir, err := regressionsDir.ReadDir(dirPath)
		if err != nil {
			panic(err)
		}

		for _, file := range releaseDir {
			if !strings.HasSuffix(file.Name(), ".json") {
				continue // only interested in json files under each release
			}

			filePath := dirPath + "/" + file.Name()
			regressionsFile, err := regressionsDir.ReadFile(filePath)
			if err != nil {
				panic(err)
			}

			importIntentionalRegressions(release(releaseStr), filePath, regressionsFile)
		}
	}
}

func importIntentionalRegressions(releaseTarget release, path string, jsonRegressions []byte) {
	regressions := []IntentionalRegression{}
	regression := IntentionalRegression{}

	listErr := json.Unmarshal(jsonRegressions, &regressions)
	if listErr != nil {
		singleErr := json.Unmarshal(jsonRegressions, &regression)
		if singleErr != nil {
			panic(fmt.Errorf("could not load json file %q either as a list (%s) or single (%s) regression", path, listErr, singleErr))
		}
		regressions = []IntentionalRegression{regression}
	}

	for _, regression := range regressions {
		mustAddIntentionalRegression(
			releaseTarget,
			regression,
		)
	}
}

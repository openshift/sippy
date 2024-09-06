package regressionallowances

import (
	"encoding/json"
	"fmt"
)

// embed regressions415.json
// var regressions4_15 []byte

var (
	release415 release = "4.15"
)

//nolint:all
func init() {
	// importIntentionalRegressions(release415, regressions4_15)
}

func importIntentionalRegressions(releaseTarget release, jsonRegressions []byte) {
	regressions := []IntentionalRegression{}

	err := json.Unmarshal(jsonRegressions, &regressions)

	if err != nil {
		panic(err)
	}

	if len(regressions) == 0 {
		panic(fmt.Sprintf("Empty IntentionalRegressions for %s", releaseTarget))
	}

	for _, regression := range regressions {
		mustAddIntentionalRegression(
			releaseTarget,
			regression,
		)
	}
}

package regressionallowances

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed consecutiveoverrides/overrides.json
var overrideBytes []byte

func Test_ApprovalRequiredForRegressionInConsecutiveReleases(t *testing.T) {
	// Find the most recent release in the list
	// We're assuming OpenShift 4.x for all releases
	var maxReleaseMinor int
	for r := range intentionalRegressions {
		releaseMajorMinor := strings.Split(string(r), ".")
		releaseMinor, err := strconv.Atoi(releaseMajorMinor[1])
		require.NoError(t, err)

		if releaseMinor > maxReleaseMinor {
			maxReleaseMinor = releaseMinor
		}
	}

	latestRelease := release(fmt.Sprintf("4.%d", maxReleaseMinor))
	prevRelease := release(fmt.Sprintf("4.%d", maxReleaseMinor-1))

	// Do we have *any* regressions for the prior release? If not, pass.
	if _, ok := intentionalRegressions[prevRelease]; !ok {
		t.Logf("Previous release %s has no regressions", prevRelease)
		return
	}

	// We now do just a very rudimentary check on test ID, without considering variants, if
	// the same test is regressed in two releases consecutively, extra approval is required.
	// Find all the test IDs in latest release:
	prevTestIDs := map[string]bool{}
	for rKey := range intentionalRegressions[prevRelease] {
		regKey, err := parseRegressionKey(rKey)
		require.NoError(t, err, "unable to parse regression key: %s", rKey)
		prevTestIDs[regKey.TestID] = true
	}
	t.Logf("%s regression allowance test IDs: %v", prevRelease, prevTestIDs)

	// Parse the overrides file:
	overrides := map[string]map[string]bool{}
	err := json.Unmarshal(overrideBytes, &overrides)
	require.NoError(t, err, "unable to parse the overrides.json file")
	t.Logf("override allowance test IDs: %v", overrides)

	// Now check if any are also in the latest release without an override and if so, fail the test
	for rKey := range intentionalRegressions[latestRelease] {
		regKey, err := parseRegressionKey(rKey)
		require.NoError(t, err, "unable to parse regression key: %s", rKey)

		var hasOverride bool
		if ros, ok := overrides[string(latestRelease)]; ok {
			hasOverride = ros[regKey.TestID]
		}

		if !hasOverride {
			assert.False(t, prevTestIDs[regKey.TestID],
				"test ID %s found in both %s and %s",
				regKey.TestID, latestRelease, prevRelease)
		}

	}

	if t.Failed() {
		t.Logf("In scenarios where a test requires a regression allowance for two releases in a row, special approvals are required.")
		t.Logf("Please see https://github.com/openshift/sippy/blob/master/pkg/regressionallowances/consecutiveoverrides/README.md for instructions.")
	}
}

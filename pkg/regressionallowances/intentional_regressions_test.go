package regressionallowances

import (
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed consecutiveoverrides/overrides.json
var overrideBytes []byte

func Test_ApprovalRequiredForRegressionInConsecutiveReleases(t *testing.T) {

	// Look up the test IDs that were allowed regressions in each release
	allowedTestIDs := map[release]map[string]bool{}
	for rel, regressions := range intentionalRegressions {
		allowedTestIDs[rel] = map[string]bool{}
		for key := range regressions {
			regKey, err := parseRegressionKey(key)
			require.NoError(t, err, "unable to parse regression key: %s", key)
			if err == nil {
				allowedTestIDs[rel][regKey.TestID] = true
			}
		}
	}

	// Parse the overrides file:
	overrides := map[string]map[string]bool{}
	err := json.Unmarshal(overrideBytes, &overrides)
	require.NoError(t, err, "unable to parse the overrides.json file")
	t.Logf("override allowance test IDs: %v", overrides)

	for thisRelease, regressions := range intentionalRegressions {
		for thisKey, ir := range regressions {
			prevRelease := release(ir.PreviousRelease)

			// We now do just a very rudimentary check on test ID, without considering variants; if
			// the same test is regressed in two releases consecutively, extra approval is required.
			regKey, err := parseRegressionKey(thisKey)
			require.NoError(t, err, "unable to parse regression key: %s", thisKey)
			if prevAllowed, ok := allowedTestIDs[prevRelease]; !ok {
				t.Logf("no allowances in previous release %s, no need to check further", prevRelease)
				continue
			} else if !prevAllowed[regKey.TestID] {
				t.Logf("no allowance for test %s in previous release %s, no need to check further", regKey.TestID, prevRelease)
				continue
			}

			// this release's testID was in the previous; does it have an override?
			releaseOverrides, ok := overrides[string(thisRelease)]
			assert.True(t, ok && releaseOverrides[regKey.TestID],
				"test ID %s found in both %s and %s without an override",
				regKey.TestID, thisRelease, prevRelease)
		}
	}

	if t.Failed() {
		t.Logf("In scenarios where a test requires a regression allowance for two releases in a row, special approvals are required.")
		t.Logf("Please see https://github.com/openshift/sippy/blob/master/pkg/regressionallowances/consecutiveoverrides/README.md for instructions.")
	}
}

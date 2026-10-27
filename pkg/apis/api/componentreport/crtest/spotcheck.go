package crtest

import (
	"fmt"
	"strings"
)

// SpotCheckTestIDPrefix starts every synthetic spot-check test ID. Spot-check results have no real test,
// so the sample name, component and capability are encoded into the ID instead.
const SpotCheckTestIDPrefix = "spotcheck-"

// IsSpotCheckTestID returns true if the test ID was generated for a spot-check test.
func IsSpotCheckTestID(testID string) bool {
	return strings.HasPrefix(testID, SpotCheckTestIDPrefix)
}

// SpotCheckTestID creates the synthetic test ID for a spot-check component/capability group, in the form
// "spotcheck-30d:etcd:scaling". The sample name is the spot-check sample (tier) the results came from.
func SpotCheckTestID(sampleName, component, capability string) string {
	return fmt.Sprintf("%s:%s:%s",
		sampleName,
		strings.ToLower(component),
		strings.ToLower(strings.ReplaceAll(capability, " ", "-")))
}

// ParseSpotCheckTestID reverses SpotCheckTestID, returning the sample name, component and capability, or
// empty strings if the ID is malformed. The encoding is lossy (case, and dashes in the capability become
// spaces), so callers must compare the results case-insensitively.
func ParseSpotCheckTestID(testID string) (sampleName, component, capability string) {
	parts := strings.SplitN(testID, ":", 3)
	if len(parts) != 3 {
		return "", "", ""
	}
	return parts[0], parts[1], strings.ReplaceAll(parts[2], "-", " ")
}

// SpotCheckTestName creates a display name for a spot-check test.
func SpotCheckTestName(component, capability string) string {
	return fmt.Sprintf("[spot-check] %s / %s job must pass at least once per sample window",
		component, capability)
}

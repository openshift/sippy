package resolvedissues

import (
	"fmt"
	"sort"

	"github.com/openshift/sippy/pkg/util/sets"

	"github.com/openshift/sippy/pkg/apis/api"
)

// sync with https://github.com/openshift/sippy/pull/1531/files#diff-3f72919066e1ec3ae4b037dfc91c09ef6d6eac0488762ef35c5a116f73ff1637R237 eventually
const variantArchitecture = "Architecture"
const variantNetwork = "Network"
const variantPlatform = "Platform"
const variantUpgrade = "Upgrade"
const variantVariant = "Variant"

var triageMatchVariants = buildTriageMatchVariants([]string{variantArchitecture, variantNetwork, variantPlatform, variantUpgrade, variantVariant})

func buildTriageMatchVariants(in []string) sets.String {
	if in == nil || len(in) < 1 {
		return nil
	}

	set := sets.NewString()

	for _, l := range in {
		set.Insert(l)
	}

	return set
}
func TransformVariant(variant api.ComponentReportColumnIdentification) []api.TriagedVariant {

	return []api.TriagedVariant{{
		Key:   variantArchitecture,
		Value: variant.Arch,
	}, {
		Key:   variantNetwork,
		Value: variant.Network,
	}, {
		Key:   variantPlatform,
		Value: variant.Platform,
	}, {
		Key:   variantUpgrade,
		Value: variant.Upgrade,
	}, {
		Key:   variantVariant,
		Value: variant.Variant,
	}}
}
func KeyForTriagedIssue(testID string, variants []api.TriagedVariant) TriagedIssueKey {

	matchVariants := make([]api.TriagedVariant, 0)
	for _, v := range variants {
		// currently we ignore variants that aren't in api.ComponentReportColumnIdentification
		if triageMatchVariants.Has(v.Key) {
			matchVariants = append(matchVariants, v)
		}
	}

	sort.Slice(matchVariants,
		func(a, b int) bool {
			return matchVariants[a].Key < matchVariants[b].Key
		})

	vKey := ""
	for _, v := range matchVariants {
		if len(vKey) > 0 {
			vKey += ","
		}
		vKey += fmt.Sprintf("%s_%s", v.Key, v.Value)
	}

	return TriagedIssueKey{
		testID:   testID,
		variants: vKey,
	}
}

type Release string

type TriagedIssueKey struct {
	testID   string
	variants string
}

type TriagedIncidentsForRelease struct {
	Release          Release                                   `json:"release"`
	TriagedIncidents map[TriagedIssueKey][]api.TriagedIncident `json:"triaged_incidents"`
}

func NewTriagedIncidentsForRelease(release Release) TriagedIncidentsForRelease {
	return TriagedIncidentsForRelease{
		Release:          release,
		TriagedIncidents: map[TriagedIssueKey][]api.TriagedIncident{},
	}
}

package resolvedissues

import (
	"fmt"
	"sort"

	"github.com/openshift/sippy/pkg/util/sets"
	"github.com/openshift/sippy/pkg/variantregistry"

	"github.com/openshift/sippy/pkg/apis/api"
)

// VariantVariant is a temporary holdover until we have full variant registry support
// in component readiness.
const VariantVariant = "Variant"

var triageMatchVariants = buildTriageMatchVariants([]string{variantregistry.VariantArch, variantregistry.VariantNetwork, variantregistry.VariantPlatform, variantregistry.VariantUpgrade, VariantVariant})

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
func TransformVariant(variant api.ComponentReportColumnIdentification) []api.ComponentReportVariant {

	return []api.ComponentReportVariant{{
		Key:   variantregistry.VariantArch,
		Value: variant.Arch,
	}, {
		Key:   variantregistry.VariantNetwork,
		Value: variant.Network,
	}, {
		Key:   variantregistry.VariantPlatform,
		Value: variant.Platform,
	}, {
		Key:   variantregistry.VariantUpgrade,
		Value: variant.Upgrade,
	}, {
		Key:   VariantVariant,
		Value: variant.Variant,
	}}
}
func KeyForTriagedIssue(testID string, variants []api.ComponentReportVariant) TriagedIssueKey {

	matchVariants := make([]api.ComponentReportVariant, 0)
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

type TriageIssueType string

const TriageIssueTypeInfrastructure TriageIssueType = "Infrastructure"

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

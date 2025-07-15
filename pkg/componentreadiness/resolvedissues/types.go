package resolvedissues

import (
	"encoding/json"

	"github.com/openshift/sippy/pkg/util/sets"
	"github.com/openshift/sippy/pkg/variantregistry"
)

var TriageMatchVariants = buildTriageMatchVariants([]string{variantregistry.VariantPlatform, variantregistry.VariantArch, variantregistry.VariantNetwork,
	variantregistry.VariantTopology, variantregistry.VariantFeatureSet, variantregistry.VariantUpgrade,
	variantregistry.VariantSuite, variantregistry.VariantInstaller})

func buildTriageMatchVariants(in []string) sets.String {
	if len(in) < 1 {
		return nil
	}

	set := sets.NewString()

	for _, l := range in {
		set.Insert(l)
	}

	return set
}

type TriagedIssueKey struct {
	TestID   string
	Variants string
}

// implement encoding.TextMarshaler for json map key marshalling support
func (s TriagedIssueKey) MarshalText() (text []byte, err error) {
	type t TriagedIssueKey
	return json.Marshal(t(s))
}

func (s *TriagedIssueKey) UnmarshalText(text []byte) error {
	type t TriagedIssueKey
	return json.Unmarshal(text, (*t)(s))
}

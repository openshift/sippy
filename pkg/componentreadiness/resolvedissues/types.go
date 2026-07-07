package resolvedissues

import (
	"encoding/json"

	"github.com/openshift/sippy/pkg/variantregistry"
	"k8s.io/apimachinery/pkg/util/sets"
)

var TriageMatchVariants = buildTriageMatchVariants([]string{variantregistry.VariantPlatform, variantregistry.VariantArch, variantregistry.VariantNetwork,
	variantregistry.VariantTopology, variantregistry.VariantFeatureSet, variantregistry.VariantUpgrade,
	variantregistry.VariantSuite, variantregistry.VariantInstaller})

func buildTriageMatchVariants(in []string) sets.Set[string] {
	if len(in) < 1 {
		return nil
	}

	return sets.New(in...)
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

package testidentification

import (
	"github.com/openshift/sippy/pkg/util/sets"
)

type noVariants struct{}

func NewEmptyVariantManager() VariantManager {
	return noVariants{}
}

func (noVariants) AllPlatforms() sets.String {
	return sets.String{}
}

func (v noVariants) IdentifyVariants(jobName string) []string {
	return []string{}
}
func (noVariants) IsJobNeverStable(jobName string) bool {
	return false
}

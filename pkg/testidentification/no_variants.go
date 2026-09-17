package testidentification

import (
	"k8s.io/apimachinery/pkg/util/sets"
)

type noVariants struct{}

func NewEmptyVariantManager() VariantManager {
	return noVariants{}
}

func (noVariants) AllPlatforms() sets.Set[string] {
	return sets.New[string]()
}

func (v noVariants) IdentifyVariants(jobName string) []string {
	return []string{}
}
func (noVariants) IsJobNeverStable(jobName string) bool {
	return false
}

package testgridconversion

import (
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
)

type emptySyntheticManager struct{}

func NewEmptySythenticTestManager() SythenticTestManager {
	return emptySyntheticManager{}
}

func (emptySyntheticManager) CreateSyntheticTests(rawJobResults testgridanalysisapi.RawData) []string {
	return []string{}
}

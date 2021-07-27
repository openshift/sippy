package testgridconversion

import (
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
)

type emptySyntheticManager struct{}

func NewEmptySyntheticTestManager() SyntheticTestManager {
	return emptySyntheticManager{}
}

func (emptySyntheticManager) CreateSyntheticTests(rawJobResults testgridanalysisapi.RawData) []string {
	return []string{}
}

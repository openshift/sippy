package synthetictests

import (
	"github.com/openshift/sippy/pkg/apis/junit"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
)

type SyntheticTestManager interface {
	CreateSyntheticTests(jobResults testgridanalysisapi.RawJobRunResult) []junit.TestCase
}

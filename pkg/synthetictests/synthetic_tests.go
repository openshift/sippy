package synthetictests

import (
	"github.com/openshift/sippy/pkg/apis/junit"
	"github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

type SyntheticTestManager interface {
	CreateSyntheticTests(jrr *v1.RawJobRunResult) *junit.TestSuite
}

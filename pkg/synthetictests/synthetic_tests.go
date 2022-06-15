package synthetictests

import (
	"github.com/openshift/sippy/pkg/apis/junit"
	v1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
)

type SyntheticTestManager interface {
	CreateSyntheticTests(jrr *v1.RawJobRunResult) *junit.TestSuite
}

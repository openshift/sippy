package synthetictests

import (
	"github.com/openshift/sippy/pkg/apis/junit"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
)

type kubeSyntheticManager struct{}

func NewEmptySyntheticTestManager() SyntheticTestManager {
	return kubeSyntheticManager{}
}

func (k kubeSyntheticManager) CreateSyntheticTests(jrr *testgridanalysisapi.RawJobRunResult) *junit.TestSuite {
	jrr.OverallResult = kubeJobRunStatus(jrr)
	return &junit.TestSuite{
		Name: testgridanalysisapi.SippySuiteName,
	}
}

func kubeJobRunStatus(result *testgridanalysisapi.RawJobRunResult) sippyprocessingv1.JobOverallResult {
	if result.Succeeded {
		return sippyprocessingv1.JobSucceeded
	}

	if !result.Failed {
		return sippyprocessingv1.JobRunning
	}

	if result.InstallStatus == failure {
		return sippyprocessingv1.JobInstallFailure
	}

	if result.Failed {
		return sippyprocessingv1.JobTestFailure
	}

	return sippyprocessingv1.JobUnknown
}

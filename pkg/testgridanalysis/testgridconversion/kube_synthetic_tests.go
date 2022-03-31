package testgridconversion

import (
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridanalysisapi"
)

type kubeSyntheticManager struct{}

func NewEmptySyntheticTestManager() SyntheticTestManager {
	return kubeSyntheticManager{}
}

func (k kubeSyntheticManager) CreateSyntheticTests(rawJobResults testgridanalysisapi.RawData) []string {
	warnings := []string{}

	for jobName, jobResults := range rawJobResults.JobResults {
		newWarnings := k.CreateSyntheticTestsForJob(jobResults)
		warnings = append(warnings, newWarnings...)
		rawJobResults.JobResults[jobName] = jobResults
	}
	return warnings
}

func (k kubeSyntheticManager) CreateSyntheticTestsForJob(jobResults testgridanalysisapi.RawJobResult) []string {
	// Kube does not use any synthetic tests, but we do need to populate the job OverallResult for important functionality.
	for jrrKey, jrr := range jobResults.JobRunResults {
		jrr.OverallResult = kubeJobRunStatus(jrr)
		jobResults.JobRunResults[jrrKey] = jrr
	}
	return []string{}
}

func kubeJobRunStatus(result testgridanalysisapi.RawJobRunResult) sippyprocessingv1.JobOverallResult {
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

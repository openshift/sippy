package sippyserver

import (
	"regexp"
	"time"

	testgridv1 "github.com/openshift/sippy/pkg/apis/testgrid/v1"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridhelpers"
)

// Allows one to pass in an alternative testgrid loader func for testing.
type TestGridLoader func(string, []string, *regexp.Regexp) ([]testgridv1.JobDetails, time.Time)

// TestGridLoadingOptions control the data which is loaded from disk into the testgrid structs
type TestGridLoadingConfig struct {
	// LocalData is the directory where the testgrid data is stored
	LocalData string
	// JobFilter is a regex run against job names. Only match names are loaded.
	JobFilter *regexp.Regexp
	// The function to load TestGrid results from disk, used for testing.
	Loader TestGridLoader
}

func (t TestGridLoadingConfig) loadWithFilter(dashboards []string, jobFilter *regexp.Regexp) ([]testgridv1.JobDetails, time.Time) {
	// If TestGridLoader isn't defined, use the default one
	if t.Loader == nil {
		return testgridhelpers.LoadTestGridDataFromDisk(t.LocalData, dashboards, jobFilter)
	}

	return t.Loader(t.LocalData, dashboards, jobFilter)
}

func (t TestGridLoadingConfig) load(dashboards []string) ([]testgridv1.JobDetails, time.Time) {
	return t.loadWithFilter(dashboards, t.JobFilter)
}

// RawJobResultsAnalysisOptions control which subset of data from the testgrid data is analyzed into the rawJobResults
type RawJobResultsAnalysisConfig struct {
	StartDay int
	NumDays  int
}

// DisplayDataOptions controls how the RawJobResults are processed and prepared for display
type DisplayDataConfig struct {
	MinTestRuns             int
	TestSuccessThreshold    float64
	FailureClusterThreshold int
}

// TestReportGeneratorConfig is a static configuration that can be re-used across multiple invocations of PrepareTestReport with different versions
type TestReportGeneratorConfig struct {
	TestGridLoadingConfig       TestGridLoadingConfig
	RawJobResultsAnalysisConfig RawJobResultsAnalysisConfig
	DisplayDataConfig           DisplayDataConfig
}

func (a TestReportGeneratorConfig) deepCopy() TestReportGeneratorConfig {
	ret := TestReportGeneratorConfig{
		TestGridLoadingConfig: TestGridLoadingConfig{
			LocalData: a.TestGridLoadingConfig.LocalData,
		},
		RawJobResultsAnalysisConfig: RawJobResultsAnalysisConfig{
			StartDay: a.RawJobResultsAnalysisConfig.StartDay,
			NumDays:  a.RawJobResultsAnalysisConfig.NumDays,
		},
		DisplayDataConfig: DisplayDataConfig{
			MinTestRuns:             a.DisplayDataConfig.MinTestRuns,
			TestSuccessThreshold:    a.DisplayDataConfig.TestSuccessThreshold,
			FailureClusterThreshold: a.DisplayDataConfig.FailureClusterThreshold,
		},
	}
	if a.TestGridLoadingConfig.JobFilter != nil {
		//nolint:staticcheck // Copy() is appropriate since this performs a deep copy.
		ret.TestGridLoadingConfig.JobFilter = a.TestGridLoadingConfig.JobFilter.Copy()
	}

	return ret
}

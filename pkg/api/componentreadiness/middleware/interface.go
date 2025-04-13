package middleware

import (
	"context"
	"sync"

	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
)

// Middleware interface defines the available integration points for complex features
// being added to component readiness. It's important to note that the interface covers
// both major code paths through, component reports, and test details reports.
type Middleware interface {
	// Query phase allows middleware to inject additional TestStatus beyond the normal base/sample queries.
	// Base and sample status can be submitted using the provided channels for a map of ALL test keys
	// (ID plus variant info serialized) to TestStatus.
	Query(ctx context.Context, wg *sync.WaitGroup, allJobVariants crtype.JobVariants,
		baseStatusCh, sampleStatusCh chan map[string]crtype.TestStatus, errCh chan error)

	// QueryTestDetails phase allow middleware to load data that will later be used.
	QueryTestDetails(ctx context.Context, wg *sync.WaitGroup, errCh chan error, allJobVariants crtype.JobVariants)

	// Transform gives middleware opportunity to adjust test data prior to running analysis.
	// proceed to analysis.
	Transform(testKey crtype.ReportTestIdentification, testStats *crtype.ReportTestStats) error

	// TransformTestDetails gives middleware opportunity to adjust the queried base and sample job run data
	// before we proceed to analysis.
	TransformTestDetails(status *crtype.JobRunTestReportStatus) error

	// TestDetailsAnalyze gives middleware opportunity to analyze data and adjust the final report before
	// being returned over the API.
	TestDetailsAnalyze(report *crtype.ReportTestDetails) error
}

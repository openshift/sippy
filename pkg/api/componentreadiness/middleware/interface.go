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

	// PreAnalysis gives middleware opportunity to adjust test analysis data prior to running analysis.
	// Implementations can alter base/sample data as needed, request confidence levels, and add explanations for
	// what they did.
	// NOTE: due to differences in test details reports, this function is not used there.
	PreAnalysis(testKey crtype.ReportTestIdentification, testStats *crtype.ReportTestStats) error

	// PostAnalysis gives middleware opportunity to adjust test analysis data after running the core of the analysis.
	// Implementations can alter Status code and add explanations for what they did.
	// Used in both ComponentReport and TestDetails.
	PostAnalysis(testKey crtype.ReportTestIdentification, testStats *crtype.ReportTestStats) error

	// PreTestDetailsAnalysis gives middleware the opportunity to adjust inputs to the report status
	// prior to analysis.
	PreTestDetailsAnalysis(status *crtype.TestJobRunStatuses) error
}

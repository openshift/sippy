package middleware

import (
	"context"
	"sync"

	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
)

type Middleware interface {
	// Query phase allows middleware to inject additional TestStatus beyond the normal base/sample queries.
	// Base and sample status can be submitted using the provided channels for a map of ALL test keys
	// (ID plus variant info serialized) to TestStatus.
	Query(ctx context.Context, wg *sync.WaitGroup, allJobVariants crtype.JobVariants,
		baseStatusCh, sampleStatusCh chan map[string]crtype.TestStatus, errCh chan error)

	// QueryTestDetails phase allow middleware to load data that will later be used.
	QueryTestDetails(ctx context.Context, wg *sync.WaitGroup, errCh chan error, allJobVariants crtype.JobVariants)

	// Transform gives middleware opportunity to adjust the queried base and sample TestStatuses before we
	// proceed to analysis.
	Transform(status *crtype.ReportTestStatus) error

	TransformTestDetails(status *crtype.JobRunTestReportStatus) error
}

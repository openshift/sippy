package middleware

import (
	"context"
	"sync"

	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
)

type Middleware interface {
	// Query phase allows middleware to inject additional TestStatus beyond the normal base/sample queries.
	// TODO: pass channels for submitting base/sample status, will be needed for rarely run jobs to become middleware
	Query(ctx context.Context, wg *sync.WaitGroup, allJobVariants crtype.JobVariants) error

	// Transform gives middleware opportunity to adjust the queried base and sample TestStatuses before we
	// proceed to analysis.
	Transform(baseStatus, sampleStatus map[string]crtype.TestStatus) (map[string]crtype.TestStatus, map[string]crtype.TestStatus, error)
}

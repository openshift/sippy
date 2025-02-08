package middleware

import (
	"context"
	"sync"

	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
)

type Middleware interface {
	Query(ctx context.Context, wg *sync.WaitGroup, allJobVariants crtype.JobVariants)
}

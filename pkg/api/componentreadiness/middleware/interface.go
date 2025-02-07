package middleware

import (
	"context"
	"sync"
)

type Middleware interface {
	Query(ctx context.Context, wg *sync.WaitGroup)
}

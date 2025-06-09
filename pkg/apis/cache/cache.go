package cache

import (
	"context"
	"net/http"
	"time"
)

type Cache interface {
	Get(ctx context.Context, key string, duration time.Duration) ([]byte, error)
	Set(ctx context.Context, key string, content []byte, duration time.Duration) error
}

type APIResponse struct {
	Headers  http.Header
	Response []byte
}

// RequestOptions specifies options for an individual
// request, such as forcing the cache to be bypassed.
type RequestOptions struct {
	ForceRefresh bool
	// CRTimeRoundingFactor is used to calculate cache expiration time
	CRTimeRoundingFactor time.Duration
	// SkipCacheWrites will disable setting keys in the cache. Used in some scenarios where a lot of data is in play and serves no purpose being in the cache.
	SkipCacheWrites bool
}

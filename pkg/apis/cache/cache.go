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
	// Expiry is how long to set TTL, unless modified by other factors
	Expiry time.Duration

	// CRTimeRoundingFactor is used to round cache expiration time to the nearest time boundary of blocks that size.
	// for example, when set to 12 hours, the day is divided into 12-hour blocks and the TTL will be at the next boundary.
	CRTimeRoundingFactor time.Duration
	// CRTimeRoundingOffset shifts the rounding boundaries by this duration.
	// For example, 4h shifts 12h boundaries from 00:00/12:00 UTC to 04:00/16:00 UTC.
	CRTimeRoundingOffset time.Duration

	// SkipCacheWrites will disable setting keys in the cache. Used in some scenarios where a lot of data is in play and serves no purpose being in the cache.
	SkipCacheWrites bool

	// StableExpiry specifies how long to cache data that is "stable" - older than StableAge (if an age is given)
	StableExpiry time.Duration
	StableAge    time.Duration
	// ForceRefresh when set means: do not read from cache, generate fresh data and cache it
	ForceRefresh bool
	// RefreshRecent indicates a more discriminating approach to ForceRefresh.
	// When set, queries that provide a data end date will refresh if that end date is newer than "StableAge".
	RefreshRecent bool
}

var StandardStableAgeCR = time.Hour * 24 * 7    // age at which component readiness data should be considered "stable"
var StandardStableExpiryCR = time.Hour * 24 * 7 // how long to cache stable component readiness data
func NewStandardCROptions(crTimeRoundingFactor, crTimeRoundingOffset time.Duration) RequestOptions {
	return RequestOptions{
		CRTimeRoundingFactor: crTimeRoundingFactor,
		CRTimeRoundingOffset: crTimeRoundingOffset,
		StableAge:            StandardStableAgeCR,
		StableExpiry:         StandardStableExpiryCR,
	}
}

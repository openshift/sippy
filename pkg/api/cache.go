package api

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var defaultCacheDuration = 8 * time.Hour

// CacheSpec specifies caching parameters for an individual query
type CacheSpec struct {
	// function to turn the specified key struct into cacheable bytes
	cacheKey func() ([]byte, error)
	// if specified, the end of the date range that the query covers
	queryEndDate *time.Time
}

// NewCacheSpec specifies caching parameters to be used in caching an individual query.
// The cacheKey can be any struct with public fields that identifies the query parameters for caching.
// The prefix should be set to distinguish multiple queries that use the same cache keys.
// The queryEndDate can be set to enable distinguishing whether the query data is considered stable for longer caching.
func NewCacheSpec(cacheKey interface{}, prefix string, queryEndDate *time.Time) CacheSpec {
	if isStructWithNoPublicFields(cacheKey) {
		panic(fmt.Sprintf("you cannot use struct %s with no exported fields as a cache key", reflect.TypeOf(cacheKey)))
	}

	return CacheSpec{
		queryEndDate: queryEndDate,
		cacheKey: func() ([]byte, error) {
			b, err := json.Marshal(cacheKey)
			if err != nil {
				return nil, err
			}
			if len(prefix) > 0 {
				return append([]byte(prefix), b...), nil
			}
			return b, nil
		}}
}

func (c *CacheSpec) GetCacheKey() ([]byte, error) {
	if c.cacheKey == nil {
		panic("cache key is nil")
	}
	return c.cacheKey()
}

// HasStableData determines whether this individual query's end date is old enough (per general cacheOpts) to be considered stable and unchanging.
func (c *CacheSpec) HasStableData(cacheOpts cache.RequestOptions) bool {
	if c.queryEndDate == nil || cacheOpts.StableAge == 0 {
		return false
	}
	return c.queryEndDate.Add(cacheOpts.StableAge).Before(time.Now())
}

// GetDataFromCacheOrGenerate attempts to find a cached record otherwise generates new data.
func GetDataFromCacheOrGenerate[T any](
	ctx context.Context, c cache.Cache, cacheOptions cache.RequestOptions, cacheSpec CacheSpec,
	generateFn func(context.Context) (T, []error),
	defaultVal T,
) (T, []error) {
	if c != nil {
		cacheKey, err := cacheSpec.GetCacheKey()
		if err != nil {
			return defaultVal, []error{err}
		}

		// If someone is giving us an uncacheable cacheKey, we should panic so it gets detected in testing
		if len(cacheKey) == 0 {
			panic(fmt.Sprintf("cache key is empty for %s", reflect.TypeOf(defaultVal)))
		}

		cacheDuration := CalculateRoundedCacheDuration(cacheOptions)
		hasStableData := cacheSpec.HasStableData(cacheOptions)
		if cacheOptions.StableExpiry != 0 && hasStableData {
			// if we're querying against older data, it probably won't change. so make the cache last longer
			// so that we don't spend a ton of BQ quota querying the same data repeatedly.
			cacheDuration = cacheOptions.StableExpiry
		}
		logrus.Debugf("cache duration set to %s or approx %s for key %s", cacheDuration, time.Now().Add(cacheDuration).Format(time.RFC3339), cacheKey)

		refreshRecent := cacheOptions.RefreshRecent && !hasStableData
		if !cacheOptions.ForceRefresh && !refreshRecent {
			if res, err := c.Get(ctx, string(cacheKey), cacheDuration); err == nil {
				logrus.WithFields(logrus.Fields{
					"key":  string(cacheKey),
					"type": reflect.TypeOf(defaultVal).String(),
				}).Infof("cache hit")
				var cr T
				if err := json.Unmarshal(res, &cr); err != nil {
					return defaultVal, []error{errors.WithMessagef(err, "failed to unmarshal cached item.  cacheKey=%+v", cacheKey)}
				}
				return cr, nil
			} else if strings.Contains(err.Error(), "connection refused") {
				logrus.WithError(err).Fatalf("redis URL specified but got connection refused, exiting due to cost issues in this configuration")
			}
			logrus.WithFields(logrus.Fields{
				"key": string(cacheKey),
			}).Infof("cache miss")
		}

		// Cache has missed or we're deliberately refreshing the data:
		result, errs := generateFn(ctx)
		if len(errs) == 0 && !cacheOptions.SkipCacheWrites {
			CacheSet(ctx, c, result, cacheKey, cacheDuration)
		}
		return result, errs
	}

	return generateFn(ctx)
}

func CacheSet[T any](ctx context.Context, c cache.Cache, result T, cacheKey []byte, cacheDuration time.Duration) {
	cr, err := json.Marshal(result)
	if err == nil {
		if err := c.Set(ctx, string(cacheKey), cr, cacheDuration); err != nil {
			if strings.Contains(err.Error(), "connection refused") {
				logrus.WithError(err).Fatalf("redis URL specified but got connection refused, exiting due to cost issues in this configuration")
			}
			logrus.WithError(err).Warningf("couldn't persist new item to cache")
		} else {
			logrus.Debugf("cache set for cache key: %s", string(cacheKey))
		}
	} else {
		logrus.WithError(err).Errorf("Failed to marshall cache item: %v", result)
	}
}

func CalculateRoundedCacheDuration(cacheOptions cache.RequestOptions) time.Duration {
	// require cacheDuration for persistence logic
	cacheDuration := defaultCacheDuration
	if cacheOptions.CRTimeRoundingFactor > 0 {
		now := time.Now().UTC()
		// Only cache until the next rounding duration
		cacheDuration = now.Truncate(cacheOptions.CRTimeRoundingFactor).Add(cacheOptions.CRTimeRoundingFactor).Sub(now)
	}
	return cacheDuration
}

// isStructWithNoPublicFields checks if the given interface is a struct with no public fields.
func isStructWithNoPublicFields(v interface{}) bool {
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Struct {
		return false
	}
	for i := 0; i < val.NumField(); i++ {
		if val.Type().Field(i).IsExported() {
			return false
		}
	}
	return true
}

// GetDataFromCacheOrMatview caches data that is based on a matview and invalidates it when the matview is refreshed
func GetDataFromCacheOrMatview[T any](ctx context.Context,
	cacheClient cache.Cache, cacheSpec CacheSpec,
	matview string,
	cacheDuration time.Duration,
	generateFn func(context.Context) (T, []error),
	defaultVal T,
) (T, []error) {
	if cacheClient == nil {
		return generateFn(ctx)
	}

	cacheKey, err := cacheSpec.GetCacheKey()
	if err != nil {
		return defaultVal, []error{err}
	}

	// If someone gives us an uncacheable cacheKey, panic so it gets detected in testing
	if len(cacheKey) == 0 {
		panic(fmt.Sprintf("cache key is empty for %s", reflect.TypeOf(defaultVal)))
	}
	// If someone gives us an uncacheable value, panic so it gets detected in testing
	if isStructWithNoPublicFields(defaultVal) {
		panic(fmt.Sprintf("cannot cache type %s that exports no fields", reflect.TypeOf(defaultVal)))
	}

	var cacheVal struct {
		Val       T         // the actual value we want to cache
		Timestamp time.Time // the time when it was cached (for comparison to matview refresh time)
	}
	if cached, err := cacheClient.Get(ctx, string(cacheKey), 0); err == nil {
		logrus.WithFields(logrus.Fields{
			"key":  string(cacheKey),
			"type": reflect.TypeOf(defaultVal).String(),
		}).Debugf("cache hit")

		if err := json.Unmarshal(cached, &cacheVal); err != nil {
			return defaultVal, []error{errors.WithMessagef(err, "failed to unmarshal cached item.  cacheKey=%+v", cacheKey)}
		}

		// look up when the matview was refreshed to see if the cached value is stale
		var lastRefresh time.Time
		if lastRefreshBytes, err := cacheClient.Get(ctx, RefreshMatviewKey(matview), 0); err == nil {
			if parsed, err := time.Parse(time.RFC3339, string(lastRefreshBytes)); err != nil {
				logrus.WithError(err).Warnf("failed to parse matview refresh timestamp %q for %q; cache will not be invalidated", lastRefreshBytes, matview)
			} else {
				lastRefresh = parsed
			}
		}

		if lastRefresh.Before(cacheVal.Timestamp) {
			// not invalidated by a newer refresh, so use it (if we don't know the last refresh, still use it)
			return cacheVal.Val, nil
		}
		logrus.Debugf("matview %q refreshed at %v, will not use earlier cache entry from %v", matview, lastRefresh, cacheVal.Timestamp)
	} else if strings.Contains(err.Error(), "connection refused") {
		logrus.WithError(err).Fatalf("redis URL specified but got connection refused; exiting due to cost issues in this configuration")
	} else {
		logrus.WithFields(logrus.Fields{"key": string(cacheKey)}).Debugf("cache miss")
	}

	// Cache missed or refresh invalidated the data, so generate it.
	logrus.Debugf("cache duration set to %s or approx %s for key %s", cacheDuration, time.Now().Add(cacheDuration).Format(time.RFC3339), cacheKey)
	result, errs := generateFn(ctx)
	if len(errs) == 0 {
		cacheVal.Val = result
		cacheVal.Timestamp = time.Now().UTC()
		CacheSet(ctx, cacheClient, cacheVal, cacheKey, cacheDuration)
	}
	return result, errs
}

func RefreshMatviewKey(matview string) string {
	return "matview_refreshed:" + matview
}

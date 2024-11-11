package flags

import (
	"os"
	"time"

	"github.com/openshift/sippy/pkg/bigquery"

	"github.com/openshift/sippy/pkg/cache/bigquerycache"
	"github.com/spf13/pflag"

	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/openshift/sippy/pkg/cache/redis"
)

// CacheFlags holds caching configuration information for Sippy.
type CacheFlags struct {
	RedisURL                   string
	PersistentCacheDurationMax time.Duration
	EnablePersistentCacheWrite bool
	EnablePersistentCaching    bool
}

func NewCacheFlags() *CacheFlags {
	return &CacheFlags{}
}

func (f *CacheFlags) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&f.RedisURL,
		"redis-url",
		os.Getenv("REDIS_URL"),
		"Redis URL for caching")

	fs.BoolVar(&f.EnablePersistentCaching,
		"enable-persistent-cache",
		false,
		"Enable pesisted cache storage")

	fs.DurationVar(&f.PersistentCacheDurationMax,
		"persistent-cache-duration-max",
		time.Hour*24,
		"Maximum duration before a cache item is considered expired")

	// if we are running main sippy in RO we can't persist to bigquery
	// we can have regression tracking priming the cache
	// if we run it with a shorter PersistentCacheDuration
	// so it will regenerate before main sippy would need it
	fs.BoolVar(&f.EnablePersistentCacheWrite,
		"enable-persistent-cache-write",
		false,
		"Boolean indicating if the cache should attempt to writ or is read only")
}

func (f *CacheFlags) GetCacheClient() (cache.Cache, error) {
	if f.RedisURL != "" {
		return redis.NewRedisCache(f.RedisURL)
	}

	return nil, nil
}

func (f *CacheFlags) GetPersistentCacheClient(bqclient *bigquery.Client) (cache.Cache, error) {
	if f.EnablePersistentCaching {
		return bigquerycache.NewBigQueryCache(bqclient, f.PersistentCacheDurationMax, !f.EnablePersistentCacheWrite)
	}

	return nil, nil
}

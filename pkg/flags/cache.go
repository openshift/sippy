package flags

import (
	"os"
	"time"

	log "github.com/sirupsen/logrus"

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
	PersistentCacheDurationMin time.Duration
	EnablePersistentCacheWrite bool
	EnablePersistentCaching    bool
	ForcePersistentLookup      bool
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
		"Enable persisted cache storage")

	fs.BoolVar(&f.ForcePersistentLookup,
		"force-persistent-cache-lookup",
		false,
		"Skips any intermediate caching like redis and queries the persisted cache")

	fs.DurationVar(&f.PersistentCacheDurationMax,
		"persistent-cache-duration-max",
		time.Hour*24,
		"Maximum duration before a cache item is considered expired")

	fs.DurationVar(&f.PersistentCacheDurationMin,
		"persistent-cache-duration-min",
		time.Hour*12,
		"Minimum duration for persisting a cache item")

	// if we are running main sippy in RO we can't persist to bigquery
	// we can have regression tracking priming the cache
	// if we run it with a shorter PersistentCacheDuration
	// so it will regenerate before main sippy would need it
	fs.BoolVar(&f.EnablePersistentCacheWrite,
		"enable-persistent-cache-write",
		false,
		"Boolean indicating if the cache should attempt to write or is read only")
}

func (f *CacheFlags) GetCacheClient() (cache.Cache, error) {
	if f.RedisURL != "" {
		return redis.NewRedisCache(f.RedisURL)
	}

	return nil, nil
}

func (f *CacheFlags) GetPersistentCacheClient(bqclient *bigquery.Client) (cache.Cache, error) {
	if f.EnablePersistentCaching {
		// create a clone of the bigquery client to preserve the current cache
		return bigquerycache.NewBigQueryCache(&bigquery.Client{
			BQ:      bqclient.BQ,
			Cache:   bqclient.Cache,
			Dataset: bqclient.Dataset,
		}, f.PersistentCacheDurationMax, f.PersistentCacheDurationMin, !f.EnablePersistentCacheWrite, f.ForcePersistentLookup)
	}

	return nil, nil
}

func (f *CacheFlags) DecorateBiqQueryClientWithPersistentCache(bigQueryClient *bigquery.Client) *bigquery.Client {
	persistentCacheClient, err := f.GetPersistentCacheClient(bigQueryClient)
	if err != nil {
		log.WithError(err).Warn("couldn't get persistent cache client")
	} else if persistentCacheClient != nil {
		// we want to change out the cache with the persistent version
		// the persistent cache retains the original cache client if any
		bigQueryClient.Cache = persistentCacheClient
	}
	return bigQueryClient
}

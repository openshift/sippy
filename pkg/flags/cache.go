package flags

import (
	"fmt"
	"os"

	"github.com/spf13/pflag"

	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/openshift/sippy/pkg/cache/redis"
)

// CacheFlags holds caching configuration information for Sippy such as the location
// of its configuration file.
type CacheFlags struct {
	RedisURL string
}

func NewCacheFlags() *CacheFlags {
	return &CacheFlags{}
}

func (f *CacheFlags) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&f.RedisURL,
		"redis-url",
		os.Getenv("REDIS_URL"),
		"Redis URL for caching")
}

func (f *CacheFlags) GetCacheClient() (cache.Cache, error) {
	if f.RedisURL != "" {
		return redis.NewRedisCache(f.RedisURL)
	}

	return nil, fmt.Errorf("no redis URL was specified")
}

package bigquery

import (
	"testing"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
)

func TestJobVariantsCacheOptionsPreserveReadOnlyComparison(t *testing.T) {
	options := reqopts.RequestOptions{}
	options.CacheOption.ForceRefresh = true
	options.CacheOption.SkipCacheWrites = true

	cacheOptions := jobVariantsCacheOptions(options)
	if !cacheOptions.ForceRefresh || !cacheOptions.SkipCacheWrites {
		t.Fatalf("job variants cache options = %#v, want forced fresh read without cache writes", cacheOptions)
	}
}

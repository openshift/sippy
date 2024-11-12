package api

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/pkg/errors"

	log "github.com/sirupsen/logrus"

	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/cache"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	bqclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/util/sets"
)

var (
	defaultCacheDuration = 8 * time.Hour
)

const releasePresubmits = "Presubmits"

type CacheData struct {
	cacheKey func() ([]byte, error)
}

func (c *CacheData) GetCacheKey() ([]byte, error) {
	if c.cacheKey == nil {
		panic("cache key is nil")
	}
	return c.cacheKey()
}

func GetPrefixedCacheKey(prefix string, cacheKey interface{}) CacheData {
	if isStructWithNoPublicFields(cacheKey) {
		panic(fmt.Sprintf("you cannot use struct %s with no exported fields as a cache key", reflect.TypeOf(cacheKey)))
	}

	return CacheData{cacheKey: func() ([]byte, error) {
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

// GetDataFromCacheOrGenerate attempts to find a cached record otherwise generates new data.
func GetDataFromCacheOrGenerate[T any](
	ctx context.Context, c cache.Cache, cacheOptions cache.RequestOptions, cacheData CacheData,
	generateFn func(context.Context) (T, []error),
	defaultVal T,
) (T, []error) {
	if c != nil {
		cacheKey, err := cacheData.GetCacheKey()
		if err != nil {
			return defaultVal, []error{err}
		}

		// If someone is giving us an uncacheable cacheKey, we should panic so it gets detected in testing
		if len(cacheKey) == 0 {
			panic(fmt.Sprintf("cache key is empty for %s", reflect.TypeOf(defaultVal)))
		}

		// require cacheDuration for persistence logic
		cacheDuration := defaultCacheDuration
		if cacheOptions.CRTimeRoundingFactor > 0 {
			now := time.Now().UTC()
			// Only cache until the next rounding duration
			cacheDuration = now.Truncate(cacheOptions.CRTimeRoundingFactor).Add(cacheOptions.CRTimeRoundingFactor).Sub(now)
		}

		if !cacheOptions.ForceRefresh {
			if res, err := c.Get(ctx, string(cacheKey), cacheDuration); err == nil {
				log.WithFields(log.Fields{
					"key":  string(cacheKey),
					"type": reflect.TypeOf(defaultVal).String(),
				}).Infof("cache hit")
				var cr T
				if err := json.Unmarshal(res, &cr); err != nil {
					return defaultVal, []error{errors.WithMessagef(err, "failed to unmarshal cached item.  cacheKey=%+v", cacheKey)}
				}
				return cr, nil
			}
			log.WithFields(log.Fields{
				"key": string(cacheKey),
			}).Infof("cache miss")
		}
		result, errs := generateFn(ctx)
		if len(errs) == 0 {
			cr, err := json.Marshal(result)
			if err == nil {
				if err := c.Set(ctx, string(cacheKey), cr, cacheDuration); err != nil {
					log.WithError(err).Warningf("couldn't persist new item to cache")
				} else {
					log.Debugf("cache set for cache key: %s", string(cacheKey))
				}
			} else {
				log.WithError(err).Errorf("Failed to marshall cache item: %v", result)
			}
		}
		return result, errs
	}

	return generateFn(ctx)
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

type releaseGenerator struct {
	client *bqclient.Client
}

func (r *releaseGenerator) ListReleases(ctx context.Context) ([]v1.Release, []error) {
	releases, err := GetReleasesFromBigQuery(ctx, r.client)
	if err != nil {
		log.WithError(err).Error("error getting releases from bigquery")
		return releases, []error{err}
	}
	// Add special release Presubmits for prow jobs
	releases = append(releases, v1.Release{Release: releasePresubmits})
	return releases, nil
}

// GetReleases gets all the releases defined in the BQ Releases table.
func GetReleases(ctx context.Context, bqc *bqclient.Client) ([]v1.Release, error) {
	releaseGen := releaseGenerator{bqc}

	var err error
	rels, errs := GetDataFromCacheOrGenerate[[]v1.Release](
		ctx,
		bqc.Cache,
		cache.RequestOptions{},
		GetPrefixedCacheKey("Releases~", v1.Release{}), // no cache options needed here, global list
		releaseGen.ListReleases,
		[]v1.Release{})
	if len(errs) > 0 {
		err = errs[0]
	}
	return rels, err
}

// VariantsStringToSet converts comma separated variant string into a set
func VariantsStringToSet(allJobVariants crtype.JobVariants, variantsString string) (sets.String, error) {
	variantSet := sets.String{}
	variants := strings.Split(variantsString, ",")
	for _, v := range variants {
		if _, ok := allJobVariants.Variants[v]; !ok {
			return variantSet, fmt.Errorf("invalid variant %s in variants string %s", v, variantsString)
		}
		variantSet.Insert(v)
	}
	return variantSet, nil
}

func VariantListToMap(allJobVariants crtype.JobVariants, variants []string) (map[string][]string, error) {
	variantsMap := map[string][]string{}
	var err error
	for _, variant := range variants {
		kv := strings.Split(variant, ":")
		if len(kv) != 2 {
			err = fmt.Errorf("invalid variant %s in list", variant)
			return variantsMap, err
		}
		values, ok := allJobVariants.Variants[kv[0]]
		if !ok {
			err = fmt.Errorf("invalid name from list variant %s", variant)
			return variantsMap, err
		}
		found := false
		for _, v := range values {
			if v == kv[1] {
				variantsMap[kv[0]] = append(variantsMap[kv[0]], kv[1])
				found = true
				break
			}
		}
		if !found {
			err = fmt.Errorf("invalid value from list variant %s", variant)
			return variantsMap, err
		}
	}
	return variantsMap, err
}

// CleanseSQLName removes all non-alphanumeric characters from a string that could be used as a SQL name (table, column, etc)
// This is useful for sanitizing dynamic queries built from user input.
func CleanseSQLName(name string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return -1
	}, name)
}

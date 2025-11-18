package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/pkg/errors"

	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/apis/cache"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	bqclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/util/sets"
)

var (
	defaultCacheDuration = 8 * time.Hour
)

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

		cacheDuration := CalculateRoundedCacheDuration(cacheOptions)
		log.Debugf("cache duration set to %s or approx %s for key %s", cacheDuration, time.Now().Add(cacheDuration).Format(time.RFC3339), cacheKey)

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
			} else if strings.Contains(err.Error(), "connection refused") {
				log.WithError(err).Fatalf("redis URL specified but got connection refused, exiting due to cost issues in this configuration")
			}
			log.WithFields(log.Fields{
				"key": string(cacheKey),
			}).Infof("cache miss")
		}

		// Cache has missed or we're explicitly forcing a refresh:
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
				log.WithError(err).Fatalf("redis URL specified but got connection refused, exiting due to cost issues in this configuration")
			}
			log.WithError(err).Warningf("couldn't persist new item to cache")
		} else {
			log.Debugf("cache set for cache key: %s", string(cacheKey))
		}
	} else {
		log.WithError(err).Errorf("Failed to marshall cache item: %v", result)
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

type releaseGenerator struct {
	client *bqclient.Client
}

func (r *releaseGenerator) ListReleases(ctx context.Context) ([]v1.Release, []error) {
	releases, err := GetReleasesFromBigQuery(ctx, r.client)
	if err != nil {
		log.WithError(err).Error("error getting releases from bigquery")
		return releases, []error{err}
	}
	return releases, nil
}

// GetReleases gets all the releases defined in the BQ Releases table.
func GetReleases(ctx context.Context, bqc *bqclient.Client, forceRefresh bool) ([]v1.Release, error) {
	releaseGen := releaseGenerator{bqc}

	var err error
	rels, errs := GetDataFromCacheOrGenerate[[]v1.Release](
		ctx,
		bqc.Cache,
		cache.RequestOptions{ForceRefresh: forceRefresh},
		GetPrefixedCacheKey("Releases~", v1.Release{}), // no cache options needed here, global list
		releaseGen.ListReleases,
		[]v1.Release{})
	if len(errs) > 0 {
		err = errs[0]
	}
	return rels, err
}

// VariantsStringToSet converts comma separated variant string into a set; also validates that the variants are known
func VariantsStringToSet(allJobVariants crtest.JobVariants, variantsString string) (sets.String, error) {
	variantSet := sets.String{}
	variants := strings.Split(variantsString, ",")
	for _, v := range variants {
		// ensure the variant is one we've recorded in BQ, not just some random string
		if _, ok := allJobVariants.Variants[v]; !ok {
			return variantSet, fmt.Errorf("invalid variant %s in variants string %s", v, variantsString)
		}
		variantSet.Insert(v)
	}
	return variantSet, nil
}

// VariantListToMap collects a list of variants like "Architecture:amd64" into a map [Architecture -> amd64];
// it also validates that the variants are known
func VariantListToMap(allJobVariants crtest.JobVariants, variants []string) (map[string][]string, error) {
	variantsMap := map[string][]string{}
	var err error
	for _, variant := range variants {
		kv := strings.Split(variant, ":")
		if len(kv) != 2 {
			err = fmt.Errorf("invalid variant %s in list", variant)
			return variantsMap, err
		}
		// ensure the variant name/value is one we've recorded in BQ, not just some random string
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

// ValidateVariants checks if variant names and values exist in BigQuery and returns warnings for any that don't.
// The source parameter is appended to warning messages to indicate where the variants came from (e.g., " from view").
func ValidateVariants(allJobVariants crtest.JobVariants, variantsMap map[string][]string, source string) []string {
	var warnings []string
	for variantName, variantValues := range variantsMap {
		validValues, ok := allJobVariants.Variants[variantName]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("Unknown variant name: %s%s", variantName, source))
			continue
		}

		// Build a set for O(1) lookup instead of O(n) iteration
		validSet := make(map[string]bool, len(validValues))
		for _, v := range validValues {
			validSet[v] = true
		}

		for _, variantValue := range variantValues {
			if !validSet[variantValue] {
				warnings = append(warnings, fmt.Sprintf("Unknown variant value: %s (for variant %s%s)", variantValue, variantName, source))
			}
		}
	}
	return warnings
}

// VariantListToMapWithWarnings collects a list of variants like "Architecture:amd64" into a map [Architecture -> amd64];
// it validates that the variants are known, but collects warnings instead of failing for invalid variants
func VariantListToMapWithWarnings(allJobVariants crtest.JobVariants, variants []string) (map[string][]string, []string, error) {
	variantsMap := map[string][]string{}
	for _, variant := range variants {
		kv := strings.Split(variant, ":")
		if len(kv) != 2 {
			// This is a fatal error as the format is completely wrong
			return variantsMap, nil, fmt.Errorf("invalid variant %s in list", variant)
		}
		variantsMap[kv[0]] = append(variantsMap[kv[0]], kv[1])
	}

	// Validate all variants and collect warnings
	warnings := ValidateVariants(allJobVariants, variantsMap, "")

	// Remove invalid variants from the map
	for variantName, variantValues := range variantsMap {
		validValues, ok := allJobVariants.Variants[variantName]
		if !ok {
			// Remove entire variant name if it doesn't exist
			delete(variantsMap, variantName)
			continue
		}

		// Build a set for O(1) lookup
		validSet := make(map[string]bool, len(validValues))
		for _, v := range validValues {
			validSet[v] = true
		}

		// Filter out invalid values
		filteredValues := make([]string, 0, len(variantValues))
		for _, variantValue := range variantValues {
			if validSet[variantValue] {
				filteredValues = append(filteredValues, variantValue)
			}
		}

		if len(filteredValues) > 0 {
			variantsMap[variantName] = filteredValues
		} else {
			delete(variantsMap, variantName)
		}
	}

	return variantsMap, warnings, nil
}

// GetBaseURL returns the base URL (protocol + host) from the request.
// It handles TLS and X-Forwarded-Proto header to determine the protocol.
func GetBaseURL(req *http.Request) string {
	protocol := "http"
	if req.TLS != nil {
		protocol = "https"
	}
	if proto := req.Header.Get("X-Forwarded-Proto"); proto != "" {
		protocol = proto
	}
	return protocol + "://" + req.Host
}

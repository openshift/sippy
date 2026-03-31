package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/apis/cache"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	bqclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/util/sets"
)

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
		NewCacheSpec(v1.Release{}, "Releases~", nil), // no cache options needed here, global list
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
	if variantsString == "" {
		return variantSet, nil
	}
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
// Otherwise it uses the request host and handles TLS and X-Forwarded-Proto for the protocol.
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

// GetBaseFrontendURL returns the base URL for the frontend using the Origin header.
// If not present, it defaults to GetBaseURL.
func GetBaseFrontendURL(req *http.Request) string {
	if origin := req.Header.Get("Origin"); origin != "" {
		if u, err := url.Parse(origin); err == nil && u.Scheme != "" && u.Host != "" {
			return u.Scheme + "://" + u.Host
		}
	}

	return GetBaseURL(req)
}

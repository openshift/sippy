package utils

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/bq"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	sippyv1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
)

func PreviousRelease(release string, releaseConfigs []sippyv1.Release) (string, error) {
	for _, config := range releaseConfigs {
		if config.Release == release {
			if config.PreviousRelease != "" {
				return config.PreviousRelease, nil
			}
			return "", fmt.Errorf("release %s has no previous release", release)
		}
	}
	return "", fmt.Errorf("release %s not found in release list", release)
}

// FindStartEndTimesForRelease finds the start and end times for a release from sippyv1.Release objects.
// The start time is calculated as 30 days before the GA date, and the end time is the GA date.
func FindStartEndTimesForRelease(timeRanges []crtest.ReleaseTimeRange, release string) (*time.Time, *time.Time, error) {
	for _, r := range timeRanges {
		if r.Release == release {
			return r.Start, r.End, nil
		}
	}
	return nil, nil, fmt.Errorf("release %s not found", release)
}

func NormalizeProwJobName(prowName string) string {
	// Remove anything that looks like versioning from the job name
	prowName = regexp.MustCompile(`\b\d+\.\d+\b`).ReplaceAllString(prowName, "X.X")

	// Some jobs encode frequency in their name, which can change
	prowName = regexp.MustCompile(`-f\d+`).ReplaceAllString(prowName, "-fXX")

	// openshift/release migrated from master to main, normalize it
	prowName = regexp.MustCompile(`-master-`).ReplaceAllString(prowName, "-main-")

	return prowName
}

// DeserializeTestKey helps us workaround the limitations of a struct as a map key, where
// we instead serialize a very small struct to json for a unit test key that includes test
// ID and a specific set of variants. This function deserializes back to a struct.
func DeserializeTestKey(stats bq.TestStatus, testKeyStr string) (crtest.Identification, error) {
	var testKey crtest.KeyWithVariants
	err := json.Unmarshal([]byte(testKeyStr), &testKey)
	if err != nil {
		logrus.WithError(err).Errorf("trying to unmarshel %s", testKeyStr)
		return crtest.Identification{}, err
	}
	testID := crtest.Identification{
		RowIdentification: crtest.RowIdentification{
			Component: stats.Component,
			TestName:  stats.TestName,
			TestSuite: stats.TestSuite,
			TestID:    testKey.TestID,
		},
		ColumnIdentification: crtest.ColumnIdentification{
			Variants: testKey.Variants,
		},
	}
	// Take the first cap for now. When we reach to a cell with specific capability, we will override the value.
	if len(stats.Capabilities) > 0 {
		testID.Capability = stats.Capabilities[0]
	}
	return testID, nil
}

// VariantsMapToStringSlice converts the map form of variants to a string slice
// where each variant is formatted key:value.
func VariantsMapToStringSlice(variants map[string]string) []string {
	vs := []string{}
	for k, v := range variants {
		vs = append(vs, fmt.Sprintf("%s:%s", k, v))
	}
	return vs
}

// VariantsStringSliceToMap converts a slice of "key:value" strings to a map
func VariantsStringSliceToMap(variants []string) map[string]string {
	variantMap := make(map[string]string)
	for _, variant := range variants {
		parts := strings.SplitN(variant, ":", 2)
		if len(parts) == 2 {
			variantMap[parts[0]] = parts[1]
		}
	}
	return variantMap
}

// addVariantParams adds variant parameters to url.Values and returns the environment string.
// This helper consolidates the duplicate variant parameter logic used in both view-based
// and legacy URL generation.
func addVariantParams(params url.Values, variantMap map[string]string) {
	if len(variantMap) == 0 {
		return
	}

	// Sort the keys to ensure consistent parameter ordering
	variantKeys := make([]string, 0, len(variantMap))
	for key := range variantMap {
		variantKeys = append(variantKeys, key)
	}
	sort.Strings(variantKeys)

	// Add individual variant parameters and build environment string
	environment := make([]string, 0, len(variantMap))
	for _, key := range variantKeys {
		value := variantMap[key]
		params.Add(key, value)
		environment = append(environment, fmt.Sprintf("%s:%s", key, value))
	}

	// Add environment parameter (space-separated variant pairs)
	params.Add("environment", strings.Join(environment, " "))
}

// addReleaseParams adds release-related parameters (dates, PR, payload options)
func addReleaseParams(
	params url.Values,
	baseReleaseOpts reqopts.Release,
	sampleReleaseOpts reqopts.Release,
	baseReleaseOverride string,
) {
	params.Add("baseRelease", baseReleaseOpts.Name)
	params.Add("sampleRelease", sampleReleaseOpts.Name)
	params.Add("baseStartTime", baseReleaseOpts.Start.Format("2006-01-02T15:04:05Z"))
	params.Add("baseEndTime", baseReleaseOpts.End.Format("2006-01-02T15:04:05Z"))
	params.Add("sampleStartTime", sampleReleaseOpts.Start.Format("2006-01-02T15:04:05Z"))
	params.Add("sampleEndTime", sampleReleaseOpts.End.Format("2006-01-02T15:04:05Z"))

	// Add PR options if present
	if sampleReleaseOpts.PullRequestOptions != nil {
		params.Add("samplePROrg", sampleReleaseOpts.PullRequestOptions.Org)
		params.Add("samplePRRepo", sampleReleaseOpts.PullRequestOptions.Repo)
		params.Add("samplePRNumber", sampleReleaseOpts.PullRequestOptions.PRNumber)
	}

	// Add Payload options if present
	if sampleReleaseOpts.PayloadOptions != nil {
		for _, tag := range sampleReleaseOpts.PayloadOptions.Tags {
			params.Add("samplePayloadTag", tag)
		}
	}

	// Check if release fallback was used and add the override
	if baseReleaseOverride != "" && baseReleaseOverride != baseReleaseOpts.Name {
		params.Add("testBasisRelease", baseReleaseOverride)
	}
}

// addAdvancedOptionsParams adds advanced options to URL parameters
func addAdvancedOptionsParams(params url.Values, advancedOptions reqopts.Advanced) {
	params.Add("confidence", strconv.Itoa(advancedOptions.Confidence))
	params.Add("minFail", strconv.Itoa(advancedOptions.MinimumFailure))
	params.Add("pity", strconv.Itoa(advancedOptions.PityFactor))
	params.Add("passRateNewTests", strconv.Itoa(advancedOptions.PassRateRequiredNewTests))
	params.Add("passRateAllTests", strconv.Itoa(advancedOptions.PassRateRequiredAllTests))
	params.Add("ignoreDisruption", strconv.FormatBool(advancedOptions.IgnoreDisruption))
	params.Add("ignoreMissing", strconv.FormatBool(advancedOptions.IgnoreMissing))
	params.Add("flakeAsFailure", strconv.FormatBool(advancedOptions.FlakeAsFailure))
	params.Add("includeMultiReleaseAnalysis", strconv.FormatBool(advancedOptions.IncludeMultiReleaseAnalysis))
}

// addVariantOptionsParams adds variant options to URL parameters
func addVariantOptionsParams(params url.Values, variantOptions reqopts.Variants) {
	if variantOptions.ColumnGroupBy != nil {
		params.Add("columnGroupBy", strings.Join(variantOptions.ColumnGroupBy.List(), ","))
	}
	if variantOptions.DBGroupBy != nil {
		params.Add("dbGroupBy", strings.Join(variantOptions.DBGroupBy.List(), ","))
	}

	// Add include variants
	includeVariantKeys := make([]string, 0, len(variantOptions.IncludeVariants))
	for variantKey := range variantOptions.IncludeVariants {
		includeVariantKeys = append(includeVariantKeys, variantKey)
	}
	sort.Strings(includeVariantKeys)

	for _, variantKey := range includeVariantKeys {
		variantValues := variantOptions.IncludeVariants[variantKey]
		sortedValues := make([]string, len(variantValues))
		copy(sortedValues, variantValues)
		sort.Strings(sortedValues)

		for _, variantValue := range sortedValues {
			params.Add("includeVariant", fmt.Sprintf("%s:%s", variantKey, variantValue))
		}
	}

	// Add compare variants
	if len(variantOptions.CompareVariants) > 0 {
		compareVariantKeys := make([]string, 0, len(variantOptions.CompareVariants))
		for variantKey := range variantOptions.CompareVariants {
			compareVariantKeys = append(compareVariantKeys, variantKey)
		}
		sort.Strings(compareVariantKeys)

		for _, variantKey := range compareVariantKeys {
			variantValues := variantOptions.CompareVariants[variantKey]
			sortedValues := make([]string, len(variantValues))
			copy(sortedValues, variantValues)
			sort.Strings(sortedValues)

			for _, variantValue := range sortedValues {
				params.Add("compareVariant", fmt.Sprintf("%s:%s", variantKey, variantValue))
			}
		}
	}

	// Add variant cross compare
	if len(variantOptions.VariantCrossCompare) > 0 {
		sortedCrossCompare := make([]string, len(variantOptions.VariantCrossCompare))
		copy(sortedCrossCompare, variantOptions.VariantCrossCompare)
		sort.Strings(sortedCrossCompare)

		for _, variantKey := range sortedCrossCompare {
			params.Add("variantCrossCompare", variantKey)
		}
	}
}

// GenerateTestDetailsURL creates a HATEOAS-style URL for the test_details API endpoint.
//
// Always generates a full URL with all parameters. When viewName is provided, it's added
// as an additional parameter. The view provides default values on the server side, while
// URL parameters explicitly override those defaults.
//
// Parameters included:
//   - view (if provided) - references a named view configuration
//   - testId - the test identifier
//   - release parameters (base/sample with dates, PR, payload options)
//   - advanced options (confidence, minFail, pity, etc.)
//   - variant options (columnGroupBy, dbGroupBy, includeVariant, etc.)
//   - test filters (capabilities, lifecycles)
//   - component, capability (if provided)
//   - specific variants from the variants parameter
//   - testBasisRelease (if baseReleaseOverride is provided)
//
// Note: keyTestNames are NOT included in URLs - they come from the view definition on the server.
func GenerateTestDetailsURL(
	testID string,
	baseURL string,
	viewName string,
	baseReleaseOpts reqopts.Release,
	sampleReleaseOpts reqopts.Release,
	advancedOptions reqopts.Advanced,
	variantOptions reqopts.Variants,
	testFilters reqopts.TestFilters,
	component string,
	capability string,
	variants []string,
	baseReleaseOverride string,
) (string, error) {

	if testID == "" {
		return "", fmt.Errorf("testID cannot be empty")
	}

	// Parse variants from the variants slice
	variantMap := VariantsStringSliceToMap(variants)

	// Build the URL with query parameters
	var fullURL string
	if baseURL == "" {
		// For backward compatibility, return relative URL if no baseURL provided
		logrus.Warn("GenerateTestDetailsURL was given an empty baseURL")
		fullURL = "/api/component_readiness/test_details"
	} else {
		// Create fully qualified URL
		fullURL = fmt.Sprintf("%s/api/component_readiness/test_details", baseURL)
	}

	u, err := url.Parse(fullURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	params := url.Values{}

	// Add view parameter first if provided (view provides defaults, URL params override)
	if viewName != "" {
		params.Add("view", viewName)
	}

	// Always generate full URL with all parameters
	params.Add("testId", testID)

	// Add release parameters
	addReleaseParams(params, baseReleaseOpts, sampleReleaseOpts, baseReleaseOverride)

	// Add advanced options
	addAdvancedOptionsParams(params, advancedOptions)

	if component != "" {
		params.Add("component", component)
	}
	if capability != "" {
		params.Add("capability", capability)
	}

	// Add test filter parameters
	for _, cap := range testFilters.Capabilities {
		params.Add("testCapabilities", cap)
	}
	for _, lifecycle := range testFilters.Lifecycles {
		params.Add("testLifecycles", lifecycle)
	}

	// Add variant options
	addVariantOptionsParams(params, variantOptions)

	// Add variant parameters and environment string
	addVariantParams(params, variantMap)

	u.RawQuery = params.Encode()
	return u.String(), nil
}

func ContainsOverriddenVariant(includeVariants map[string][]string, key, value string) bool {
	for k, v := range includeVariants {
		if k != key {
			continue
		}
		for _, vv := range v {
			if vv == value {
				return true
			}
		}
	}
	return false
}

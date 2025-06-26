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

	"github.com/openshift/sippy/pkg/apis/api/componentreport"
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/util"
	"github.com/sirupsen/logrus"
)

func PreviousRelease(release string) (string, error) {
	prev := release
	var err error
	var major, minor int
	if major, err = getMajor(release); err == nil {
		if minor, err = getMinor(release); err == nil && minor > 0 {
			prev = fmt.Sprintf("%d.%d", major, minor-1)
		}
	}

	return prev, err
}

func getMajor(in string) (int, error) {
	major, err := strconv.ParseInt(strings.Split(in, ".")[0], 10, 32)
	if err != nil {
		return 0, err
	}
	return int(major), err
}

func getMinor(in string) (int, error) {
	minor, err := strconv.ParseInt(strings.Split(in, ".")[1], 10, 32)
	if err != nil {
		return 0, err
	}
	return int(minor), err
}

func FindStartEndTimesForRelease(releases []componentreport.Release, release string) (*time.Time, *time.Time, error) {
	for _, r := range releases {
		if r.Release == release {
			return r.Start, r.End, nil
		}
	}
	return nil, nil, fmt.Errorf("release %s not found", release)
}

func NormalizeProwJobName(prowName string, reqOptions componentreport.RequestOptions) string {
	name := prowName
	// Build a list of all releases involved in this request to replace with X.X in normalized prow job names.
	releases := []string{}
	if reqOptions.BaseRelease.Release != "" {
		releases = append(releases, reqOptions.BaseRelease.Release)
	}
	if reqOptions.SampleRelease.Release != "" {
		releases = append(releases, reqOptions.SampleRelease.Release)
	}
	for _, tid := range reqOptions.TestIDOptions {
		if tid.BaseOverrideRelease != "" {
			releases = append(releases, tid.BaseOverrideRelease)
		}
	}

	for _, release := range releases {
		name = strings.ReplaceAll(name, release, "X.X")
		if prev, err := PreviousRelease(release); err == nil {
			name = strings.ReplaceAll(name, prev, "X.X")
		}
	}

	// Some jobs encode frequency in their name, which can change
	re := regexp.MustCompile(`-f\d+`)
	name = re.ReplaceAllString(name, "-fXX")

	return name
}

// DeserializeTestKey helps us workaround the limitations of a struct as a map key, where
// we instead serialize a very small struct to json for a unit test key that includes test
// ID and a specific set of variants. This function deserializes back to a struct.
func DeserializeTestKey(stats componentreport.TestStatus, testKeyStr string) (componentreport.ReportTestIdentification, error) {
	var testKey componentreport.TestWithVariantsKey
	err := json.Unmarshal([]byte(testKeyStr), &testKey)
	if err != nil {
		logrus.WithError(err).Errorf("trying to unmarshel %s", testKeyStr)
		return componentreport.ReportTestIdentification{}, err
	}
	testID := componentreport.ReportTestIdentification{
		RowIdentification: componentreport.RowIdentification{
			Component: stats.Component,
			TestName:  stats.TestName,
			TestSuite: stats.TestSuite,
			TestID:    testKey.TestID,
		},
		ColumnIdentification: componentreport.ColumnIdentification{
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

// GenerateTestDetailsURL creates a HATEOAS-style URL for the test_details API endpoint
// based on a TestRegression record. This mimics the URL construction logic from the UI
// in TestDetailsReport.js and uses the view configuration to populate URL parameters.
func GenerateTestDetailsURL(regression *models.TestRegression, baseURL string, views []crtype.View, releases []v1.Release, crTimeRoundingFactor time.Duration) (string, error) {
	if regression == nil {
		return "", fmt.Errorf("regression cannot be nil")
	}

	// Find the view for this regression
	var view crtype.View
	var found bool
	for i := range views {
		if views[i].Name == regression.View {
			view = views[i]
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("view %s not found", regression.View)
	}

	// Parse variants from the regression's Variants field (which is a []string of "key:value" pairs)
	variantMap := make(map[string]string)
	for _, variant := range regression.Variants {
		parts := strings.SplitN(variant, ":", 2)
		if len(parts) == 2 {
			variantMap[parts[0]] = parts[1]
		}
	}

	// Build the URL with query parameters
	u, err := url.Parse(baseURL + "/api/component_readiness/test_details")
	if err != nil {
		return "", fmt.Errorf("failed to parse base URL: %w", err)
	}

	params := url.Values{}

	params.Add("testId", regression.TestID)

	// Use view configuration to populate parameters

	// Get base and sample release options from the view
	baseReleaseOpts, err := GetViewReleaseOptions(releases, "basis", view.BaseRelease, crTimeRoundingFactor)
	if err != nil {
		return "", fmt.Errorf("failed to get base release options: %w", err)
	}

	sampleReleaseOpts, err := GetViewReleaseOptions(releases, "sample", view.SampleRelease, crTimeRoundingFactor)
	if err != nil {
		return "", fmt.Errorf("failed to get sample release options: %w", err)
	}

	// Add release and time parameters from view
	params.Add("baseRelease", baseReleaseOpts.Release)
	params.Add("sampleRelease", sampleReleaseOpts.Release)
	params.Add("baseStartTime", baseReleaseOpts.Start.Format("2006-01-02T15:04:05Z"))
	params.Add("baseEndTime", baseReleaseOpts.End.Format("2006-01-02T15:04:05Z"))
	params.Add("sampleStartTime", sampleReleaseOpts.Start.Format("2006-01-02T15:04:05Z"))
	params.Add("sampleEndTime", sampleReleaseOpts.End.Format("2006-01-02T15:04:05Z"))

	// Check if release fallback was used and the regression matched an older release:
	if regression.BaseRelease != "" && regression.BaseRelease != baseReleaseOpts.Release {
		params.Add("testBasisRelease", regression.BaseRelease)
	}

	// Add advanced options from view
	params.Add("confidence", strconv.Itoa(view.AdvancedOptions.Confidence))
	params.Add("minFail", strconv.Itoa(view.AdvancedOptions.MinimumFailure))
	params.Add("pity", strconv.Itoa(view.AdvancedOptions.PityFactor))
	params.Add("passRateNewTests", strconv.Itoa(view.AdvancedOptions.PassRateRequiredNewTests))
	params.Add("passRateAllTests", strconv.Itoa(view.AdvancedOptions.PassRateRequiredAllTests))
	params.Add("ignoreDisruption", strconv.FormatBool(view.AdvancedOptions.IgnoreDisruption))
	params.Add("ignoreMissing", strconv.FormatBool(view.AdvancedOptions.IgnoreMissing))
	params.Add("flakeAsFailure", strconv.FormatBool(view.AdvancedOptions.FlakeAsFailure))
	params.Add("includeMultiReleaseAnalysis", strconv.FormatBool(view.AdvancedOptions.IncludeMultiReleaseAnalysis))

	if regression.Component != "" {
		params.Add("component", regression.Component)
	}
	if regression.Capability != "" {
		params.Add("capability", regression.Capability)
	}

	// Add variant options from view
	if view.VariantOptions.ColumnGroupBy != nil {
		params.Add("columnGroupBy", strings.Join(view.VariantOptions.ColumnGroupBy.List(), ","))
	}
	if view.VariantOptions.DBGroupBy != nil {
		params.Add("dbGroupBy", strings.Join(view.VariantOptions.DBGroupBy.List(), ","))
	}

	// Add include variants from view
	// Sort variant keys to ensure consistent parameter ordering
	includeVariantKeys := make([]string, 0, len(view.VariantOptions.IncludeVariants))
	for variantKey := range view.VariantOptions.IncludeVariants {
		includeVariantKeys = append(includeVariantKeys, variantKey)
	}
	sort.Strings(includeVariantKeys)

	for _, variantKey := range includeVariantKeys {
		variantValues := view.VariantOptions.IncludeVariants[variantKey]
		// Sort variant values to ensure consistent parameter ordering
		sortedValues := make([]string, len(variantValues))
		copy(sortedValues, variantValues)
		sort.Strings(sortedValues)

		for _, variantValue := range sortedValues {
			params.Add("includeVariant", fmt.Sprintf("%s:%s", variantKey, variantValue))
		}
	}
	/*
		// Fallback to defaults if view not found

		// Add simplified time ranges
		now := time.Now()
		baseEndTime := now.Format("2006-01-02T15:04:05Z")
		baseStartTime := now.AddDate(0, 0, -30).Format("2006-01-02T15:04:05Z") // 30 days ago
		sampleEndTime := now.Format("2006-01-02T15:04:05Z")
		sampleStartTime := now.AddDate(0, 0, -7).Format("2006-01-02T15:04:05Z") // 7 days ago

		params.Add("baseStartTime", baseStartTime)
		params.Add("baseEndTime", baseEndTime)
		params.Add("sampleStartTime", sampleStartTime)
		params.Add("sampleEndTime", sampleEndTime)

		// Add default configuration parameters
		params.Add("confidence", "95")
		params.Add("minFail", "3")
		params.Add("pity", "5")
		params.Add("passRateNewTests", "95")
		params.Add("passRateAllTests", "0")
		params.Add("ignoreDisruption", "true")
		params.Add("ignoreMissing", "false")
		params.Add("flakeAsFailure", "false")
		params.Add("includeMultiReleaseAnalysis", "true")

		// Add column grouping (common defaults)
		params.Add("columnGroupBy", "Architecture,Network,Platform,Topology")
		params.Add("dbGroupBy", "Platform,Architecture,Network,Topology,FeatureSet,Upgrade,Suite,Installer")

		// Add default include variants
		defaultIncludeVariants := []string{
			"Architecture:amd64",
			"FeatureSet:default",
			"FeatureSet:techpreview",
			"Installer:ipi",
			"Installer:upi",
			"Network:ovn",
			"Platform:aws",
			"Platform:azure",
			"Platform:gcp",
			"Topology:ha",
		}

		for _, variant := range defaultIncludeVariants {
			params.Add("includeVariant", variant)
		}

	*/

	// Add the specific variants from the regression as individual parameters
	// Sort the keys to ensure consistent environment parameter ordering
	variantKeys := make([]string, 0, len(variantMap))
	for key := range variantMap {
		variantKeys = append(variantKeys, key)
	}
	sort.Strings(variantKeys)

	environment := make([]string, 0, len(variantMap))
	for _, key := range variantKeys {
		value := variantMap[key]
		params.Add(key, value)
		environment = append(environment, fmt.Sprintf("%s:%s", key, value))
	}

	// Add environment parameter (space-separated variant pairs)
	if len(environment) > 0 {
		params.Add("environment", strings.Join(environment, " "))
	}

	u.RawQuery = params.Encode()
	return u.String(), nil
}

// GetViewReleaseOptions translates relative start/end times to actual time.Time values.
// This is a copy of the function from queryparamparser.go to avoid circular imports.
func GetViewReleaseOptions(
	releases []v1.Release,
	releaseType string,
	viewRelease crtype.RequestRelativeReleaseOptions,
	roundingFactor time.Duration,
) (crtype.RequestReleaseOptions, error) {

	var err error
	opts := crtype.RequestReleaseOptions{Release: viewRelease.Release}
	opts.Start, err = util.ParseCRReleaseTime(releases, opts.Release, viewRelease.RelativeStart, true, nil, roundingFactor)
	if err != nil {
		return opts, fmt.Errorf("%s start time %q in wrong format: %v", releaseType, viewRelease.RelativeStart, err)
	}
	opts.End, err = util.ParseCRReleaseTime(releases, opts.Release, viewRelease.RelativeEnd, false, nil, roundingFactor)
	if err != nil {
		return opts, fmt.Errorf("%s end time %q in wrong format: %v", releaseType, viewRelease.RelativeEnd, err)
	}
	return opts, nil
}

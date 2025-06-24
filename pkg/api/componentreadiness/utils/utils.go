package utils

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/db/models"
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
// in TestDetailsReport.js but uses default values for missing information.
func GenerateTestDetailsURL(regression *models.TestRegression, baseURL string) (string, error) {
	if regression == nil {
		return "", fmt.Errorf("regression cannot be nil")
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

	// Add basic release and time parameters
	// For now, we'll use simplified defaults since we don't have access to the full view configuration
	params.Add("baseRelease", regression.Release)
	params.Add("sampleRelease", regression.Release)

	// Add simplified time ranges (these would normally come from the view configuration)
	now := time.Now()
	baseEndTime := now.Format("2006-01-02T15:04:05Z")
	baseStartTime := now.AddDate(0, 0, -30).Format("2006-01-02T15:04:05Z") // 30 days ago
	sampleEndTime := now.Format("2006-01-02T15:04:05Z")
	sampleStartTime := now.AddDate(0, 0, -7).Format("2006-01-02T15:04:05Z") // 7 days ago

	params.Add("baseStartTime", baseStartTime)
	params.Add("baseEndTime", baseEndTime)
	params.Add("sampleStartTime", sampleStartTime)
	params.Add("sampleEndTime", sampleEndTime)

	// Add test identification
	params.Add("testId", regression.TestID)
	params.Add("testBasisRelease", regression.Release)

	// Add default configuration parameters (matching typical UI defaults)
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

	// Add default include variants (common ones that are typically selected)
	defaultIncludeVariants := []string{
		"Architecture:amd64",
		"CGroupMode:v2",
		"ContainerRuntime:crun",
		"ContainerRuntime:runc",
		"FeatureSet:default",
		"FeatureSet:techpreview",
		"Installer:ipi",
		"Installer:upi",
		"JobTier:blocking",
		"JobTier:informing",
		"JobTier:standard",
		"LayeredProduct:none",
		"Network:ovn",
		"Owner:eng",
		"Owner:service-delivery",
		"Platform:aws",
		"Platform:azure",
		"Platform:gcp",
		"Platform:metal",
		"Platform:rosa",
		"Platform:vsphere",
		"Topology:ha",
		"Topology:microshift",
	}

	for _, variant := range defaultIncludeVariants {
		params.Add("includeVariant", variant)
	}

	// Add the specific variants from the regression as individual parameters
	environment := make([]string, 0, len(variantMap))
	for key, value := range variantMap {
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

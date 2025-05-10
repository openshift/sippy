package utils

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/openshift/sippy/pkg/apis/api/componentreport"
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

func NormalizeProwJobName(prowName string, reqOptions componentreport.RequestOptions) string {
	name := prowName
	if reqOptions.BaseRelease.Release != "" {
		name = strings.ReplaceAll(name, reqOptions.BaseRelease.Release, "X.X")
		if prev, err := PreviousRelease(reqOptions.BaseRelease.Release); err == nil {
			name = strings.ReplaceAll(name, prev, "X.X")
		}
	}
	if reqOptions.BaseOverrideRelease.Release != "" {
		name = strings.ReplaceAll(name, reqOptions.BaseOverrideRelease.Release, "X.X")
		if prev, err := PreviousRelease(reqOptions.BaseOverrideRelease.Release); err == nil {
			name = strings.ReplaceAll(name, prev, "X.X")
		}
	}
	if reqOptions.SampleRelease.Release != "" {
		name = strings.ReplaceAll(name, reqOptions.SampleRelease.Release, "X.X")
		if prev, err := PreviousRelease(reqOptions.SampleRelease.Release); err == nil {
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

func CalculatePassRate(success, failure, flake int, treatFlakeAsFailure bool) float64 {
	total := success + failure + flake
	if total == 0 {
		return 0.0
	}
	if treatFlakeAsFailure {
		return float64(success) / float64(total)
	}
	return float64(success+flake) / float64(total)
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

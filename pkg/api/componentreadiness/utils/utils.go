package utils

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
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

func NormalizeProwJobName(prowName string, reqOptions reqopts.RequestOptions) string {
	name := prowName
	// Build a list of all releases involved in this request to replace with X.X in normalized prow job names.
	releases := []string{}
	if reqOptions.BaseRelease.Name != "" {
		releases = append(releases, reqOptions.BaseRelease.Name)
	}
	if reqOptions.SampleRelease.Name != "" {
		releases = append(releases, reqOptions.SampleRelease.Name)
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
func DeserializeTestKey(stats componentreport.TestStatus, testKeyStr string) (crtest.Identification, error) {
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

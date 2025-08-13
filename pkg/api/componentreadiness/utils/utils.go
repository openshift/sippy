package utils

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/bq"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	sippyv1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/sirupsen/logrus"
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

func FindStartEndTimesForRelease(releases []crtest.Release, release string) (*time.Time, *time.Time, error) {
	for _, r := range releases {
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

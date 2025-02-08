package utils

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/openshift/sippy/pkg/apis/api/componentreport"
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

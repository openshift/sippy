package utils

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/openshift/sippy/pkg/api/componentreadiness"
)

func PreviousRelease(release string) (string, error) {
	prev := release
	var err error
	var major, minor int
	if major, err = componentreadiness.getMajor(release); err == nil {
		if minor, err = componentreadiness.getMinor(release); err == nil && minor > 0 {
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

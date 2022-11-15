package prowloader

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

const (
	alertREStr = `alert (?P<alert>[^\s]+) (?P<state>pending|fired) .*namespace="(?P<namespace>[a-zA-Z0-9-]+)", .*?`

	// NOTE: order important here, if ns ever moves after reason, we'd need another regex added to each test name below that matches the new order
	// NOTE: these two should just be one but I am unable to make the namespace match group optional, ? doesn't seem to work
	pathologicalEventsWithNSREStr = `(?:ns\/)(?P<namespace>[a-zA-Z0-9-]+) .* reason\/(?P<reason>[a-zA-Z0-9]+)`
	pathologicalEventsREStr       = `reason\/(?P<reason>[a-zA-Z0-9]+)`
)

func GetTestOutputMetadataExtractors() map[string]TestOutputMetadataExtractorFunc {
	return map[string]TestOutputMetadataExtractorFunc{
		"Cluster upgrade.[sig-arch] Check if alerts are firing during or after upgrade success":                                                                                      alertMetadataExtractor,
		"[sig-instrumentation][Late] Alerts shouldn't report any unexpected alerts in firing or pending state [apigroup:config.openshift.io] [Suite:openshift/conformance/parallel]": alertMetadataExtractor,
		"[sig-arch] events should not repeat pathologically":                                                                                                                         pathologicalEventsMetadataExtractor,
		"[sig-arch] events should not repeat pathologically in e2e namespaces":                                                                                                       pathologicalEventsMetadataExtractor,
	}
}

type TestOutputMetadataExtractorFunc func(testOutput string) []map[string]string

func alertMetadataExtractor(testOutput string) []map[string]string {
	re := regexp.MustCompile(alertREStr)
	return matchFirstRegexPerLine([]*regexp.Regexp{re}, testOutput)
}

func pathologicalEventsMetadataExtractor(testOutput string) []map[string]string {
	return matchFirstRegexPerLine(
		[]*regexp.Regexp{
			regexp.MustCompile(pathologicalEventsWithNSREStr),
			regexp.MustCompile(pathologicalEventsREStr),
		},
		testOutput,
	)
}

type TestFailureMetadataExtractor struct {
}

// ExtractMetadata uses regular expressions to extract metadata about the test failure, if we have
// any configured for the given test name.
// Note that a test name may have multiple regexes, each of which may match multiple times in one
// output string.
// Resulting slice of key values will eventually be serialized into the database as generic json.
func (te *TestFailureMetadataExtractor) ExtractMetadata(testName, testOutput string) []map[string]string {
	allMetadata := []map[string]string{}
	testNameToMetadataExtractorFunc := GetTestOutputMetadataExtractors()
	extractorFunc, ok := testNameToMetadataExtractorFunc[testName]
	if !ok {
		// We have no regexes for this test:
		return allMetadata
	}

	matchMaps := extractorFunc(testOutput)
	// eliminate duplicates, for some reason we often duplicate the output within one test:
	for _, mm := range matchMaps {
		dupe := false
		for _, em := range allMetadata {
			if reflect.DeepEqual(em, mm) {
				dupe = true
			}
		}
		if !dupe {
			allMetadata = append(allMetadata, mm)
		}
	}
	return allMetadata
}

// matchFirstRegexPerLine will return the results from the first regex in the given list that matches each
// test output line.
// If you wish to "and" the results from multiple regexes, call this function multiple times.
func matchFirstRegexPerLine(regexes []*regexp.Regexp, testOutput string) []map[string]string {
	allMetadata := []map[string]string{}
	for _, line := range strings.Split(testOutput, "\n") {
		for _, re := range regexes {
			matchMaps := findAllNamedMatches(re, line)

			fmt.Printf("%v\n", matchMaps)
			// eliminate duplicates, for some reason we often duplicate the output within one test:
			for _, mm := range matchMaps {
				dupe := false
				for _, em := range allMetadata {
					if reflect.DeepEqual(em, mm) {
						dupe = true
					}
				}
				if !dupe {
					allMetadata = append(allMetadata, mm)
				}
			}

			if len(matchMaps) > 0 {
				break
			}
		}
	}
	return allMetadata
}

// findAllNamedMatches returns a list of key value maps for each match of the regular expression.
// Keys are defined by the named groups in the regular expression.
func findAllNamedMatches(regex *regexp.Regexp, testOutput string) []map[string]string {
	matches := regex.FindAllStringSubmatch(testOutput, -1)
	allMatchMap := []map[string]string{}
	for _, m := range matches {
		results := map[string]string{}
		for i, name := range m {
			// Skip the empty group name to full string matched:
			if i == 0 {
				continue
			}
			results[regex.SubexpNames()[i]] = name
		}
		allMatchMap = append(allMatchMap, results)
	}
	return allMatchMap
}

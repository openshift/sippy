package prowloader

import (
	"fmt"
	"reflect"
	"regexp"
)

type TestFailureMetadataExtractor struct {
}

const (
	alertREStr = `alert (?P<alert>[^\s]+) (?P<state>pending|fired) .*namespace="(?P<namespace>[a-zA-Z0-9-]+)", .*?`

	// NOTE: order important here, if ns ever moves after reason, we'd need another regex added to each test name below that matches the new order
	// NOTE: these two should just be one but I am unable to make the namespace match group optional, ? doesn't seem to work
	pathologicalEventsWithNSREStr = `(?:ns\/)(?P<namespace>[a-zA-Z0-9-]+) .* reason\/(?P<reason>[a-zA-Z0-9]+)`
	pathologicalEventsREStr       = `reason\/(?P<reason>[a-zA-Z0-9]+)`
)

func (te *TestFailureMetadataExtractor) GetTestRegexes() map[string][]*regexp.Regexp {
	return map[string][]*regexp.Regexp{
		"Cluster upgrade.[sig-arch] Check if alerts are firing during or after upgrade success": []*regexp.Regexp{
			regexp.MustCompile(alertREStr),
		},
		"[sig-instrumentation][Late] Alerts shouldn't report any unexpected alerts in firing or pending state [apigroup:config.openshift.io] [Suite:openshift/conformance/parallel]": []*regexp.Regexp{
			regexp.MustCompile(alertREStr),
		},
		"[sig-arch] events should not repeat pathologically": []*regexp.Regexp{
			regexp.MustCompile(pathologicalEventsREStr),
			regexp.MustCompile(pathologicalEventsWithNSREStr),
		},
		"[sig-arch] events should not repeat pathologically in e2e namespaces": []*regexp.Regexp{
			regexp.MustCompile(pathologicalEventsREStr),
			regexp.MustCompile(pathologicalEventsWithNSREStr),
		},
	}
}

// ExtractMetadata uses regular expressions to extract metadata about the test failure, if we have
// any configured for the given test name.
// Note that a test name may have multiple regexes, each of which may match multiple times in one
// output string.
// Resulting slice of key values will eventually be serialized into the database as generic json.
func (te *TestFailureMetadataExtractor) ExtractMetadata(testName, testOutput string) []map[string]string {
	allMetadata := []map[string]string{}
	testRegexes := te.GetTestRegexes()
	regexes, ok := testRegexes[testName]
	if !ok {
		// We have no regexes for this test:
		return allMetadata
	}
	for _, re := range regexes {
		matchMaps := findAllNamedMatches(re, testOutput)
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
	}
	return allMetadata
}

// findAllNamedMatches returns a list of key value maps for each match of the regular expression.
// Keys are defined by the named groups in the regular expression.
func findAllNamedMatches(regex *regexp.Regexp, str string) []map[string]string {
	matches := regex.FindAllStringSubmatch(str, -1)
	allMatchMap := []map[string]string{}
	for _, m := range matches {
		results := map[string]string{}
		for i, name := range m {
			// Skip the empty group name to full string matched:
			if i == 0 {
				continue
			}
			//if results[regex.SubexpNames()[i]] != "" {
			results[regex.SubexpNames()[i]] = name
			//}
		}
		allMatchMap = append(allMatchMap, results)
	}
	return allMatchMap
}

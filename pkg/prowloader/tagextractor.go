package prowloader

import (
	"fmt"
	"reflect"
	"regexp"
)

type TagExtractor struct {
}

func (te *TagExtractor) GetTestRegexes() map[string][]*regexp.Regexp {
	return map[string][]*regexp.Regexp{
		"Cluster upgrade.[sig-arch] Check if alerts are firing during or after upgrade success": []*regexp.Regexp{
			regexp.MustCompile(`alert (?P<alert>[^\s]+) (?P<state>pending|fired) .*namespace="(?P<namespace>[a-zA-Z0-9-]+)", .*?`),
		},
		"[sig-instrumentation][Late] Alerts shouldn't report any unexpected alerts in firing or pending state [apigroup:config.openshift.io] [Suite:openshift/conformance/parallel]": []*regexp.Regexp{},
		"[sig-arch] events should not repeat pathologically":                   []*regexp.Regexp{},
		"[sig-arch] events should not repeat pathologically in e2e namespaces": []*regexp.Regexp{},
	}
}

func (te *TagExtractor) ExtractTags(name, testOutput string) []map[string]string {
	allMetadata := []map[string]string{}
	testRegexes := te.GetTestRegexes()
	regexes, ok := testRegexes[name]
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
			results[regex.SubexpNames()[i]] = name
		}
		allMatchMap = append(allMatchMap, results)
	}
	return allMatchMap
}

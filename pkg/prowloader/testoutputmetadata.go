package prowloader

import (
	"reflect"
	"regexp"
	"strings"
)

const (
	alertREStr = `alert (?P<alert>[^\s]+) (?P<state>pending|fired)`

	// NOTE: order important here, if ns ever moves after reason, we'd need another regex added to each test name below that matches the new order
	// NOTE: these two should just be one but I am unable to make the namespace match group optional, ? doesn't seem to work
	pathologicalEventsREStr = `reason\/(?P<reason>[a-zA-Z0-9]+)`
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
	return scanTestOutput(re, []string{"namespace", "service", "severity", "reason", "result", "bug"}, testOutput)
}

func pathologicalEventsMetadataExtractor(testOutput string) []map[string]string {
	return scanTestOutput(
		regexp.MustCompile(pathologicalEventsREStr),
		[]string{"ns"},
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

func scanTestOutput(re *regexp.Regexp, tokenKeys []string, testOutput string) []map[string]string {
	allMetadata := []map[string]string{}

	// Break test output into lines so we can scan each individually:
	for _, line := range strings.Split(testOutput, "\n") {
		matchMaps := scanLine(re, tokenKeys, line)

		// eliminate duplicates, for some reason we often duplicate line output within one test:
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

// scanLine returns a list of key value maps for each match of the regular expression.
// Keys are defined by the named groups in the regular expression.
func scanLine(regex *regexp.Regexp, tokenKeys []string, line string) []map[string]string {

	tokenKeyMap := map[string]bool{}
	for _, tk := range tokenKeys {
		tokenKeyMap[tk] = true
	}

	matches := regex.FindAllStringSubmatch(line, -1)
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
		if len(results) > 0 {
			// We hit on our regex, now do the more tricky scan for optional tokens, usually of the form
			// key=val or key/val, both of which we need to look out for.
			// NOTE: we do not support token values with whitespace, must be all one word so we can safely split
			// on spaces. This appears to be a reasonable limitation given our current use.
			lt := strings.ReplaceAll(line, "{", " ")
			lt = strings.ReplaceAll(lt, "}", " ")
			tokens := strings.Split(lt, " ")
			for _, t := range tokens {
				t = strings.TrimSuffix(t, ",")
				st := strings.Split(t, "=")
				if len(st) == 2 && tokenKeyMap[st[0]] {
					// Remove quotes around value if present:
					v := st[1]
					if strings.HasPrefix(v, "\"") && strings.HasSuffix(v, "\"") {
						v = v[1 : len(v)-1]
					}
					results[st[0]] = v
				}
				st = strings.Split(t, "/")
				if len(st) == 2 && tokenKeyMap[st[0]] {
					results[st[0]] = st[1]
				}
			}

		}
		allMatchMap = append(allMatchMap, results)
	}
	return allMatchMap
}

package prowloader

import (
	"fmt"
	"regexp"

	"github.com/openshift/sippy/pkg/util/sets"
)

type TagExtractor struct {
}

func (te *TagExtractor) GetTestRegexes() map[string][]*regexp.Regexp {
	return map[string][]*regexp.Regexp{
		"Cluster upgrade.[sig-arch] Check if alerts are firing during or after upgrade success": []*regexp.Regexp{
			regexp.MustCompile(`alert (?P<alert>[^\s]+) (?P<state>pending|fired)`),
		},
		"[sig-instrumentation][Late] Alerts shouldn't report any unexpected alerts in firing or pending state [apigroup:config.openshift.io] [Suite:openshift/conformance/parallel]": []*regexp.Regexp{},
		"[sig-arch] events should not repeat pathologically":                   []*regexp.Regexp{},
		"[sig-arch] events should not repeat pathologically in e2e namespaces": []*regexp.Regexp{},
	}
}

func (te *TagExtractor) ExtractTags(name, output string) []string {
	tags := sets.NewString()
	testRegexes := te.GetTestRegexes()
	regexes, ok := testRegexes[name]
	if !ok {
		return tags.List()
	}
	for _, rx := range regexes {
		groupNames := rx.SubexpNames()
		fmt.Printf("groupNames = %s\n", groupNames)
		matches := rx.FindAllStringSubmatch(output, -1)
		for _, m := range matches {
			for i, name := range m {
				if groupNames[i] != "" {
					fmt.Printf("%d %s = %s\n", i, groupNames[i], name)
				}

			}
			//fmt.Printf("%v\n", m)
			//tags.Insert(m["alert"])
		}
	}
	return tags.List()
}

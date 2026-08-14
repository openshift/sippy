package jobrunscan

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	"github.com/openshift/sippy/pkg/db/models/jobrunscan"
)

// computeSymptomHash produces a deterministic hash over the sorted symptom
// definitions. Two calls with the same set of symptoms (regardless of order)
// will return the same hash. The hash includes each symptom's ID, matcher
// type, match string, file pattern, and label IDs so that any change to
// symptom configuration produces a different hash.
func computeSymptomHash(symptoms []jobrunscan.Symptom) string {
	entries := make([]string, len(symptoms))
	for i, s := range symptoms {
		labels := make([]string, len(s.LabelIDs))
		copy(labels, s.LabelIDs)
		sort.Strings(labels)
		entries[i] = fmt.Sprintf("%s|%s|%s|%s|%s",
			s.ID, s.MatcherType, s.MatchString, s.FilePattern, strings.Join(labels, ","))
	}
	sort.Strings(entries)

	h := sha256.New()
	for _, e := range entries {
		h.Write([]byte(e))
		h.Write([]byte{0})
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

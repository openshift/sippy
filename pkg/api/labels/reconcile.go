package labels

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/openshift/sippy/pkg/componentreadiness/jobrunannotator"
)

const bucketLabelsPrefix = jobrunannotator.BucketLabelsPrefix

// DiscoveredLabel is the authoritative label state represented by one GCS label file.
// The ID is the downstream label identity; the remaining fields make metadata changes
// visible to reconciliation even though prow_job_runs stores only label IDs.
type DiscoveredLabel struct {
	ID          string
	Title       string
	Explanation string
}

// LabelDiff describes the full-set change from current to desired label state.
type LabelDiff struct {
	Add    []DiscoveredLabel
	Update []DiscoveredLabel
	Remove []string
}

var gcsPathStrip = regexp.MustCompile(`.*/gs/[^/]+/`)

// DeriveJobRunPathFromURL extracts the job-run directory from the authoritative
// Prow job URL stored on the Sippy run record.
func DeriveJobRunPathFromURL(prowJobURL string) (string, error) {
	parsedURL, err := url.Parse(prowJobURL)
	if err != nil {
		return "", fmt.Errorf("parse Prow job URL: %w", err)
	}
	jobRunPath := gcsPathStrip.ReplaceAllString(parsedURL.Path, "")
	if jobRunPath == "" || len(jobRunPath) == len(parsedURL.Path) {
		return "", fmt.Errorf("prow job URL %q does not contain an expected GCS path", prowJobURL)
	}
	if !strings.HasSuffix(jobRunPath, "/") {
		jobRunPath += "/"
	}
	return jobRunPath, nil
}

// ParseLabelFile parses one existing GCS label-file envelope. Unknown or empty
// envelope versions are rejected so a malformed file can never mean "delete all".
func ParseLabelFile(data []byte) (DiscoveredLabel, error) {
	var container jobrunannotator.JobRunBucketLabelContainer
	if err := json.Unmarshal(data, &container); err != nil {
		return DiscoveredLabel{}, fmt.Errorf("decode label file: %w", err)
	}
	if container.V1 == nil || container.V1.Label.ID == "" {
		return DiscoveredLabel{}, fmt.Errorf("label file has no supported non-empty label ID")
	}
	return DiscoveredLabel{
		ID:          container.V1.Label.ID,
		Title:       container.V1.Label.LabelTitle,
		Explanation: container.V1.Label.Explanation,
	}, nil
}

// DiffLabels computes a deterministic full-set diff. A nil or empty desired map
// intentionally removes every current label, but callers must only invoke it after
// a successful authoritative GCS read.
func DiffLabels(current, desired map[string]DiscoveredLabel) LabelDiff {
	diff := LabelDiff{}
	for id, want := range desired {
		have, ok := current[id]
		if !ok {
			diff.Add = append(diff.Add, want)
			continue
		}
		if !sameLabel(have, want) {
			diff.Update = append(diff.Update, want)
		}
	}
	for id := range current {
		if _, ok := desired[id]; !ok {
			diff.Remove = append(diff.Remove, id)
		}
	}
	sort.Slice(diff.Add, func(i, j int) bool { return diff.Add[i].ID < diff.Add[j].ID })
	sort.Slice(diff.Update, func(i, j int) bool { return diff.Update[i].ID < diff.Update[j].ID })
	sort.Strings(diff.Remove)
	return diff
}

func sameLabel(one, two DiscoveredLabel) bool {
	return one.ID == two.ID && one.Title == two.Title && one.Explanation == two.Explanation
}

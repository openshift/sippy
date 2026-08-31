package prowloader

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1config "github.com/openshift/sippy/pkg/apis/config/v1"
	"github.com/openshift/sippy/pkg/apis/prow"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/releaseoverride"
)

func TestReleaseAttributor(t *testing.T) {
	overrides := releaseoverride.New()
	require.NoError(t, overrides.AddExact("synthetic-exact", "rosa-stage"))
	require.NoError(t, overrides.AddRegexp(`^synthetic-regexp-`, "rosa-stage"))
	config := &v1config.SippyConfig{Releases: map[string]v1config.ReleaseConfig{
		"4.20":       {Jobs: map[string]bool{"configured": true, "disabled": false}, Regexp: []string{`-4\.20-`}},
		"rosa-stage": {Synthetic: true},
	}}
	attributor := NewReleaseAttributor([]string{"4.20", "rosa-stage", models.ReleasePresubmits}, config, overrides)

	tests := []struct {
		name string
		job  *prow.ProwJob
		want string
	}{
		{name: "configured exact", job: jobForAttribution("configured", nil, false), want: "4.20"},
		{name: "configured disabled", job: jobForAttribution("disabled", nil, false), want: ""},
		{name: "configured regexp", job: jobForAttribution("periodic-4.20-e2e", nil, false), want: "4.20"},
		{name: "synthetic exact has priority", job: jobForAttribution("synthetic-exact", nil, false), want: "rosa-stage"},
		{name: "synthetic regexp", job: jobForAttribution("synthetic-regexp-job", nil, false), want: "rosa-stage"},
		{name: "payload presubmit", job: jobForAttribution("generated-name", map[string]string{"releaseJobName": "canonical"}, true), want: models.ReleasePresubmits},
		{name: "payload annotation without refs", job: jobForAttribution("generated-name", map[string]string{"releaseJobName": "canonical"}, false), want: ""},
		{name: "unmatched", job: jobForAttribution("other", nil, false), want: ""},
		{name: "nil job", job: nil, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, attributor.Match(tt.job))
		})
	}
	assert.Equal(t, "", (*ReleaseAttributor)(nil).Match(jobForAttribution("configured", nil, false)))
}

func jobForAttribution(name string, annotations map[string]string, hasRefs bool) *prow.ProwJob {
	job := &prow.ProwJob{Spec: prow.ProwJobSpec{Job: name}, Annotations: annotations}
	if hasRefs {
		job.Spec.Refs = &prow.Refs{}
	}
	return job
}

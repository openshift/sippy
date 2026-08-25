package util

import (
	"encoding/json"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/sippy/pkg/api/componentreadiness"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
)

func TestFindMainComponentReportCacheKey(t *testing.T) {
	mainKey := componentReportCacheKey(t, "Network", "Platform", "Topology")
	gcpOnlyKey := componentReportCacheKey(t, "Network")

	tests := []struct {
		name string
		keys []string
	}{
		{
			name: "main before gcp-only",
			keys: []string{mainKey, gcpOnlyKey},
		},
		{
			name: "gcp-only before main",
			keys: []string{gcpOnlyKey, mainKey},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := findMainComponentReportCacheKey(test.keys, Release); got != mainKey {
				t.Fatalf("expected main cache key %q, got %q", mainKey, got)
			}
		})
	}
}

func componentReportCacheKey(t *testing.T, columnGroupBy ...string) string {
	t.Helper()

	key, err := json.Marshal(componentreadiness.GeneratorCacheKey{
		BaseRelease:   reqopts.Release{Name: BaseRelease},
		SampleRelease: reqopts.Release{Name: Release},
		VariantOption: reqopts.Variants{
			ColumnGroupBy: sets.New(columnGroupBy...),
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal cache key: %v", err)
	}
	return "_SIPPY_cc:ComponentReport~" + string(key)
}

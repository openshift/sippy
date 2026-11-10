package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExcludeReleases(t *testing.T) {
	tests := []struct {
		name     string
		releases []string
		excluded []string
		want     []string
	}{
		{name: "no exclusions", releases: []string{"4.22", "Presubmits"}, want: []string{"4.22", "Presubmits"}},
		{name: "matching exclusion", releases: []string{"4.22", "Presubmits", "4.21"}, excluded: []string{"Presubmits"}, want: []string{"4.22", "4.21"}},
		{name: "nonmatching exclusion", releases: []string{"4.22", "4.21"}, excluded: []string{"Presubmits"}, want: []string{"4.22", "4.21"}},
		{name: "preserves order", releases: []string{"4.19", "4.22", "4.20", "Presubmits"}, excluded: []string{"4.22", "Presubmits"}, want: []string{"4.19", "4.20"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ExcludeReleases(tt.releases, tt.excluded))
		})
	}
}

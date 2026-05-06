package testreportcacheloader

import (
	"testing"
	"time"

	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/stretchr/testify/assert"
)

func TestDevelopmentReleases(t *testing.T) {
	gaDate := time.Date(2024, 10, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		releases []v1.Release
		want     map[string]bool
	}{
		{
			name: "filters to dev OCP releases only",
			releases: []v1.Release{
				{
					Release:      "4.18",
					Product:      "OCP",
					Capabilities: map[v1.ReleaseCapability]bool{v1.PayloadTagsCap: true},
				},
				{
					Release:      "4.17",
					Product:      "OCP",
					GADate:       &gaDate,
					Capabilities: map[v1.ReleaseCapability]bool{v1.PayloadTagsCap: true},
				},
				{
					Release:      "Presubmits",
					Product:      "OCP",
					Capabilities: map[v1.ReleaseCapability]bool{v1.SippyClassicCap: true},
				},
				{
					Release:      "4.19",
					Product:      "OCP",
					Capabilities: map[v1.ReleaseCapability]bool{v1.PayloadTagsCap: true, v1.SippyClassicCap: true},
				},
			},
			want: map[string]bool{
				"4.18": true,
				"4.19": true,
			},
		},
		{
			name: "excludes OKD releases",
			releases: []v1.Release{
				{
					Release:      "4.18",
					Product:      "OCP",
					Capabilities: map[v1.ReleaseCapability]bool{v1.PayloadTagsCap: true},
				},
				{
					Release:      "4.18-okd",
					Product:      "OKD",
					Capabilities: map[v1.ReleaseCapability]bool{v1.PayloadTagsCap: true},
				},
			},
			want: map[string]bool{
				"4.18": true,
			},
		},
		{
			name:     "empty releases",
			releases: nil,
			want:     map[string]bool{},
		},
		{
			name: "all GA releases returns empty",
			releases: []v1.Release{
				{
					Release:      "4.17",
					Product:      "OCP",
					GADate:       &gaDate,
					Capabilities: map[v1.ReleaseCapability]bool{v1.PayloadTagsCap: true},
				},
			},
			want: map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &testReportCacheLoader{releases: tt.releases}
			got := l.developmentReleases()
			assert.Equal(t, tt.want, got)
		})
	}
}

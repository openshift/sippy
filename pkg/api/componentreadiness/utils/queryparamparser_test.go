package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
)

func TestGetViewReleaseOptionsAt(t *testing.T) {
	now := time.Date(2026, 11, 4, 7, 30, 0, 0, time.UTC)
	sample := reqopts.RelativeRelease{
		Release:       reqopts.Release{Name: "4.22"},
		RelativeStart: "now-3d",
		RelativeEnd:   "now",
	}
	basis := reqopts.RelativeRelease{
		Release:       reqopts.Release{Name: "4.21"},
		RelativeStart: "now-60d",
		RelativeEnd:   "now-30d",
	}
	tests := []struct {
		name        string
		releaseType string
		viewRelease reqopts.RelativeRelease
		factor      time.Duration
		offset      time.Duration
		wantStart   time.Time
		wantEnd     time.Time
		wantErr     bool
	}{
		{
			name:        "sample window ends at the 12h/4h boundary",
			releaseType: "sample",
			viewRelease: sample,
			factor:      12 * time.Hour,
			offset:      4 * time.Hour,
			wantStart:   time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:     time.Date(2026, 11, 4, 4, 0, 0, 0, time.UTC),
		},
		{
			name:        "sample window without rounding ends at the end of today",
			releaseType: "sample",
			viewRelease: sample,
			wantStart:   time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:     time.Date(2026, 11, 4, 23, 59, 59, 0, time.UTC),
		},
		{
			name:        "basis window covers whole days and ignores rounding",
			releaseType: "basis",
			viewRelease: basis,
			factor:      12 * time.Hour,
			offset:      4 * time.Hour,
			wantStart:   time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC),
			wantEnd:     time.Date(2026, 10, 5, 23, 59, 59, 0, time.UTC),
		},
		{
			name:        "bad start",
			releaseType: "sample",
			viewRelease: reqopts.RelativeRelease{Release: reqopts.Release{Name: "4.22"}, RelativeStart: "yesterday", RelativeEnd: "now"},
			factor:      12 * time.Hour,
			wantErr:     true,
		},
		{
			name:        "bad end",
			releaseType: "sample",
			viewRelease: reqopts.RelativeRelease{Release: reqopts.Release{Name: "4.22"}, RelativeStart: "now-3d", RelativeEnd: "tomorrow"},
			factor:      12 * time.Hour,
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetViewReleaseOptionsAt(now, nil, tt.releaseType, tt.viewRelease, tt.factor, tt.offset)
			if tt.wantErr {
				var validationErr *api.ValidationError
				require.ErrorAs(t, err, &validationErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.viewRelease.Name, got.Name)
			assert.Equal(t, tt.wantStart, got.Start)
			assert.Equal(t, tt.wantEnd, got.End)
		})
	}
}

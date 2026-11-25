package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
	"github.com/openshift/sippy/pkg/apis/cache"
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

func TestViewRequestOptionsAt(t *testing.T) {
	now := time.Date(2026, 11, 4, 7, 30, 0, 0, time.UTC)
	cacheOpts := cache.RequestOptions{CRTimeRoundingFactor: 12 * time.Hour, CRTimeRoundingOffset: 4 * time.Hour}
	view := crview.View{
		Name: "4.22-main",
		BaseRelease: reqopts.RelativeRelease{
			Release:       reqopts.Release{Name: "4.21"},
			RelativeStart: "now-60d",
			RelativeEnd:   "now-30d",
		},
		SampleRelease: reqopts.RelativeRelease{
			Release:       reqopts.Release{Name: "4.22"},
			RelativeStart: "now-3d",
			RelativeEnd:   "now",
		},
		TestFilters:     reqopts.TestFilters{Lifecycles: []string{"blocking"}},
		VariantOptions:  reqopts.Variants{IncludeVariants: map[string][]string{"Platform": {"aws"}}},
		AdvancedOptions: reqopts.Advanced{MinimumFailure: 3, Confidence: 95},
		SpotCheckJobSamples: map[string]crview.SpotCheckJobSample{
			"spotcheck-7d":  {RelativeStart: "now-7d", RelativeEnd: "now", IncludeVariants: map[string][]string{"JobTier": {"spotcheck-7d"}}},
			"spotcheck-30d": {RelativeStart: "now-30d", RelativeEnd: "now", IncludeVariants: map[string][]string{"JobTier": {"spotcheck-30d"}}},
		},
	}

	t.Run("carries every view setting that shapes the report", func(t *testing.T) {
		got, err := ViewRequestOptionsAt(now, view, nil, cacheOpts)
		require.NoError(t, err)
		assert.Equal(t, "4.22-main", got.ViewName)
		assert.Equal(t, view.TestFilters, got.TestFilters)
		assert.Equal(t, view.VariantOptions, got.VariantOption)
		assert.Equal(t, view.AdvancedOptions, got.AdvancedOption)
		assert.Equal(t, cacheOpts, got.CacheOption)
		assert.Equal(t, reqopts.Release{
			Name:  "4.21",
			Start: time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 10, 5, 23, 59, 59, 0, time.UTC),
		}, got.BaseRelease, "basis must not be rounded")
		assert.Equal(t, reqopts.Release{
			Name:  "4.22",
			Start: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 11, 4, 4, 0, 0, 0, time.UTC),
		}, got.SampleRelease, "sample must be rounded")
		assert.Equal(t, []reqopts.SpotCheckJobSampleOpts{
			{
				Name:            "spotcheck-30d",
				Release:         reqopts.Release{Name: "4.22", Start: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 11, 4, 4, 0, 0, 0, time.UTC)},
				IncludeVariants: map[string][]string{"JobTier": {"spotcheck-30d"}},
			},
			{
				Name:            "spotcheck-7d",
				Release:         reqopts.Release{Name: "4.22", Start: time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 11, 4, 4, 0, 0, 0, time.UTC)},
				IncludeVariants: map[string][]string{"JobTier": {"spotcheck-7d"}},
			},
		}, got.SpotCheckJobSamples)
	})

	t.Run("view without spot-check samples leaves spot-check disabled", func(t *testing.T) {
		noSpot := view
		noSpot.SpotCheckJobSamples = nil
		got, err := ViewRequestOptionsAt(now, noSpot, nil, cacheOpts)
		require.NoError(t, err)
		assert.Empty(t, got.SpotCheckJobSamples)
	})

	t.Run("bad spot-check window is a validation error", func(t *testing.T) {
		bad := view
		bad.SpotCheckJobSamples = map[string]crview.SpotCheckJobSample{"broken": {RelativeStart: "whenever", RelativeEnd: "now"}}
		_, err := ViewRequestOptionsAt(now, bad, nil, cacheOpts)
		var validationErr *api.ValidationError
		require.ErrorAs(t, err, &validationErr)
	})
}

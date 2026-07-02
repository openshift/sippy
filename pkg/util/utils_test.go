package util

import (
	"testing"
	"time"

	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCRReleaseTime(t *testing.T) {
	releases := []v1.Release{
		{Release: "4.16", Status: "", GADate: CivilDatePtr(2024, time.June, 27)},
	}

	nowMinus7d := time.Now().Add(-7 * 24 * time.Hour).UTC()
	nowMinus7dRoundDown := time.Date(nowMinus7d.Year(), nowMinus7d.Month(), nowMinus7d.Day(), 0, 0, 0, 0, time.UTC)
	nowMinus7dRoundUp := time.Date(nowMinus7d.Year(), nowMinus7d.Month(), nowMinus7d.Day(), 23, 59, 59, 0, time.UTC)
	now := time.Now().UTC()
	nowRoundDown := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	nowRoundUp := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, time.UTC)

	jan142025 := time.Date(2025, 1, 14, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		timeStr        string
		release        string
		isStart        bool
		endTime        *time.Time
		roundingFactor time.Duration
		expectedTime   time.Time
		expectedErr    bool
	}{
		{
			name:         "fully qualified RFC3339",
			isStart:      false,
			timeStr:      "2024-06-27T23:50:50Z",
			expectedTime: time.Date(2024, 6, 27, 23, 50, 50, 0, time.UTC),
		},
		{
			name:         "now start date",
			timeStr:      "now",
			isStart:      true,
			expectedTime: nowRoundDown,
		},
		{
			name:         "now end date",
			timeStr:      "now",
			isStart:      false,
			expectedTime: nowRoundUp,
		},
		{
			name:           "now end date with cache rounding",
			timeStr:        "now",
			roundingFactor: 12 * time.Hour,
			isStart:        false,
			expectedTime:   now.Truncate(12 * time.Hour),
		},
		{
			name:         "now-7d start date",
			timeStr:      "now-7d",
			isStart:      true,
			expectedTime: nowMinus7dRoundDown,
		},
		{
			name:         "now-7d end date",
			timeStr:      "now-7d",
			isStart:      false,
			expectedTime: nowMinus7dRoundUp,
		},
		{
			name:         "4.16 ga start date",
			release:      "4.16",
			timeStr:      "ga",
			isStart:      true,
			expectedTime: time.Date(2024, 6, 27, 0, 0, 0, 0, time.UTC),
		},
		{
			name:        "missing ga start date",
			release:     "3.1",
			timeStr:     "ga",
			isStart:     true,
			expectedErr: true,
		},
		{
			name:         "4.16 ga end date",
			release:      "4.16",
			timeStr:      "ga",
			isStart:      false,
			expectedTime: time.Date(2024, 6, 27, 23, 59, 59, 0, time.UTC),
		},
		{
			name:         "4.16 ga-30d start date",
			release:      "4.16",
			timeStr:      "ga-30d",
			isStart:      true,
			expectedTime: time.Date(2024, 5, 28, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "end-90d",
			timeStr:      "end-90d",
			endTime:      &jan142025,
			isStart:      true,
			expectedTime: time.Date(2024, 10, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			name:        "end-90d for end date",
			timeStr:     "end-90d",
			endTime:     &jan142025,
			isStart:     false,
			expectedErr: true,
		},
		{
			name:        "end-90d with no end date provided",
			timeStr:     "end-90d",
			isStart:     true,
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resultTime, err := ParseCRReleaseTime(releases, tt.release, tt.timeStr, tt.isStart, tt.endTime, tt.roundingFactor, 0)
			if tt.expectedErr {
				require.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedTime, resultTime)
			}
		})
	}
}

func TestTruncateAligned(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Time
		factor   time.Duration
		offset   time.Duration
		expected time.Time
	}{
		{
			name:     "zero factor returns input unchanged",
			input:    time.Date(2025, 6, 2, 10, 30, 0, 0, time.UTC),
			factor:   0,
			offset:   4 * time.Hour,
			expected: time.Date(2025, 6, 2, 10, 30, 0, 0, time.UTC),
		},
		{
			name:     "zero offset behaves like Truncate",
			input:    time.Date(2025, 6, 2, 10, 30, 0, 0, time.UTC),
			factor:   12 * time.Hour,
			offset:   0,
			expected: time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "before first boundary truncates to previous day",
			input:    time.Date(2025, 6, 2, 3, 0, 0, 0, time.UTC),
			factor:   12 * time.Hour,
			offset:   4 * time.Hour,
			expected: time.Date(2025, 6, 1, 16, 0, 0, 0, time.UTC),
		},
		{
			name:     "mid-morning truncates to 04:00 UTC",
			input:    time.Date(2025, 6, 2, 10, 0, 0, 0, time.UTC),
			factor:   12 * time.Hour,
			offset:   4 * time.Hour,
			expected: time.Date(2025, 6, 2, 4, 0, 0, 0, time.UTC),
		},
		{
			name:     "evening truncates to 16:00 UTC",
			input:    time.Date(2025, 6, 2, 20, 0, 0, 0, time.UTC),
			factor:   12 * time.Hour,
			offset:   4 * time.Hour,
			expected: time.Date(2025, 6, 2, 16, 0, 0, 0, time.UTC),
		},
		{
			name:     "exactly on first boundary returns boundary",
			input:    time.Date(2025, 6, 2, 4, 0, 0, 0, time.UTC),
			factor:   12 * time.Hour,
			offset:   4 * time.Hour,
			expected: time.Date(2025, 6, 2, 4, 0, 0, 0, time.UTC),
		},
		{
			name:     "exactly on second boundary returns boundary",
			input:    time.Date(2025, 6, 2, 16, 0, 0, 0, time.UTC),
			factor:   12 * time.Hour,
			offset:   4 * time.Hour,
			expected: time.Date(2025, 6, 2, 16, 0, 0, 0, time.UTC),
		},
		{
			name:     "one second before boundary truncates to previous",
			input:    time.Date(2025, 6, 2, 3, 59, 59, 0, time.UTC),
			factor:   12 * time.Hour,
			offset:   4 * time.Hour,
			expected: time.Date(2025, 6, 1, 16, 0, 0, 0, time.UTC),
		},
		{
			name:     "4h factor with no offset gives UTC-aligned boundaries",
			input:    time.Date(2025, 6, 2, 10, 30, 0, 0, time.UTC),
			factor:   4 * time.Hour,
			offset:   0,
			expected: time.Date(2025, 6, 2, 8, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateAligned(tt.input, tt.factor, tt.offset)
			assert.Equal(t, tt.expected, result)
		})
	}
}

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
		{Release: "4.16", Status: "", GADate: DatePtr(2024, 6, 27, 0, 0, 0, 0, time.UTC)},
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
			roundingFactor: 4 * time.Hour,
			isStart:        false,
			expectedTime:   now.Truncate(4 * time.Hour),
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
			release:      "4.16",
			timeStr:      "end-90d",
			endTime:      &jan142025,
			isStart:      true,
			expectedTime: time.Date(2024, 10, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			name:        "end-90d for end date",
			release:     "4.16",
			timeStr:     "end-90d",
			endTime:     &jan142025,
			isStart:     false,
			expectedErr: true,
		},
		{
			name:        "end-90d with no end date provided",
			release:     "4.16",
			timeStr:     "end-90d",
			isStart:     true,
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resultTime, err := ParseCRReleaseTime(releases, tt.release, tt.timeStr, tt.isStart, tt.endTime, tt.roundingFactor)
			if tt.expectedErr {
				require.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedTime, resultTime)
			}
		})
	}
}

package crtest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCountFailures(t *testing.T) {
	tests := []struct {
		name     string
		count    Count
		expected int
	}{
		{
			name:     "normal case",
			count:    Count{TotalCount: 10, SuccessCount: 7, FlakeCount: 1},
			expected: 2,
		},
		{
			name:     "all success",
			count:    Count{TotalCount: 5, SuccessCount: 5, FlakeCount: 0},
			expected: 0,
		},
		{
			name:     "all flakes",
			count:    Count{TotalCount: 5, SuccessCount: 0, FlakeCount: 5},
			expected: 0,
		},
		{
			name:     "all failures",
			count:    Count{TotalCount: 5, SuccessCount: 0, FlakeCount: 0},
			expected: 5,
		},
		{
			name:     "zero total",
			count:    Count{TotalCount: 0, SuccessCount: 0, FlakeCount: 0},
			expected: 0,
		},
		{
			name:     "data inconsistency floors at zero",
			count:    Count{TotalCount: 5, SuccessCount: 3, FlakeCount: 3},
			expected: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.count.Failures())
		})
	}
}

func TestCalculatePassRate(t *testing.T) {
	tests := []struct {
		name                string
		success             int
		failure             int
		flake               int
		treatFlakeAsFailure bool
		expected            float64
	}{
		{
			name:                "zero total without flake flag",
			success:             0,
			failure:             0,
			flake:               0,
			treatFlakeAsFailure: false,
			expected:            0.0,
		},
		{
			name:                "zero total with flake flag",
			success:             0,
			failure:             0,
			flake:               0,
			treatFlakeAsFailure: true,
			expected:            0.0,
		},
		{
			name:                "flakes as success",
			success:             8,
			failure:             0,
			flake:               2,
			treatFlakeAsFailure: false,
			expected:            1.0,
		},
		{
			name:                "flakes as failure",
			success:             8,
			failure:             0,
			flake:               2,
			treatFlakeAsFailure: true,
			expected:            0.8,
		},
		{
			name:                "mixed results flakes as success",
			success:             6,
			failure:             2,
			flake:               2,
			treatFlakeAsFailure: false,
			expected:            0.8,
		},
		{
			name:                "mixed results flakes as failure",
			success:             6,
			failure:             2,
			flake:               2,
			treatFlakeAsFailure: true,
			expected:            0.6,
		},
		{
			name:                "all failures",
			success:             0,
			failure:             10,
			flake:               0,
			treatFlakeAsFailure: false,
			expected:            0.0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculatePassRate(tt.success, tt.failure, tt.flake, tt.treatFlakeAsFailure)
			assert.InDelta(t, tt.expected, result, 1e-9)
		})
	}
}

func TestCountAdd(t *testing.T) {
	a := Count{TotalCount: 3, SuccessCount: 2, FlakeCount: 1}
	b := Count{TotalCount: 7, SuccessCount: 4, FlakeCount: 2}
	result := a.Add(b)

	assert.Equal(t, 10, result.TotalCount)
	assert.Equal(t, 6, result.SuccessCount)
	assert.Equal(t, 3, result.FlakeCount)

	// receiver should not be mutated (value receiver)
	assert.Equal(t, 3, a.TotalCount)
}

func TestCountToTestStats(t *testing.T) {
	count := Count{TotalCount: 10, SuccessCount: 6, FlakeCount: 2}

	t.Run("flake as success", func(t *testing.T) {
		stats := count.ToTestStats(false)
		assert.Equal(t, 6, stats.SuccessCount)
		assert.Equal(t, 2, stats.FailureCount)
		assert.Equal(t, 2, stats.FlakeCount)
		assert.InDelta(t, 0.8, stats.SuccessRate, 1e-9) // (6+2)/10
	})

	t.Run("flake as failure", func(t *testing.T) {
		stats := count.ToTestStats(true)
		assert.Equal(t, 6, stats.SuccessCount)
		assert.Equal(t, 2, stats.FailureCount)
		assert.Equal(t, 2, stats.FlakeCount)
		assert.InDelta(t, 0.6, stats.SuccessRate, 1e-9) // 6/10
	})
}

func TestStatsAdd(t *testing.T) {
	a := NewTestStats(5, 3, 2, false)
	b := NewTestStats(3, 1, 1, false)

	t.Run("recalculates rate on add", func(t *testing.T) {
		result := a.Add(b, false)
		assert.Equal(t, 8, result.SuccessCount)
		assert.Equal(t, 4, result.FailureCount)
		assert.Equal(t, 3, result.FlakeCount)
		// (8+3)/15 = 11/15
		assert.InDelta(t, 11.0/15.0, result.SuccessRate, 1e-9)
	})

	t.Run("recalculates with flakes as failure", func(t *testing.T) {
		result := a.Add(b, true)
		assert.Equal(t, 8, result.SuccessCount)
		assert.Equal(t, 4, result.FailureCount)
		assert.Equal(t, 3, result.FlakeCount)
		// 8/15
		assert.InDelta(t, 8.0/15.0, result.SuccessRate, 1e-9)
	})
}

func TestStatsFailPassWithFlakes(t *testing.T) {
	stats := NewTestStats(5, 3, 2, false)

	t.Run("flakes as failure", func(t *testing.T) {
		fail, pass := stats.FailPassWithFlakes(true)
		assert.Equal(t, 5, fail) // 3 failures + 2 flakes
		assert.Equal(t, 5, pass) // 5 success only
	})

	t.Run("flakes as success", func(t *testing.T) {
		fail, pass := stats.FailPassWithFlakes(false)
		assert.Equal(t, 3, fail) // 3 failures only
		assert.Equal(t, 7, pass) // 5 success + 2 flakes
	})
}

func TestStatsAddTestCount(t *testing.T) {
	stats := NewTestStats(5, 2, 1, false)
	count := Count{TotalCount: 10, SuccessCount: 6, FlakeCount: 2}
	// count.Failures() = 10 - 6 - 2 = 2

	t.Run("flakes as success", func(t *testing.T) {
		result := stats.AddTestCount(count, false)
		assert.Equal(t, 11, result.SuccessCount) // 5 + 6
		assert.Equal(t, 4, result.FailureCount)  // 2 + 2
		assert.Equal(t, 3, result.FlakeCount)    // 1 + 2
		// (11+3)/18
		assert.InDelta(t, 14.0/18.0, result.SuccessRate, 1e-9)
	})

	t.Run("flakes as failure", func(t *testing.T) {
		result := stats.AddTestCount(count, true)
		assert.Equal(t, 11, result.SuccessCount)
		assert.Equal(t, 4, result.FailureCount)
		assert.Equal(t, 3, result.FlakeCount)
		// 11/18
		assert.InDelta(t, 11.0/18.0, result.SuccessRate, 1e-9)
	})

	t.Run("with inconsistent count data", func(t *testing.T) {
		bad := Count{TotalCount: 5, SuccessCount: 3, FlakeCount: 3}
		// bad.Failures() should be 0, not negative
		result := stats.AddTestCount(bad, false)
		assert.Equal(t, 8, result.SuccessCount) // 5 + 3
		assert.Equal(t, 2, result.FailureCount) // 2 + 0
		assert.Equal(t, 4, result.FlakeCount)   // 1 + 3
	})
}

func TestStatsPasses(t *testing.T) {
	stats := NewTestStats(5, 3, 2, false)

	t.Run("without flakes as failure", func(t *testing.T) {
		assert.Equal(t, 7, stats.Passes(false)) // 5 + 2
	})

	t.Run("with flakes as failure", func(t *testing.T) {
		assert.Equal(t, 5, stats.Passes(true)) // 5 only
	})
}

func TestStatsTotal(t *testing.T) {
	stats := NewTestStats(5, 3, 2, false)
	assert.Equal(t, 10, stats.Total())

	empty := NewTestStats(0, 0, 0, false)
	assert.Equal(t, 0, empty.Total())
}

func TestStringForStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   Status
		expected string
	}{
		{"ExtremeRegression", ExtremeRegression, "Extreme"},
		{"SignificantRegression", SignificantRegression, "Significant"},
		{"ExtremeTriagedRegression", ExtremeTriagedRegression, "ExtremeTriaged"},
		{"SignificantTriagedRegression", SignificantTriagedRegression, "SignificantTriaged"},
		{"MissingSample", MissingSample, "MissingSample"},
		{"FixedRegression", FixedRegression, "Fixed"},
		{"FailedFixedRegression", FailedFixedRegression, "FailedFixed"},
		{"NotSignificant falls through to Unknown", NotSignificant, "Unknown"},
		{"MissingBasis falls through to Unknown", MissingBasis, "Unknown"},
		{"MissingBasisAndSample falls through to Unknown", MissingBasisAndSample, "Unknown"},
		{"SignificantImprovement falls through to Unknown", SignificantImprovement, "Unknown"},
		{"undefined status", Status(9999), "Unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, StringForStatus(tt.status))
		})
	}
}

func TestKeyWithVariantsKeyOrDie(t *testing.T) {
	t.Run("stable serialization regardless of insertion order", func(t *testing.T) {
		k1 := KeyWithVariants{
			TestID:   "test-1",
			Variants: map[string]string{"b": "2", "a": "1"},
		}
		k2 := KeyWithVariants{
			TestID:   "test-1",
			Variants: map[string]string{"a": "1", "b": "2"},
		}
		assert.Equal(t, k1.KeyOrDie(), k2.KeyOrDie())
	})

	t.Run("different test IDs produce different keys", func(t *testing.T) {
		k1 := KeyWithVariants{TestID: "test-1", Variants: map[string]string{"a": "1"}}
		k2 := KeyWithVariants{TestID: "test-2", Variants: map[string]string{"a": "1"}}
		assert.NotEqual(t, k1.KeyOrDie(), k2.KeyOrDie())
	})

	t.Run("different variants produce different keys", func(t *testing.T) {
		k1 := KeyWithVariants{TestID: "test-1", Variants: map[string]string{"a": "1"}}
		k2 := KeyWithVariants{TestID: "test-1", Variants: map[string]string{"a": "2"}}
		assert.NotEqual(t, k1.KeyOrDie(), k2.KeyOrDie())
	})

	t.Run("nil variants", func(t *testing.T) {
		k := KeyWithVariants{TestID: "test-1"}
		result := k.KeyOrDie()
		assert.Contains(t, result, "test-1")
	})
}

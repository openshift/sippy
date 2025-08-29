package componentreadiness

import (
	"database/sql"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/db/models"
	"github.com/stretchr/testify/assert"
)

func TestCalculateEditDistance(t *testing.T) {
	tests := []struct {
		name     string
		s1       string
		s2       string
		expected int
	}{
		{
			name:     "identical strings",
			s1:       "test",
			s2:       "test",
			expected: 0,
		},
		{
			name:     "completely different strings",
			s1:       "abc",
			s2:       "xyz",
			expected: 3,
		},
		{
			name:     "single character difference",
			s1:       "test",
			s2:       "best",
			expected: 1,
		},
		{
			name:     "empty strings",
			s1:       "",
			s2:       "",
			expected: 0,
		},
		{
			name:     "one empty string",
			s1:       "test",
			s2:       "",
			expected: 4,
		},
		{
			name:     "typical test name similarity",
			s1:       "[sig-storage] PersistentVolumes-test-name",
			s2:       "[sig-storage] PersistentVolume-test-name",
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateEditDistance(tt.s1, tt.s2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsSimilarTestName(t *testing.T) {
	tests := []struct {
		name           string
		testName1      string
		testName2      string
		expectSimilar  bool
		expectDistance int
	}{
		{
			name:           "identical names",
			testName1:      "test-name",
			testName2:      "test-name",
			expectSimilar:  true,
			expectDistance: 0,
		},
		{
			name:           "similar names within threshold",
			testName1:      "test-name-1",
			testName2:      "test-name-2",
			expectSimilar:  true,
			expectDistance: 1,
		},
		{
			name:           "similar names at threshold boundary",
			testName1:      "abcde",
			testName2:      "fghij",
			expectSimilar:  true,
			expectDistance: 5,
		},
		{
			name:           "different names beyond threshold",
			testName1:      "completely-different-test-name",
			testName2:      "another-unrelated-test",
			expectSimilar:  false,
			expectDistance: 23,
		},
		{
			name:           "empty names",
			testName1:      "",
			testName2:      "",
			expectSimilar:  true,
			expectDistance: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			similar, distance := isSimilarTestName(tt.testName1, tt.testName2)
			assert.Equal(t, tt.expectSimilar, similar)
			assert.Equal(t, tt.expectDistance, distance)
		})
	}
}

func TestIsSameLastFailure(t *testing.T) {
	testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	differentTime := time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		time1    sql.NullTime
		time2    sql.NullTime
		expected bool
	}{
		{
			name:     "both null times",
			time1:    sql.NullTime{Valid: false},
			time2:    sql.NullTime{Valid: false},
			expected: true,
		},
		{
			name:     "same valid times",
			time1:    sql.NullTime{Time: testTime, Valid: true},
			time2:    sql.NullTime{Time: testTime, Valid: true},
			expected: true,
		},
		{
			name:     "different valid times",
			time1:    sql.NullTime{Time: testTime, Valid: true},
			time2:    sql.NullTime{Time: differentTime, Valid: true},
			expected: false,
		},
		{
			name:     "one null one valid",
			time1:    sql.NullTime{Valid: false},
			time2:    sql.NullTime{Time: testTime, Valid: true},
			expected: false,
		},
		{
			name:     "one valid one null",
			time1:    sql.NullTime{Time: testTime, Valid: true},
			time2:    sql.NullTime{Valid: false},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSameLastFailure(tt.time1, tt.time2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateConfidenceLevel(t *testing.T) {
	tests := []struct {
		name     string
		match    PotentialMatch
		expected int
	}{
		{
			name: "no matches",
			match: PotentialMatch{
				SimilarlyNamedTests: []SimilarlyNamedTest{},
				SameLastFailures:    []models.TestRegression{},
			},
			expected: 0,
		},
		{
			name: "single similar name with low edit distance",
			match: PotentialMatch{
				SimilarlyNamedTests: []SimilarlyNamedTest{
					{EditDistance: 1},
				},
				SameLastFailures: []models.TestRegression{},
			},
			expected: 4, // 5 - 1 = 4
		},
		{
			name: "single similar name with high edit distance",
			match: PotentialMatch{
				SimilarlyNamedTests: []SimilarlyNamedTest{
					{EditDistance: 5},
				},
				SameLastFailures: []models.TestRegression{},
			},
			expected: 0, // 5 - 5 = 0
		},
		{
			name: "single same last failure",
			match: PotentialMatch{
				SimilarlyNamedTests: []SimilarlyNamedTest{},
				SameLastFailures:    []models.TestRegression{{}},
			},
			expected: 1,
		},
		{
			name: "multiple matches",
			match: PotentialMatch{
				SimilarlyNamedTests: []SimilarlyNamedTest{
					{EditDistance: 1}, // 4 points
					{EditDistance: 2}, // 3 points
				},
				SameLastFailures: []models.TestRegression{{}, {}}, // 2 points
			},
			expected: 9, // 4 + 3 + 2 = 9
		},
		{
			name: "score capped at 10",
			match: PotentialMatch{
				SimilarlyNamedTests: []SimilarlyNamedTest{
					{EditDistance: 0}, // 5 points
					{EditDistance: 0}, // 5 points
					{EditDistance: 0}, // 5 points
				},
				SameLastFailures: []models.TestRegression{{}, {}, {}}, // 3 points
			},
			expected: 10, // capped at 10
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateConfidenceLevel(tt.match)
			assert.Equal(t, tt.expected, result)
		})
	}
}

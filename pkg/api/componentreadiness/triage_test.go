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

func TestPotentialMatch_CalculateConfidenceLevel(t *testing.T) {
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
			expected: 1, // min of 1
		},
		{
			name: "single similar name with low edit distance",
			match: PotentialMatch{
				SimilarlyNamedTests: []SimilarlyNamedTest{
					{EditDistance: 1},
				},
				SameLastFailures: []models.TestRegression{},
			},
			expected: 5, // 6 - 1 = 5
		},
		{
			name: "single similar name with high edit distance",
			match: PotentialMatch{
				SimilarlyNamedTests: []SimilarlyNamedTest{
					{EditDistance: 5},
				},
				SameLastFailures: []models.TestRegression{},
			},
			expected: 1, // 5 - 5 = 0; min of 1
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
					{EditDistance: 1}, // 5 points
					{EditDistance: 2}, // 3 points
				},
				SameLastFailures: []models.TestRegression{{}, {}}, // 2 points
			},
			// nolint:gocritic
			expected: 10, // 5 + 3 + 2 = 10
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
			result := tt.match.calculateConfidenceLevel()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCompareTriageObjects(t *testing.T) {
	now := time.Now()
	resolvedTime := sql.NullTime{Time: now, Valid: true}
	bugID1 := uint(123)
	bugID2 := uint(456)

	testCases := []struct {
		name            string
		oldTriage       *models.Triage
		newTriage       *models.Triage
		expectedChanges []FieldChange
	}{
		{
			name:            "no changes - identical triages",
			oldTriage:       &models.Triage{URL: "https://issues.redhat.com/browse/OCPBUGS-1234", Description: "test"},
			newTriage:       &models.Triage{URL: "https://issues.redhat.com/browse/OCPBUGS-1234", Description: "test"},
			expectedChanges: nil,
		},
		{
			name:      "create operation - empty old triage",
			oldTriage: &models.Triage{},
			newTriage: &models.Triage{
				URL:         "https://issues.redhat.com/browse/OCPBUGS-5678",
				Description: "New triage",
				Type:        models.TriageTypeProduct,
				BugID:       &bugID1,
				Regressions: []models.TestRegression{{ID: 1}, {ID: 2}},
			},
			expectedChanges: []FieldChange{
				{FieldName: "url", Original: "", Modified: "https://issues.redhat.com/browse/OCPBUGS-5678"},
				{FieldName: "description", Original: "", Modified: "New triage"},
				{FieldName: "type", Original: "", Modified: "product"},
				{FieldName: "bug_id", Original: "", Modified: "123"},
				{FieldName: "regressions", Original: "", Modified: "[1 2]"},
			},
		},
		{
			name: "delete operation - empty new triage",
			oldTriage: &models.Triage{
				URL:         "https://issues.redhat.com/browse/OCPBUGS-9999",
				Description: "Old triage",
				Type:        models.TriageTypeProduct,
				BugID:       &bugID1,
				Regressions: []models.TestRegression{{ID: 3}, {ID: 4}},
			},
			newTriage: &models.Triage{},
			expectedChanges: []FieldChange{
				{FieldName: "url", Original: "https://issues.redhat.com/browse/OCPBUGS-9999", Modified: ""},
				{FieldName: "description", Original: "Old triage", Modified: ""},
				{FieldName: "type", Original: "product", Modified: ""},
				{FieldName: "bug_id", Original: "123", Modified: ""},
				{FieldName: "regressions", Original: "[3 4]", Modified: ""},
			},
		},
		{
			name: "update operation - multiple field changes",
			oldTriage: &models.Triage{
				URL:         "https://issues.redhat.com/browse/OCPBUGS-1111",
				Description: "Old description",
				Type:        models.TriageTypeProduct,
				BugID:       &bugID1,
				Regressions: []models.TestRegression{{ID: 1}, {ID: 2}},
			},
			newTriage: &models.Triage{
				URL:         "https://issues.redhat.com/browse/OCPBUGS-2222",
				Description: "New description",
				Type:        models.TriageTypeCIInfra,
				BugID:       &bugID2,
				Regressions: []models.TestRegression{{ID: 3}, {ID: 4}},
			},
			expectedChanges: []FieldChange{
				{FieldName: "url", Original: "https://issues.redhat.com/browse/OCPBUGS-1111", Modified: "https://issues.redhat.com/browse/OCPBUGS-2222"},
				{FieldName: "description", Original: "Old description", Modified: "New description"},
				{FieldName: "type", Original: "product", Modified: "ci-infra"},
				{FieldName: "bug_id", Original: "123", Modified: "456"},
				{FieldName: "regressions", Original: "[1 2]", Modified: "[3 4]"},
			},
		},
		{
			name: "resolved field changes",
			oldTriage: &models.Triage{
				URL: "https://issues.redhat.com/browse/OCPBUGS-3333",
			},
			newTriage: &models.Triage{
				URL:      "https://issues.redhat.com/browse/OCPBUGS-3333",
				Resolved: resolvedTime,
			},
			expectedChanges: []FieldChange{
				{FieldName: "resolved", Original: "", Modified: now.String()},
			},
		},
		{
			name: "bug ID nil to value",
			oldTriage: &models.Triage{
				URL: "https://issues.redhat.com/browse/OCPBUGS-4444",
			},
			newTriage: &models.Triage{
				URL:   "https://issues.redhat.com/browse/OCPBUGS-4444",
				BugID: &bugID1,
			},
			expectedChanges: []FieldChange{
				{FieldName: "bug_id", Original: "", Modified: "123"},
			},
		},
		{
			name: "bug ID value to nil",
			oldTriage: &models.Triage{
				URL:   "https://issues.redhat.com/browse/OCPBUGS-5555",
				BugID: &bugID1,
			},
			newTriage: &models.Triage{
				URL: "https://issues.redhat.com/browse/OCPBUGS-5555",
			},
			expectedChanges: []FieldChange{
				{FieldName: "bug_id", Original: "123", Modified: ""},
			},
		},
		{
			name: "regressions order independence",
			oldTriage: &models.Triage{
				URL:         "https://issues.redhat.com/browse/OCPBUGS-6666",
				Regressions: []models.TestRegression{{ID: 2}, {ID: 1}}, // Different order
			},
			newTriage: &models.Triage{
				URL:         "https://issues.redhat.com/browse/OCPBUGS-6666",
				Regressions: []models.TestRegression{{ID: 1}, {ID: 2}}, // Different order
			},
			expectedChanges: nil, // Should be no changes due to sorting
		},
		{
			name: "empty regressions",
			oldTriage: &models.Triage{
				URL:         "https://issues.redhat.com/browse/OCPBUGS-7777",
				Regressions: []models.TestRegression{{ID: 1}, {ID: 2}},
			},
			newTriage: &models.Triage{
				URL:         "https://issues.redhat.com/browse/OCPBUGS-7777",
				Regressions: []models.TestRegression{}, // Empty slice
			},
			expectedChanges: []FieldChange{
				{FieldName: "regressions", Original: "[1 2]", Modified: ""},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := compareTriageObjects(tc.oldTriage, tc.newTriage)
			assert.Equal(t, tc.expectedChanges, result, "Expected changes should match actual changes")
		})
	}
}

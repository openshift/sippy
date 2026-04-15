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

func TestCalculateJobRunOverlap(t *testing.T) {
	tests := []struct {
		name            string
		candidateRunIDs map[string]bool
		triageReg       models.TestRegression
		expectNil       bool
		expectShared    []string
		expectPercent   float64
	}{
		{
			name:            "no candidate runs",
			candidateRunIDs: map[string]bool{},
			triageReg:       models.TestRegression{JobRuns: []models.RegressionJobRun{{ProwJobRunID: "run-1"}}},
			expectNil:       true,
		},
		{
			name:            "no triage runs",
			candidateRunIDs: map[string]bool{"run-1": true},
			triageReg:       models.TestRegression{},
			expectNil:       true,
		},
		{
			name:            "no overlap",
			candidateRunIDs: map[string]bool{"run-1": true, "run-2": true},
			triageReg: models.TestRegression{JobRuns: []models.RegressionJobRun{
				{ProwJobRunID: "run-3"},
				{ProwJobRunID: "run-4"},
			}},
			expectNil: true,
		},
		{
			name:            "full overlap same size",
			candidateRunIDs: map[string]bool{"run-1": true, "run-2": true},
			triageReg: models.TestRegression{JobRuns: []models.RegressionJobRun{
				{ProwJobRunID: "run-1"},
				{ProwJobRunID: "run-2"},
			}},
			expectShared:  []string{"run-1", "run-2"},
			expectPercent: 100,
		},
		{
			name:            "partial overlap",
			candidateRunIDs: map[string]bool{"run-1": true, "run-2": true, "run-3": true, "run-4": true},
			triageReg: models.TestRegression{JobRuns: []models.RegressionJobRun{
				{ProwJobRunID: "run-1"},
				{ProwJobRunID: "run-2"},
				{ProwJobRunID: "run-5"},
				{ProwJobRunID: "run-6"},
			}},
			expectShared:  []string{"run-1", "run-2"},
			expectPercent: 50, // 2 shared / 4 (min of both sets)
		},
		{
			name:            "overlap uses smaller set as denominator",
			candidateRunIDs: map[string]bool{"run-1": true, "run-2": true},
			triageReg: models.TestRegression{JobRuns: []models.RegressionJobRun{
				{ProwJobRunID: "run-1"},
				{ProwJobRunID: "run-2"},
				{ProwJobRunID: "run-3"},
				{ProwJobRunID: "run-4"},
				{ProwJobRunID: "run-5"},
				{ProwJobRunID: "run-6"},
			}},
			expectShared:  []string{"run-1", "run-2"},
			expectPercent: 100, // 2 shared / 2 (candidate is smaller)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateJobRunOverlap(tt.candidateRunIDs, tt.triageReg)
			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.ElementsMatch(t, tt.expectShared, result.SharedJobRunIDs)
				assert.InDelta(t, tt.expectPercent, result.OverlapPercent, 0.1)
			}
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
			name: "no matches gives minimum score",
			match: PotentialMatch{
				SimilarlyNamedTests: []SimilarlyNamedTest{},
				OverlappingJobRuns:  []JobRunOverlap{},
			},
			expected: 1,
		},
		{
			name: "single similar name with low edit distance",
			match: PotentialMatch{
				SimilarlyNamedTests: []SimilarlyNamedTest{
					{EditDistance: 1},
				},
			},
			expected: 5, // 6 - 1 = 5
		},
		{
			name: "single similar name with high edit distance",
			match: PotentialMatch{
				SimilarlyNamedTests: []SimilarlyNamedTest{
					{EditDistance: 5},
				},
			},
			expected: 1, // 6 - 5 = 1
		},
		{
			name: "high job run overlap dominates",
			match: PotentialMatch{
				OverlappingJobRuns: []JobRunOverlap{
					{OverlapPercent: 80},
				},
			},
			expected: 9, // int(80/10) + 1 = 9
		},
		{
			name: "full job run overlap",
			match: PotentialMatch{
				OverlappingJobRuns: []JobRunOverlap{
					{OverlapPercent: 100},
				},
			},
			expected: 10, // int(100/10) + 1 = 11 → capped at 10
		},
		{
			name: "low job run overlap",
			match: PotentialMatch{
				OverlappingJobRuns: []JobRunOverlap{
					{OverlapPercent: 10},
				},
			},
			expected: 2, // int(10/10) + 1 = 2
		},
		{
			name: "overlap plus similar name",
			match: PotentialMatch{
				OverlappingJobRuns: []JobRunOverlap{
					{OverlapPercent: 50}, // int(50/10)+1 = 6
				},
				SimilarlyNamedTests: []SimilarlyNamedTest{
					{EditDistance: 2}, // 6 - 2 = 4
				},
			},
			expected: 10, // 6 + 4 = 10
		},
		{
			name: "score capped at 10",
			match: PotentialMatch{
				OverlappingJobRuns: []JobRunOverlap{
					{OverlapPercent: 100}, // 11
				},
				SimilarlyNamedTests: []SimilarlyNamedTest{
					{EditDistance: 0}, // 6
				},
			},
			expected: 10,
		},
		{
			name: "best overlap used across multiple regressions",
			match: PotentialMatch{
				OverlappingJobRuns: []JobRunOverlap{
					{OverlapPercent: 20}, // 3
					{OverlapPercent: 70}, // 8 ← best
				},
			},
			expected: 8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.match.calculateConfidenceLevel()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDeterminePotentialMatch(t *testing.T) {
	tests := []struct {
		name           string
		regression     models.TestRegression
		triage         *models.Triage
		expectNil      bool
		expectOverlaps int
		expectSimilar  int
	}{
		{
			name:       "already linked returns nil",
			regression: models.TestRegression{ID: 1, TestName: "TestFoo"},
			triage: &models.Triage{
				Regressions: []models.TestRegression{{ID: 1}},
			},
			expectNil: true,
		},
		{
			name: "no matching criteria returns nil",
			regression: models.TestRegression{
				ID:       1,
				TestName: "CompletelyDifferent",
				JobRuns:  []models.RegressionJobRun{{ProwJobRunID: "run-1"}},
			},
			triage: &models.Triage{
				Regressions: []models.TestRegression{{
					ID:       2,
					TestName: "AnotherTotallyUnrelatedTest",
					JobRuns:  []models.RegressionJobRun{{ProwJobRunID: "run-99"}},
				}},
			},
			expectNil: true,
		},
		{
			name: "match by job run overlap",
			regression: models.TestRegression{
				ID:       1,
				TestName: "CompletelyDifferent",
				JobRuns:  []models.RegressionJobRun{{ProwJobRunID: "run-1"}, {ProwJobRunID: "run-2"}},
			},
			triage: &models.Triage{
				Regressions: []models.TestRegression{{
					ID:       2,
					TestName: "AnotherUnrelated",
					JobRuns:  []models.RegressionJobRun{{ProwJobRunID: "run-1"}, {ProwJobRunID: "run-3"}},
				}},
			},
			expectOverlaps: 1,
		},
		{
			name: "match by similar name only",
			regression: models.TestRegression{
				ID:       1,
				TestName: "TestSomething",
			},
			triage: &models.Triage{
				Regressions: []models.TestRegression{{
					ID:       2,
					TestName: "TestSomethng", // edit distance 1
				}},
			},
			expectSimilar: 1,
		},
		{
			name: "match by both name and overlap",
			regression: models.TestRegression{
				ID:       1,
				TestName: "TestSomething",
				JobRuns:  []models.RegressionJobRun{{ProwJobRunID: "run-1"}},
			},
			triage: &models.Triage{
				Regressions: []models.TestRegression{{
					ID:       2,
					TestName: "TestSomethng",
					JobRuns:  []models.RegressionJobRun{{ProwJobRunID: "run-1"}},
				}},
			},
			expectOverlaps: 1,
			expectSimilar:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determinePotentialMatch(tt.regression, tt.triage)
			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Len(t, result.OverlappingJobRuns, tt.expectOverlaps)
				assert.Len(t, result.SimilarlyNamedTests, tt.expectSimilar)
			}
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

func TestValidateJiraPrefix(t *testing.T) {
	testCases := []struct {
		name               string
		url                string
		expectedValidation bool
	}{
		{
			name:               "issues.redhat.com",
			url:                "https://issues.redhat.com/browse/OCPBUGS-1234",
			expectedValidation: true,
		},
		{
			name:               "redhat.atlassian.net",
			url:                "https://redhat.atlassian.net/browse/OCPBUGS-1234",
			expectedValidation: true,
		},
		{
			name:               "invalid",
			url:                "https://invalid.redhat.com/browse/OCPBUGS-1234",
			expectedValidation: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := validateJiraPrefix(tc.url)
			assert.Equal(t, tc.expectedValidation, result, "Expected validation should match actual validation")
		})
	}
}

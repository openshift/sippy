package variantregistry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompareVariants(t *testing.T) {
	tests := []struct {
		name             string
		currentVariants  map[string]map[string]string
		expectedVariants map[string]map[string]string

		expectedInserts    []jobVariant
		expectedUpdates    []jobVariant
		expectedDeletes    []jobVariant
		expectedDeleteJobs []string
	}{
		{
			name:            "initial population",
			currentVariants: map[string]map[string]string{},
			expectedVariants: map[string]map[string]string{
				"job1": {
					"a": "1",
					"b": "2",
				},
			},
			expectedInserts: []jobVariant{
				{
					JobName:      "job1",
					VariantName:  "a",
					VariantValue: "1",
				},
				{
					JobName:      "job1",
					VariantName:  "b",
					VariantValue: "2",
				},
			},
			expectedUpdates:    []jobVariant{},
			expectedDeletes:    []jobVariant{},
			expectedDeleteJobs: []string{},
		},
		{
			name: "variants changed for existing job",
			currentVariants: map[string]map[string]string{
				"job1": {
					"a": "1",
					"b": "2",
					"d": "4",
				},
			},
			expectedVariants: map[string]map[string]string{
				"job1": {
					"a": "5",
					"b": "2",
					"c": "3",
				},
			},
			expectedInserts: []jobVariant{
				{
					JobName:      "job1",
					VariantName:  "c",
					VariantValue: "3",
				},
			},
			expectedUpdates: []jobVariant{
				{
					JobName:      "job1",
					VariantName:  "a",
					VariantValue: "5",
				},
			},
			expectedDeletes: []jobVariant{
				{
					JobName:      "job1",
					VariantName:  "d",
					VariantValue: "4",
				},
			},
			expectedDeleteJobs: []string{},
		},
		{
			name: "job removed",
			currentVariants: map[string]map[string]string{
				"job1": {
					"a": "1",
					"b": "2",
				},
			},
			expectedVariants:   map[string]map[string]string{},
			expectedInserts:    []jobVariant{},
			expectedUpdates:    []jobVariant{},
			expectedDeletes:    []jobVariant{},
			expectedDeleteJobs: []string{"job1"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inserts, updates, deletes, deleteJobs := compareVariants(test.expectedVariants, test.currentVariants)
			assert.ElementsMatch(t, test.expectedInserts, inserts, "mismatched inserts")
			assert.ElementsMatch(t, test.expectedUpdates, updates, "mismatched updates")
			assert.ElementsMatch(t, test.expectedDeletes, deletes, "mismatched deletes")
			assert.ElementsMatch(t, test.expectedDeleteJobs, deleteJobs, "mismatched delete jobs")
		})
	}
}

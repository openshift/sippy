package bigquery

import (
	"reflect"
	"testing"

	configv1 "github.com/openshift/sippy/pkg/apis/config/v1"
	"github.com/stretchr/testify/assert"
)

func TestCopyIncludeVariantsAndRemoveOverrides(t *testing.T) {
	tests := []struct {
		name              string
		overrides         []configv1.VariantJunitTableOverride
		currOverride      int
		includeVariants   map[string][]string
		expected          map[string][]string
		expectedSkipQuery bool
	}{
		{
			name:         "No overrides, no variants removed",
			overrides:    []configv1.VariantJunitTableOverride{},
			currOverride: -1,
			includeVariants: map[string][]string{
				"key1": {"value1", "value2"},
				"key2": {"value3"},
			},
			expected: map[string][]string{
				"key1": {"value1", "value2"},
				"key2": {"value3"},
			},
		},
		{
			name: "Single override removes matching variant",
			overrides: []configv1.VariantJunitTableOverride{
				{VariantName: "key1", VariantValue: "value1"},
			},
			currOverride: -1,
			includeVariants: map[string][]string{
				"key1": {"value1", "value2"},
				"key2": {"value3"},
			},
			expected: map[string][]string{
				"key1": {"value2"},
				"key2": {"value3"},
			},
		},
		{
			name: "Override does not remove its own variant",
			overrides: []configv1.VariantJunitTableOverride{
				{VariantName: "key1", VariantValue: "value1"},
			},
			currOverride: 0,
			includeVariants: map[string][]string{
				"key1": {"value1", "value2"},
				"key2": {"value3"},
			},
			expected: map[string][]string{
				"key1": {"value1", "value2"},
				"key2": {"value3"},
			},
		},
		{
			name: "Multiple overrides remove multiple variants",
			overrides: []configv1.VariantJunitTableOverride{
				{VariantName: "key1", VariantValue: "value1"},
				{VariantName: "key2", VariantValue: "value3"},
			},
			currOverride: -1,
			includeVariants: map[string][]string{
				"key1": {"value1", "value2"},
				"key2": {"value3", "value4"},
			},
			expected: map[string][]string{
				"key1": {"value2"},
				"key2": {"value4"},
			},
		},
		{
			name: "All variants removed",
			overrides: []configv1.VariantJunitTableOverride{
				{VariantName: "key1", VariantValue: "value1"},
				{VariantName: "key1", VariantValue: "value2"},
				{VariantName: "key2", VariantValue: "value3"},
			},
			currOverride: -1,
			includeVariants: map[string][]string{
				"key1": {"value1", "value2"},
				"key2": {"value3"},
			},
			expected:          map[string][]string{},
			expectedSkipQuery: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, skipQuery := copyIncludeVariantsAndRemoveOverrides(tt.overrides, tt.currOverride, tt.includeVariants)
			assert.Equal(t, tt.expectedSkipQuery, skipQuery)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

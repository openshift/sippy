package reqopts

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAnyAreBaseOverrides(t *testing.T) {
	tests := []struct {
		name     string
		opts     []TestIdentification
		expected bool
	}{
		{
			name:     "nil slice returns false",
			opts:     nil,
			expected: false,
		},
		{
			name:     "empty slice returns false",
			opts:     []TestIdentification{},
			expected: false,
		},
		{
			name: "single item with empty BaseOverrideRelease returns false",
			opts: []TestIdentification{
				{TestID: "test-1"},
			},
			expected: false,
		},
		{
			name: "single item with BaseOverrideRelease set returns true",
			opts: []TestIdentification{
				{TestID: "test-1", BaseOverrideRelease: "4.20"},
			},
			expected: true,
		},
		{
			name: "multiple items none have override returns false",
			opts: []TestIdentification{
				{TestID: "test-1"},
				{TestID: "test-2"},
				{TestID: "test-3"},
			},
			expected: false,
		},
		{
			name: "multiple items only last has override returns true",
			opts: []TestIdentification{
				{TestID: "test-1"},
				{TestID: "test-2"},
				{TestID: "test-3", BaseOverrideRelease: "4.20"},
			},
			expected: true,
		},
		{
			name: "multiple items first has override returns true",
			opts: []TestIdentification{
				{TestID: "test-1", BaseOverrideRelease: "4.20"},
				{TestID: "test-2"},
				{TestID: "test-3"},
			},
			expected: true,
		},
		{
			name: "multiple items all have override returns true",
			opts: []TestIdentification{
				{TestID: "test-1", BaseOverrideRelease: "4.19"},
				{TestID: "test-2", BaseOverrideRelease: "4.20"},
				{TestID: "test-3", BaseOverrideRelease: "4.21"},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AnyAreBaseOverrides(tt.opts)
			assert.Equal(t, tt.expected, result)
		})
	}
}

package bqlabel

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeLabelValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "lowercase and replace spaces",
			input:    "Hello World",
			expected: "hello_world",
		},
		{
			name:     "already valid chars unchanged",
			input:    "test-value_123",
			expected: "test-value_123",
		},
		{
			name:     "empty string stays empty",
			input:    "",
			expected: "",
		},
		{
			name:     "truncate 64 chars to 63",
			input:    strings.Repeat("a", 64),
			expected: strings.Repeat("a", 63),
		},
		{
			name:     "exactly 63 chars unchanged",
			input:    strings.Repeat("b", 63),
			expected: strings.Repeat("b", 63),
		},
		{
			name:     "uppercase folded to lowercase",
			input:    "UPPER",
			expected: "upper",
		},
		{
			name:     "special chars replaced",
			input:    "foo@bar.com",
			expected: "foo_bar_com",
		},
		{
			name:     "non-latin chars replaced",
			input:    "日本語",
			expected: "___",
		},
		{
			name:     "slashes in URI paths replaced",
			input:    "/api/v1/test",
			expected: "_api_v1_test",
		},
		{
			name:     "dots in IP addresses replaced",
			input:    "192.168.1.1",
			expected: "192_168_1_1",
		},
		{
			name:     "spaces replaced with underscores",
			input:    "with spaces",
			expected: "with_spaces",
		},
		{
			name:     "mixed case with valid special chars",
			input:    "MiXeD-CaSe_123",
			expected: "mixed-case_123",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := sanitizeLabelValue(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasCapabilityIntersection(t *testing.T) {
	tests := []struct {
		name          string
		testCaps      []string
		requestedCaps map[string]bool
		want          bool
	}{
		{
			name:          "matching capability",
			testCaps:      []string{"install", "networking"},
			requestedCaps: map[string]bool{"install": true},
			want:          true,
		},
		{
			name:          "no matching capability",
			testCaps:      []string{"networking"},
			requestedCaps: map[string]bool{"install": true},
			want:          false,
		},
		{
			name:          "empty test caps",
			testCaps:      []string{},
			requestedCaps: map[string]bool{"install": true},
			want:          false,
		},
		{
			name:          "empty requested caps",
			testCaps:      []string{"install"},
			requestedCaps: map[string]bool{},
			want:          false,
		},
		{
			name:          "multiple overlapping capabilities",
			testCaps:      []string{"install", "networking", "storage"},
			requestedCaps: map[string]bool{"networking": true, "storage": true},
			want:          true,
		},
		{
			name:          "nil test caps",
			testCaps:      nil,
			requestedCaps: map[string]bool{"install": true},
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasCapabilityIntersection(tt.testCaps, tt.requestedCaps)
			assert.Equal(t, tt.want, got)
		})
	}
}

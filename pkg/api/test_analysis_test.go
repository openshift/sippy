package api

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openshift/sippy/pkg/filter"
)

func TestExtractVariantFilters(t *testing.T) {
	tests := []struct {
		name            string
		filters         *filter.Filter
		expectedAllowed []string
		expectedBlocked []string
	}{
		{
			name:            "nil filter",
			filters:         nil,
			expectedAllowed: nil,
			expectedBlocked: nil,
		},
		{
			name:            "no variant filters",
			filters:         &filter.Filter{Items: []filter.FilterItem{{Field: "name", Value: "test1"}}},
			expectedAllowed: nil,
			expectedBlocked: nil,
		},
		{
			name: "allowed variants",
			filters: &filter.Filter{Items: []filter.FilterItem{
				{Field: "variants", Value: "Platform:aws"},
				{Field: "variants", Value: "Network:ovn"},
			}},
			expectedAllowed: []string{"Platform:aws", "Network:ovn"},
			expectedBlocked: nil,
		},
		{
			name: "blocked variants",
			filters: &filter.Filter{Items: []filter.FilterItem{
				{Field: "variants", Value: "Platform:aws", Not: true},
			}},
			expectedAllowed: nil,
			expectedBlocked: []string{"Platform:aws"},
		},
		{
			name: "mixed allowed and blocked",
			filters: &filter.Filter{Items: []filter.FilterItem{
				{Field: "variants", Value: "Platform:aws"},
				{Field: "variants", Value: "Topology:single", Not: true},
				{Field: "name", Value: "ignored"},
			}},
			expectedAllowed: []string{"Platform:aws"},
			expectedBlocked: []string{"Topology:single"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, blocked := extractVariantFilters(tt.filters)
			assert.Equal(t, tt.expectedAllowed, allowed)
			assert.Equal(t, tt.expectedBlocked, blocked)
		})
	}
}

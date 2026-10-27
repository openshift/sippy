package crtest

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSpotCheckTestID(t *testing.T) {
	assert.True(t, IsSpotCheckTestID("spotcheck-30d:etcd:scaling"))
	assert.False(t, IsSpotCheckTestID("openshift-tests:some-real-test"))
	assert.False(t, IsSpotCheckTestID(""))
}

func TestParseSpotCheckTestID(t *testing.T) {
	tests := []struct {
		name           string
		testID         string
		wantSample     string
		wantComponent  string
		wantCapability string
	}{
		{
			name:           "valid ID",
			testID:         "spotcheck-30d:etcd:scaling",
			wantSample:     "spotcheck-30d",
			wantComponent:  "etcd",
			wantCapability: "scaling",
		},
		{
			name:           "capability with dashes becomes spaces",
			testID:         "spotcheck-30d:node-/-kubelet:cpu-partitioning",
			wantSample:     "spotcheck-30d",
			wantComponent:  "node-/-kubelet",
			wantCapability: "cpu partitioning",
		},
		{
			name:   "too few parts",
			testID: "spotcheck-30d:etcd",
		},
		{
			name:   "empty",
			testID: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sample, comp, capability := ParseSpotCheckTestID(tt.testID)
			assert.Equal(t, tt.wantSample, sample)
			assert.Equal(t, tt.wantComponent, comp)
			assert.Equal(t, tt.wantCapability, capability)
		})
	}
}

func TestSpotCheckTestIDRoundTrip(t *testing.T) {
	tests := []struct {
		name       string
		sample     string
		component  string
		capability string
	}{
		{name: "simple", sample: "spotcheck-30d", component: "Etcd", capability: "Scaling"},
		{name: "spaces in capability", sample: "spotcheck-30d", component: "Node / Kubelet", capability: "CPU Partitioning"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := SpotCheckTestID(tt.sample, tt.component, tt.capability)
			assert.True(t, IsSpotCheckTestID(id))

			sample, component, capability := ParseSpotCheckTestID(id)
			assert.Equal(t, tt.sample, sample)
			// The encoding lowercases, so parsed values match case-insensitively.
			assert.True(t, strings.EqualFold(tt.component, component), "component %q vs %q", tt.component, component)
			assert.True(t, strings.EqualFold(tt.capability, capability), "capability %q vs %q", tt.capability, capability)
		})
	}
}

package crtest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncodeStability(t *testing.T) {
	key := KeyWithVariants{
		TestID: "test-1",
		Variants: map[string]string{
			"Zebra":  "z",
			"Alpha":  "a",
			"Middle": "m",
		},
	}
	first := key.Encode()
	for range 100 {
		assert.Equal(t, first, key.Encode(), "Encode() must be deterministic")
	}
	assert.Equal(t, "test-1\x00Alpha:a\x00Middle:m\x00Zebra:z", first)
}

func TestEncodeNilVariants(t *testing.T) {
	key := KeyWithVariants{TestID: "test-nil"}
	encoded := key.Encode()
	assert.Equal(t, "test-nil", encoded)
}

func TestColumnEncodeDecodeRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		col  ColumnIdentification
	}{
		{
			name: "typical columns",
			col: ColumnIdentification{
				Variants: map[string]string{
					"Network":  "ovn",
					"Platform": "aws",
					"Topology": "ha",
				},
			},
		},
		{
			name: "single column",
			col: ColumnIdentification{
				Variants: map[string]string{"Platform": "gcp"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := tt.col.Encode()
			decoded := DecodeColumnID(encoded)
			assert.Equal(t, tt.col.Variants, decoded.Variants)
		})
	}
}

func TestVariantKeyValueToString(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		expected string
	}{
		{name: "typical variant", key: "Platform", value: "aws", expected: "Platform:aws"},
		{name: "empty value", key: "Platform", value: "", expected: "Platform:"},
		{name: "empty key", key: "", value: "aws", expected: ":aws"},
		{name: "value with colon", key: "Label", value: "a:b", expected: "Label:a:b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, VariantKeyValueToString(tt.key, tt.value))
		})
	}
}

func TestVariantStringToKeyValue(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantKey   string
		wantValue string
	}{
		{name: "typical variant", input: "Platform:aws", wantKey: "Platform", wantValue: "aws"},
		{name: "empty value", input: "Platform:", wantKey: "Platform", wantValue: ""},
		{name: "value with colon", input: "Label:a:b", wantKey: "Label", wantValue: "a:b"},
		{name: "no colon", input: "invalid", wantKey: "", wantValue: ""},
		{name: "empty string", input: "", wantKey: "", wantValue: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, value := VariantStringToKeyValue(tt.input)
			assert.Equal(t, tt.wantKey, key)
			assert.Equal(t, tt.wantValue, value)
		})
	}
}

func TestVariantRoundTrip(t *testing.T) {
	key, value := "Architecture", "amd64"
	s := VariantKeyValueToString(key, value)
	gotKey, gotValue := VariantStringToKeyValue(s)
	assert.Equal(t, key, gotKey)
	assert.Equal(t, value, gotValue)
}

func TestColumnEncodeEmpty(t *testing.T) {
	col := ColumnIdentification{Variants: map[string]string{}}
	encoded := col.Encode()
	assert.Equal(t, ColumnID(""), encoded)
	decoded := DecodeColumnID(encoded)
	assert.Empty(t, decoded.Variants)
}

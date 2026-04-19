package resolvedissues

import (
	"encoding/json"
	"testing"

	"github.com/openshift/sippy/pkg/variantregistry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTriagedIssueKey_MarshalUnmarshalRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		key  TriagedIssueKey
	}{
		{
			name: "basic key",
			key:  TriagedIssueKey{TestID: "openshift-tests.sig-auth", Variants: "aws-amd64-ovn-ha"},
		},
		{
			name: "special chars in TestID",
			key:  TriagedIssueKey{TestID: "test:with-special_chars.v2", Variants: "aws-amd64"},
		},
		{
			name: "empty Variants",
			key:  TriagedIssueKey{TestID: "some-test-id", Variants: ""},
		},
		{
			name: "empty TestID",
			key:  TriagedIssueKey{TestID: "", Variants: "aws-amd64-ovn"},
		},
		{
			name: "both fields empty",
			key:  TriagedIssueKey{TestID: "", Variants: ""},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			marshaled, err := tc.key.MarshalText()
			require.NoError(t, err)

			var got TriagedIssueKey
			err = got.UnmarshalText(marshaled)
			require.NoError(t, err)

			assert.Equal(t, tc.key, got)
		})
	}
}

func TestTriagedIssueKey_DifferentKeysProdduceDifferentSerializations(t *testing.T) {
	t.Run("differ only in Variants", func(t *testing.T) {
		key1 := TriagedIssueKey{TestID: "same-test", Variants: "aws-amd64"}
		key2 := TriagedIssueKey{TestID: "same-test", Variants: "gcp-arm64"}

		m1, err := key1.MarshalText()
		require.NoError(t, err)
		m2, err := key2.MarshalText()
		require.NoError(t, err)

		assert.NotEqual(t, string(m1), string(m2))
	})

	t.Run("differ only in TestID", func(t *testing.T) {
		key1 := TriagedIssueKey{TestID: "test-alpha", Variants: "same-variant"}
		key2 := TriagedIssueKey{TestID: "test-beta", Variants: "same-variant"}

		m1, err := key1.MarshalText()
		require.NoError(t, err)
		m2, err := key2.MarshalText()
		require.NoError(t, err)

		assert.NotEqual(t, string(m1), string(m2))
	})
}

func TestTriagedIssueKey_JSONMapKey(t *testing.T) {
	key := TriagedIssueKey{TestID: "test:with-special_chars.v2", Variants: "aws-amd64-ovn"}

	original := map[TriagedIssueKey]int{key: 42}
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var restored map[TriagedIssueKey]int
	err = json.Unmarshal(data, &restored)
	require.NoError(t, err)

	val, ok := restored[key]
	require.True(t, ok, "key should be present in restored map")
	assert.Equal(t, 42, val)
}

func TestTriagedIssueKey_JSONMapKeyMultipleEntries(t *testing.T) {
	key1 := TriagedIssueKey{TestID: "test-a", Variants: "aws"}
	key2 := TriagedIssueKey{TestID: "test-b", Variants: "gcp"}

	original := map[TriagedIssueKey]int{key1: 1, key2: 2}
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var restored map[TriagedIssueKey]int
	err = json.Unmarshal(data, &restored)
	require.NoError(t, err)

	assert.Len(t, restored, 2)
	assert.Equal(t, 1, restored[key1])
	assert.Equal(t, 2, restored[key2])
}

func TestBuildTriageMatchVariants(t *testing.T) {
	t.Run("empty slice returns nil", func(t *testing.T) {
		result := buildTriageMatchVariants([]string{})
		assert.Nil(t, result)
	})

	t.Run("single item", func(t *testing.T) {
		result := buildTriageMatchVariants([]string{"Platform"})
		require.NotNil(t, result)
		assert.Equal(t, 1, result.Len())
		assert.True(t, result.Has("Platform"))
	})

	t.Run("multiple items", func(t *testing.T) {
		input := []string{"Platform", "Architecture", "Network"}
		result := buildTriageMatchVariants(input)
		require.NotNil(t, result)
		assert.Equal(t, 3, result.Len())
		for _, v := range input {
			assert.True(t, result.Has(v), "set should contain %s", v)
		}
	})
}

func TestTriageMatchVariants_ContainsExpectedVariants(t *testing.T) {
	expected := []string{
		variantregistry.VariantPlatform,
		variantregistry.VariantArch,
		variantregistry.VariantNetwork,
		variantregistry.VariantTopology,
		variantregistry.VariantFeatureSet,
		variantregistry.VariantUpgrade,
		variantregistry.VariantSuite,
		variantregistry.VariantInstaller,
	}

	assert.Equal(t, 8, TriageMatchVariants.Len(), "TriageMatchVariants should contain exactly 8 variant names")

	for _, v := range expected {
		assert.True(t, TriageMatchVariants.Has(v), "TriageMatchVariants should contain %s", v)
	}
}

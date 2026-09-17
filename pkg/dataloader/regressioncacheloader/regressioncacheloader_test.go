package regressioncacheloader

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNew_NilDataProvider verifies that New rejects a nil dataProvider. Without
// this guard a caller would receive a valid-looking loader that later panics when
// Load dereferences the provider via dataProvider.Cache().
func TestNew_NilDataProvider(t *testing.T) {
	loader, err := New(nil, nil, nil, nil, nil, 0, 0, nil)
	require.Error(t, err)
	assert.Nil(t, loader)
	assert.Contains(t, err.Error(), "dataProvider must not be nil")
}

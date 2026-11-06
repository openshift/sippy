package redis

import (
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRedisCacheSanitizesParseErrors(t *testing.T) {
	credential := rand.Text()
	tests := []struct {
		name string
		url  string
	}{
		{
			name: "malformed port",
			url:  fmt.Sprintf("redis://user:%s@localhost:not-a-port/0", credential),
		},
		{
			name: "malformed escaped userinfo",
			url:  fmt.Sprintf("redis://user:%s%%zz@localhost:6379/0", credential),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cache, err := NewRedisCache(test.url)

			require.Error(t, err)
			assert.Nil(t, cache)
			assert.Equal(t, "invalid Redis URL configuration", err.Error())
			assert.NotContains(t, err.Error(), credential)
			assert.NotContains(t, err.Error(), test.url)
		})
	}
}

func TestNewRedisCacheAcceptsValidURL(t *testing.T) {
	credential := rand.Text()
	redisURL := fmt.Sprintf("redis://user:%s@localhost:6379/1", credential)

	cache, err := NewRedisCache(redisURL)

	require.NoError(t, err)
	require.NotNil(t, cache)
	require.NoError(t, cache.client.Close())
}

package compressed

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type PseudoCache struct {
	cache map[string][]byte
}

func (c *PseudoCache) Get(ctx context.Context, key string) ([]byte, error) {
	return c.cache[key], nil
}

func (c *PseudoCache) Set(ctx context.Context, key string, content []byte, duration time.Duration) error {
	c.cache[key] = content
	return nil
}

func TestPseudoCache(t *testing.T) {
	data := "It is useful mainly in compressed network protocols, to ensure that a remote reader has enough data to reconstruct a packet. Flush does not return until the data has been written. If the underlying writer returns an error, Flush returns that error. "

	cache, err := NewCompressedCache(&PseudoCache{cache: make(map[string][]byte)})
	assert.Nil(t, err, "Failed to create compression cache: %v", err)

	err = cache.Set(context.TODO(), "testKey", []byte(data), time.Hour)
	assert.Nil(t, err, "Failed to set cache data: %v", err)

	cacheData, err := cache.Get(context.TODO(), "testKey")
	assert.Nil(t, err, "Failed to get cache data: %v", err)

	validation := string(cacheData)
	assert.Equal(t, data, validation)
}

func TestCompression(t *testing.T) {
	data := "It is useful mainly in compressed network protocols, to ensure that a remote reader has enough data to reconstruct a packet. Flush does not return until the data has been written. If the underlying writer returns an error, Flush returns that error. "

	compressed, checksum, err := compress([]byte(data))
	assert.Nil(t, err, "Compression failed: %v", err)
	assert.NotNil(t, compressed, "Invalid compressed content")

	uncompressed, err := uncompress(compressed, checksum)
	assert.Nil(t, err, "Uncompression failed: %v", err)
	assert.NotNil(t, uncompressed, "Invalid uncompressed content")

	validation := string(uncompressed)
	assert.Equal(t, data, validation)
}

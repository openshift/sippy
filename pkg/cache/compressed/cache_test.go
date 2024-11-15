package compressed

import (
	"context"
	"testing"
	"time"
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

	if err != nil {
		t.Fatalf("Failed to create compression cache")
	}

	err = cache.Set(context.TODO(), "testKey", []byte(data), time.Hour)
	if err != nil {
		t.Fatalf("Failed to set cache data")
	}

	cacheData, err := cache.Get(context.TODO(), "testKey")
	if err != nil {
		t.Fatalf("Failed to get cache data")
	}

	validation := string(cacheData)
	if data != validation {
		t.Fatalf("Invalid uncompressed data returned: %s", validation)
	}

}

func TestCompression(t *testing.T) {
	data := "It is useful mainly in compressed network protocols, to ensure that a remote reader has enough data to reconstruct a packet. Flush does not return until the data has been written. If the underlying writer returns an error, Flush returns that error. "

	compressed, checksum, err := compress([]byte(data))
	if err != nil {
		t.Fatalf("Compression failed: %v", err)
	}

	if compressed == nil {
		t.Fatal("Invalid compressed content")
	}

	uncompressed, err := uncompress(compressed, checksum)

	if err != nil {
		t.Fatalf("Compression failed: %v", err)
	}

	if uncompressed == nil {
		t.Fatal("Invalid compressed content")
	}

	validation := string(uncompressed)
	if data != validation {
		t.Fatalf("Invalid uncompressed data returned: %s", validation)
	}
}

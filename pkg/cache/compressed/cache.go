package compressed

import (
	"bytes"
	"compress/gzip"
	"context"

	log "github.com/sirupsen/logrus"

	// simple checksum usage for validation
	"crypto/md5" // nolint:gosec
	"fmt"
	"time"

	"github.com/openshift/sippy/pkg/apis/cache"
)

const (
	cachePrefix = "cc:"
)

type Cache struct {
	Cache cache.Cache
}

func NewCompressedCache(c cache.Cache) (*Cache, error) {
	return &Cache{
		Cache: c,
	}, nil
}

func (c Cache) Get(ctx context.Context, key string, duration time.Duration) ([]byte, error) {
	// add our own prefix to the key
	b, err := c.Cache.Get(ctx, cachePrefix+key, duration)
	if err != nil {
		return nil, err
	}

	dataLen := len(b)
	if dataLen < 16 {
		return nil, fmt.Errorf("invalid cache item length")
	}

	// strip off the last 16 bytes which is the checksum
	data := b[:dataLen-16]
	var checksum [16]byte
	copy(checksum[:], b[dataLen-16:])
	return uncompress(data, checksum)
}

func (c Cache) Set(ctx context.Context, key string, content []byte, duration time.Duration) error {
	startLen := len(content)
	if startLen <= 0 {
		log.Warningf("Key: %s data size is 0", key)
		return nil
	}

	startSz := float64(startLen) / float64(1024)

	// append the checksum
	data, checksum, err := compress(content)
	if err != nil {
		return err
	}
	data = append(data, checksum[:]...)
	afterSz := float64(len(data)) / float64(1024)
	ratio := 100.0 - (afterSz / startSz)

	log.Infof("Compressed cache start size: %.2fk, after compression: %.2fk.  Ratio: %.2f%%", startSz, afterSz, ratio)

	// add our own prefix
	return c.Cache.Set(ctx, cachePrefix+key, data, duration)
}

func compress(value []byte) ([]byte, [16]byte, error) {
	var buf bytes.Buffer
	sum := md5.Sum(value) // nolint:gosec

	zw := gzip.NewWriter(&buf)

	_, err := zw.Write(value)

	if err != nil {
		return nil, sum, err
	}

	err = zw.Close()
	if err != nil {
		return nil, sum, err
	}
	return buf.Bytes(), sum, nil
}

func uncompress(value []byte, vSum [16]byte) ([]byte, error) {
	var buf, uncompressed bytes.Buffer
	buf.Write(value)

	zr, err := gzip.NewReader(&buf)

	if err != nil {
		return nil, err
	}

	_, err = uncompressed.ReadFrom(zr)
	if err != nil {
		return nil, err
	}

	if err := zr.Close(); err != nil {
		return nil, err
	}

	ret := uncompressed.Bytes()
	sum := md5.Sum(ret) // nolint:gosec
	if sum != vSum {
		return nil, fmt.Errorf("check sum validation did not match")
	}

	return uncompressed.Bytes(), nil
}

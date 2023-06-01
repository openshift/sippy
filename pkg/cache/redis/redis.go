package redis

import (
	"time"

	r "gopkg.in/redis.v5"
)

const prefix = "_SIPPY_"

type Cache struct {
	client *r.Client
}

func NewRedisCache(url string) (*Cache, error) {
	var opts *r.Options
	var err error

	if opts, err = r.ParseURL(url); err != nil {
		return nil, err
	}

	return &Cache{
		client: r.NewClient(opts),
	}, nil
}

func (c Cache) Get(key string) ([]byte, error) {
	return c.client.Get(prefix + key).Bytes()
}

func (c Cache) Set(key string, content []byte, duration time.Duration) error {
	return c.client.Set(prefix+key, content, duration).Err()
}

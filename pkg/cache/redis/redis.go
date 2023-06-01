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

func (r Cache) Get(key string) ([]byte, error) {
	return r.client.Get(prefix + key).Bytes()
}

func (r Cache) Set(key string, content []byte, duration time.Duration) error {
	return r.client.Set(prefix+key, content, duration).Err()
}

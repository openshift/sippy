package cache

import "time"

type Cache interface {
	Get(key string) ([]byte, error)
	Set(key string, content []byte, duration time.Duration) error
}

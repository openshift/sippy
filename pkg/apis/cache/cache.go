package cache

import (
	"net/http"
	"time"
)

type Cache interface {
	Get(key string) ([]byte, error)
	Set(key string, content []byte, duration time.Duration) error
}

type APIResponse struct {
	Headers  http.Header
	Response []byte
}

type CacheOptions struct {
	ForceRefresh bool
}

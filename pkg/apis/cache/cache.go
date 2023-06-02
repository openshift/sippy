package cache

import (
	"net/http"
	"time"
)

type Cache interface {
	Get(key string) ([]byte, error)
	Set(key string, content []byte, duration time.Duration) error
}

type ApiResponse struct {
	Headers  http.Header
	Response []byte
}

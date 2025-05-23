package bigquerycache

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChunk(t *testing.T) {
	data := "It is useful mainly in compressed network protocols, to ensure that a remote reader has enough data to reconstruct a packet. Flush does not return until the data has been written. If the underlying writer returns an error, Flush returns that error. "

	chunked := chunk([]byte(data), 10)
	assert.NotNil(t, chunked, "Invalid chunked content")

	unchunked := unchunk(chunked)
	assert.NotNil(t, unchunked, "Invalid unchunked content")

	validation := string(unchunked)
	assert.Equal(t, data, validation)

}

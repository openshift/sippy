package bigquerycache

import "testing"

func TestChunk(t *testing.T) {
	data := "It is useful mainly in compressed network protocols, to ensure that a remote reader has enough data to reconstruct a packet. Flush does not return until the data has been written. If the underlying writer returns an error, Flush returns that error. "

	chunked := chunk([]byte(data), 10)

	if chunked == nil {
		t.Fatal("Invalid chunked content")
	}

	unchunked := unchunk(chunked)

	if unchunked == nil {
		t.Fatal("Invalid compressed content")
	}

	validation := string(unchunked)
	if data != validation {
		t.Fatalf("Invalid unchunked data returned: %s", validation)
	}
}

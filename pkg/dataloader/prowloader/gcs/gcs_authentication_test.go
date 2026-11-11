package gcs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/oauth2"
)

func TestSaveTokenCreatesOwnerOnlyCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gcp-token.json")
	token := &oauth2.Token{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
	}

	saveToken(path, token)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat token cache: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("token cache permissions = %o, want 600", got)
	}

	f, err := os.Open(path) //nolint:gosec // Test path is rooted in t.TempDir.
	if err != nil {
		t.Fatalf("open token cache: %v", err)
	}
	defer f.Close()

	decoded := &oauth2.Token{}
	if err := json.NewDecoder(f).Decode(decoded); err != nil {
		t.Fatalf("decode token cache: %v", err)
	}
	if decoded.AccessToken != token.AccessToken || decoded.RefreshToken != token.RefreshToken {
		t.Fatal("token cache did not preserve OAuth token fields")
	}
}

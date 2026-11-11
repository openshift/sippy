package gcs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/oauth2"
)

func TestSaveTokenUsesOwnerOnlyCache(t *testing.T) {
	for _, tc := range []struct {
		name               string
		precreateCacheFile bool
	}{
		{name: "new cache file"},
		{name: "existing permissive cache file", precreateCacheFile: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "gcp-token.json")
			if tc.precreateCacheFile {
				if err := os.WriteFile(path, nil, 0o644); err != nil { //nolint:gosec // G306: fixture must start permissive to verify saveToken restricts it.
					t.Fatalf("create existing token cache: %v", err)
				}
				if err := os.Chmod(path, 0o644); err != nil {
					t.Fatalf("set existing token cache permissions: %v", err)
				}
			}

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
		})
	}
}

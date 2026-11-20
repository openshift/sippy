package gcs

import (
	"strings"
	"testing"
)

func TestJobRunListingPrefixKeepsBuildBoundary(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
		want string
	}{
		{name: "build path", path: "logs/job/123", want: "logs/job/123/"},
		{name: "already terminated", path: "logs/job/123/", want: "logs/job/123/"},
		{name: "empty path", path: "", want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := jobRunListingPrefix(tc.path); got != tc.want {
				t.Fatalf("jobRunListingPrefix(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
	prefix := jobRunListingPrefix("logs/job/123")
	if !strings.HasPrefix("logs/job/123/artifacts/junit.xml", prefix) {
		t.Fatal("listing prefix does not include the requested build")
	}
	if strings.HasPrefix("logs/job/1234/artifacts/junit.xml", prefix) {
		t.Fatal("listing prefix includes a sibling build")
	}
}
